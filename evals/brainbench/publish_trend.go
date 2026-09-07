//go:build ignore

// Command publish_trend is plan T5.10's nightly publisher: it runs the
// same real scoring gen_trend.go does (every vendored BrainBench fixture's
// should_retrieve:true gold turns through Serenity's real hybrid search,
// T1.21) and appends the resulting Row to a durable trend file (see
// internal/eval/brainbench/trend.go's AppendRow) instead of gen_trend.go's
// own single-row, gitignored, per-run artifact.
//
// This command is deliberately never invoked against a file inside this
// working tree's own evals/brainbench-trend.json path in CI: the nightly
// workflow (.github/workflows/brainbench-trend-nightly.yml) points
// -trend-path at a checkout of a separate results branch
// (results/brainbench-trend), commits the update there, and pushes --
// never to main. This file only computes and appends; it has no git
// knowledge of its own.
//
// Two guards run before anything is written, matching plan T5.10's own
// acceptance bar ("the workflow runs on cached outputs with a hard USD
// cap, alerts on cache miss instead of re-billing"):
//
//   - CheckCacheHit refuses to publish a row that measured nothing real
//     (the vendored corpus produced zero scoreable fixtures -- an
//     "alert instead of re-billing" moment, since this adapter has no
//     live/paid fallback path to re-bill against in the first place --
//     see trend.go's own doc comment).
//   - CheckBudget refuses to publish a row whose recorded spend exceeds
//     -budget-usd. Spend is always $0 today (Evaluate's search call
//     always passes a nil embedder -- zero live model calls,
//     structurally, not by configuration), so this never trips in
//     practice; it still runs for real every publish so a future change
//     that wires a live embedder into this adapter hits a hard failure
//     instead of a silent overspend.
//
// Either guard failing prints a GitHub Actions `::error::` annotation and
// exits non-zero -- the nightly job goes red and nothing is committed,
// rather than silently publishing a bad point.
//
// Run from the repo root:
//
//	go run evals/brainbench/publish_trend.go -trend-path /path/to/results-branch/evals/brainbench-trend.json
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/eval/brainbench"
)

func main() {
	fixturesDir := flag.String("fixtures-dir", "evals/brainbench/fixtures", "vendored BrainBench fixtures directory")
	goldDir := flag.String("gold-dir", "evals/brainbench/gold", "vendored BrainBench gold directory")
	pinPath := flag.String("pin-path", "evals/brainbench/PIN", "vendored gbrain commit pin file")
	trendPath := flag.String("trend-path", "evals/brainbench-trend.json", "durable trend file to append this run's row to (a results-branch checkout in CI, never main)")
	limit := flag.Int("limit", 10, "search result limit per query (matches internal/cli/search.go's own default)")
	budgetUSD := flag.Float64("budget-usd", defaultBudgetUSD(), "hard USD cap for this run; <= 0 means unlimited (default: $SERENITY_BRAINBENCH_BUDGET_USD, or 0)")
	flag.Parse()

	fixtures, err := brainbench.LoadFixtures(*fixturesDir)
	if err != nil {
		fail("publish_trend: %v", err)
	}
	gold, err := brainbench.LoadGold(*goldDir)
	if err != nil {
		fail("publish_trend: %v", err)
	}

	report, err := brainbench.Evaluate(context.Background(), fixtures, gold, *limit)
	if err != nil {
		fail("publish_trend: evaluate: %v", err)
	}

	if err := brainbench.CheckCacheHit(report); err != nil {
		alertf("brainbench cache miss", "%v", err)
		os.Exit(1)
	}

	const spentUSD = 0.0 // see the package doc: this adapter makes zero live model calls, structurally
	if err := brainbench.CheckBudget(*budgetUSD, spentUSD); err != nil {
		alertf("brainbench budget exceeded", "%v", err)
		os.Exit(1)
	}

	pinBytes, err := os.ReadFile(*pinPath)
	if err != nil {
		fail("publish_trend: read %s: %v", *pinPath, err)
	}
	pin := strings.TrimSpace(string(pinBytes))

	row := brainbench.NewRow(report, resolveCommit(), pin)
	row.BudgetUSD = *budgetUSD
	row.SpentUSD = spentUSD

	rows, err := brainbench.AppendRow(*trendPath, row)
	if err != nil {
		fail("publish_trend: %v", err)
	}

	log.Printf("publish_trend: appended row %d/%d to %s -- %d/%d fixtures scored, %d queries, precision=%.4f recall=%.4f f1=%.4f, budget_usd=%.2f spent_usd=%.2f",
		len(rows), len(rows), *trendPath, report.FixturesScored, report.FixturesTotal, report.Overall.Queries,
		report.Overall.Precision, report.Overall.Recall, report.Overall.F1, *budgetUSD, spentUSD)
}

// defaultBudgetUSD mirrors nightly-eval.yml's SERENITY_EVAL_BUDGET_USD
// convention with its own env var, so the hard cap can be set once as a
// repository variable rather than edited into the workflow file. "" or an
// unparseable value falls back to 0 (unlimited-only-if-explicitly-set,
// never accidentally-unlimited from a typo).
func defaultBudgetUSD() float64 {
	v := strings.TrimSpace(os.Getenv("SERENITY_BRAINBENCH_BUDGET_USD"))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// resolveCommit mirrors gen_trend.go's own helper: prefer $GITHUB_SHA so
// this never shells out in CI, fall back to `git rev-parse HEAD` locally,
// and never fail the whole run over a missing commit id.
func resolveCommit() string {
	if sha := os.Getenv("GITHUB_SHA"); sha != "" {
		return sha
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// alertf prints a GitHub Actions error annotation (recognized by the
// Actions log UI and surfaced as a run annotation/notification on a
// scheduled workflow) -- the concrete "alert" this task's acceptance bar
// asks for, distinct from fail's plain log line.
func alertf(title, format string, args ...any) {
	fmt.Printf("::error title=%s::%s\n", title, fmt.Sprintf(format, args...))
}

func fail(format string, args ...any) {
	log.Fatalf(format, args...)
}
