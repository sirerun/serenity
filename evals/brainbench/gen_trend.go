//go:build ignore

// Command gen_trend runs every vendored BrainBench fixture's retrieval-
// scoreable gold turns through Serenity's real hybrid search
// (internal/eval/brainbench, T1.21) and writes evals/brainbench-trend.json
// -- this run's score row (see internal/eval/brainbench/report.go's Row
// doc comment; T5.10 is what appends rows like this one to a persistent
// trend file on a results branch and renders the chart).
//
// Run from the repo root:
//
//	go run evals/brainbench/gen_trend.go
//
// Reads the pin from evals/brainbench/PIN and the commit from $GITHUB_SHA
// (falls back to `git rev-parse HEAD`, then "unknown" if neither works --
// a missing commit id should never fail the whole run).
package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/sirerun/serenity/internal/eval/brainbench"
)

const (
	fixturesDir = "evals/brainbench/fixtures"
	goldDir     = "evals/brainbench/gold"
	pinPath     = "evals/brainbench/PIN"
	outPath     = "evals/brainbench-trend.json"
	searchLimit = 10 // matches internal/cli/search.go's own default --limit
)

func main() {
	fixtures, err := brainbench.LoadFixtures(fixturesDir)
	if err != nil {
		log.Fatalf("gen_trend: %v", err)
	}
	gold, err := brainbench.LoadGold(goldDir)
	if err != nil {
		log.Fatalf("gen_trend: %v", err)
	}

	report, err := brainbench.Evaluate(context.Background(), fixtures, gold, searchLimit)
	if err != nil {
		log.Fatalf("gen_trend: evaluate: %v", err)
	}

	pinBytes, err := os.ReadFile(pinPath)
	if err != nil {
		log.Fatalf("gen_trend: read %s: %v", pinPath, err)
	}
	pin := strings.TrimSpace(string(pinBytes))

	row := brainbench.NewRow(report, resolveCommit(), pin)
	if err := brainbench.WriteRow(outPath, row); err != nil {
		log.Fatalf("gen_trend: %v", err)
	}

	log.Printf("gen_trend: wrote %s -- %d/%d fixtures scored, %d queries, precision=%.4f recall=%.4f f1=%.4f",
		outPath, report.FixturesScored, report.FixturesTotal, report.Overall.Queries,
		report.Overall.Precision, report.Overall.Recall, report.Overall.F1)
}

// resolveCommit prefers $GITHUB_SHA (set by every GitHub Actions job) so
// this never shells out in CI, and falls back to `git rev-parse HEAD` for
// a local run; "unknown" rather than failing the whole artifact if
// neither is available.
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
