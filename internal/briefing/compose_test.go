package briefing

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
)

// requireGit and runInit are internal/cli's own test/production helpers;
// this package has no CLI dependency, so the fixture below drives the
// same `git init` + config.Default().Save() sequence runInit performs,
// directly, the same way internal/consolidate's own tests do (rather
// than importing internal/cli, which would create an import cycle:
// internal/cli already imports internal/briefing once T2.17's own CLI
// wiring lands, see the disclosed-scope note on Compose).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

func initBrainRepo(t *testing.T, root string) {
	t.Helper()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_CONFIG_COUNT=0")
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, b)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "Test")
	run("config", "core.hooksPath", "/dev/null")

	dbDir := filepath.Join(root, ".serenity")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// providers.OpenIndex only needs .serenity/index.db to exist as a
	// path it can open/create -- index.Open creates the schema itself.
	eng, err := index.Open(filepath.Join(dbDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "fixture", "--allow-empty")
}

// TestComposeGoldenRenderOnFixtureBrain is plan T2.17's own acc-line
// clause: "golden render on the fixture brain." Seeds one pending item
// (drives Needs you), one disposed item inside the Moved-forward
// lookback (drives Moved forward), enough spend rows to produce a
// nonzero Watched projection, and leaves the queue under both SLO
// thresholds (Blocked renders empty) and Drift empty (T3.9 unshipped,
// disclosed on Compose) -- then asserts Render's exact byte-for-byte
// text output, the same golden-string discipline
// internal/cli.TestStatusGoldenOutput already established for `serenity
// status`.
func TestComposeGoldenRenderOnFixtureBrain(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initBrainRepo(t, root)

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)

	// Needs you: one pending item, 90 minutes old.
	pending, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-90*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Moved forward: one item created 3h ago, disposed 20 minutes ago
	// (well inside the default 24h lookback).
	resolved, err := ds.Create(ctx, disposition.KindDistill, nil, "", now.Add(-3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(ctx, resolved.ID, disposition.VerdictAccept, nil, "", "human:t", "", now.Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// An item disposed 2 days ago must NOT appear in Moved forward --
	// outside the default 24h lookback.
	stale, err := ds.Create(ctx, disposition.KindDistill, nil, "", now.Add(-72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(ctx, stale.ID, disposition.VerdictReject, nil, "not relevant", "human:t", "", now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Watched: one spend row this month.
	if err := eng.RecordSpend(ctx, index.SpendRow{
		ID: "s1", TaskClass: "judgment", Tier: "judgment",
		Provider: "anthropic", ModelVersion: "claude-x@v1",
		InputTokens: 500, OutputTokens: 200, CostUSD: 1.5,
		OccurredAt: now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	eng, err = providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()

	got, err := Compose(ctx, eng, DefaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}

	want := "" +
		"## Blocked\n(nothing)\n\n" +
		"## Needs you\n- reconcile " + pending.ID + " (1h30m0s old)\n\n" +
		"## Moved forward\n- distill " + resolved.ID + " -> accept (20m0s ago)\n\n" +
		"## Watched\n- spend $1.50 month-to-date, projected $6.43 of $50.00 ceiling\n\n" +
		"## Drift\n(nothing)\n"

	if renderedGot := Render(got); renderedGot != want {
		t.Fatalf("Compose golden render mismatch:\ngot:\n%s\nwant:\n%s", renderedGot, want)
	}
}

// TestComposeBlockedSectionReflectsQueueAlerts proves Blocked actually
// wires to queue.Compute's own Snapshot (T2.15) rather than always
// rendering empty: 60 pending items trips the depth alert (RFC 0001 §7:
// "depth > 50").
func TestComposeBlockedSectionReflectsQueueAlerts(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initBrainRepo(t, root)

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)
	for i := 0; i < 60; i++ {
		if _, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Compose(ctx, eng, DefaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	var blocked PackedSection
	for _, sec := range got.Sections {
		if sec.Name == SectionBlocked {
			blocked = sec
		}
	}
	if len(blocked.Items) != 1 {
		t.Fatalf("Blocked = %+v, want exactly one depth-alert line", blocked)
	}
	if want := "queue depth 60 exceeds the 50-item threshold"; blocked.Items[0].Text != want {
		t.Errorf("Blocked item = %q, want %q", blocked.Items[0].Text, want)
	}
}

// TestComposeFreshBrainRendersEveryFixedSectionNotEmptyOutput proves a
// never-touched brain still renders all five section headings (RFC 0001
// §7's "five fixed sections" is unconditional -- a quiet brain must not
// omit a heading, only its content), each showing "(nothing)".
func TestComposeFreshBrainRendersEveryFixedSectionNotEmptyOutput(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initBrainRepo(t, root)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()

	got, err := Compose(ctx, eng, DefaultConfig(), time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sections) != len(Sections) {
		t.Fatalf("got %d sections, want %d (the five fixed sections, always)", len(got.Sections), len(Sections))
	}
	for i, want := range Sections {
		if got.Sections[i].Name != want {
			t.Errorf("section %d = %s, want %s (RFC 0001 §7's fixed order)", i, got.Sections[i].Name, want)
		}
		if got.Sections[i].Omitted != 0 {
			t.Errorf("section %s omitted=%d on a fresh brain, want 0 (nothing to omit, not dropped)", got.Sections[i].Name, got.Sections[i].Omitted)
		}
	}
	rendered := Render(got)
	for _, want := range []string{"## Blocked\n", "## Needs you\n", "## Moved forward\n", "## Watched\n", "## Drift\n"} {
		if !bytes.Contains([]byte(rendered), []byte(want)) {
			t.Errorf("fresh-brain render missing heading %q:\n%s", want, rendered)
		}
	}
}

// TestComposeDropsSectionOverWordBudgetWhole is plan T2.17's own
// acc-line clause -- "a section over budget is dropped whole with
// omitted: N" -- exercised through Compose's real section-building path
// (not just Pack in isolation, see TestPackDropsSectionOverBudgetWhole
// in briefing_test.go): enough pending items that Needs you alone
// exceeds a deliberately tiny word budget.
func TestComposeDropsSectionOverWordBudgetWhole(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	initBrainRepo(t, root)

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)
	for i := 0; i < 5; i++ {
		if _, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DefaultConfig()
	cfg.WordBudget = 3 // each Needs-you line alone is well over 3 words.
	got, err := Compose(ctx, eng, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	var needsYou PackedSection
	for _, sec := range got.Sections {
		if sec.Name == SectionNeedsYou {
			needsYou = sec
		}
	}
	if needsYou.Items != nil {
		t.Fatalf("Needs you kept items under a 3-word budget: %+v", needsYou.Items)
	}
	if needsYou.Omitted != 5 {
		t.Fatalf("Needs you omitted=%d, want 5 (all pending items, whole-section drop)", needsYou.Omitted)
	}
}
