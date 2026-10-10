package authority

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/deletion"
)

type memoryAtomic struct {
	mu           sync.Mutex
	r            Record
	exists       bool
	items        map[string][]byte
	ops          map[string]Receipt
	lostCommit   bool
	lostComplete bool
}

func newMemoryAtomic() *memoryAtomic {
	return &memoryAtomic{items: map[string][]byte{}, ops: map[string]Receipt{}}
}
func clone[T any](x T) T { b, _ := json.Marshal(x); var y T; _ = json.Unmarshal(b, &y); return y }
func (m *memoryAtomic) Load(context.Context) (Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(m.r), m.exists, nil
}
func (m *memoryAtomic) Receipt(_ context.Context, id string) (Receipt, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.ops[id]
	return clone(r), ok, nil
}
func (m *memoryAtomic) Commit(_ context.Context, rev uint64, r Record, items []ImmutableItem, op Receipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Revision != rev || m.r.Pending != nil {
		return ErrConflict
	}
	if _, ok := m.ops[op.OperationID]; ok {
		return ErrConflict
	}
	for _, it := range items {
		if _, ok := m.items[it.Key]; ok {
			return ErrConflict
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
		return ErrUnavailable
	}
	return nil
}
func (m *memoryAtomic) Complete(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Pending == nil || r.Pending == nil || m.r.Revision != r.Revision || m.r.Pending.OperationID != r.Pending.OperationID {
		return ErrConflict
	}
	op := m.ops[r.Pending.OperationID]
	op.Complete = true
	m.ops[op.OperationID] = op
	m.r.Tip = r.Pending.Next
	m.r.Pending = nil
	if m.lostComplete {
		m.lostComplete = false
		return ErrUnavailable
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
	tip   WitnessTip
	items map[string][]byte
	fail  bool
}

func (m *memoryWitness) Tip(context.Context) (WitnessTip, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tip, m.tip.Sequence > 0, nil
}
func (m *memoryWitness) Advance(_ context.Context, p Pending) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return ErrUnavailable
	}
	if m.tip == p.Next {
		return nil
	}
	if m.tip != p.Previous {
		return ErrConflict
	}
	m.items[p.Next.Key] = append([]byte(nil), p.Witness...)
	m.tip = p.Next
	return nil
}
func (m *memoryWitness) Read(_ context.Context, t WitnessTip) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[t.Key]
	if !ok || digest(b) != t.SHA256 {
		return nil, ErrConflict
	}
	return append([]byte(nil), b...), nil
}
func fixture(t *testing.T) (*Kernel, *memoryAtomic, *memoryWitness) {
	t.Helper()
	a := newMemoryAtomic()
	w := &memoryWitness{items: map[string][]byte{}}
	return &Kernel{Store: a, Witness: w, Lineage: "test-lineage", TableARN: "arn:aws:dynamodb:eu-west-1:123456789012:table/test"}, a, w
}
func opID(t *testing.T) string {
	t.Helper()
	id, e := NewOperationID()
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func TestKernelAmbiguityAndWitnessBarrier(t *testing.T) {
	k, a, w := fixture(t)
	ctx := context.Background()
	op := Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("bootstrap")), Principal: "owner"}
	a.lostCommit = true
	w.fail = true
	if _, err := k.Apply(ctx, Record{}, json.RawMessage(`{"state":"enrolled"}`), nil, op, true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want unknown: %v", err)
	}
	if a.r.Pending == nil || a.ops[op.OperationID].Complete {
		t.Fatal("unwitnessed operation acknowledged")
	}
	if _, err := k.Current(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pending publication admitted: %v", err)
	}
	w.fail = false
	a.lostComplete = true
	if _, err := k.Apply(ctx, Record{}, json.RawMessage(`{"state":"enrolled"}`), nil, op, true); err != nil {
		t.Fatal(err)
	}
	if !a.ops[op.OperationID].Complete || a.r.Pending != nil {
		t.Fatal("retry did not complete exact receipt")
	}
	bad := op
	bad.RequestSHA256 = digest([]byte("different"))
	if _, err := k.Apply(ctx, Record{}, json.RawMessage(`{}`), nil, bad, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("operation reuse accepted: %v", err)
	}
}
func TestCurrentDetectsRestoredOlderTable(t *testing.T) {
	k, a, w := fixture(t)
	ctx := context.Background()
	r := Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("enroll")), Principal: "owner"}
	if _, e := k.Apply(ctx, Record{}, json.RawMessage(`{}`), nil, r, true); e != nil {
		t.Fatal(e)
	}
	old := clone(a.r)
	r.OperationID = opID(t)
	if _, e := k.Apply(ctx, a.r, json.RawMessage(`{"next":true}`), nil, r, false); e != nil {
		t.Fatal(e)
	}
	a.r = old
	if _, e := k.Current(ctx); !errors.Is(e, ErrConflict) {
		t.Fatalf("rollback accepted: %v", e)
	}
	h, e := History(ctx, w, w.tip, 20, 1024*1024)
	if e != nil || len(h) != 2 {
		t.Fatalf("history %d %v", len(h), e)
	}
}
func TestServerGenesisReplayAndLeaseFence(t *testing.T) {
	k, _, _ := fixture(t)
	now := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	exe := digest([]byte("binary"))
	s, e := NewServer(k, ServerConfig{RuntimePrincipal: "runtime", OwnerPrincipal: "owner", ApprovedBuilds: map[string]string{exe: strings.Repeat("a", 40)}, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if _, e = s.execute(ctx, "runtime", Request{Action: "enroll", OperationID: opID(t)}); !errors.Is(e, ErrForbidden) {
		t.Fatal("runtime enrolled namespace")
	}
	if _, e = s.execute(ctx, "owner", Request{Action: "enroll", OperationID: opID(t)}); e != nil {
		t.Fatal(e)
	}
	ch, e := s.execute(ctx, "runtime", Request{Action: "challenge", OperationID: opID(t)})
	if e != nil {
		t.Fatal(e)
	}
	q := Request{Action: "prepare", OperationID: ch.Challenge.OperationID, ChallengeID: ch.Challenge.ID, Nonce: ch.Challenge.Nonce, Measurement: &Measurement{ExecutableSHA256: exe, ProcessLifetime: "kernel-pid-start"}}
	prepared, e := s.execute(ctx, "runtime", q)
	if e != nil {
		t.Fatal(e)
	}
	replayed, e := s.execute(ctx, "runtime", q)
	if e != nil || replayed.Prepared.ReservationID != prepared.Prepared.ReservationID {
		t.Fatal("prepare replay failed")
	}
	reader, e := deletion.NewJournalReader(&boundedReader{store: k.Store, position: prepared.Prepared.Position, strict: true}, prepared.Prepared.Position)
	if e != nil {
		t.Fatal(e)
	}
	ob, e := reader.Observe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	active, e := s.execute(ctx, "runtime", Request{Action: "activate", OperationID: opID(t), ReservationID: prepared.Prepared.ReservationID, HistorySHA256: ob.HistorySHA256})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.execute(ctx, "runtime", Request{Action: "current", LeaseID: active.Lease.ID}); e != nil {
		t.Fatal(e)
	}
	now = now.Add(2 * time.Hour)
	if _, e = s.execute(ctx, "runtime", Request{Action: "renew", OperationID: opID(t), LeaseID: active.Lease.ID, Measurement: q.Measurement}); !errors.Is(e, ErrExpired) {
		t.Fatalf("expired lease resurrected: %v", e)
	}
}

func (m *memoryAtomic) Import(_ context.Context, items []ImmutableItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range items {
		if old, ok := m.items[it.Key]; ok && string(old) != string(it.Body) {
			return ErrConflict
		}
		if strings.HasPrefix(it.Key, "OP#") {
			var receipt Receipt
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

func (m *memoryAtomic) ListObjects(_ context.Context, prefix, after string, n int) ([]ImmutableItem, bool, error) {
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
	out := make([]ImmutableItem, len(keys))
	for i, key := range keys {
		out[i] = ImmutableItem{Key: key, Body: append([]byte(nil), m.items[key]...)}
	}
	return out, more, nil
}
