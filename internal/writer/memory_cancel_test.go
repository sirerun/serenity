package writer

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestMemoryCancellationFencesMissingOperationAcrossRestart(t *testing.T) {
	sources := store.NewSourceStore(t.TempDir())
	q := NewQueue(nil)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Now().UTC()
	first, err := w.CancelRemoteOperation("lost-write", "withdrawn", now)
	if err != nil || first.Record.SHA256 != "" || first.Expired {
		t.Fatalf("cancel missing: %+v %v", first, err)
	}
	all, err := sources.All()
	if err != nil || len(all) != 1 {
		t.Fatal("missing durable fence", err)
	}
	q.Close()
	q = NewQueue(nil)
	defer q.Close()
	w.Queue = q
	if _, err := w.CancelRemoteOperation("lost-write", "different retry reason", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	again, err := sources.All()
	if err != nil || len(again) != len(all) {
		t.Fatal("retry duplicated fence", err)
	}
	input := RememberInput{OperationKey: "lost-write", Fact: "revoked before arrival", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	if _, err = w.Remember(input, now); !errors.Is(err, ErrMemoryOperationCanceled) {
		t.Fatalf("late write: %v", err)
	}
	p, err := store.LoadMemoryProjection(sources)
	if err != nil || len(p.All()) != 0 {
		t.Fatal("canceled write made content", err)
	}
	input.OperationKey = "allowed-positive-control"
	if _, err = w.Remember(input, now); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryCancellationOrdersWithConcurrentRemember(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(t.TempDir())
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Now().UTC()
	input := RememberInput{OperationKey: "concurrent", Fact: "shared record", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			_, err := w.Remember(input, now)
			if err != nil && !errors.Is(err, ErrMemoryOperationCanceled) {
				t.Error(err)
			}
		})
	}
	wg.Go(func() {
		if _, err := w.CancelRemoteOperation(input.OperationKey, "withdrawn", now); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	p, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.OperationCancellation(input.OperationKey); !ok {
		t.Fatal("no durable cancellation")
	}
	for _, rec := range p.All() {
		if !rec.Expired(now) {
			t.Fatal("acknowledged cancellation left live fact")
		}
	}
	recovered, err := w.Remember(input, now)
	if err == nil && (recovered.Inserted || !recovered.Record.Expired(now)) {
		t.Fatal("retry revived canceled operation")
	}
	if err != nil && !errors.Is(err, ErrMemoryOperationCanceled) {
		t.Fatal(err)
	}
}

func TestMemoryCancellationPreservesPrivateScope(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(t.TempDir())
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Now().UTC()
	input := RememberInput{OperationKey: "private-key", Fact: "private", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityPrivate}
	rec, err := w.Remember(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.CancelRemoteOperation(input.OperationKey, "remote", now); !errors.Is(err, ErrMemoryScopeDenied) {
		t.Fatalf("private cancel: %v", err)
	}
	p, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := p.Get(rec.Record.SHA256)
	if current.Expired(now) {
		t.Fatal("remote changed private fact")
	}
	if _, ok := p.OperationCancellation(input.OperationKey); ok {
		t.Fatal("remote fenced private operation")
	}
}
