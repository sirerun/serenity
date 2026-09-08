package disposition

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

func TestTextStoredDispositionCanBeDecidedAndBookkept(t *testing.T) {
	for _, mode := range []string{"dispose", "result"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			eng, err := index.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = eng.Close() }()
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			store := NewStore(eng)
			ctx := context.Background()
			item, err := store.Create(ctx, KindReconcile, nil, "", fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "result" {
				if _, err := store.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:reviewer", "", fixedNow); err != nil {
					t.Fatal(err)
				}
			}
			// SQLite accepts TEXT in this BLOB-affinity column. SQL JSON edits also
			// produce TEXT; Store.Get already accepts those same valid JSON bytes.
			if _, err := db.Exec(`UPDATE disposition_items SET payload=CAST(payload AS TEXT) WHERE id=?`, item.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(ctx, item.ID); err != nil {
				t.Fatalf("TEXT row unreadable: %v", err)
			}
			bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if mode == "dispose" {
				_, err = store.Dispose(bounded, item.ID, VerdictAccept, nil, "", "human:reviewer", "", fixedNow)
			} else {
				err = store.RecordResultClaimID(bounded, item.ID, "human-claim", fixedNow.Add(time.Minute))
			}
			if err != nil {
				t.Fatalf("readable TEXT row spun until failure: %v", err)
			}
			got, err := store.Get(ctx, item.ID)
			if err != nil || got.State != StateDisposed || got.Actor != "human:reviewer" {
				t.Fatalf("decision=%+v err=%v", got, err)
			}
			if mode == "result" && got.AppliedClaimID != "human-claim" {
				t.Fatal("bookkeeping lost")
			}
			history, err := store.HistoryFor(ctx, item.ID)
			if err != nil || len(history) != 1 {
				t.Fatalf("history=%d err=%v", len(history), err)
			}
		})
	}
}
