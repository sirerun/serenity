package authority

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type Client struct{ transport Transport }

func NewClient(t Transport) (*Client, error) {
	if t == nil {
		return nil, ErrInvalid
	}
	return &Client{transport: t}, nil
}
func NewOperationID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (c *Client) invoke(ctx context.Context, r Request) (Response, error) {
	if c == nil || c.transport == nil {
		return Response{}, ErrInvalid
	}
	return c.transport.Invoke(ctx, r)
}
func (c *Client) Challenge(ctx context.Context) (Challenge, error) {
	op, err := NewOperationID()
	if err != nil {
		return Challenge{}, err
	}
	r, err := c.invoke(ctx, Request{Action: "challenge", OperationID: op})
	if err != nil {
		return Challenge{}, err
	}
	if r.Challenge == nil {
		return Challenge{}, ErrUnavailable
	}
	return *r.Challenge, nil
}

// Prepare accepts no measurement. The privileged broker inserts the trusted
// process measurement before forwarding this request to the AWS authority.
func (c *Client) Prepare(ctx context.Context, ch Challenge) (Prepared, error) {
	r, err := c.invoke(ctx, Request{Action: "prepare", OperationID: ch.OperationID, ChallengeID: ch.ID, Nonce: ch.Nonce})
	if err != nil {
		return Prepared{}, err
	}
	if r.Prepared == nil {
		return Prepared{}, ErrUnavailable
	}
	return *r.Prepared, nil
}
func (c *Client) Activate(ctx context.Context, p Prepared, history, op string) (Lease, error) {
	r, err := c.invoke(ctx, Request{Action: "activate", OperationID: op, ReservationID: p.ReservationID, HistorySHA256: history})
	if err != nil {
		return Lease{}, err
	}
	if r.Lease == nil {
		return Lease{}, ErrUnavailable
	}
	return *r.Lease, nil
}
func (c *Client) Current(ctx context.Context, l Lease) (Current, error) {
	r, err := c.invoke(ctx, Request{Action: "current", LeaseID: l.ID})
	if err != nil {
		return Current{}, err
	}
	if r.Current == nil {
		return Current{}, ErrUnavailable
	}
	return *r.Current, nil
}
func (c *Client) Close(ctx context.Context, l Lease, op string) error {
	_, err := c.invoke(ctx, Request{Action: "close", OperationID: op, LeaseID: l.ID})
	return err
}
func (c *Client) ClosePrepared(ctx context.Context, p Prepared, op string) error {
	_, err := c.invoke(ctx, Request{Action: "close", OperationID: op, ReservationID: p.ReservationID})
	return err
}
func (c *Client) Cut(ctx context.Context, l Lease, snapshot, op string) (Cut, error) {
	r, err := c.invoke(ctx, Request{Action: "cut", OperationID: op, LeaseID: l.ID, SnapshotAttemptID: snapshot})
	if err != nil {
		return Cut{}, err
	}
	if r.Cut == nil {
		return Cut{}, ErrUnavailable
	}
	return *r.Cut, nil
}
func (c *Client) FinalizeCut(ctx context.Context, cut Cut, manifest, op string) (FinalizedCut, error) {
	r, err := c.invoke(ctx, Request{Action: "finalize_cut", OperationID: op, CutID: cut.ID, ManifestSHA256: manifest})
	if err != nil {
		return FinalizedCut{}, err
	}
	if r.FinalizedCut == nil {
		return FinalizedCut{}, ErrUnavailable
	}
	return *r.FinalizedCut, nil
}
func (c *Client) ReadFinalizedCut(ctx context.Context, id string) (FinalizedCut, error) {
	r, err := c.invoke(ctx, Request{Action: "read_finalized_cut", CutID: id})
	if err != nil {
		return FinalizedCut{}, err
	}
	if r.FinalizedCut == nil {
		return FinalizedCut{}, ErrUnavailable
	}
	return *r.FinalizedCut, nil
}
func (c *Client) Reader(p Prepared) contracts.JournalObjectReader {
	return &objectClient{c: c, reservation: p.ReservationID}
}
func (c *Client) CutReader(cut Cut) contracts.JournalObjectReader {
	return &objectClient{c: c, cut: cut.ID}
}
func (c *Client) Store(l Lease) contracts.JournalObjectStore { return &objectClient{c: c, lease: l.ID} }

type objectClient struct {
	c                       *Client
	reservation, lease, cut string
}

func (o *objectClient) request(action string) Request {
	return Request{Action: action, ReservationID: o.reservation, LeaseID: o.lease, CutID: o.cut}
}
func (o *objectClient) Get(ctx context.Context, key string) ([]byte, bool, error) {
	q := o.request("get")
	q.Key = key
	r, err := o.c.invoke(ctx, q)
	if err != nil {
		return nil, false, err
	}
	if len(r.Body) > MaxObjectBytes {
		return nil, false, ErrInvalid
	}
	return r.Body, r.Found, nil
}
func (o *objectClient) ListAfter(ctx context.Context, prefix, after string, limit int) ([]string, bool, error) {
	if limit < 1 || limit > MaxPageKeys {
		return nil, false, ErrInvalid
	}
	q := o.request("list")
	q.Prefix = prefix
	q.StartAfter = after
	q.Limit = limit
	r, err := o.c.invoke(ctx, q)
	if err != nil {
		return nil, false, err
	}
	if len(r.Keys) > limit {
		return nil, false, ErrInvalid
	}
	return r.Keys, r.More, nil
}
func (o *objectClient) PutIfAbsent(ctx context.Context, key string, body []byte) (bool, error) {
	if o.lease == "" || len(body) > MaxObjectBytes {
		return false, ErrForbidden
	}
	q := o.request("append")
	q.Key = key
	q.Body = body
	b, err := json.Marshal(struct {
		Domain, Lease, Key string
		Body               []byte
	}{"serenity-authority-append-v1", o.lease, key, body})
	if err != nil {
		return false, err
	}
	q.OperationID = digest(b)
	r, err := o.c.invoke(ctx, q)
	return r.Created, err
}

// Renew extends the same epoch and writer; it cannot reacquire an expired lease.
func (c *Client) Renew(ctx context.Context, l Lease, op string) (Lease, error) {
	r, err := c.invoke(ctx, Request{Action: "renew", OperationID: op, LeaseID: l.ID})
	if err != nil {
		return Lease{}, err
	}
	if r.Lease == nil {
		return Lease{}, ErrUnavailable
	}
	return *r.Lease, nil
}
