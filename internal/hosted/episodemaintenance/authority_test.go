package episodemaintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

// Minimal in-memory AtomicStore/WitnessStore adapted from authority's existing
// transaction tests. These exercise the real Kernel, Server, Gateway and Client;
// they make no AWS persistence, transport or IAM qualification claim.
type memoryAtomic struct {
	mu           sync.Mutex
	r            authority.Record
	exists       bool
	items        map[string][]byte
	ops          map[string]authority.Receipt
	lostCommit   bool
	lostComplete bool
}

func newMemoryAtomic() *memoryAtomic {
	return &memoryAtomic{items: map[string][]byte{}, ops: map[string]authority.Receipt{}}
}
func clone[T any](x T) T { b, _ := json.Marshal(x); var y T; _ = json.Unmarshal(b, &y); return y }
func (m *memoryAtomic) Load(context.Context) (authority.Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(m.r), m.exists, nil
}
func (m *memoryAtomic) Receipt(_ context.Context, id string) (authority.Receipt, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.ops[id]
	return clone(r), ok, nil
}
func (m *memoryAtomic) Commit(_ context.Context, rev uint64, r authority.Record, items []authority.ImmutableItem, op authority.Receipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Revision != rev || m.r.Pending != nil {
		return authority.ErrConflict
	}
	if _, ok := m.ops[op.OperationID]; ok {
		return authority.ErrConflict
	}
	for _, it := range items {
		if _, ok := m.items[it.Key]; ok {
			return authority.ErrConflict
		}
	}
	for _, it := range items {
		m.items[it.Key] = append([]byte(nil), it.Body...)
	}
	m.ops[op.OperationID] = clone(op)
	m.r = clone(r)
	m.exists = true
	if m.lostCommit {
		m.lostCommit = false
		return authority.ErrUnavailable
	}
	return nil
}
func (m *memoryAtomic) Complete(_ context.Context, r authority.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Pending == nil || r.Pending == nil || m.r.Revision != r.Revision || m.r.Pending.OperationID != r.Pending.OperationID {
		return authority.ErrConflict
	}
	op := m.ops[r.Pending.OperationID]
	op.Complete = true
	m.ops[op.OperationID] = op
	m.r.Tip = r.Pending.Next
	m.r.Pending = nil
	if m.lostComplete {
		m.lostComplete = false
		return authority.ErrUnavailable
	}
	return nil
}
func (m *memoryAtomic) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[key]
	return append([]byte(nil), b...), ok, nil
}
func (m *memoryAtomic) List(_ context.Context, prefix, after string, n int) ([]string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for k := range m.items {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	more := len(keys) > n
	if more {
		keys = keys[:n]
	}
	return keys, more, nil
}

type memoryWitness struct {
	mu    sync.Mutex
	tip   authority.WitnessTip
	items map[string][]byte
	fail  bool
}

func (m *memoryWitness) Tip(context.Context) (authority.WitnessTip, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tip, m.tip.Sequence > 0, nil
}
func (m *memoryWitness) Advance(_ context.Context, p authority.Pending) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return authority.ErrUnavailable
	}
	if m.tip == p.Next {
		return nil
	}
	if m.tip != p.Previous {
		return authority.ErrConflict
	}
	m.items[p.Next.Key] = append([]byte(nil), p.Witness...)
	m.tip = p.Next
	return nil
}
func (m *memoryWitness) Read(_ context.Context, t authority.WitnessTip) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[t.Key]
	if !ok || digest(b) != t.SHA256 {
		return nil, authority.ErrConflict
	}
	return append([]byte(nil), b...), nil
}

func (m *memoryAtomic) Import(_ context.Context, items []authority.ImmutableItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range items {
		if old, ok := m.items[it.Key]; ok && string(old) != string(it.Body) {
			return authority.ErrConflict
		}
		if strings.HasPrefix(it.Key, "OP#") {
			var receipt authority.Receipt
			if err := json.Unmarshal(it.Body, &receipt); err != nil {
				return err
			}
			m.ops[receipt.OperationID] = receipt
		} else {
			m.items[it.Key] = append([]byte(nil), it.Body...)
		}
	}
	return nil
}

func (m *memoryAtomic) ListObjects(_ context.Context, prefix, after string, n int) ([]authority.ImmutableItem, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for k := range m.items {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	more := len(keys) > n
	if more {
		keys = keys[:n]
	}
	out := make([]authority.ImmutableItem, len(keys))
	for i, key := range keys {
		out[i] = authority.ImmutableItem{Key: key, Body: append([]byte(nil), m.items[key]...)}
	}
	return out, more, nil
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type gatewayTransport struct {
	gateway     *authority.Gateway
	binding     authority.GatewayBinding
	owner       bool
	measurement authority.Measurement
}

func (t gatewayTransport) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	if q.Action == "prepare" || q.Action == "renew" {
		m := t.measurement
		q.Measurement = &m
	}
	path, arn, caller := t.binding.RuntimePath, t.binding.RuntimeARN, t.binding.RuntimeCallerID
	if t.owner {
		path, arn, caller = t.binding.OwnerPath, t.binding.OwnerARN, t.binding.OwnerCallerID
	}
	body, err := json.Marshal(q)
	if err != nil {
		return authority.Response{}, err
	}
	response, err := t.gateway.Handle(ctx, events.APIGatewayProxyRequest{
		Resource: path, Path: path, HTTPMethod: "POST", Body: string(body),
		RequestContext: events.APIGatewayProxyRequestContext{APIID: t.binding.APIID, AccountID: t.binding.AccountID, Stage: t.binding.Stage, DomainName: t.binding.DomainName, HTTPMethod: "POST", RequestID: "synthetic-request", ResourcePath: path, Identity: events.APIGatewayRequestIdentity{UserArn: arn, Caller: caller}},
	})
	if err != nil || response.StatusCode != 200 {
		return authority.Response{}, authority.ErrUnavailable
	}
	var out authority.Response
	if err = json.Unmarshal([]byte(response.Body), &out); err != nil {
		return authority.Response{}, err
	}
	return out, nil
}

func operationID(t *testing.T) string {
	t.Helper()
	id, err := authority.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type authorityFixture struct {
	client *authority.Client
	lease  authority.Lease
	now    time.Time
	store  *memoryAtomic
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	f := &authorityFixture{now: time.Now().UTC().Truncate(time.Second), store: newMemoryAtomic()}
	witness := &memoryWitness{items: map[string][]byte{}}
	kernel := &authority.Kernel{Store: f.store, Witness: witness, Lineage: "synthetic-lineage", TableARN: "arn:aws:dynamodb:eu-west-1:123456789012:table/synthetic"}
	measurement := authority.Measurement{ExecutableSHA256: digest([]byte("synthetic-binary")), ProcessLifetime: "synthetic-pid-start"}
	binding := authority.GatewayBinding{APIID: "synthetic-api", AccountID: "123456789012", Stage: "synthetic", DomainName: "synthetic.execute-api.test", RuntimePath: "/runtime", OwnerPath: "/owner", RuntimeARN: "arn:aws:iam::123456789012:role/runtime", RuntimeCallerID: "runtime-caller", OwnerARN: "arn:aws:iam::123456789012:role/owner", OwnerCallerID: "owner-caller"}
	server, err := authority.NewServer(kernel, authority.ServerConfig{RuntimePrincipal: binding.RuntimeARN, OwnerPrincipal: binding.OwnerARN, ApprovedBuilds: map[string]string{measurement.ExecutableSHA256: strings.Repeat("a", 40)}, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour, Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := authority.NewGateway(server, binding)
	if err != nil {
		t.Fatal(err)
	}
	owner := gatewayTransport{gateway: gateway, binding: binding, owner: true}
	if _, err = owner.Invoke(context.Background(), authority.Request{Action: "enroll", OperationID: operationID(t)}); err != nil {
		t.Fatal(err)
	}
	f.client, err = authority.NewClient(gatewayTransport{gateway: gateway, binding: binding, measurement: measurement})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := f.client.Challenge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := f.client.Prepare(context.Background(), challenge)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := deletion.NewJournalReader(f.client.Reader(prepared), prepared.Position)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reader.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.client.Activate(context.Background(), prepared, observation.HistorySHA256, operationID(t))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *authorityFixture) source(t *testing.T, a Authority) *source {
	t.Helper()
	s, err := NewSource(a, func() authority.Lease { return f.lease })
	if err != nil {
		t.Fatal(err)
	}
	actual := s.(*source)
	actual.now = func() time.Time { return f.now }
	return actual
}
func (f *authorityFixture) append(t *testing.T, sequence int64) {
	t.Helper()
	identity := contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash("synthetic-brain"), ConversationHash: strings.Repeat("b", 64), CommandHash: strings.Repeat("c", 64), RequestDigest: strings.Repeat("d", 64)}
	object := contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: f.lease.Position.ActiveGeneration, SequenceID: sequence, WriterID: f.lease.WriterID, SubjectType: contracts.DeletionSubjectEpisodeConversation, SubjectID: identity.String(), Outcome: contracts.DeletionIntentRequested, RecordedAt: f.now}
	if sequence > 1 {
		current, err := f.client.Current(context.Background(), f.lease)
		if err != nil {
			t.Fatal(err)
		}
		object.PrevHash = current.Position.LastObjectInGeneration.EntryHash
	}
	body, err := object.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.client.Store(f.lease).PutIfAbsent(context.Background(), contracts.JournalKey(object.Generation, sequence), body); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoritySourceRealServerClientCutAndRecheck(t *testing.T) {
	f := newAuthorityFixture(t)
	f.append(t, 1)
	s := f.source(t, f.client)
	snapshot, err := s.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CutID == "" || snapshot.Recheck == nil || len(snapshot.Observation.Read.Entries) != 1 || snapshot.Observation.Position.LastObjectInGeneration.SequenceID != 1 || snapshot.Observation.Read.Sealed {
		t.Fatalf("wrong active-prefix observation: %+v", snapshot)
	}
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Renew the existing issuance; local factory callback and authenticated
	// Current must agree. Renewal does not change journal position or identity.
	f.now = f.now.Add(time.Minute)
	f.lease, err = f.client.Renew(context.Background(), f.lease, operationID(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatal("same-issuance renewal refused", err)
	}
	f.append(t, 2)
	if err = snapshot.Recheck(context.Background()); !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatalf("advanced authoritative history admitted: %v", err)
	}
}

type faultAuthority struct {
	Authority
	mutateCut     func(*authority.Cut)
	mutateCurrent func(*authority.Current)
	cutErr        error
	nilReader     bool
}

func (a *faultAuthority) Cut(ctx context.Context, l authority.Lease, attempt, op string) (authority.Cut, error) {
	if a.cutErr != nil {
		return authority.Cut{}, a.cutErr
	}
	cut, err := a.Authority.Cut(ctx, l, attempt, op)
	if err == nil && a.mutateCut != nil {
		a.mutateCut(&cut)
	}
	return cut, err
}
func (a *faultAuthority) Current(ctx context.Context, l authority.Lease) (authority.Current, error) {
	current, err := a.Authority.Current(ctx, l)
	if err == nil && a.mutateCurrent != nil {
		a.mutateCurrent(&current)
	}
	return current, err
}
func (a *faultAuthority) CutReader(c authority.Cut) contracts.JournalObjectReader {
	if a.nilReader {
		return nil
	}
	return a.Authority.CutReader(c)
}

func TestAuthoritySourceRefusesInvalidAuthenticatedResponses(t *testing.T) {
	for _, scenario := range []string{"cut_unknown", "cut_id", "lease_id", "attempt_id", "cut_expiry", "generation", "genesis", "anchor_missing", "anchor_key", "anchor_hash", "nil_reader", "current_epoch", "current_position", "current_expiry", "missing_history"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAuthorityFixture(t)
			f.append(t, 1)
			a := &faultAuthority{Authority: f.client}
			switch scenario {
			case "cut_unknown":
				a.cutErr = errors.New("untrusted detail must not escape")
			case "cut_id":
				a.mutateCut = func(c *authority.Cut) { c.ID = "bad" }
			case "lease_id":
				a.mutateCut = func(c *authority.Cut) { c.LeaseID = strings.Repeat("e", 64) }
			case "attempt_id":
				a.mutateCut = func(c *authority.Cut) { c.SnapshotAttemptID = strings.Repeat("e", 64) }
			case "cut_expiry":
				a.mutateCut = func(c *authority.Cut) { c.ExpiresAt = f.now }
			case "generation":
				a.mutateCut = func(c *authority.Cut) { c.Position.ActiveGeneration++ }
			case "genesis":
				a.mutateCut = func(c *authority.Cut) { c.Position.GenesisIssuanceID = strings.Repeat("e", 64) }
			case "anchor_missing":
				a.mutateCut = func(c *authority.Cut) { c.Anchor = authority.WitnessTip{} }
			case "anchor_key":
				a.mutateCut = func(c *authority.Cut) { c.Anchor.Key = "../foreign" }
			case "anchor_hash":
				a.mutateCut = func(c *authority.Cut) { c.Anchor.SHA256 = strings.Repeat("A", 64) }
			case "nil_reader":
				a.nilReader = true
			case "current_epoch":
				a.mutateCurrent = func(c *authority.Current) { c.Epoch++ }
			case "current_position":
				a.mutateCurrent = func(c *authority.Current) { c.Position.LastObjectInGeneration = contracts.DeletionWatermark{} }
			case "current_expiry":
				a.mutateCurrent = func(c *authority.Current) { c.ExpiresAt = f.now }
			case "missing_history":
				f.store.mu.Lock()
				delete(f.store.items, "OBJ#"+contracts.JournalKey(1, 1))
				f.store.mu.Unlock()
			}
			snapshot, err := f.source(t, a).Observe(context.Background())
			if !errors.Is(err, ErrAuthorityUnavailable) || snapshot.Recheck != nil || snapshot.CutID != "" {
				t.Fatalf("unqualified response admitted: %v", err)
			}
			if strings.Contains(err.Error(), "untrusted detail") {
				t.Fatal("authority content escaped")
			}
		})
	}
}

func TestAuthoritySourceLeaseIdentityAndExpiryFailClosed(t *testing.T) {
	for _, scenario := range []string{"id", "reservation", "writer", "epoch", "build", "expiry", "cut_expiry", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAuthorityFixture(t)
			snapshot, err := f.source(t, f.client).Observe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch scenario {
			case "id":
				f.lease.ID = strings.Repeat("e", 64)
			case "reservation":
				f.lease.ReservationID = strings.Repeat("e", 64)
			case "writer":
				f.lease.WriterID = strings.Repeat("e", 64)
			case "epoch":
				f.lease.Epoch++
			case "build":
				f.lease.BuildSHA = strings.Repeat("e", 40)
			case "expiry":
				f.lease.ExpiresAt = f.now
			case "cut_expiry":
				f.now = f.now.Add(2 * time.Hour)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err = snapshot.Recheck(ctx); !errors.Is(err, ErrAuthorityUnavailable) {
				t.Fatalf("invalid maintenance issuance admitted: %v", err)
			}
		})
	}
}

func TestAuthoritySourceConstructorAndInvalidLease(t *testing.T) {
	var typedNil *faultAuthority
	for _, a := range []Authority{nil, typedNil} {
		if s, err := NewSource(a, func() authority.Lease { return authority.Lease{} }); s != nil || !errors.Is(err, ErrAuthorityUnavailable) {
			t.Fatal("nil authority admitted")
		}
	}
	f := newAuthorityFixture(t)
	if s, err := NewSource(f.client, nil); s != nil || err == nil {
		t.Fatal("nil lease callback admitted")
	}
	f.lease.ID = "plaintext"
	if snapshot, err := f.source(t, f.client).Observe(context.Background()); snapshot.Recheck != nil || !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatal("invalid lease admitted")
	}
}
