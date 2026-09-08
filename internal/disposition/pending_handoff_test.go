package disposition

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/sirerun/serenity/internal/index"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

func writePendingFixture(t *testing.T, root, key string, rec writer.PendingRecord) string {
	t.Helper()
	path := writer.PendingPath(root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPendingReplayPreservesHumanDecisionAndDistinctConflict(t *testing.T) {
	s := openTestStore(t)
	root := t.TempDir()
	ctx := context.Background()
	rec := writer.PendingRecord{Path: "brain/entities/person/ava.md", Human: "Original human prose.", Machine: "First proposed update.", DetectedAt: fixedNow.Format(time.RFC3339)}
	writePendingFixture(t, root, "ava", rec)
	if _, err := s.ImportPending(ctx, root, fixedNow); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("initial import: %d %v", len(items), err)
	}
	original := items[0]
	if _, err := s.Dispose(ctx, original.ID, VerdictReject, nil, "keep my prose", "human:reviewer", "review", fixedNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Attempt time can change while the same conflict evidence is retried.
	rec.DetectedAt = fixedNow.Add(time.Hour).Format(time.RFC3339)
	writePendingFixture(t, root, "ava", rec)
	if _, err := s.ImportPending(ctx, root, fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Get(ctx, original.ID)
	if err != nil || stored.State != StateDisposed || stored.Verdict != VerdictReject || stored.Note != "keep my prose" || stored.Actor != "human:reviewer" {
		t.Fatalf("replayed producer reset human decision: %+v %v", stored, err)
	}
	rec.Machine = "A genuinely different proposal."
	rec.DetectedAt = fixedNow.Add(2 * time.Hour).Format(time.RFC3339)
	writePendingFixture(t, root, "ava", rec)
	if _, err := s.ImportPending(ctx, root, fixedNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = s.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("distinct conflict overwrote existing decision: count=%d err=%v", len(items), err)
	}
	stored, err = s.Get(ctx, original.ID)
	if err != nil || stored.State != StateDisposed || string(stored.Payload) != string(original.Payload) {
		t.Fatalf("original evidence changed: %+v %v", stored, err)
	}
}

type pendingProducerBackend struct {
	Backend
	once    sync.Once
	produce func()
}

func (b *pendingProducerBackend) PutDispositionItem(ctx context.Context, id string, payload []byte) error {
	err := b.Backend.PutDispositionItem(ctx, id, payload)
	if err == nil {
		b.once.Do(b.produce)
	}
	return err
}

func (b *pendingProducerBackend) InsertDispositionItem(ctx context.Context, id string, payload []byte) (bool, error) {
	inserted, err := b.Backend.InsertDispositionItem(ctx, id, payload)
	if err == nil {
		b.once.Do(b.produce)
	}
	return inserted, err
}

func TestPendingImportCannotDeleteNewerProducerRecord(t *testing.T) {
	s := openTestStore(t)
	root := t.TempDir()
	ctx := context.Background()
	old := writer.PendingRecord{Path: "brain/entities/person/ava.md", Human: "Human prose.", Machine: "First update.", DetectedAt: fixedNow.Format(time.RFC3339)}
	newer := old
	newer.Machine = "Second update while import stages first."
	path := writePendingFixture(t, root, "ava", old)
	backend := &pendingProducerBackend{Backend: s.backend, produce: func() { writePendingFixture(t, root, "ava", newer) }}
	if _, err := NewStore(backend).ImportPending(ctx, root, fixedNow); err != nil {
		t.Fatal(err)
	}
	// The importer may consume the new record in the same pass or leave it for
	// the next pass; it must never unlink it without recording its content.
	if _, err := os.Stat(path); err == nil {
		if _, err := s.ImportPending(ctx, root, fixedNow.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	items, err := s.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("newer producer record disappeared: items=%d err=%v", len(items), err)
	}
	seen := map[string]bool{}
	for _, item := range items {
		var rec writer.PendingRecord
		if err := json.Unmarshal(item.Payload, &rec); err != nil {
			t.Fatal(err)
		}
		seen[rec.Machine] = true
	}
	if !seen[old.Machine] || !seen[newer.Machine] {
		t.Fatalf("producer evidence lost: %v", seen)
	}
}

func pendingStoreFixture(t *testing.T) (string, *Store, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".serenity", "index.db")
	eng, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Error(err)
		}
	})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return root, NewStore(eng), db
}

func TestPendingStorageFailureRetainsClaimedEvidence(t *testing.T) {
	root, s, db := pendingStoreFixture(t)
	ctx := context.Background()
	old := writer.PendingRecord{Path: "brain/entities/person/ava.md", Human: "Human prose.", Machine: "Old paused update.", DetectedAt: fixedNow.Format(time.RFC3339)}
	writePendingFixture(t, root, "ava", old)
	if _, err := db.Exec(`CREATE TRIGGER audit_reject_pending BEFORE INSERT ON disposition_items WHEN json_extract(NEW.payload, '$.kind') = 'dirty_edit' BEGIN SELECT RAISE(ABORT, 'review store unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportPending(ctx, root, fixedNow); err == nil {
		t.Fatal("storage failure swallowed")
	}
	claimed, err := filepath.Glob(filepath.Join(root, ".serenity", "pending", ".claimed", "*", "*.json"))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("unconsumed evidence not retained: count=%d err=%v", len(claimed), err)
	}
	raw, err := os.ReadFile(claimed[0])
	if err != nil {
		t.Fatal(err)
	}
	var retained writer.PendingRecord
	if err := json.Unmarshal(raw, &retained); err != nil || retained != old {
		t.Fatalf("claimed evidence changed: %+v %v", retained, err)
	}
	newer := old
	newer.Machine = "New producer update."
	writePendingFixture(t, root, "ava", newer)
	if _, err := db.Exec(`DROP TRIGGER audit_reject_pending`); err != nil {
		t.Fatal(err)
	}
	if n, err := NewStore(s.backend).ImportPending(ctx, root, fixedNow.Add(time.Hour)); err != nil || n != 2 {
		t.Fatalf("recovery consumed=%d err=%v", n, err)
	}
	items, err := s.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("recovery items=%d err=%v", len(items), err)
	}
	if n, err := s.ImportPending(ctx, root, fixedNow.Add(2*time.Hour)); err != nil || n != 0 {
		t.Fatalf("recovery replay=%d err=%v", n, err)
	}
}

func TestPendingLegacyReviewSurvivesMovedBrain(t *testing.T) {
	s := openTestStore(t)
	root := t.TempDir()
	ctx := context.Background()
	rec := writer.PendingRecord{Path: filepath.Join(t.TempDir(), "old-brain", "page.md"), Human: "Legacy human content.", Machine: "Legacy proposal.", DetectedAt: fixedNow.Format(time.RFC3339)}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.createWithID(ctx, "dirty_edit:ava", KindDirtyEdit, raw, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispose(ctx, item.ID, VerdictReject, nil, "keep old decision", "human:legacy", "legacy-key", fixedNow); err != nil {
		t.Fatal(err)
	}
	writePendingFixture(t, root, "ava", rec)
	if _, err := s.ImportPending(ctx, root, fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(ctx)
	if err != nil || len(items) != 1 || items[0].Verdict != VerdictReject || string(items[0].Payload) != string(raw) {
		t.Fatalf("legacy decision changed: %+v %v", items, err)
	}
	rec.Machine = "A different proposal after moving."
	writePendingFixture(t, root, "ava", rec)
	if _, err := s.ImportPending(ctx, root, fixedNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = s.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("new proposal lost: count=%d err=%v", len(items), err)
	}
}

func TestPendingIndependentImportersConsumeEachRecordOnce(t *testing.T) {
	root, first, _ := pendingStoreFixture(t)
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	stores := []*Store{first, NewStore(eng)}
	ctx := context.Background()
	for attempt := 0; attempt < 16; attempt++ {
		rec := writer.PendingRecord{Path: "page.md", Human: "Original prose.", Machine: fmt.Sprintf("Proposal %d.", attempt), DetectedAt: fixedNow.Format(time.RFC3339)}
		writePendingFixture(t, root, "page", rec)
		start := make(chan struct{})
		var done sync.WaitGroup
		var counts [2]int
		var errs [2]error
		for i := range 2 {
			done.Add(1)
			go func(i int) {
				defer done.Done()
				<-start
				counts[i], errs[i] = stores[i].ImportPending(ctx, root, fixedNow)
			}(i)
		}
		close(start)
		done.Wait()
		if errs[0] != nil || errs[1] != nil || counts[0]+counts[1] != 1 {
			t.Fatalf("attempt %d: counts=%v errors=%v", attempt, counts, errs)
		}
	}
	items, err := first.List(ctx)
	if err != nil || len(items) != 16 {
		t.Fatalf("concurrent staging: count=%d err=%v", len(items), err)
	}
}

func TestPendingInvalidInputIsRetainedWithoutStaging(t *testing.T) {
	for _, mode := range []string{"malformed-json", "symlink-record", "symlink-directory"} {
		t.Run(mode, func(t *testing.T) {
			s := openTestStore(t)
			root := t.TempDir()
			outside := t.TempDir()
			raw := []byte("{incomplete")
			outsideFile := filepath.Join(outside, "record.json")
			if err := os.WriteFile(outsideFile, raw, 0600); err != nil {
				t.Fatal(err)
			}
			pending := filepath.Join(root, ".serenity", "pending")
			if err := os.MkdirAll(pending, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(pending, "page.json")
			switch mode {
			case "malformed-json":
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink-record":
				if err := os.Symlink(outsideFile, path); err != nil {
					t.Fatal(err)
				}
			case "symlink-directory":
				if err := os.Remove(pending); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, pending); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ImportPending(context.Background(), root, fixedNow); err == nil {
				t.Fatal("unsafe/corrupt input silently consumed")
			}
			items, err := s.List(context.Background())
			if err != nil || len(items) != 0 {
				t.Fatalf("invalid input staged: count=%d err=%v", len(items), err)
			}
			got, err := os.ReadFile(outsideFile)
			if err != nil || string(got) != string(raw) {
				t.Fatalf("outside evidence changed: %v", err)
			}
			if mode == "malformed-json" {
				claimed, err := filepath.Glob(filepath.Join(pending, ".claimed", "*", "page.json"))
				if err != nil || len(claimed) != 1 {
					t.Fatalf("corrupt record not retained: %v %v", claimed, err)
				}
			}
		})
	}
}
