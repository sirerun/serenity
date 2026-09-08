package supersede

import (
	"context"
	"encoding/json"
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

func distillPublicationFixture(t *testing.T, family string) (*Writer, *disposition.Store, disposition.Item, func(...string) string) {
	t.Helper()
	root, git := gitRepoFixture(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "ignore runtime")
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	w := New(q, store.NewFenceWriter(root), store.NewShardStore(root), config.Default())
	ds := dispositionStoreFixture(t, root)
	raw, err := json.Marshal(disposition.ExtractionPayload{Origin: disposition.ExtractionOrigin, Observation: domain.Observation{ID: "candidate", SubjectSlug: "demo-person", Predicate: family, Object: "Uncertain value", Confidence: .2, SourceSHA256: strings.Repeat("a", 64), Span: "0-10", Model: "fixture@v1", CreatedAt: fixedNow}})
	if err != nil {
		t.Fatal(err)
	}
	item, err := ds.Create(context.Background(), disposition.KindDistill, raw, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return w, ds, item, git
}

func confirmDistill(t *testing.T, w *Writer, ds *disposition.Store, item disposition.Item) disposition.Item {
	t.Helper()
	ctx := context.Background()
	decision, err := w.PreviewDistillAssertion(ctx, item, "Human assertion", "human:reviewer", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ds.Dispose(ctx, item.ID, disposition.VerdictEditAccept, raw, "confirmed human assertion", "human:reviewer", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return result.Item
}

func TestDistillPublicationCommitFailureResumesWithoutRedisposal(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			w, ds, item, git := distillPublicationFixture(t, family)
			ctx := context.Background()
			original := string(item.Payload)
			item = confirmDistill(t, w, ds, item)
			hook := filepath.Join(w.Fence.Root, ".git/hooks/pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
				t.Fatal(err)
			}
			before := git("rev-parse", "HEAD")
			if _, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow); err == nil {
				t.Fatal("commit failure reported as success")
			}
			got, err := ds.Get(ctx, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.AppliedClaimID != "" || git("rev-parse", "HEAD") != before {
				t.Fatal("uncommitted effect marked applied")
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			id, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			committed := git("rev-parse", "HEAD")
			if committed == before || git("status", "--porcelain") != "" {
				t.Fatal("retry did not commit canonical output")
			}
			if again, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow.Add(2*time.Hour)); err != nil || again != id || git("rev-parse", "HEAD") != committed {
				t.Fatalf("retry changed publication: %s %v", again, err)
			}
			got, err = ds.Get(ctx, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			history, err := ds.HistoryFor(ctx, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.AppliedClaimID != id || string(got.Payload) != original || len(history) != 1 {
				t.Fatalf("evidence or decision changed: %+v", got)
			}
			if family == "has_balance" {
				rows, err := store.NewShardStore(w.Fence.Root).Lines("demo-person", family)
				if err != nil || len(rows) != 1 {
					t.Fatalf("duplicate shard output: %v %+v", err, rows)
				}
			}
		})
	}
}

func TestDistillPublicationPreservesInterveningHumanEdit(t *testing.T) {
	w, ds, item, _ := distillPublicationFixture(t, "works_at")
	item = confirmDistill(t, w, ds, item)
	ctx := context.Background()
	hook := filepath.Join(w.Fence.Root, ".git/hooks/pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow); err == nil {
		t.Fatal("expected failed commit")
	}
	path := w.Fence.PathFor("topic", "demo-person")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	human := append(raw, []byte("\nIntervening human prose.\n")...)
	if err := os.WriteFile(path, human, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow); err == nil {
		t.Fatal("overwrote intervening edit")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(human) {
		t.Fatalf("human bytes lost: %v", err)
	}
	got, err := ds.Get(ctx, item.ID)
	if err != nil || got.AppliedClaimID != "" {
		t.Fatalf("failed effect marked applied: %v %+v", err, got)
	}
}

func TestDistillPreviewRejectsDirtyOrChangedConflict(t *testing.T) {
	w, ds, item, git := distillPublicationFixture(t, "works_at")
	ctx := context.Background()
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
	page.Claims = []domain.Claim{claimFixture("prior", "demo-person", "works_at", "Prior value")}
	path, err := w.Fence.WriteEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.PreviewDistillAssertion(ctx, item, "Human assertion", "human:reviewer", fixedNow); err == nil {
		t.Fatal("preview accepted an uncommitted prior")
	}
	git("add", "brain")
	git("commit", "--quiet", "-m", "prior")
	item = confirmDistill(t, w, ds, item)
	page.Claims[0].Object = "Changed prior"
	if _, err := w.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	git("add", "brain")
	git("commit", "--quiet", "-m", "intervening prior change")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitDistill(ctx, ds, item, fixedNow); err == nil {
		t.Fatal("published against a different prior than confirmed")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("changed prior lost: %v", err)
	}
}
