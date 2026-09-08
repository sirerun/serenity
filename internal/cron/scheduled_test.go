package cron

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
)

func scheduledFixture(t *testing.T) (string, time.Time) {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "Test"}, {"config", "core.hooksPath", "/dev/null"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	cfg := config.Default()
	cfg.Families["has_role"] = config.Family{Tier: domain.TierFence, HalfLifeDays: 30}
	if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	fw := store.NewFenceWriter(root)
	for _, slug := range []string{"ava", "ava-lee"} {
		p := store.NewEntityPage(domain.Entity{Slug: slug, Type: "person"})
		p.Claims = []domain.Claim{{ID: "role", SubjectSlug: slug, Family: "has_role", Predicate: "has_role", Object: "Engineer", Confidence: .9, State: domain.StateActive, Provenance: domain.Provenance{ObservedAt: at.Add(-90 * 24 * time.Hour)}}}
		if _, err := fw.WriteEntity(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "seed"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	return root, at
}

func TestDecayStagesCanonicalReviewWithoutChangingClaims(t *testing.T) {
	root, at := scheduledFixture(t)
	ctx := context.Background()
	before, err := os.ReadFile(filepath.Join(root, "brain/entities/person/ava.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "decay", root, fakeClock{at}); err != nil {
		t.Fatal(err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	ds := disposition.NewStore(eng)
	items, err := ds.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("scheduled decay staged %d items, want two distill and one alias", len(items))
	}
	var counts = map[disposition.Kind]int{}
	for _, it := range items {
		counts[it.Kind]++
		if _, err := ds.Dispose(ctx, it.ID, disposition.VerdictReject, nil, "retain judgment", "human:test", "", at); err != nil {
			t.Fatal(err)
		}
	}
	if counts[disposition.KindDistill] != 2 || counts[disposition.KindEntityMerge] != 1 {
		t.Fatal(counts)
	}
	if err := Run(ctx, "decay", root, fakeClock{at.Add(7 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	again, err := ds.List(ctx)
	if err != nil || len(again) != 3 {
		t.Fatalf("retry %d %v", len(again), err)
	}
	for _, it := range again {
		if it.Verdict != disposition.VerdictReject {
			t.Fatal("review reset")
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "brain/entities/person/ava.md"))
	if err != nil || string(after) != string(before) {
		t.Fatal("decay changed canonical confidence or state")
	}

	fw := store.NewFenceWriter(root)
	page, err := fw.ParseEntity(fw.PathFor("person", "ava"))
	if err != nil {
		t.Fatal(err)
	}
	page.Claims[0].Object = "Architect"
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "brain/entities/person/ava.md"}, {"commit", "-qm", "new evidence"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v %s", err, out)
		}
	}
	if err := Run(ctx, "decay", root, fakeClock{at.Add(8 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	changed, err := ds.List(ctx)
	if err != nil || len(changed) != 4 {
		t.Fatalf("changed evidence: %d %v", len(changed), err)
	}
	var pending int
	for _, it := range changed {
		if it.State == disposition.StatePending {
			pending++
			p, ok := disposition.DecayedClaim(it)
			if !ok || p.Claim.Object != "Architect" {
				t.Fatalf("wrong new review %+v", it)
			}
		}
	}
	if pending != 1 {
		t.Fatalf("new pending %d", pending)
	}
}

func TestSLOJobComputesRealQueue(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)
	for i := 0; i < 60; i++ {
		if _, err := ds.Create(ctx, disposition.KindDistill, json.RawMessage(`{"text":"review"}`), "", at.Add(-4*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, "slo", root, fakeClock{at}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(RecordPath(root, "slo"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Details struct {
			Depth      int
			DepthAlert bool
			AgeAlert   bool
		}
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.Details.Depth != 60 || !record.Details.DepthAlert || !record.Details.AgeAlert {
		t.Fatalf("scheduled metrics absent: %s", raw)
	}
}

func TestDecayRejectsDirtyOrMalformedEvidenceBeforeStaging(t *testing.T) {
	for _, mode := range []string{"dirty", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			root, at := scheduledFixture(t)
			path := filepath.Join(root, "brain/entities/person/ava-lee.md")
			if mode == "dirty" {
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.WriteString("human note\n")
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, []byte("not an entity"), 0600); err != nil {
					t.Fatal(err)
				}
				for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "malformed fixture"}} {
					if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
						t.Fatalf("git %v %s", err, out)
					}
				}
			}
			if err := Run(context.Background(), "decay", root, fakeClock{at}); err == nil {
				t.Fatal("invalid evidence reported success")
			}
			if _, err := ReadRecord(root, "decay"); !os.IsNotExist(err) {
				t.Fatalf("success record exists: %v", err)
			}
			eng, err := providers.OpenIndex(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = eng.Close() }()
			items, err := disposition.NewStore(eng).List(context.Background())
			if err != nil || len(items) != 0 {
				t.Fatalf("partial staging %d %v", len(items), err)
			}
		})
	}
}

func TestDecayReadsResolvedSegmentsAndExcludesInactiveClaims(t *testing.T) {
	root, at := scheduledFixture(t)
	fw := store.NewFenceWriter(root)
	p, err := fw.ParseEntity(fw.PathFor("person", "ava"))
	if err != nil {
		t.Fatal(err)
	}
	p.Claims[0].State = domain.StateRetracted
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}
	ss := store.NewShardStore(root)
	ss.RolloverBytes = 1
	old := domain.Claim{ID: "old", SubjectSlug: "ava", Family: "has_balance", Predicate: "has_balance", Object: "old balance", ObjectKey: "balance", Confidence: .1, State: domain.StateActive, Provenance: domain.Provenance{ObservedAt: at.Add(-90 * 24 * time.Hour)}}
	if err := ss.Append(old); err != nil {
		t.Fatal(err)
	}
	current := old
	current.ID = "current"
	current.Object = "current balance"
	current.Supersedes = old.ID
	if err := ss.Append(current); err != nil {
		t.Fatal(err)
	}
	archived := old
	archived.ID = "archived"
	raw, err := json.Marshal(archived)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "brain/claims/ava/has_balance.archive.jsonl"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "shard fixture"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v %s", err, out)
		}
	}
	if err := Run(context.Background(), "decay", root, fakeClock{at}); err != nil {
		t.Fatal(err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	items, err := disposition.NewStore(eng).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		p, ok := disposition.DecayedClaim(item)
		if !ok {
			continue
		}
		if p.Claim.SubjectSlug == "ava" {
			if p.Claim.ID != "current" {
				t.Fatalf("noncurrent claim staged: %+v", p.Claim)
			}
			found = true
			if p.Claim.Confidence != .1 {
				t.Fatal("stored confidence overwritten")
			}
		}
	}
	if !found {
		t.Fatal("resolved numbered segment omitted")
	}
}

func TestSLOFailureDoesNotAdvanceSuccess(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if err := Run(context.Background(), "slo", root, fakeClock{at}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, "slo", root, fakeClock{at.Add(time.Hour)}); err == nil {
		t.Fatal("canceled computation reported success")
	}
	record, err := ReadRecord(root, "slo")
	if err != nil || record.RunCount != 1 || !record.LastRun.Equal(at) {
		t.Fatalf("record %+v %v", record, err)
	}
}

func TestDecayCanceledEmptyBrainDoesNotRecordSuccess(t *testing.T) {
	root := t.TempDir()
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, "decay", root, fakeClock{time.Now()}); err == nil {
		t.Fatal("canceled empty scan reported success")
	}
	if _, err := ReadRecord(root, "decay"); !os.IsNotExist(err) {
		t.Fatalf("success record exists: %v", err)
	}
}
