package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func TestLoadLabels(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "span-001.yaml", `
span: "alice works at acme corp since june 2025"
expected:
  predicate: works_at
  object: acme-corp
  valid_from: "2025-06"
labeler: model-a
adjudicated: false
`)
	writeFixture(t, dir, "span-002.yaml", `
span: "alice left acme corp in march 2026 to join beta llc"
expected:
  predicate: works_at
  object: beta-llc
  valid_from: "2026-03"
labeler: model-b
adjudicated: true
`)
	writeFixture(t, dir, "span-003.yaml", `
span: "bob prefers dark roast coffee"
expected:
  predicate: prefers
  object: dark-roast-coffee
labeler: human
adjudicated: false
`)

	labels, err := LoadLabels(dir)
	if err != nil {
		t.Fatalf("LoadLabels: %v", err)
	}
	want := []Label{
		{Span: "alice works at acme corp since june 2025", Expected: ExpectedFact{Predicate: "works_at", Object: "acme-corp", ValidFrom: "2025-06"}, Labeler: "model-a", Adjudicated: false},
		{Span: "alice left acme corp in march 2026 to join beta llc", Expected: ExpectedFact{Predicate: "works_at", Object: "beta-llc", ValidFrom: "2026-03"}, Labeler: "model-b", Adjudicated: true},
		{Span: "bob prefers dark roast coffee", Expected: ExpectedFact{Predicate: "prefers", Object: "dark-roast-coffee"}, Labeler: "human", Adjudicated: false},
	}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels, want %d: %+v", len(labels), len(want), labels)
	}
	for i, l := range labels {
		if l != want[i] {
			t.Errorf("label %d (filename-sorted) = %+v, want %+v", i, l, want[i])
		}
	}
}

func TestLoadLabelsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	labels, err := LoadLabels(dir)
	if err != nil {
		t.Fatalf("LoadLabels on empty dir: %v", err)
	}
	if labels == nil {
		t.Fatal("LoadLabels on an empty dir returned nil, want an empty non-nil slice")
	}
	if len(labels) != 0 {
		t.Fatalf("got %d labels, want 0", len(labels))
	}
}

func TestLoadLabelsMissingDir(t *testing.T) {
	labels, err := LoadLabels(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("LoadLabels on a missing dir: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("got %d labels, want 0", len(labels))
	}
}

// TestLoadLabelsUnknownFieldIgnored pins the documented choice: unrecognized
// YAML fields are silently ignored (yaml.v3's default decode behavior), so
// a corpus can carry forward-compatible metadata (labeling notes, reviewer
// ids, ...) without breaking an older harness version.
func TestLoadLabelsUnknownFieldIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "span-001.yaml", `
span: "carol has a deadline on the launch project"
expected:
  predicate: deadline_on
  object: "2026-09-01"
labeler: model-a
adjudicated: false
notes: "this field does not exist on Label and must be silently ignored"
`)
	labels, err := LoadLabels(dir)
	if err != nil {
		t.Fatalf("LoadLabels with an unknown field present: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	want := Label{Span: "carol has a deadline on the launch project", Expected: ExpectedFact{Predicate: "deadline_on", Object: "2026-09-01"}, Labeler: "model-a", Adjudicated: false}
	if labels[0] != want {
		t.Errorf("got %+v, want %+v", labels[0], want)
	}
}

// TestLoadLabelsIgnoresNonYAMLFiles confirms LoadLabels only reads *.yaml
// files from the directory (e.g. a manifest's checksums.yaml sibling file
// does not get parsed as a Label -- this test uses a non-matching extension
// as the simplest stand-in for "some other file lives in this directory").
func TestLoadLabelsIgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "span-001.yaml", `
span: "s1"
expected:
  predicate: works_at
  object: acme
labeler: model-a
adjudicated: false
`)
	writeFixture(t, dir, "README.md", "# not a label file\n")

	labels, err := LoadLabels(dir)
	if err != nil {
		t.Fatalf("LoadLabels: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1 (README.md must be ignored): %+v", len(labels), labels)
	}
}
