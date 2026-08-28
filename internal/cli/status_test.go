package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

// fakeStatusClock is a settable index.Clock: each seeded job's
// StartedAt/FinishedAt comes from whatever fakeStatusClock.t holds at the
// moment index.SQLite.StartJob/FinishJob calls Now(), so the fixture
// controls every timestamp exactly instead of racing a real clock. Same
// injectable-clock shape as internal/connector/file's fakeClock (T1.3)
// and internal/extract's ex.now hook.
type fakeStatusClock struct{ t time.Time }

func (c *fakeStatusClock) Now() time.Time { return c.t }

// TestStatusGoldenOutput is the golden test plan T1.17's acc line asks
// for: a fixture brain with known jobs/spend/rebuild-timing data (seeded
// via an injectable clock, RFC section 16 fields) renders byte-stable
// `serenity status` output.
func TestStatusGoldenOutput(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	clk := &fakeStatusClock{}

	dbDir := filepath.Join(root, ".serenity")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "index.db")
	seed, err := index.Open(dbPath, index.WithClock(clk))
	if err != nil {
		t.Fatal(err)
	}

	// connector "file": one successful run, 2h ago.
	clk.t = now.Add(-3 * time.Hour)
	fileJob, err := seed.StartJob(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-2 * time.Hour)
	if err := seed.FinishJob(ctx, fileJob, index.JobSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}

	// connector "imap:jane@example.com": succeeded 45m ago, then failed
	// 9m ago -- proves lag comes from the last SUCCEEDED run even though
	// the most recent run (and thus the reported health) is unhealthy.
	clk.t = now.Add(-90 * time.Minute)
	imapOK, err := seed.StartJob(ctx, "imap:jane@example.com")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-45 * time.Minute)
	if err := seed.FinishJob(ctx, imapOK, index.JobSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-10 * time.Minute)
	imapFail, err := seed.StartJob(ctx, "imap:jane@example.com")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-9 * time.Minute)
	if err := seed.FinishJob(ctx, imapFail, index.JobFailed, nil, errors.New("dial tcp: timeout")); err != nil {
		t.Fatal(err)
	}

	// connector "git-repo:demo": started, never finished (mid-poll, not
	// yet swept) -- never succeeded, lag reports "never".
	clk.t = now.Add(-5 * time.Minute)
	if _, err := seed.StartJob(ctx, "git-repo:demo"); err != nil {
		t.Fatal(err)
	}

	// Spend ledger: two calls with OccurredAt set explicitly (RecordSpend
	// persists whatever's given -- no clock involved).
	if err := seed.RecordSpend(ctx, index.SpendRow{
		ID: "s1", TaskClass: "extract_bulk", Tier: "local_cheap",
		Provider: "fake", ModelVersion: "fake-extractor@v1",
		InputTokens: 120, OutputTokens: 40, CostUSD: 0.0021,
		OccurredAt: now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := seed.RecordSpend(ctx, index.SpendRow{
		ID: "s2", TaskClass: "judgment", Tier: "judgment",
		Provider: "anthropic", ModelVersion: "claude-x@v1",
		InputTokens: 500, OutputTokens: 200, CostUSD: 0.1234,
		OccurredAt: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	// Rebuild timing: last rebuild took 250ms, finished 10m ago.
	if err := seed.RecordRebuildTiming(ctx, 250*time.Millisecond, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runStatus(ctx, root, &out, now); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"models    embedding=none@v0 extraction=none@v0\n" +
		"engine    sqlite\n" +
		"chunks     0\n" +
		"claims     0\n" +
		"entities   0\n" +
		"vectors    0\n" +
		"connector  file status=succeeded lag=2h0m0s\n" +
		"connector  git-repo:demo status=running lag=never\n" +
		"connector  imap:jane@example.com status=failed lag=45m0s\n" +
		"jobs       total=4 running=1 succeeded=2 failed=1 interrupted=0\n" +
		"spend      calls=2 cost_usd=$0.1255\n" +
		"rebuild    last=10m0s ago took=250ms\n"

	if out.String() != want {
		t.Fatalf("status output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

// TestStatusFreshBrainNoJobsNoRebuild covers the empty-fixture edge the
// golden test doesn't: a brain that has only run `serenity init` --
// never synced, never ingested, never spent a dollar. Every T1.17 field
// must render an explicit "nothing yet" state, never crash or fall
// silent.
func TestStatusFreshBrainNoJobsNoRebuild(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runStatus(ctx, root, &out, time.Now()); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"connector  none configured (no jobs recorded yet)\n",
		"jobs       total=0 running=0 succeeded=0 failed=0 interrupted=0\n",
		"spend      calls=0 cost_usd=$0.0000\n",
		"rebuild    never (run `serenity sync`)\n",
	} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("expected fresh-brain status to contain %q, got:\n%s", want, out.String())
		}
	}
}
