package supersede

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func publicationFixture(t *testing.T, family, entityType string) (*Writer, *disposition.Store, disposition.Item, func(...string) string) {
	t.Helper()
	root, git := gitRepoFixture(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fw, ss := store.NewFenceWriter(root), store.NewShardStore(root)
	old := claimFixture("old-claim", "demo-person", family, "Old value")
	page := store.NewEntityPage(domain.Entity{Type: entityType, Slug: old.SubjectSlug})
	head := old
	if config.Default().TierOf(family) == domain.TierShard {
		if err := ss.Append(old); err != nil {
			t.Fatal(err)
		}
		head.SourceRef = "shard"
	}
	page.Claims = []domain.Claim{head}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "canonical old claim")
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	w := New(q, fw, ss, config.Default())
	ds := dispositionStoreFixture(t, root)
	proposed := claimFixture("proposal", old.SubjectSlug, family, "New value")
	item := seedReconcileItem(t, ds, context.Background(), fixedNow, proposed, old)
	res, err := ds.Dispose(context.Background(), item.ID, disposition.VerdictAccept, nil, "", "human:original-reviewer", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return w, ds, res.Item, git
}

func TestReconcilePublicationCommitsAndRetriesWithoutDuplicate(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			w, ds, item, git := publicationFixture(t, family, "person")
			before := git("rev-parse", "HEAD")
			id, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			after := git("rev-parse", "HEAD")
			if before == after {
				t.Fatal("accepted claim was not committed")
			}
			if status := git("status", "--porcelain"); status != "" {
				t.Fatalf("dirty after publication: %s", status)
			}
			page, err := w.Fence.ParseEntity(w.Fence.PathFor("person", "demo-person"))
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, claim := range page.Claims {
				if claim.ID == id && claim.State == domain.StateActive && claim.Object == "New value" {
					found++
				}
			}
			if found != 1 {
				t.Fatalf("active accepted rows = %d", found)
			}
			got, err := ds.Get(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.AppliedClaimID != id || got.Actor != "human:original-reviewer" {
				t.Fatalf("incorrect result: %+v", got)
			}
			again, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if again != id || git("rev-parse", "HEAD") != after {
				t.Fatal("retry changed claim identity or committed again")
			}
			history, err := ds.HistoryFor(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(history) != 1 {
				t.Fatalf("retry added decision history: %d", len(history))
			}
			if family == "has_balance" {
				lines, err := w.Shard.Lines("demo-person", family)
				if err != nil {
					t.Fatal(err)
				}
				if len(lines) != 2 {
					t.Fatalf("shard lines = %d, want 2", len(lines))
				}
			}
		})
	}
}

func TestReconcilePublicationCommitFailureCanResume(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			w, ds, item, git := publicationFixture(t, family, "topic")
			hook := filepath.Join(w.Fence.Root, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
				t.Fatal(err)
			}
			before := git("rev-parse", "HEAD")
			if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err == nil || !strings.Contains(err.Error(), "awaits commit") {
				t.Fatalf("expected commit failure, got %v", err)
			}
			got, err := ds.Get(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.AppliedClaimID != "" || before != git("rev-parse", "HEAD") {
				t.Fatal("failed commit recorded as applied")
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if status := git("status", "--porcelain"); status != "" {
				t.Fatalf("retry did not commit all output: %s", status)
			}
			if family == "has_balance" {
				lines, err := w.Shard.Lines("demo-person", family)
				if err != nil {
					t.Fatal(err)
				}
				if len(lines) != 2 {
					t.Fatalf("retry duplicated shard: %d", len(lines))
				}
			}
		})
	}
}

func TestReconcilePublicationPreservesHumanChangeDuringFailedCommit(t *testing.T) {
	w, ds, item, git := publicationFixture(t, "works_at", "topic")
	hook := filepath.Join(w.Fence.Root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err == nil {
		t.Fatal("expected commit failure")
	}
	path := w.Fence.PathFor("topic", "demo-person")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	human := append(raw, []byte("\nHuman explanation after failed commit.\n")...)
	if err := os.WriteFile(path, human, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	before := git("rev-parse", "HEAD")
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err == nil || !strings.Contains(err.Error(), "changed since approval") {
		t.Fatalf("expected preserved edit, got %v", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(human) || before != git("rev-parse", "HEAD") {
		t.Fatal("retry changed human bytes or committed them")
	}
}

func TestReconcilePublicationRejectsStaleOrUncommittedPriorClaim(t *testing.T) {
	for _, change := range []string{"dirty-prose", "committed-value", "retracted", "deleted"} {
		t.Run(change, func(t *testing.T) {
			w, ds, item, git := publicationFixture(t, "works_at", "topic")
			path := w.Fence.PathFor("topic", "demo-person")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "dirty-prose":
				err = os.WriteFile(path, append(raw, []byte("\nHuman note\n")...), 0644)
			case "deleted":
				err = os.Remove(path)
			default:
				page, parseErr := w.Fence.ParseEntity(path)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				if change == "retracted" {
					page.Claims[0].State = domain.StateRetracted
				} else {
					page.Claims[0].Object = "Human replacement"
				}
				_, err = w.Fence.WriteEntity(page)
			}
			if err != nil {
				t.Fatal(err)
			}
			if change != "dirty-prose" {
				git("add", "brain")
				git("commit", "--quiet", "-m", "human revision")
			}
			before := git("rev-parse", "HEAD")
			if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err == nil {
				t.Fatal("stale proposal unexpectedly applied")
			}
			if before != git("rev-parse", "HEAD") {
				t.Fatal("stale proposal created a commit")
			}
			got, err := ds.Get(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.AppliedClaimID != "" {
				t.Fatal("stale proposal marked applied")
			}
		})
	}
}

func TestReconcilePublicationPreservesUnrelatedStagedFile(t *testing.T) {
	w, ds, item, git := publicationFixture(t, "has_balance", "topic")
	path := filepath.Join(w.Fence.Root, "seed.txt")
	if err := os.WriteFile(path, []byte("human staged change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "seed.txt")
	before := git("show", ":seed.txt")
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err != nil {
		t.Fatal(err)
	}
	if git("show", ":seed.txt") != before || git("show", "HEAD:seed.txt") != "seed\n" {
		t.Fatal("publication swept unrelated staged file into commit")
	}
}

func TestReconcilePublicationCompletedReceiptDoesNotReplayLaterRetraction(t *testing.T) {
	w, ds, item, git := publicationFixture(t, "works_at", "topic")
	id, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	path := w.Fence.PathFor("topic", "demo-person")
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range page.Claims {
		if page.Claims[i].ID == id {
			page.Claims[i].State = domain.StateRetracted
		}
	}
	if _, err := w.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	git("add", "brain")
	git("commit", "--quiet", "-m", "human retraction")
	before := git("rev-parse", "HEAD")
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if git("rev-parse", "HEAD") != before || git("status", "--porcelain") != "" {
		t.Fatal("completed retry replayed over later human retraction")
	}
}

func TestReconcilePublicationShardRotationResumesExactlyOnce(t *testing.T) {
	w, ds, item, git := publicationFixture(t, "has_balance", "topic")
	w.Shard.RolloverBytes = 1
	paths, err := filepath.Glob(filepath.Join(w.Fence.Root, "brain", "claims", "demo-person", "*.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("seed shard: %v %v", paths, err)
	}
	old, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(w.Fence.Root, ".git/hooks/pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow); err == nil {
		t.Fatal("expected failed commit")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitReconcile(context.Background(), ds, item, fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(old) {
		t.Fatal("rotation rewrote old canonical segment")
	}
	paths, err = filepath.Glob(filepath.Join(w.Fence.Root, "brain", "claims", "demo-person", "*.jsonl"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("expected exactly one new segment: %v %v", paths, err)
	}
	lines, err := w.Shard.Lines("demo-person", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("duplicate rotated claim: %d", len(lines))
	}
	if git("status", "--porcelain") != "" {
		t.Fatal("rotation left uncommitted files")
	}
}
