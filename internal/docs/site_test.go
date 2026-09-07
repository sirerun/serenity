package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsSiteSources(t *testing.T) {
	want := []string{
		"mkdocs.yml",
		"docs/operator/server.md",
		"docs/operator/conformance.md",
		"docs/protocol/MEMORY_VERBS_v1.md",
		"docs/protocol/DISPOSITION_v1.md",
		"docs/protocol/DIRECTION_v1.md",
		"docs/operator/threat-model.md",
	}
	for _, path := range want {
		if _, err := os.Stat(filepath.Join("../..", path)); err != nil {
			t.Errorf("required docs source %s: %v", path, err)
		}
	}
	b, err := os.ReadFile("../../docs/operator/threat-model.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `--8<-- "threat-model.md"`) {
		t.Fatal("threat model page must include docs/threat-model.md at build time")
	}
}

// TestBrainBenchTrendPageWired is plan T5.10's own acceptance check:
// the trend page exists, mkdocs.yml's nav actually links to it (a page
// that exists but isn't in nav is unreachable from the docs site), and
// the page fetches the real results-branch trend file rather than some
// placeholder or a copy -- a future edit that drops any of the three
// silently breaks "the docs site shows the trend" without this failing.
func TestBrainBenchTrendPageWired(t *testing.T) {
	pagePath := filepath.Join("../..", "docs/evals/brainbench-trend.md")
	page, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("required docs source docs/evals/brainbench-trend.md: %v", err)
	}

	nav, err := os.ReadFile(filepath.Join("../..", "mkdocs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nav), "evals/brainbench-trend.md") {
		t.Fatal("mkdocs.yml nav must reference docs/evals/brainbench-trend.md, or the page is unreachable from the docs site")
	}

	const rawTrendURL = "https://raw.githubusercontent.com/sirerun/serenity/results/brainbench-trend/evals/brainbench-trend.json"
	if !strings.Contains(string(page), rawTrendURL) {
		t.Fatalf("brainbench-trend.md must fetch the real results-branch trend file (%s)", rawTrendURL)
	}
}
