package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	budget "github.com/sirerun/serenity/internal/eval/importbudget"
)

func fixture() budget.Report {
	r := budget.Report{Version: budget.Version, Revision: "abc", Environment: "fixture", CorpusSHA256: strings.Repeat("a", 64), CacheVersion: "fixture-v1", Messages: 10000, Claims: 10000, Vectors: 10050, CacheHits: 10000, TotalSeconds: 100}
	for _, name := range budget.StageNames {
		n := 10000
		if name == "embed" {
			n = r.Vectors
		}
		r.Stages = append(r.Stages, budget.Stage{Name: name, Items: n, Seconds: 1})
	}
	return r
}
func write(t *testing.T, path string, r budget.Report) {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
func TestRegressionEmitsTimingArtifactBeforeFailing(t *testing.T) {
	dir := t.TempDir()
	cur, prev, table := filepath.Join(dir, "current.json"), filepath.Join(dir, "previous.json"), filepath.Join(dir, "timing.md")
	r := fixture()
	write(t, prev, r)
	r.TotalSeconds = 120
	write(t, cur, r)
	if err := run([]string{"-current", cur, "-previous", prev, "-markdown", table}, io.Discard); err == nil || !strings.Contains(err.Error(), "20%") {
		t.Fatalf("expected regression: %v", err)
	}
	raw, err := os.ReadFile(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "120.000") || !strings.Contains(string(raw), "| embed |") {
		t.Fatal("missing timing evidence")
	}
}
func TestCheckBootstrapAndInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	r := fixture()
	write(t, path, r)
	if err := run([]string{"-current", path}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-current", path, "-previous", filepath.Join(dir, "missing")}, io.Discard); err == nil {
		t.Fatal("silently bootstrapped unreadable baseline")
	}
	r.Messages = 25
	r.Claims = 25
	r.CacheHits = 25
	write(t, path, r)
	if err := run([]string{"-current", path}, io.Discard); err == nil {
		t.Fatal("accepted smoke run as full benchmark")
	}
	write(t, path, fixture())
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(" {}"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readReport(path); err == nil {
		t.Fatal("accepted trailing data")
	}
}
