package cron

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
)

// fakeClock is a fixed clock: every call returns the same instant. Real
// scheduled behavior (T2.6/T2.12/T2.14/T2.15, once they land) is asserted
// against this rather than time.Now() so tests never flake on wall-clock
// skew.
type fakeClock struct{ at time.Time }

func (f fakeClock) Now() time.Time { return f.at }

func TestNamesIsSortedAndComplete(t *testing.T) {
	want := []string{"consolidate", "decay", "slo", "sweep"}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestRunUnknownJobNamesEveryValidJob(t *testing.T) {
	err := Run(context.Background(), "bogus", t.TempDir(), fakeClock{at: time.Now()})
	if err == nil {
		t.Fatal("Run(\"bogus\", ...) = nil, want an error")
	}
	for _, name := range Names() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q does not name valid job %q", err, name)
		}
	}
}

// TestEachJobExitsCleanAndIsIdempotentWithFakeClock covers T2.19's own acc
// line directly: every registered job (not just sweep) runs against a
// fixture root with an injected clock, exits nil, and running it a second
// time is a genuine no-op — never an error, never a second unrelated
// side effect — with the run record reflecting exactly two runs at the
// clock's fixed instant.
func TestEachJobExitsCleanAndIsIdempotentWithFakeClock(t *testing.T) {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	clock := fakeClock{at: at}

	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()

			if err := Run(ctx, name, root, clock); err != nil {
				t.Fatalf("first run: %v", err)
			}
			rec, err := ReadRecord(root, name)
			if err != nil {
				t.Fatalf("read record after first run: %v", err)
			}
			if rec.Job != name || !rec.LastRun.Equal(at) || rec.RunCount != 1 {
				t.Fatalf("after first run: got %+v, want Job=%s LastRun=%s RunCount=1", rec, name, at)
			}

			if err := Run(ctx, name, root, clock); err != nil {
				t.Fatalf("second run: %v", err)
			}
			rec, err = ReadRecord(root, name)
			if err != nil {
				t.Fatalf("read record after second run: %v", err)
			}
			if rec.RunCount != 2 || !rec.LastRun.Equal(at) {
				t.Fatalf("after second run: got %+v, want RunCount=2 LastRun=%s", rec, at)
			}
		})
	}
}

// TestSweepJobAdvancesExpiredDispositionItem proves `serenity cron sweep`
// is real, not a placeholder (T2.6): a disposition item seeded directly
// against the same brain root's index, aged past the default 14-day
// threshold, actually transitions to deferred when the Sweep job runs
// through the CLI-facing entry point (Run), not just internal/disposition
// package's own tests.
func TestSweepJobAdvancesExpiredDispositionItem(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	store := disposition.NewStore(eng)
	item, err := store.Create(ctx, disposition.KindReconcile, nil, "", start)
	if err != nil {
		_ = eng.Close()
		t.Fatalf("Create: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	later := start.Add(15 * 24 * time.Hour)
	if err := Run(ctx, "sweep", root, fakeClock{at: later}); err != nil {
		t.Fatalf("Run(sweep): %v", err)
	}

	eng2, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex (reopen): %v", err)
	}
	defer func() { _ = eng2.Close() }()
	got, err := disposition.NewStore(eng2).Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != disposition.StateDeferred || got.DeferCount != 1 {
		t.Fatalf("after cron sweep: State=%q DeferCount=%d, want deferred/1", got.State, got.DeferCount)
	}
}

// TestConsolidateRealSweep proves `serenity cron consolidate` is real,
// not a placeholder (T2.14): a fence page seeded with a stale
// hand-written summary, plus a shard-tier claim with no fence head yet,
// actually get consolidated -- the summary regenerated and the shard
// head refreshed to match ResolveHeads -- when the Consolidate job runs
// through the CLI-facing entry point (Run), not just
// internal/consolidate's own tests. The write also lands as a real git
// commit (RFC §7.7): consolidate's canonical writes go through the same
// writer queue + Flush every other write path in this repo uses.
func TestConsolidateRealSweep(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	root := t.TempDir()

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "--quiet")
	run("config", "user.email", "cron-test@example.com")
	run("config", "user.name", "cron test")

	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)

	observed := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Summary = "stale hand-written summary"
	page.Claims = []domain.Claim{{
		ID: "claim-a", SubjectSlug: "alice-tan", Predicate: "works_at",
		Object: "acme", ObjectKey: store.NormalizeKey("acme"), Confidence: 0.9,
		State: domain.StateActive, SourceRef: "e1#1", Family: "works_at",
		Provenance: domain.Provenance{ObservedAt: observed, Actor: "machine"},
	}}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}

	shardClaim := domain.Claim{
		ID: "bal-1", SubjectSlug: "acme-corp", Predicate: "has_balance",
		Object: "$500", ObjectKey: store.NormalizeKey("$500"), Confidence: 0.9,
		State: domain.StateActive, SourceRef: "e1#1", Family: "has_balance",
		Provenance: domain.Provenance{ObservedAt: observed, Actor: "machine"},
	}
	if err := ss.Append(shardClaim); err != nil {
		t.Fatalf("seed shard: %v", err)
	}

	run("add", ".")
	run("commit", "--quiet", "-m", "seed")
	beforeHEAD := strings.TrimSpace(run("rev-parse", "HEAD"))

	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if err := Run(ctx, "consolidate", root, fakeClock{at: at}); err != nil {
		t.Fatalf("Run(consolidate): %v", err)
	}

	afterHEAD := strings.TrimSpace(run("rev-parse", "HEAD"))
	if afterHEAD == beforeHEAD {
		t.Fatal("consolidate did not create a new commit -- it must land its writes through the writer queue and Flush")
	}

	p, err := fw.ParseEntity(fw.PathFor("topic", "alice-tan"))
	if err != nil {
		t.Fatalf("ParseEntity(alice-tan): %v", err)
	}
	if p.Summary == "stale hand-written summary" {
		t.Fatal("summary fence was not regenerated -- consolidate must always overwrite the DERIVED summary")
	}

	acmeP, err := fw.ParseEntity(fw.PathFor("topic", "acme-corp"))
	if err != nil {
		t.Fatalf("ParseEntity(acme-corp): %v", err)
	}
	wantHeads, err := ss.ResolveHeads("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("ResolveHeads: %v", err)
	}
	if len(acmeP.Claims) != len(wantHeads) || len(acmeP.Claims) != 1 {
		t.Fatalf("acme-corp has %d claim row(s), want 1 (== len(ResolveHeads)): %+v", len(acmeP.Claims), acmeP.Claims)
	}
	if acmeP.Claims[0].ID != "bal-1" || acmeP.Claims[0].SourceRef != "shard" {
		t.Fatalf("acme-corp shard head row = %+v, want id=bal-1 src=shard", acmeP.Claims[0])
	}

	rec, err := ReadRecord(root, "consolidate")
	if err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}
	if rec.RunCount != 1 || !rec.LastRun.Equal(at) {
		t.Fatalf("record = %+v, want RunCount=1 LastRun=%s", rec, at)
	}
}

func TestReadRecordMissingIsNotExist(t *testing.T) {
	if _, err := ReadRecord(t.TempDir(), "sweep"); err == nil {
		t.Fatal("ReadRecord on a fresh root = nil error, want a not-exist error")
	}
}
