package index

import (
	"context"
	"path/filepath"
	"testing"
)

func openDispositionTestDB(t *testing.T) *SQLite {
	t.Helper()
	eng, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

func TestPutDispositionItemThenReadRoundTrips(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)

	if err := eng.PutDispositionItem(ctx, "item-1", []byte(`{"state":"pending"}`)); err != nil {
		t.Fatalf("PutDispositionItem: %v", err)
	}
	payload, found, err := eng.DispositionItem(ctx, "item-1")
	if err != nil {
		t.Fatalf("DispositionItem: %v", err)
	}
	if !found {
		t.Fatal("DispositionItem: found = false, want true")
	}
	if string(payload) != `{"state":"pending"}` {
		t.Fatalf("payload = %q, want %q", payload, `{"state":"pending"}`)
	}
}

func TestPutDispositionItemUpsertsOnConflict(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)

	if err := eng.PutDispositionItem(ctx, "item-1", []byte(`{"state":"pending"}`)); err != nil {
		t.Fatalf("PutDispositionItem: %v", err)
	}
	if err := eng.PutDispositionItem(ctx, "item-1", []byte(`{"state":"disposed"}`)); err != nil {
		t.Fatalf("PutDispositionItem (update): %v", err)
	}
	payload, found, err := eng.DispositionItem(ctx, "item-1")
	if err != nil || !found {
		t.Fatalf("DispositionItem: found=%v err=%v", found, err)
	}
	if string(payload) != `{"state":"disposed"}` {
		t.Fatalf("payload = %q, want the updated value", payload)
	}

	items, err := eng.DispositionItems(ctx)
	if err != nil {
		t.Fatalf("DispositionItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("DispositionItems returned %d rows, want 1 (upsert must not duplicate)", len(items))
	}
}

func TestDispositionItemMissingIsNotFoundNotError(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)

	payload, found, err := eng.DispositionItem(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("DispositionItem: unexpected error %v", err)
	}
	if found {
		t.Fatal("found = true for a nonexistent id")
	}
	if payload != nil {
		t.Fatalf("payload = %v, want nil", payload)
	}
}

func TestAppendDispositionHistoryIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)

	if err := eng.AppendDispositionHistory(ctx, "hist-1", []byte(`{"verdict":"accept"}`)); err != nil {
		t.Fatalf("AppendDispositionHistory: %v", err)
	}
	// A second append under the SAME id is a caller bug (ids are freshly
	// random per event) and must fail loudly rather than silently
	// overwrite the prior event -- proving history rows are never updated.
	if err := eng.AppendDispositionHistory(ctx, "hist-1", []byte(`{"verdict":"reject"}`)); err == nil {
		t.Fatal("AppendDispositionHistory: second append with the same id succeeded, want a conflict error")
	}

	history, err := eng.DispositionHistory(ctx)
	if err != nil {
		t.Fatalf("DispositionHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("DispositionHistory returned %d rows, want 1", len(history))
	}
	if string(history[0]) != `{"verdict":"accept"}` {
		t.Fatalf("history[0] = %q, want the original (unoverwritten) payload", history[0])
	}
}

func TestDispositionItemsListsAllRows(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := eng.PutDispositionItem(ctx, id, []byte(`{}`)); err != nil {
			t.Fatalf("PutDispositionItem(%s): %v", id, err)
		}
	}
	items, err := eng.DispositionItems(ctx)
	if err != nil {
		t.Fatalf("DispositionItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("DispositionItems returned %d rows, want 3", len(items))
	}
}
