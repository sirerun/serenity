package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/writer"
)

// TestBatchDropsUnsafeObservationsAndPublishesRest is the reproduction of
// SEC-H03 and FUN-03 at the ingest boundary: the review's newline-subject
// payload (a subject that would become a directory name and inject a
// duplicate `type:` YAML key), its alias-hijack variant and a benign
// non-slug subject are each dropped and counted per observation, the safe
// observations in the same batch are still published and committed, no file
// with a control character or a colon in its name is committed by anything,
// and every entity page in the brain still parses afterwards.
func TestBatchDropsUnsafeObservationsAndPublishesRest(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	batch := []domain.Observation{
		obs("acme\ntype: person\naliases: [\"alice-tan\"]", "works_at", "Evil Corp", "source-evil", "0-10", .9, now),
		obs("demo-person", "works_at", "Acme Corp", "source-good", "0-10", .9, now),
		obs("acme\naliases: [\"alice-tan\"]", "works_at", "Evil Corp", "source-evil", "10-20", .9, now),
		obs("acme/inc", "works_at", "Acme Corp", "source-benign", "0-10", .9, now),
		obs("demo-person", "has_role", "Engineer", "source-good", "10-20", .9, now),
	}
	stats, err := w.Write(batch)
	if err != nil {
		t.Fatalf("Write must drop unsafe observations, not abort the batch: %v", err)
	}
	if stats.Rejected != 3 {
		t.Fatalf("Rejected = %d, want 3 (one per unsafe observation)", stats.Rejected)
	}
	if stats.Written != 2 {
		t.Fatalf("Written = %d, want 2 (the safe observations still publish)", stats.Written)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Fence.PathFor("topic", "demo-person")); err != nil {
		t.Fatalf("safe page not published: %v", err)
	}
	brain := filepath.Join(w.Fence.Root, "brain")
	var pages []string
	err = filepath.WalkDir(brain, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(brain, path)
		if err != nil {
			return err
		}
		if strings.ContainsAny(rel, "\n\r:\x00") {
			t.Fatalf("unsafe path reached the brain: %q", rel)
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".md") && strings.HasPrefix(rel, "entities") {
			pages = append(pages, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected exactly one entity page, got %v", pages)
	}
	for _, page := range pages {
		if _, err := w.Fence.ParseEntity(page); err != nil {
			t.Fatalf("brain no longer parsable after the batch: %s: %v", page, err)
		}
	}
	tracked := batchGit(t, w.Fence.Root, "ls-files", "-z", "brain")
	for _, path := range strings.Split(strings.TrimSuffix(tracked, "\x00"), "\x00") {
		if strings.ContainsAny(path, "\n\r:") {
			t.Fatalf("unsafe path committed: %q", path)
		}
	}
	if status := batchGit(t, w.Fence.Root, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("working tree not clean after Flush:\n%s", status)
	}
	// Re-running the same batch is a no-op for the safe observations and
	// still drops the unsafe ones without aborting.
	again, err := w.Write(batch)
	if err != nil {
		t.Fatalf("second Write aborted: %v", err)
	}
	if again.Rejected != 3 || again.Written != 0 || again.Skipped != 2 {
		t.Fatalf("second Write stats = %+v, want Rejected 3, Written 0, Skipped 2", again)
	}
}

// TestReviewDropsUnsafeSubjectAndKeepsBatch pins FUN-03 at the review
// step that sync calls before Write: an observation whose subject is not a
// canonical slug is counted in ReviewPlan.Rejected and the remaining
// observations are still planned, instead of the whole batch aborting.
func TestReviewDropsUnsafeSubjectAndKeepsBatch(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ds := reviewStore(t, w)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	batch := []domain.Observation{
		obs("acme\ntype: person", "works_at", "Evil Corp", "source-evil", "0-10", .9, now),
		obs("demo-person", "works_at", "Acme Corp", "source-good", "0-10", .9, now),
		obs("x[1]", "works_at", "Acme Corp", "source-glob", "0-10", .9, now),
	}
	plan, err := w.ReviewObservations(context.Background(), ds, batch, now)
	if err != nil {
		t.Fatalf("ReviewObservations must drop unsafe observations, not abort: %v", err)
	}
	if plan.Rejected != 2 {
		t.Fatalf("Rejected = %d, want 2", plan.Rejected)
	}
	if len(plan.Ready) != 1 || plan.Ready[0].SubjectSlug != "demo-person" {
		t.Fatalf("Ready = %+v, want only demo-person", plan.Ready)
	}
}
