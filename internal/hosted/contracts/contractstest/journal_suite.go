package contractstest

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// JournalFactory builds the journal under test on top of store.
type JournalFactory func(store contracts.JournalObjectStore, writerID string, generation int64, now func() time.Time) contracts.DeletionJournal

func entry(subject string, outcome contracts.DeletionOutcome) contracts.DeletionEntry {
	return contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: subject, Outcome: outcome}
}

func subjects(es []contracts.DeletionEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.SubjectID)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RunJournalSuite is the executable specification of contracts.DeletionJournal
// over the object layout fixed in contracts/deletion.go. The suite reads and
// tampers with the store directly, so an implementation must use that layout.
func RunJournalSuite(t *testing.T, factory JournalFactory) {
	ctx := context.Background()
	clock := NewClock()
	mk := func() (*MemObjectStore, func(writer string, gen int64) contracts.DeletionJournal) {
		store := NewMemObjectStore()
		return store, func(writer string, gen int64) contracts.DeletionJournal {
			return factory(store, writer, gen, clock.Now)
		}
	}
	must := func(t *testing.T, j contracts.DeletionJournal, e contracts.DeletionEntry) contracts.DeletionEntry {
		t.Helper()
		got, err := j.AppendDeletion(ctx, e)
		if err != nil {
			t.Fatalf("append %s: %v", e.SubjectID, err)
		}
		return got
	}

	t.Run("append_then_read_round_trip", func(t *testing.T) {
		_, journal := mk()
		a := journal("w1", 1)
		for _, s := range []string{"acc-1", "acc-2", "brain-3"} {
			must(t, a, entry(s, contracts.DeletionIntentRequested))
		}
		res, err := a.ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || !sameStrings(subjects(res.Entries), []string{"acc-1", "acc-2", "brain-3"}) || res.Sealed {
			t.Fatalf("read = %+v, %v", res, err)
		}
		for i, e := range res.Entries {
			if e.Watermark.Generation != 1 || e.Watermark.SequenceID != int64(i+1) || e.Watermark.EntryHash == "" {
				t.Fatalf("entry %d watermark %+v", i, e.Watermark)
			}
		}
		again, err := a.ReadThrough(ctx, res.To)
		if err != nil || len(again.Entries) != 0 || again.To != res.To {
			t.Fatalf("resumed read = %+v, %v; want nothing new", again, err)
		}
		if _, err := a.AppendDeletion(ctx, entry("person@example.com", contracts.DeletionIntentRequested)); !errors.Is(err, contracts.ErrDeletionEntryInvalid) {
			t.Fatalf("an email subject was accepted: %v", err)
		}
	})

	t.Run("incremental_read_sees_entries_whose_subject_sorts_earlier", func(t *testing.T) {
		_, journal := mk()
		a := journal("w1", 1)
		must(t, a, entry("zzz-account", contracts.DeletionIntentRequested))
		first, err := a.ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || len(first.Entries) != 1 {
			t.Fatal(first, err)
		}
		must(t, a, entry("aaa-account", contracts.DeletionIntentRequested)) // sorts before the consumed subject
		next, err := a.ReadThrough(ctx, first.To)
		if err != nil || !sameStrings(subjects(next.Entries), []string{"aaa-account"}) {
			t.Fatalf("incremental read = %+v, %v; the earlier-sorting entry was lost", next, err)
		}
	})

	t.Run("missing_middle_entry_is_incomplete", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		var es []contracts.DeletionEntry
		for _, s := range []string{"a1", "a2", "a3", "a4"} {
			es = append(es, must(t, a, entry(s, contracts.DeletionIntentRequested)))
		}
		store.Delete(contracts.JournalKey(1, 2))
		if _, err := a.ReadThrough(ctx, contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("read across a missing middle entry: %v, want ErrDeletionJournalIncomplete", err)
		}
		if _, err := a.ReadThrough(ctx, es[0].Watermark); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("resumed read across a missing middle entry: %v, want ErrDeletionJournalIncomplete", err)
		}
	})

	t.Run("entry_removed_after_seal_breaks_the_chain", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		for _, s := range []string{"a1", "a2", "a3"} {
			must(t, a, entry(s, contracts.DeletionIntentRequested))
		}
		if _, err := journal("recovery", 2).Seal(ctx, 1); err != nil {
			t.Fatal(err)
		}
		store.Delete(contracts.JournalKey(1, 3)) // the last entry, immediately before the seal
		if _, err := a.ReadThrough(ctx, contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("read after a tail entry was removed behind a seal: %v, want ErrDeletionJournalIncomplete", err)
		}
	})

	t.Run("a_snapshot_watermark_detects_a_rewritten_tail", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		var last contracts.DeletionEntry
		for _, s := range []string{"a1", "a2", "a3"} {
			last = must(t, a, entry(s, contracts.DeletionIntentRequested))
		}
		snapshotWatermark := last.Watermark
		store.Delete(contracts.JournalKey(1, 3)) // history is rewritten after the snapshot
		if _, err := journal("recovery", 2).Seal(ctx, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := a.ReadThrough(ctx, snapshotWatermark); !errors.Is(err, contracts.ErrDeletionJournalHistoryMismatch) {
			t.Fatalf("read from a watermark whose object changed: %v, want ErrDeletionJournalHistoryMismatch", err)
		}
		if _, err := a.ReadThrough(ctx, contracts.DeletionWatermark{Generation: 1, SequenceID: 2, EntryHash: "not-the-hash"}); !errors.Is(err, contracts.ErrDeletionJournalHistoryMismatch) {
			t.Fatalf("read from a watermark with a wrong hash: %v, want ErrDeletionJournalHistoryMismatch", err)
		}
	})

	t.Run("a_foreign_append_fences_the_writer", func(t *testing.T) {
		_, journal := mk()
		a, b := journal("old-writer", 1), journal("new-writer", 1)
		must(t, a, entry("a1", contracts.DeletionIntentRequested))
		must(t, b, entry("b2", contracts.DeletionIntentRequested)) // takes the position a expects next
		if _, err := a.AppendDeletion(ctx, entry("a3", contracts.DeletionIntentRequested)); !errors.Is(err, contracts.ErrDeletionJournalFenced) {
			t.Fatalf("stale writer append = %v, want ErrDeletionJournalFenced", err)
		}
	})

	t.Run("a_lost_put_response_is_retried_idempotently", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		store.LoseNextPutResponse = true
		must(t, a, entry("a1", contracts.DeletionIntentRequested))
		res, err := a.ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || len(res.Entries) != 1 {
			t.Fatalf("read = %+v, %v; want exactly one entry", res, err)
		}
	})

	t.Run("seal_stops_a_stale_writer_and_is_idempotent", func(t *testing.T) {
		_, journal := mk()
		old := journal("old-writer", 1)
		must(t, old, entry("a1", contracts.DeletionIntentRequested))
		must(t, old, entry("a2", contracts.DeletionIntentRequested))
		seal, err := journal("recovery", 2).Seal(ctx, 1)
		if err != nil || seal.Generation != 1 || seal.SequenceID != 3 {
			t.Fatalf("seal = %+v, %v; want generation 1 sequence 3", seal, err)
		}
		if _, err := old.AppendDeletion(ctx, entry("a3", contracts.DeletionIntentRequested)); !errors.Is(err, contracts.ErrDeletionJournalSealed) {
			t.Fatalf("stale writer append after seal = %v, want ErrDeletionJournalSealed", err)
		}
		res, err := old.ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || !res.Sealed || res.To != seal || !sameStrings(subjects(res.Entries), []string{"a1", "a2"}) {
			t.Fatalf("sealed read = %+v, %v", res, err)
		}
		if again, err := journal("recovery", 2).Seal(ctx, 1); err != nil || again != seal {
			t.Fatalf("second seal = %+v, %v; want the same watermark", again, err)
		}
	})

	t.Run("an_object_beyond_the_seal_violates_the_fence", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		must(t, a, entry("a1", contracts.DeletionIntentRequested))
		if _, err := journal("recovery", 2).Seal(ctx, 1); err != nil {
			t.Fatal(err)
		}
		store.Overwrite(contracts.JournalKey(1, 3), []byte(`{"kind":"entry"}`)) // a writer that ignored the seal
		if _, err := a.ReadThrough(ctx, contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
			t.Fatalf("read = %v, want ErrDeletionJournalFenceViolated", err)
		}
		if _, err := journal("recovery", 2).Seal(ctx, 1); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
			t.Fatalf("seal = %v, want ErrDeletionJournalFenceViolated", err)
		}
	})

	t.Run("a_replaced_middle_entry_breaks_the_chain", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		for _, s := range []string{"a1", "a2", "a3"} {
			must(t, a, entry(s, contracts.DeletionIntentRequested))
		}
		// Same key and coordinates, different content: keys stay contiguous, so
		// only the hash chain can notice.
		forged, err := contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: 2, WriterID: "forger",
			SubjectType: contracts.DeletionSubjectAccount, SubjectID: "a2-forged", Outcome: contracts.DeletionIntentRequested}.Encode()
		if err != nil {
			t.Fatal(err)
		}
		store.Overwrite(contracts.JournalKey(1, 2), forged)
		if _, err := a.ReadThrough(ctx, contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("read over a replaced middle entry = %v, want ErrDeletionJournalIncomplete", err)
		}
	})

	t.Run("a_writer_skipping_ahead_of_the_seal_is_caught", func(t *testing.T) {
		store, journal := mk()
		a := journal("w1", 1)
		must(t, a, entry("a1", contracts.DeletionIntentRequested))
		var once sync.Once
		store.BeforePut = func(key string) {
			// A non-cooperating writer jumps past the seal position just before the seal lands.
			if key == contracts.JournalKey(1, 2) {
				once.Do(func() {
					store.BeforePut = nil
					store.Overwrite(contracts.JournalKey(1, 3), []byte(`{"kind":"entry","generation":1,"sequence_id":3}`))
				})
			}
		}
		if _, err := journal("recovery", 2).Seal(ctx, 1); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
			t.Fatalf("seal with a writer already beyond it = %v, want ErrDeletionJournalFenceViolated", err)
		}
	})

	t.Run("a_new_generation_requires_a_sealed_predecessor", func(t *testing.T) {
		_, journal := mk()
		old, next := journal("old-writer", 1), journal("new-writer", 2)
		must(t, old, entry("a1", contracts.DeletionIntentRequested))
		if _, err := next.AppendDeletion(ctx, entry("b1", contracts.DeletionIntentRequested)); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("append to generation 2 with generation 1 unsealed = %v, want ErrDeletionJournalIncomplete", err)
		}
		seal, err := journal("recovery", 2).Seal(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		must(t, next, entry("b1", contracts.DeletionIntentRequested))
		all, err := next.ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || !sameStrings(subjects(all.Entries), []string{"a1", "b1"}) || all.Sealed || all.To.Generation != 2 {
			t.Fatalf("read across generations = %+v, %v", all, err)
		}
		after, err := next.ReadThrough(ctx, seal)
		if err != nil || !sameStrings(subjects(after.Entries), []string{"b1"}) {
			t.Fatalf("read after the seal = %+v, %v", after, err)
		}
	})

	t.Run("seal_that_loses_a_position_to_a_live_writer_retries_after_it", func(t *testing.T) {
		store, journal := mk()
		live := journal("old-writer", 1)
		must(t, live, entry("a1", contracts.DeletionIntentRequested))
		var once sync.Once
		store.BeforePut = func(key string) {
			// The recovery seal is about to claim position 2; the live writer's
			// append lands there first.
			if key == contracts.JournalKey(1, 2) {
				once.Do(func() {
					store.BeforePut = nil
					must(t, live, entry("a2-late", contracts.DeletionIntentRequested))
				})
			}
		}
		seal, err := journal("recovery", 2).Seal(ctx, 1)
		if err != nil || seal.SequenceID != 3 {
			t.Fatalf("seal = %+v, %v; want it to land at sequence 3, after the late append", seal, err)
		}
		res, err := journal("recovery", 2).ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || !res.Sealed || !sameStrings(subjects(res.Entries), []string{"a1", "a2-late"}) {
			t.Fatalf("sealed read = %+v, %v; the late acknowledged append must be inside the seal", res, err)
		}
		if _, err := live.AppendDeletion(ctx, entry("a3", contracts.DeletionIntentRequested)); !errors.Is(err, contracts.ErrDeletionJournalSealed) {
			t.Fatalf("append after seal = %v, want ErrDeletionJournalSealed", err)
		}
	})

	t.Run("a_concurrent_seal_loses_no_acknowledged_append", func(t *testing.T) {
		// Bounded and order-independent: however the writer and the seal
		// interleave, every append the writer was told succeeded is inside the
		// sealed journal, and nothing the writer was told failed is.
		_, journal := mk()
		writer := journal("old-writer", 1)
		var (
			mu    sync.Mutex
			acked []string
		)
		started, done := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 200; i++ {
				id := fmt.Sprintf("acc-%03d", i)
				if _, err := writer.AppendDeletion(ctx, entry(id, contracts.DeletionIntentRequested)); err != nil {
					if !errors.Is(err, contracts.ErrDeletionJournalSealed) {
						t.Errorf("writer stopped with %v, want ErrDeletionJournalSealed", err)
					}
					return
				}
				mu.Lock()
				acked = append(acked, id)
				mu.Unlock()
				if i == 2 {
					close(started)
				}
				runtime.Gosched()
			}
		}()
		<-started
		seal, err := journal("recovery", 2).Seal(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		<-done
		res, err := journal("recovery", 2).ReadThrough(ctx, contracts.DeletionWatermark{})
		if err != nil || !res.Sealed || res.To != seal {
			t.Fatalf("sealed read = %+v, %v", res, err)
		}
		mu.Lock()
		defer mu.Unlock()
		if !sameStrings(subjects(res.Entries), acked) {
			t.Fatalf("sealed journal holds %d entries, writer acknowledged %d", len(res.Entries), len(acked))
		}
	})
}
