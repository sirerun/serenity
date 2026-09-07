package brainbench

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTrendMissingFileIsEmptyNotError(t *testing.T) {
	rows, err := LoadTrend(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadTrend on a missing file: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want 0 rows, got %d", len(rows))
	}
}

func TestLoadTrendRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trend.json")
	writeFile(t, path, "{not valid json")
	if _, err := LoadTrend(path); err == nil {
		t.Fatal("want an error for malformed trend JSON, got nil")
	}
}

// TestAppendRowGrowsADurableHistory exercises the real production path
// end to end: two real Evaluate() calls against the same real, small
// corpus (twoFixtureCorpus, evaluate_test.go) produce two rows, appended
// to the same file -- this is exactly what two nights of the real
// nightly job do to the durable results-branch file, proving the
// mechanism this task's acceptance bar depends on ("the docs site shows
// the trend with >= 2 points") actually produces >= 2 real points from
// real scoring, not two copies of a hand-built fixture Row.
func TestAppendRowGrowsADurableHistory(t *testing.T) {
	fixtures, gold := twoFixtureCorpus()
	report, err := Evaluate(context.Background(), fixtures, gold, 10)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if err := CheckCacheHit(report); err != nil {
		t.Fatalf("real corpus must not be a cache miss: %v", err)
	}

	path := filepath.Join(t.TempDir(), "trend.json")

	row1 := NewRow(report, "commit-1", "pin-1")
	rows, err := AppendRow(path, row1)
	if err != nil {
		t.Fatalf("AppendRow #1: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("after first append: want 1 row, got %d", len(rows))
	}

	row2 := NewRow(report, "commit-2", "pin-1")
	rows, err = AppendRow(path, row2)
	if err != nil {
		t.Fatalf("AppendRow #2: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("after second append: want 2 rows (>= 2 points), got %d", len(rows))
	}
	if rows[0].Commit != "commit-1" || rows[1].Commit != "commit-2" {
		t.Fatalf("append order not preserved: got commits %q, %q", rows[0].Commit, rows[1].Commit)
	}

	onDisk, err := LoadTrend(path)
	if err != nil {
		t.Fatalf("LoadTrend after two appends: %v", err)
	}
	if len(onDisk) != 2 {
		t.Fatalf("trend file on disk: want 2 rows, got %d", len(onDisk))
	}
}

func TestCheckCacheHitRejectsZeroScoredFixtures(t *testing.T) {
	err := CheckCacheHit(Report{FixturesTotal: 5, FixturesScored: 0})
	if err == nil {
		t.Fatal("want an error when FixturesScored == 0, got nil")
	}
	if !strings.Contains(err.Error(), "cache miss") {
		t.Fatalf("error should name the condition as a cache miss, got: %v", err)
	}
}

func TestCheckCacheHitAcceptsRealScoring(t *testing.T) {
	if err := CheckCacheHit(Report{FixturesTotal: 5, FixturesScored: 3}); err != nil {
		t.Fatalf("FixturesScored > 0 must not be treated as a cache miss: %v", err)
	}
}

func TestCheckBudgetUnlimitedWhenCapIsNonPositive(t *testing.T) {
	for _, capUSD := range []float64{0, -1} {
		if err := CheckBudget(capUSD, 1_000_000); err != nil {
			t.Fatalf("cap %.2f must mean unlimited, got error: %v", capUSD, err)
		}
	}
}

func TestCheckBudgetPassesWithinCap(t *testing.T) {
	if err := CheckBudget(1.00, 0); err != nil {
		t.Fatalf("zero spend must never exceed any positive cap: %v", err)
	}
	if err := CheckBudget(1.00, 1.00); err != nil {
		t.Fatalf("spend exactly at the cap must pass: %v", err)
	}
}

func TestCheckBudgetTripsOverCap(t *testing.T) {
	err := CheckBudget(1.00, 1.01)
	if err == nil {
		t.Fatal("want an error when spend exceeds a positive hard cap, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds hard cap") {
		t.Fatalf("error should name the hard-cap condition, got: %v", err)
	}
}
