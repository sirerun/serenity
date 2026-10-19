package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if _, e := k.ObserveCurrent(ctx); !errors.Is(e, ErrConflict) {
		t.Fatalf("observational rollback accepted: %v", e)
	}
	h, e := History(ctx, w, w.tip, 20, 1024*1024)
	if e != nil || len(h) != 2 {
		t.Fatalf("history %d %v", len(h), e)
	}
}

type mutationCountingStore struct {
	AtomicStore
	writes int
}

func (s *mutationCountingStore) Commit(ctx context.Context, revision uint64, record Record, items []ImmutableItem, receipt Receipt) error {
	s.writes++
	return s.AtomicStore.Commit(ctx, revision, record, items, receipt)
}
func (s *mutationCountingStore) Complete(ctx context.Context, record Record) error {
	s.writes++
	return s.AtomicStore.Complete(ctx, record)
}
func (s *mutationCountingStore) Import(ctx context.Context, items []ImmutableItem) error {
	s.writes++
	return s.AtomicStore.Import(ctx, items)
}

type mutationCountingWitness struct {
	WitnessStore
	writes int
}

func (w *mutationCountingWitness) Advance(ctx context.Context, pending Pending) error {
	w.writes++
	return w.WitnessStore.Advance(ctx, pending)
}

func TestObserveCurrentRefusesPendingWithoutMutation(t *testing.T) {
	k, _, w := fixture(t)
	w.fail = true
	if _, err := k.Apply(context.Background(), Record{}, json.RawMessage(`{"phase":"pending"}`), nil, Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("pending")), Principal: "owner"}, true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("setup did not leave an unknown transaction: %v", err)
	}
	store := &mutationCountingStore{AtomicStore: k.Store}
	witness := &mutationCountingWitness{WitnessStore: k.Witness}
	k.Store, k.Witness = store, witness
	before, _, _ := store.Load(context.Background())
	beforeTip, _, _ := witness.Tip(context.Background())
	if _, err := k.ObserveCurrent(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatalf("pending state observed as current: %v", err)
	}
	after, _, _ := store.Load(context.Background())
	afterTip, _, _ := witness.Tip(context.Background())
	if store.writes != 0 || witness.writes != 0 || after.Pending == nil || after.Revision != before.Revision || after.Tip != before.Tip || afterTip != beforeTip {
		t.Fatalf("observation mutated pending authority state: store=%d witness=%d before=%+v after=%+v", store.writes, witness.writes, before, after)
	}
}

func TestObserveCurrentBindsWitnessStateAndReturnsDetachedRecord(t *testing.T) {
	k, a, w := fixture(t)
	if _, err := k.Apply(context.Background(), Record{}, json.RawMessage(`{"phase":"closed"}`), nil, Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("closed")), Principal: "owner"}, true); err != nil {
		t.Fatal(err)
	}
	record, err := k.ObserveCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	record.State[0] = ' '
	again, err := k.ObserveCurrent(context.Background())
	if err != nil || string(again.State) != `{"phase":"closed"}` {
		t.Fatalf("returned record aliases canonical state: %s %v", again.State, err)
	}
	a.mu.Lock()
	a.r.State = []byte(`{"phase":"different"}`)
	a.mu.Unlock()
	if _, err = k.ObserveCurrent(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatalf("META state differing from exact witness state accepted: %v", err)
	}
	a.mu.Lock()
	a.r.State = []byte(`{"phase":"closed"}`)
	a.mu.Unlock()
	w.mu.Lock()
	w.items[again.Tip.Key] = []byte(`{"version":1,"tampered":true}`)
	w.mu.Unlock()
	if _, err = k.ObserveCurrent(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatalf("witness body mismatch accepted: %v", err)
	}
}

func TestObserveCurrentRejectsMalformedAndOversizedLatestWitnessBodies(t *testing.T) {
	for _, test := range []struct {
		name string
		body func() []byte
	}{
		{name: "malformed", body: func() []byte { return []byte("{") }},
		{name: "oversized", body: func() []byte { return bytes.Repeat([]byte("x"), 4*MaxBodyBytes+1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			k, a, w := fixture(t)
			if _, err := k.Apply(context.Background(), Record{}, json.RawMessage(`{"phase":"closed"}`), nil, Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("closed")), Principal: "owner"}, true); err != nil {
				t.Fatal(err)
			}
			a.mu.Lock()
			record := clone(a.r)
			a.mu.Unlock()
			body, err := w.Read(context.Background(), record.Tip)
			if err != nil {
				t.Fatal(err)
			}
			var latest witnessRecord
			if err = strictDecode(body, &latest); err != nil {
				t.Fatal(err)
			}
			bad := test.body()
			tip := record.Tip
			tip.SHA256 = digest(bad)
			tip.TotalBytes = latest.Previous.TotalBytes + int64(len(bad))
			tip.Key = fmt.Sprintf("%020d/%s.json", tip.Sequence, tip.SHA256)
			a.mu.Lock()
			a.r.Tip = tip
			a.mu.Unlock()
			w.mu.Lock()
			delete(w.items, record.Tip.Key)
			w.items[tip.Key] = bad
			w.tip = tip
			w.mu.Unlock()
			if _, err = k.ObserveCurrent(context.Background()); err == nil {
				t.Fatalf("%s latest witness accepted: %v", test.name, err)
			}
		})
	}
}

type changingLoadStore struct {
	AtomicStore
	loads  int
	change func()
}

func (s *changingLoadStore) Load(ctx context.Context) (Record, bool, error) {
	s.loads++
	if s.loads == 2 && s.change != nil {
		s.change()
	}
	return s.AtomicStore.Load(ctx)
}

func TestObserveCurrentRejectsRevisionOrTipDriftBetweenReads(t *testing.T) {
	for _, field := range []string{"revision", "tip"} {
		t.Run(field, func(t *testing.T) {
			k, a, _ := fixture(t)
			if _, err := k.Apply(context.Background(), Record{}, json.RawMessage(`{"phase":"closed"}`), nil, Receipt{OperationID: opID(t), RequestSHA256: digest([]byte("closed")), Principal: "owner"}, true); err != nil {
				t.Fatal(err)
			}
			k.Store = &changingLoadStore{AtomicStore: a, change: func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if field == "revision" {
					a.r.Revision++
				} else {
					a.r.Tip.TotalBytes++
				}
			}}
			if _, err := k.ObserveCurrent(context.Background()); !errors.Is(err, ErrConflict) {
				t.Fatalf("second-read %s drift accepted: %v", field, err)
			}
		})
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
