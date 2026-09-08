package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/writer"
)

func reviewStore(t *testing.T, w *Writer) *disposition.Store {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(w.Fence.Root, ".serenity"), 0755); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(w.Fence.Root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

func TestReviewConflictStagesOnceAndRetainsRejection(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			w, closeQ := newTestWriter(t)
			defer closeQ()
			ctx := context.Background()
			now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			old := obs("demo-person", family, "Old value", "source-old", "0-10", .9, now)
			if _, err := w.Write([]domain.Observation{old}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
				t.Fatal(err)
			}
			ds := reviewStore(t, w)
			incoming := obs("demo-person", family, "New value", "source-new", "0-10", .9, now)
			plan, err := w.ReviewObservations(ctx, ds, []domain.Observation{incoming}, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Ready) != 0 || len(plan.Proposals) != 1 || plan.Proposals[0].B.Object != "Old value" {
				t.Fatalf("incorrect review plan: %+v", plan)
			}
			created, existing, err := w.StageReview(ctx, ds, plan.Proposals, now)
			if err != nil {
				t.Fatal(err)
			}
			if created != 1 || existing != 0 {
				t.Fatalf("stage counts=%d/%d", created, existing)
			}
			incoming.CreatedAt = now.Add(time.Hour)
			repeat, err := w.ReviewObservations(ctx, ds, []domain.Observation{incoming}, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			created, existing, err = w.StageReview(ctx, ds, repeat.Proposals, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if created != 0 || existing != 1 {
				t.Fatalf("duplicate stage counts=%d/%d", created, existing)
			}
			items, err := ds.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("items=%d", len(items))
			}
			if _, err := ds.Dispose(ctx, items[0].ID, disposition.VerdictReject, nil, "human rejects unchanged source", "human:reviewer", "", now); err != nil {
				t.Fatal(err)
			}
			repeat, err = w.ReviewObservations(ctx, ds, []domain.Observation{incoming}, now.Add(2*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if len(repeat.Ready) != 0 || len(repeat.Proposals) != 0 || repeat.PriorDecision != 1 {
				t.Fatalf("rejection not retained: %+v", repeat)
			}
		})
	}
}

func TestReviewSameBatchPriorMustCommitBeforeStaging(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ds := reviewStore(t, w)
	ctx := context.Background()
	now := time.Now()
	observations := []domain.Observation{obs("demo-person", "works_at", "First company", "source-one", "0-10", .9, now), obs("demo-person", "works_at", "Second company", "source-two", "0-10", .9, now)}
	plan, err := w.ReviewObservations(ctx, ds, observations, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Ready) != 1 || len(plan.Proposals) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if _, _, err := w.StageReview(ctx, ds, plan.Proposals, now); err == nil {
		t.Fatal("staged a nonexistent canonical prior")
	}
	items, err := ds.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatal("failed preflight wrote review items")
	}
	if _, err := w.Write(plan.Ready); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.StageReview(ctx, ds, plan.Proposals, now); err == nil {
		t.Fatal("staged against uncommitted machine output")
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	created, _, err := w.StageReview(ctx, ds, plan.Proposals, now)
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatal("published prior did not permit review")
	}
}

func TestReviewUsesHumanCanonicalChangeAndRejectsLaterMutation(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ctx := context.Background()
	now := time.Now()
	old := obs("demo-person", "works_at", "Old company", "source-old", "0-10", .9, now)
	if _, err := w.Write([]domain.Observation{old}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	ds := reviewStore(t, w)
	path := w.Fence.PathFor("topic", "demo-person")
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	page.Claims[0].Object = "Human current company"
	if _, err := w.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	batchGit(t, w.Fence.Root, "add", "brain")
	batchGit(t, w.Fence.Root, "commit", "--quiet", "-m", "human change")
	incoming := obs("demo-person", "works_at", "Machine proposal", "source-new", "0-10", .9, now)
	plan, err := w.ReviewObservations(ctx, ds, []domain.Observation{incoming}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Proposals) != 1 || plan.Proposals[0].B.Object != "Human current company" {
		t.Fatalf("review used stale canonical value: %+v", plan)
	}
	page.Claims[0].State = domain.StateRetracted
	if _, err := w.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	batchGit(t, w.Fence.Root, "add", "brain")
	batchGit(t, w.Fence.Root, "commit", "--quiet", "-m", "human retraction")
	if _, _, err := w.StageReview(ctx, ds, plan.Proposals, now); err == nil {
		t.Fatal("staged against a retracted prior")
	}
	repeated, err := w.ReviewObservations(ctx, ds, []domain.Observation{old}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated.Ready) != 0 || repeated.AlreadyPresent != 1 {
		t.Fatalf("reintroduced a retracted source claim: %+v", repeated)
	}
}

func TestReviewCompactedShardDoesNotReactivateArchivedObservation(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ctx := context.Background()
	now := time.Now().UTC()
	old := obs("demo-person", "has_balance", "100 USD", "old-source", "0-10", .9, now)
	if _, err := w.Write([]domain.Observation{old}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	replacement := ClaimFromObservation(obs("demo-person", "has_balance", "200 USD", "new-source", "0-10", .9, now))
	replacement.Supersedes = ClaimFromObservation(old).ID
	if err := w.Shard.Append(replacement); err != nil {
		t.Fatal(err)
	}
	if moved, err := w.Shard.Compact("demo-person", "has_balance"); err != nil || moved != 1 {
		t.Fatalf("compact: moved=%d err=%v", moved, err)
	}
	batchGit(t, w.Fence.Root, "add", "brain")
	batchGit(t, w.Fence.Root, "commit", "--quiet", "-m", "reviewed replacement and compaction")
	ds := reviewStore(t, w)
	plan, err := w.ReviewObservations(ctx, ds, []domain.Observation{old}, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AlreadyPresent != 1 || len(plan.Ready) != 0 || len(plan.Proposals) != 0 {
		t.Fatalf("archived evidence reactivated: %+v", plan)
	}
	incoming := obs("demo-person", "has_balance", "300 USD", "third-source", "0-10", .9, now)
	plan, err = w.ReviewObservations(ctx, ds, []domain.Observation{incoming}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Proposals) != 1 || plan.Proposals[0].B.ID != replacement.ID {
		t.Fatalf("wrong compacted canonical prior: %+v", plan)
	}
}
