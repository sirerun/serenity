package authority

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"

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
func (c *Client) ObserveClosed(ctx context.Context) (ClosedPosition, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return ClosedPosition{}, ErrInvalid
		}
		return ClosedPosition{}, ctx.Err()
	}
	r, err := c.invoke(ctx, Request{Action: "closed_current"})
	if err != nil {
		return ClosedPosition{}, err
	}
	if err = ctx.Err(); err != nil {
		return ClosedPosition{}, err
	}
	if r.ClosedPosition == nil || !validClosedPosition(*r.ClosedPosition) {
		return ClosedPosition{}, ErrUnavailable
	}
	return *r.ClosedPosition, nil
}
func (c *Client) Reader(p Prepared) contracts.JournalObjectReader {
	return &objectClient{c: c, reservation: p.ReservationID}
}
func (c *Client) CutReader(cut Cut) contracts.JournalObjectReader {
	return &objectClient{c: c, cut: cut.ID}
}
func (c *Client) ClosedReader(position ClosedPosition) contracts.JournalObjectReader {
	return &objectClient{c: c, closed: &position, invalidClosed: !validClosedPosition(position)}
}
func (c *Client) Store(l Lease) contracts.JournalObjectStore { return &objectClient{c: c, lease: l.ID} }

type objectClient struct {
	mu                      sync.Mutex
	cache                   map[string][]byte
	c                       *Client
	reservation, lease, cut string
	closed                  *ClosedPosition
	invalidClosed           bool
}

func (o *objectClient) request(action string) Request {
	return Request{Action: action, ReservationID: o.reservation, LeaseID: o.lease, CutID: o.cut, ClosedPosition: o.closed}
}
func (o *objectClient) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if ctx == nil {
		return nil, false, ErrInvalid
	}
	if o.invalidClosed || o.closed != nil && !validClosedPosition(*o.closed) {
		return nil, false, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	o.mu.Lock()
	cached, found := o.cache[key]
	cached = bytes.Clone(cached)
	o.mu.Unlock()
	if found {
		return cached, true, nil
	}
	q := o.request("get")
	q.Key = key
	r, err := o.c.invoke(ctx, q)
	if err != nil {
		return nil, false, err
	}
	if o.closed != nil {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
	}
	if len(r.Body) > MaxObjectBytes {
		return nil, false, ErrInvalid
	}
	return r.Body, r.Found, nil
}
func (o *objectClient) ListAfter(ctx context.Context, prefix, after string, limit int) ([]string, bool, error) {
	if ctx == nil {
		return nil, false, ErrInvalid
	}
	if o.invalidClosed || o.closed != nil && !validClosedPosition(*o.closed) {
		return nil, false, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
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
	if o.closed != nil {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
	}

	if len(r.Keys) > limit || len(r.Objects) != len(r.Keys) || r.More && len(r.Keys) == 0 {
		return nil, false, ErrInvalid
	}
	encoded, e := json.Marshal(r)
	if e != nil || len(encoded) > MaxPageBytes {
		return nil, false, ErrInvalid
	}
	cache := make(map[string][]byte, len(r.Keys))
	last := after
	for i, key := range r.Keys {
		if _, _, ok := journalCoordinates(key); !ok || key <= last || !strings.HasPrefix(key, prefix) || r.Objects[i].Key != key || len(r.Objects[i].Body) > MaxObjectBytes {
			return nil, false, ErrInvalid
		}
		last = key
		cache[key] = bytes.Clone(r.Objects[i].Body)
	}
	o.mu.Lock()
	o.cache = cache
	o.mu.Unlock()

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

// Seal is privileged recovery work. Runtime credentials cannot invoke it;
// the same transaction closes the exact active lease and advances generation.
func (c *Client) Seal(ctx context.Context, l Lease, key string, body []byte, op string) error {
	_, err := c.invoke(ctx, Request{Action: "seal", OperationID: op, LeaseID: l.ID, Key: key, Body: body})
	return err
}

// Inspect discovers the exact durable capability for owner reconciliation of
// an ambiguous startup response. Runtime credentials cannot use this route.
func (c *Client) Inspect(ctx context.Context) (Response, error) {
	return c.invoke(ctx, Request{Action: "inspect"})
}
