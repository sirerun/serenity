package disposition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/writer"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return NewStore(eng)
}

var fixedNow = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func TestCreateThenGetRoundTrips(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindReconcile, json.RawMessage(`{"a":"1"}`), "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item.ID == "" {
		t.Fatal("Create returned an empty id")
	}
	if item.State != StatePending {
		t.Fatalf("State = %q, want %q", item.State, StatePending)
	}

	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != KindReconcile {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindReconcile)
	}
	if string(got.Payload) != `{"a":"1"}` {
		t.Fatalf("Payload = %q, want %q", got.Payload, `{"a":"1"}`)
	}
}

func TestGetMissingReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	_, err := s.Get(ctx, "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get: err = %v, want ErrNotFound", err)
	}
}

// TestDisposeRetriedWithSameIdempotencyKeyReturnsOriginalResult is the
// acc-line clause: "dispose retried with the same idempotency_key returns
// the original result."
func TestDisposeRetriedWithSameIdempotencyKeyReturnsOriginalResult(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	first, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:david", "idem-1", fixedNow)
	if err != nil {
		t.Fatalf("Dispose (first): %v", err)
	}
	if first.AlreadyDisposed || first.Replayed {
		t.Fatalf("first dispose: AlreadyDisposed=%v Replayed=%v, want both false", first.AlreadyDisposed, first.Replayed)
	}
	if first.Item.State != StateDisposed || first.Item.Verdict != VerdictAccept {
		t.Fatalf("first dispose: State=%q Verdict=%q, want disposed/accept", first.Item.State, first.Item.Verdict)
	}

	// Retry with the SAME idempotency_key, a later clock tick, and even a
	// different verdict -- the original result must come back unchanged,
	// not be reprocessed.
	retry, err := s.Dispose(ctx, item.ID, VerdictReject, nil, "reason", "human:someone-else", "idem-1", fixedNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("Dispose (retry): %v", err)
	}
	if !retry.Replayed {
		t.Fatal("retry with the same idempotency_key: Replayed = false, want true")
	}
	if retry.Item.Verdict != VerdictAccept {
		t.Fatalf("retry result Verdict = %q, want the ORIGINAL verdict %q", retry.Item.Verdict, VerdictAccept)
	}

	// Exactly one history row must exist for this item -- the retry must
	// not have appended a second one.
	history, err := s.HistoryFor(ctx, item.ID)
	if err != nil {
		t.Fatalf("HistoryFor: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("HistoryFor returned %d rows, want exactly 1 (retry must not append)", len(history))
	}
}

// TestSecondDisposeOfDisposedItemReturnsAlreadyDisposed is the acc-line
// clause: "a second dispose of a disposed item returns already_disposed
// with the recorded verdict."
func TestSecondDisposeOfDisposedItemReturnsAlreadyDisposed(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindEffect, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:david", "key-a", fixedNow); err != nil {
		t.Fatalf("Dispose (first): %v", err)
	}

	// A genuinely new dispose call (different idempotency_key, simulating
	// a second client) against the now-disposed item.
	second, err := s.Dispose(ctx, item.ID, VerdictReject, nil, "too late", "human:other-client", "key-b", fixedNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("Dispose (second): %v", err)
	}
	if !second.AlreadyDisposed {
		t.Fatal("second dispose of a disposed item: AlreadyDisposed = false, want true")
	}
	if second.Item.Verdict != VerdictAccept {
		t.Fatalf("second dispose result Verdict = %q, want the RECORDED verdict %q", second.Item.Verdict, VerdictAccept)
	}

	// The second (losing) client's verdict must never have been applied.
	history, err := s.HistoryFor(ctx, item.ID)
	if err != nil {
		t.Fatalf("HistoryFor: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("HistoryFor returned %d rows, want exactly 1 (the losing dispose must not append)", len(history))
	}
}

// TestRejectWithoutNoteErrors is the acc-line clause: "reject without a
// note errors."
func TestRejectWithoutNoteErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindPreceptDraft, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = s.Dispose(ctx, item.ID, VerdictReject, nil, "", "human:david", "", fixedNow)
	if !errors.Is(err, ErrRejectRequiresNote) {
		t.Fatalf("Dispose(reject, no note): err = %v, want ErrRejectRequiresNote", err)
	}

	// The item must be untouched -- still pending, no history row.
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StatePending {
		t.Fatalf("State = %q after a rejected (errored) dispose call, want unchanged %q", got.State, StatePending)
	}
	history, err := s.HistoryFor(ctx, item.ID)
	if err != nil {
		t.Fatalf("HistoryFor: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("HistoryFor returned %d rows, want 0 (a rejected-for-no-note call must not write)", len(history))
	}

	// Reject WITH a note succeeds.
	res, err := s.Dispose(ctx, item.ID, VerdictReject, nil, "not a real precept", "human:david", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose(reject, with note): %v", err)
	}
	if res.Item.State != StateDisposed || res.Item.Verdict != VerdictReject {
		t.Fatalf("State=%q Verdict=%q, want disposed/reject", res.Item.State, res.Item.Verdict)
	}
}

func TestDisposeInvalidVerdictErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindDistill, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = s.Dispose(ctx, item.ID, Verdict("approve"), nil, "", "human:david", "", fixedNow)
	if !errors.Is(err, ErrInvalidVerdict) {
		t.Fatalf("Dispose(bad verdict): err = %v, want ErrInvalidVerdict", err)
	}
}

func TestDisposeDeferIncrementsDeferCountAndStaysOpen(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindTombstone, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	res, err := s.Dispose(ctx, item.ID, VerdictDefer, nil, "", "human:david", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose(defer): %v", err)
	}
	if res.Item.State != StateDeferred {
		t.Fatalf("State = %q, want %q", res.Item.State, StateDeferred)
	}
	if res.Item.DeferCount != 1 {
		t.Fatalf("DeferCount = %d, want 1", res.Item.DeferCount)
	}
	// A deferred item is not disposed -- a further dispose call processes
	// normally rather than reporting already_disposed.
	res2, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:david", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose(accept after defer): %v", err)
	}
	if res2.AlreadyDisposed {
		t.Fatal("a deferred (not yet disposed) item reported AlreadyDisposed")
	}
	if res2.Item.State != StateDisposed {
		t.Fatalf("State = %q, want %q", res2.Item.State, StateDisposed)
	}
}

// TestImportPendingCreatesDirtyEditItemsAndDeletesFiles is the acc-line
// clause: ".serenity/pending/*.json records import as dirty_edit items and
// are deleted."
func TestImportPendingCreatesDirtyEditItemsAndDeletesFiles(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	root := t.TempDir()

	pendingDir := filepath.Join(root, ".serenity", "pending")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	rec := writer.PendingRecord{
		Path:       "brain/entities/person/alice-tan.md",
		Human:      "human bytes",
		Machine:    "machine bytes",
		DetectedAt: fixedNow.Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	pendingPath := filepath.Join(pendingDir, "alice-tan.json")
	if err := os.WriteFile(pendingPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	n, err := s.ImportPending(ctx, root, fixedNow)
	if err != nil {
		t.Fatalf("ImportPending: %v", err)
	}
	if n != 1 {
		t.Fatalf("ImportPending returned %d, want 1", n)
	}

	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending record file still exists after import: err = %v", err)
	}

	items, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}
	if items[0].Kind != KindDirtyEdit {
		t.Fatalf("Kind = %q, want %q", items[0].Kind, KindDirtyEdit)
	}
	if items[0].State != StatePending {
		t.Fatalf("State = %q, want %q", items[0].State, StatePending)
	}
	var gotRec writer.PendingRecord
	if err := json.Unmarshal(items[0].Payload, &gotRec); err != nil {
		t.Fatalf("decode item payload: %v", err)
	}
	if gotRec.Path != rec.Path || gotRec.Human != rec.Human || gotRec.Machine != rec.Machine {
		t.Fatalf("imported payload = %+v, want %+v", gotRec, rec)
	}
}

// TestImportPendingOnMissingDirectoryIsZeroNotError proves a brain that has
// never had a paused write does not error.
func TestImportPendingOnMissingDirectoryIsZeroNotError(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	root := t.TempDir()

	n, err := s.ImportPending(ctx, root, fixedNow)
	if err != nil {
		t.Fatalf("ImportPending: %v", err)
	}
	if n != 0 {
		t.Fatalf("ImportPending = %d, want 0", n)
	}
}

func TestImportPendingIsIdempotentAcrossRepeatedRuns(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	root := t.TempDir()

	pendingDir := filepath.Join(root, ".serenity", "pending")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	rec := writer.PendingRecord{Path: "brain/entities/person/bob.md", Human: "h", Machine: "m", DetectedAt: fixedNow.Format(time.RFC3339)}
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(pendingDir, "bob.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if n, err := s.ImportPending(ctx, root, fixedNow); err != nil || n != 1 {
		t.Fatalf("first ImportPending: n=%d err=%v", n, err)
	}
	// The file is gone, so a second run is a true no-op.
	if n, err := s.ImportPending(ctx, root, fixedNow); err != nil || n != 0 {
		t.Fatalf("second ImportPending: n=%d err=%v, want 0/nil", n, err)
	}
	items, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items after two import runs, want 1 (no duplicate)", len(items))
	}
}

// TestTwoClientDisposeRaceExactlyOneWins is T2.8's acc line: "go test -race
// with two goroutine clients over 100 iterations: exactly one wins per item
// and the loser receives already_disposed carrying the winner's verdict."
// Each iteration races two goroutines -- simulating two clients disposing
// the same item concurrently -- against a fresh item over a real *index.SQLite
// backend (openTestStore), each with its own idempotency_key so neither call
// can take the replay path (TestDisposeRetriedWithSameIdempotencyKeyReturnsOriginalResult
// covers that path separately); this exercises the genuine cross-client
// conflict rule instead.
func TestTwoClientDisposeRaceExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	const iterations = 100
	verdicts := [2]Verdict{VerdictAccept, VerdictReject}
	notes := [2]string{"", "client-b lost the race"} // reject requires a note
	actors := [2]string{"human:client-a", "human:client-b"}

	for i := range iterations {
		item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
		if err != nil {
			t.Fatalf("iteration %d: Create: %v", i, err)
		}

		var results [2]Result
		var errs [2]error
		var wg sync.WaitGroup
		wg.Add(2)
		for c := range 2 {
			go func() {
				defer wg.Done()
				key := fmt.Sprintf("iter-%d-client-%d", i, c)
				results[c], errs[c] = s.Dispose(ctx, item.ID, verdicts[c], nil, notes[c], actors[c], key, fixedNow)
			}()
		}
		wg.Wait()

		for c := range 2 {
			if errs[c] != nil {
				t.Fatalf("iteration %d client %d: Dispose: %v", i, c, errs[c])
			}
		}

		winners, losers := 0, 0
		var winnerVerdict Verdict
		for c := range 2 {
			if results[c].AlreadyDisposed {
				losers++
			} else {
				winners++
				winnerVerdict = results[c].Item.Verdict
			}
		}
		if winners != 1 || losers != 1 {
			t.Fatalf("iteration %d: winners=%d losers=%d, want exactly 1 each (results=%+v)", i, winners, losers, results)
		}

		for c := range 2 {
			if results[c].AlreadyDisposed && results[c].Item.Verdict != winnerVerdict {
				t.Fatalf("iteration %d: loser's returned Verdict = %q, want the WINNER's verdict %q", i, results[c].Item.Verdict, winnerVerdict)
			}
		}

		history, err := s.HistoryFor(ctx, item.ID)
		if err != nil {
			t.Fatalf("iteration %d: HistoryFor: %v", i, err)
		}
		if len(history) != 1 {
			t.Fatalf("iteration %d: HistoryFor returned %d rows, want exactly 1 (the loser must not append)", i, len(history))
		}
	}
}

func TestListOrdersByCreatedAt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	first, err := s.Create(ctx, KindDistill, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := s.Create(ctx, KindDistill, nil, "", fixedNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	items, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("List order = %+v, want [%s, %s]", items, first.ID, second.ID)
	}
}
