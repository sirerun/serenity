//go:build ignore

// Command gen_calibration runs the T2.11 earned-automation ladder
// calibration sweep (internal/ladder's Sweep/Calibrate, RFC 0001 §10.3)
// over DefaultGrid() x DefaultScenarios() and writes
// evals/calibration/report.json -- the committed record of the grid
// searched, every candidate's per-scenario correctness, the chosen
// defaults, and the rationale for that choice.
//
// Run from the repo root:
//
//	go run evals/calibration/gen_calibration.go
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/ladder"
)

const outPath = "evals/calibration/report.json"

func main() {
	prior := ladder.GridPoint{
		MinDispositions: ladder.DefaultConfig().Default.MinDispositions,
		MinAccept:       ladder.DefaultConfig().Default.MinAccept,
		SampleRate:      ladder.DefaultConfig().Default.SampleRate,
	}

	report, err := ladder.Calibrate(
		ladder.DefaultGrid(),
		ladder.DefaultConfig().CorrelationGuards,
		ladder.DefaultScenarios(),
		prior,
		time.Now(),
	)
	if err != nil {
		log.Fatalf("gen_calibration: %v", err)
	}
	report.GeneratedAt = report.GeneratedAt.Truncate(time.Second)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("gen_calibration: marshal report: %v", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		log.Fatalf("gen_calibration: write %s: %v", outPath, err)
	}

	commit := commitSHA()
	log.Printf("gen_calibration: wrote %s (commit %s, chosen min_dispositions=%d min_accept=%.2f sample_rate=%.2f)",
		outPath, commit, report.Chosen.MinDispositions, report.Chosen.MinAccept, report.Chosen.SampleRate)
}

// commitSHA is informational only (log line, not written into the
// report -- Report.GeneratedAt plus the file's own git history already
// pin provenance); a missing commit id should never fail the run.
func commitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
