package writer

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

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
	recovered, err := w.Remember(input, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Inserted || recovered.Record.SHA256 != first.Record.SHA256 || !recovered.Record.Expired(now) {
		t.Fatal("retry revived or changed withdrawn fact")
	}
	changed := input
	changed.Fact = "different claim"
	if _, err := w.Remember(changed, now); !errors.Is(err, ErrMemoryOperationConflict) {
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
	if len(projection.All()) != 2 {
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
