package authority

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

type localTransport struct {
	s           *Server
	principal   string
	measurement Measurement
}

func (l localTransport) Invoke(ctx context.Context, q Request) (Response, error) {
	if q.Action == "prepare" || q.Action == "renew" {
		m := l.measurement
		q.Measurement = &m
	}
	return l.s.execute(ctx, l.principal, q)
}
func activeFixture(t *testing.T) (*Server, *Client, Lease, *time.Time, *memoryAtomic, *memoryWitness) {
	t.Helper()
	k, a, w := fixture(t)
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	m := Measurement{ExecutableSHA256: digest([]byte("binary")), ProcessLifetime: "pid:123:start:456"}
	s, e := NewServer(k, ServerConfig{RuntimePrincipal: "runtime", OwnerPrincipal: "owner", ApprovedBuilds: map[string]string{m.ExecutableSHA256: strings.Repeat("a", 40)}, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if _, e = s.execute(ctx, "owner", Request{Action: "enroll", OperationID: opID(t)}); e != nil {
		t.Fatal(e)
	}
	c, e := NewClient(localTransport{s: s, principal: "runtime", measurement: m})
	if e != nil {
		t.Fatal(e)
	}
	ch, e := c.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Prepare(ctx, ch)
	if e != nil {
		t.Fatal(e)
	}
	r, e := deletion.NewJournalReader(c.Reader(p), p.Position)
	if e != nil {
		t.Fatal(e)
	}
	obs, e := r.Observe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	l, e := c.Activate(ctx, p, obs.HistorySHA256, opID(t))
	if e != nil {
		t.Fatal(e)
	}
	return s, c, l, &now, a, w
}
func appendBody(t *testing.T, l Lease, now time.Time) []byte {
	t.Helper()
	b, e := (contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: 1, WriterID: l.WriterID, SubjectType: contracts.DeletionSubjectAccount, SubjectID: "synthetic-account", Outcome: contracts.DeletionIntentRequested, RecordedAt: now}).Encode()
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestStaleReceiptCannotAuthorizeOldWriter(t *testing.T) {
	s, c, l, now, _, _ := activeFixture(t)
	ctx := context.Background()
	body := appendBody(t, l, *now)
	store := c.Store(l)
	key := contracts.JournalKey(1, 1)
	if _, e := store.PutIfAbsent(ctx, key, body); e != nil {
		t.Fatal(e)
	}
	if _, e := store.PutIfAbsent(ctx, key, body); e != nil {
		t.Fatal(e)
	}
	ch, e := c.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Prepare(ctx, ch); e != nil {
		t.Fatal(e)
	}
	if _, e = store.PutIfAbsent(ctx, key, body); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale receipt returned success: %v", e)
	}
	if _, e = s.execute(ctx, "runtime", Request{Action: "seal", OperationID: opID(t), LeaseID: l.ID, Key: key, Body: body}); !errors.Is(e, ErrForbidden) {
		t.Fatalf("runtime owner seal: %v", e)
	}
}
func TestRenewExactMeasurementAndBoundedExpiry(t *testing.T) {
	s, c, l, now, _, _ := activeFixture(t)
	ctx := context.Background()
	if _, e := c.Renew(ctx, l, opID(t)); !errors.Is(e, ErrConflict) {
		t.Fatalf("renew did not strictly extend: %v", e)
	}
	*now = now.Add(10 * time.Minute)
	op := opID(t)
	renewed, e := c.Renew(ctx, l, op)
	if e != nil {
		t.Fatal(e)
	}
	if !renewed.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatal("renewal unbounded")
	}
	again, e := c.Renew(ctx, l, op)
	if e != nil || again.ExpiresAt != renewed.ExpiresAt {
		t.Fatal("renewal replay changed result")
	}
	m := Measurement{ExecutableSHA256: digest([]byte("binary")), ProcessLifetime: "different-process"}
	if _, e = s.execute(ctx, "runtime", Request{Action: "renew", OperationID: opID(t), LeaseID: l.ID, Measurement: &m}); !errors.Is(e, ErrForbidden) {
		t.Fatal("renew accepted different process")
	}
	*now = now.Add(2 * time.Hour)
	if _, e = c.Renew(ctx, l, op); !errors.Is(e, ErrExpired) {
		t.Fatalf("expired cached renewal resurrected: %v", e)
	}
}
func TestHistoricalCutOperatorReadAndTableRestore(t *testing.T) {
	s, c, l, now, oldStore, w := activeFixture(t)
	ctx := context.Background()
	body := appendBody(t, l, *now)
	if _, e := c.Store(l).PutIfAbsent(ctx, contracts.JournalKey(1, 1), body); e != nil {
		t.Fatal(e)
	}
	cut, e := c.Cut(ctx, l, opID(t), opID(t))
	if e != nil {
		t.Fatal(e)
	}
	manifest := digest([]byte("manifest"))
	f, e := c.FinalizeCut(ctx, cut, manifest, opID(t))
	if e != nil {
		t.Fatal(e)
	}
	*now = now.Add(2 * time.Hour)
	owner, _ := NewClient(localTransport{s: s, principal: "owner"})
	got, e := owner.ReadFinalizedCut(ctx, cut.ID)
	if e != nil || got.ManifestSHA256 != f.ManifestSHA256 {
		t.Fatalf("historical finalized cut unavailable: %v", e)
	}
	if _, found, e := owner.CutReader(cut).Get(ctx, contracts.JournalKey(1, 1)); e != nil || !found {
		t.Fatalf("owner historical read failed: %v", e)
	}
	if _, _, e = c.CutReader(cut).Get(ctx, contracts.JournalKey(1, 1)); !errors.Is(e, ErrExpired) {
		t.Fatalf("runtime expired cut accepted: %v", e)
	}
	replacement := newMemoryAtomic()
	newKernel := &Kernel{Store: replacement, Witness: w, Lineage: s.kernel.Lineage, TableARN: s.kernel.TableARN + "-replacement"}
	newServer, e := NewServer(newKernel, s.config)
	if e != nil {
		t.Fatal(e)
	}
	op := opID(t)
	if _, e = newServer.execute(ctx, "runtime", Request{Action: "restore", OperationID: op}); !errors.Is(e, ErrForbidden) {
		t.Fatal("runtime restored authority")
	}
	if _, e = newServer.execute(ctx, "owner", Request{Action: "restore", OperationID: op}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.kernel.Current(ctx); !errors.Is(e, ErrConflict) {
		t.Fatalf("retired table current accepted: %v", e)
	}
	if _, e = newKernel.Current(ctx); e != nil {
		t.Fatal(e)
	}
	if len(replacement.items) == 0 || len(oldStore.items) == 0 {
		t.Fatal("restore did not preserve history")
	}
	newOwner, _ := NewClient(localTransport{s: newServer, principal: "owner"})
	got, e = newOwner.ReadFinalizedCut(ctx, cut.ID)
	if e != nil || got.ManifestSHA256 != manifest {
		t.Fatalf("restored cut invalid: %v", e)
	}
}
func TestRequestDuplicateFieldsAndIrrelevantMeasurement(t *testing.T) {
	var r Request
	if strictDecode([]byte(`{"action":"current","action":"enroll"}`), &r) == nil {
		t.Fatal("duplicate action accepted")
	}
	if validateRequest(Request{Action: "current", LeaseID: strings.Repeat("a", 64), Measurement: &Measurement{}}) == nil {
		t.Fatal("unrelated assertion accepted")
	}
}

type advancingImport struct {
	*memoryAtomic
	advance func() error
}

func (a *advancingImport) Import(ctx context.Context, items []ImmutableItem) error {
	if err := a.memoryAtomic.Import(ctx, items); err != nil {
		return err
	}
	return a.advance()
}
func TestRestoreSourceAdvanceCannotPublishCandidate(t *testing.T) {
	s, c, _, _, _, w := activeFixture(t)
	ctx := context.Background()
	destination := &advancingImport{memoryAtomic: newMemoryAtomic(), advance: func() error { _, e := c.Challenge(ctx); return e }}
	k := &Kernel{Store: destination, Witness: w, Lineage: s.kernel.Lineage, TableARN: s.kernel.TableARN + "-candidate"}
	candidate, e := NewServer(k, s.config)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = candidate.execute(ctx, "owner", Request{Action: "restore", OperationID: opID(t)}); !errors.Is(e, ErrConflict) {
		t.Fatalf("moving-source restore accepted: %v", e)
	}
	if _, e = k.Current(ctx); !errors.Is(e, ErrConflict) {
		t.Fatalf("failed candidate published: %v", e)
	}
	if _, e = s.kernel.Current(ctx); e != nil {
		t.Fatalf("source corrupted by rejected restore: %v", e)
	}
}
func TestConcurrentDifferentAppendOneCanonicalWinner(t *testing.T) {
	s, c, l, now, _, _ := activeFixture(t)
	ctx := context.Background()
	body := appendBody(t, l, *now)
	other := []byte(strings.Replace(string(body), "synthetic-account", "other-account", 1))
	results := make(chan error, 2)
	for _, b := range [][]byte{body, other} {
		go func(b []byte) { _, e := c.Store(l).PutIfAbsent(ctx, contracts.JournalKey(1, 1), b); results <- e }(b)
	}
	success := 0
	for range 2 {
		if e := <-results; e == nil {
			success++
		} else if !errors.Is(e, ErrConflict) {
			t.Fatalf("unexpected append error: %v", e)
		}
	}
	if success != 1 {
		t.Fatalf("got %d winners", success)
	}
	r, e := s.kernel.Current(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var state protocolState
	if e = strictDecode(r.State, &state); e != nil {
		t.Fatal(e)
	}
	if state.Position.LastObjectInGeneration.SequenceID != 1 {
		t.Fatal("head advanced twice")
	}
}

func TestFailedPreparationCannotConsumeFreshDataGuard(t *testing.T) {
	k, _, _ := fixture(t)
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	m := Measurement{ExecutableSHA256: digest([]byte("fresh")), ProcessLifetime: "process"}
	s, e := NewServer(k, ServerConfig{RuntimePrincipal: "runtime", OwnerPrincipal: "owner", ApprovedBuilds: map[string]string{m.ExecutableSHA256: strings.Repeat("a", 40)}, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	owner, _ := NewClient(localTransport{s: s, principal: "owner"})
	if e = owner.Enroll(ctx, opID(t)); e != nil {
		t.Fatal(e)
	}
	c, _ := NewClient(localTransport{s: s, principal: "runtime", measurement: m})
	for range 2 {
		ch, e := c.Challenge(ctx)
		if e != nil {
			t.Fatal(e)
		}
		p, e := c.Prepare(ctx, ch)
		if e != nil {
			t.Fatal(e)
		}
		if !p.RequireEmptyData {
			t.Fatal("failed prepare consumed initial empty-data requirement")
		}
		if e = c.ClosePrepared(ctx, p, opID(t)); e != nil {
			t.Fatal(e)
		}
	}
	ch, e := c.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Prepare(ctx, ch)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := deletion.NewJournalReader(c.Reader(p), p.Position)
	if e != nil {
		t.Fatal(e)
	}
	ob, e := reader.Observe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := c.Activate(ctx, p, ob.HistorySHA256, opID(t))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(ctx, lease, opID(t)); e != nil {
		t.Fatal(e)
	}
	ch, e = c.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e = c.Prepare(ctx, ch)
	if e != nil {
		t.Fatal(e)
	}
	if p.RequireEmptyData {
		t.Fatal("initialized empty journal incorrectly treated as fresh namespace")
	}
}

func TestOwnerInspectAndExactCloseAfterLostResponse(t *testing.T) {
	s, c, lease, _, _, _ := activeFixture(t)
	ctx := context.Background()
	if _, e := c.Inspect(ctx); !errors.Is(e, ErrForbidden) {
		t.Fatalf("runtime inspected owner state: %v", e)
	}
	owner, _ := NewClient(localTransport{s: s, principal: "owner"})
	status, e := owner.Inspect(ctx)
	if e != nil || status.Lease == nil || status.Lease.ID != lease.ID {
		t.Fatalf("owner could not discover exact lease: %v", e)
	}
	wrong := lease
	wrong.ID = opID(t)
	if e = owner.Close(ctx, wrong, opID(t)); !errors.Is(e, ErrConflict) {
		t.Fatalf("wrong lease closed active epoch: %v", e)
	}
	if e = owner.Close(ctx, *status.Lease, opID(t)); e != nil {
		t.Fatal(e)
	}
	status, e = owner.Inspect(ctx)
	if e != nil || status.Lease != nil || status.Prepared != nil {
		t.Fatalf("owner close not durably visible: %v", e)
	}
	ch, e := c.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Prepare(ctx, ch)
	if e != nil {
		t.Fatal(e)
	}
	status, e = owner.Inspect(ctx)
	if e != nil || status.Prepared == nil || status.Prepared.ReservationID != p.ReservationID {
		t.Fatalf("owner could not discover prepared reservation: %v", e)
	}
	if e = owner.ClosePrepared(ctx, *status.Prepared, opID(t)); e != nil {
		t.Fatal(e)
	}
}
