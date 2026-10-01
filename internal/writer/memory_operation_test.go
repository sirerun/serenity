package writer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestCanonicalOperationEntersInsideQueueBeforeSourceWrite(t *testing.T) {
	sources := store.NewSourceStore(t.TempDir())
	q := NewQueue(nil)
	defer q.Close()
	w := MemoryFact{Queue: q, Sources: sources}
	var entered bool
	opID := "fedcba9876543210"
	ctx := WithCanonicalOperation(context.Background(), CanonicalOperation{
		ID: opID,
		BeforeCommit: func(_ context.Context, got string) error {
			if got != opID {
				t.Fatalf("operation ID = %q, want %q", got, opID)
			}
			if all, err := sources.All(); err != nil || len(all) != 0 {
				t.Fatalf("source write preceded EnterCanonical: facts=%d err=%v", len(all), err)
			}
			entered = true
			return nil
		},
	})
	result, err := w.RememberContext(ctx, RememberInput{Fact: "remembered", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, time.Now())
	if err != nil || !entered {
		t.Fatalf("RememberContext: entered=%v err=%v", entered, err)
	}
	if result.Record.Payload.CanonicalOperationID != opID {
		t.Fatalf("canonical operation ID = %q", result.Record.Payload.CanonicalOperationID)
	}
	data, _, err := sources.Read(result.Record.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := store.DecodeMemoryFact(data)
	if err != nil || decoded.CanonicalOperationID != opID {
		t.Fatalf("stored canonical operation ID = %q, err=%v", decoded.CanonicalOperationID, err)
	}
}

func TestCanonicalOperationCallbackFailureWritesNoSource(t *testing.T) {
	sources := store.NewSourceStore(t.TempDir())
	q := NewQueue(nil)
	defer q.Close()
	w := MemoryFact{Queue: q, Sources: sources}
	wantErr := errors.New("ledger refused operation")
	ctx := WithCanonicalOperation(context.Background(), CanonicalOperation{
		ID:           "fedcba9876543210",
		BeforeCommit: func(context.Context, string) error { return wantErr },
	})
	_, err := w.RememberContext(ctx, RememberInput{Fact: "must not land", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, time.Now())
	if !errors.Is(err, wantErr) {
		t.Fatalf("RememberContext err = %v", err)
	}
	if all, err := sources.All(); err != nil || len(all) != 0 {
		t.Fatalf("source persisted after callback refusal: facts=%d err=%v", len(all), err)
	}
}

func TestMemoryOperationRecoverySurvivesWithdrawalAndRestart(t *testing.T) {
	sources := store.NewSourceStore(t.TempDir())
	q := NewQueue(nil)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	until := now.Add(time.Hour)
	input := RememberInput{OperationKey: "ajent:post-1:revision-1", Fact: "bounded evidence", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, ValidUntil: &until}
	first, err := w.Remember(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Forget(first.Record.SHA256, "withdrawn", now); err != nil {
		t.Fatal(err)
	}
	q.Close()
	q = NewQueue(nil)
	defer q.Close()
	w.Queue = q
	// Forget erased the fact (ADR 019) and fenced its key: a retry, with the
	// same or a changed payload, is refused and never writes the text back.
	if _, err := w.Remember(input, now.Add(2*time.Hour)); !errors.Is(err, ErrMemoryOperationCanceled) {
		t.Fatalf("retry after withdrawal: %v", err)
	}
	if _, _, err := sources.Read(first.Record.SHA256); err == nil {
		t.Fatal("retry revived withdrawn fact")
	}
	changed := input
	changed.Fact = "different claim"
	if _, err := w.Remember(changed, now); !errors.Is(err, ErrMemoryOperationCanceled) {
		t.Fatalf("changed payload: %v", err)
	}
	changed = input
	changed.OperationKey = "ajent:post-1:revision-2"
	next, err := w.Remember(changed, now)
	if err != nil || !next.Inserted || next.Record.SHA256 == first.Record.SHA256 {
		t.Fatalf("distinct key did not own distinct fact: %v", err)
	}
	projection, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.All()) != 1 {
		t.Fatal("unexpected canonical fact count")
	}
}

func TestMemoryOperationConcurrentRetryHasOneCanonicalFact(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(t.TempDir())
	w := MemoryFact{Queue: q, Sources: sources}
	input := RememberInput{OperationKey: "same-key", Fact: "same fact", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	var wg sync.WaitGroup
	ids := make(chan string, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := w.Remember(input, time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			ids <- result.Record.SHA256
		}()
	}
	wg.Wait()
	close(ids)
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	if len(unique) != 1 {
		t.Fatalf("got %d fact identities", len(unique))
	}
	projection, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.All()) != 1 {
		t.Fatal("duplicate durable records")
	}
}

func TestMemoryOperationRejectsEveryChangedDurableField(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(t.TempDir())
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	input := RememberInput{OperationKey: "immutable", Fact: "fact", Provenance: "source", EntitySlug: "item", EntityType: "project", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, ValidUntil: &expiry}
	if _, err := w.Remember(input, now); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*RememberInput){
		"fact":           func(p *RememberInput) { p.Fact = "changed" },
		"provenance":     func(p *RememberInput) { p.Provenance = "changed" },
		"entity_slug":    func(p *RememberInput) { p.EntitySlug = "other" },
		"entity_type":    func(p *RememberInput) { p.EntityType = "company" },
		"kind":           func(p *RememberInput) { p.Kind = store.MemoryFactKindBelief },
		"visibility":     func(p *RememberInput) { p.Visibility = store.MemoryVisibilityPrivate },
		"expiry":         func(p *RememberInput) { later := expiry.Add(time.Second); p.ValidUntil = &later },
		"omitted_expiry": func(p *RememberInput) { p.ValidUntil = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p := input
			change(&p)
			if _, err := w.Remember(p, now); !errors.Is(err, ErrMemoryOperationConflict) {
				t.Fatalf("got %v", err)
			}
		})
	}
	projection, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.All()) != 1 {
		t.Fatal("conflict changed canonical records")
	}
}
