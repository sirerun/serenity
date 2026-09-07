package brainbench

import (
	"os"
	"strings"
	"testing"
)

// TestNightlyTrendWorkflow is the CI-side companion to plan T5.10, the
// same pattern internal/gate/release_gate_test.go's
// TestAdversarialReleaseWorkflow established for release.yml: greps the
// workflow file for the exact pieces its acceptance bar depends on, so a
// future edit that silently drops the schedule, the hard-cap flag, or the
// results-branch target fails a test before it can ship.
func TestNightlyTrendWorkflow(t *testing.T) {
	b, err := os.ReadFile("../../../.github/workflows/brainbench-trend-nightly.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(b)
	for _, want := range []string{
		"cron:",
		"publish_trend.go",
		"-budget-usd",
		"SERENITY_BRAINBENCH_BUDGET_USD",
		"results/brainbench-trend",
		"contents: write",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("brainbench-trend-nightly.yml is missing %q", want)
		}
	}
}
