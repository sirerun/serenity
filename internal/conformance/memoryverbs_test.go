package conformance

import (
	"path/filepath"
	"runtime"
	"testing"
)

// memoryVerbsFixture locates testdata/conformance/memory_verbs/cases.json
// relative to this test file's own path, the same technique
// fixtures_test.go's own repoRoot helper uses.
func memoryVerbsFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "conformance", "memory_verbs", "cases.json")
}

func TestLoadMemoryVerbCasesLoadsAllPinnedCases(t *testing.T) {
	cases, err := LoadMemoryVerbCases(memoryVerbsFixture(t))
	if err != nil {
		t.Fatalf("LoadMemoryVerbCases: %v", err)
	}
	if len(cases) != 17 {
		t.Fatalf("got %d cases, want 17 (the pinned gbrain@d35c9c9e441e set)", len(cases))
	}
	for i, c := range cases {
		if c.Name == "" || c.Verb == "" {
			t.Fatalf("case %d missing name/verb: %+v", i, c)
		}
	}
}

func TestInterpolateParamsResolvesMarkerAndSavedID(t *testing.T) {
	params := map[string]any{
		"fact":   "conformance {{marker}} fact",
		"entity": "people/conformance-{{marker}}",
		"parent": "{{id:fact1}}",
	}
	saved := map[string]string{"fact1": "abc123"}
	resolved, err := InterpolateParams(params, "my-marker", saved)
	if err != nil {
		t.Fatalf("InterpolateParams: %v", err)
	}
	if resolved["fact"] != "conformance my-marker fact" {
		t.Fatalf("fact = %v, want marker substituted", resolved["fact"])
	}
	if resolved["entity"] != "people/conformance-my-marker" {
		t.Fatalf("entity = %v", resolved["entity"])
	}
	if resolved["parent"] != "abc123" {
		t.Fatalf("parent = %v, want abc123", resolved["parent"])
	}
}

func TestInterpolateParamsRejectsUnresolvedPlaceholder(t *testing.T) {
	params := map[string]any{"parent": "{{id:never-saved}}"}
	if _, err := InterpolateParams(params, "marker", map[string]string{}); err == nil {
		t.Fatal("expected an error for an unresolved {{id:...}} placeholder")
	}
}

func TestLookupPathWalksObjectsAndArrays(t *testing.T) {
	value := map[string]any{
		"facts": []any{
			map[string]any{"fact_id": "f1"},
		},
	}
	got, ok := LookupPath(value, "facts.0.fact_id")
	if !ok || got != "f1" {
		t.Fatalf("LookupPath = (%v, %v), want (f1, true)", got, ok)
	}
	if _, ok := LookupPath(value, "facts.5.fact_id"); ok {
		t.Fatal("expected out-of-range index to report not found")
	}
	if _, ok := LookupPath(value, "missing"); ok {
		t.Fatal("expected missing key to report not found")
	}
}

func TestAssertExpectationEquals(t *testing.T) {
	body := map[string]any{"status": "inserted"}
	if err := AssertExpectation(body, map[string]any{"path": "status", "equals": "inserted"}); err != nil {
		t.Fatalf("expected equals match, got %v", err)
	}
	if err := AssertExpectation(body, map[string]any{"path": "status", "equals": "duplicate"}); err == nil {
		t.Fatal("expected equals mismatch to fail")
	}
}

func TestAssertExpectationOneOf(t *testing.T) {
	body := map[string]any{"status": "duplicate"}
	expect := map[string]any{"path": "status", "oneOf": []any{"inserted", "duplicate", "superseded"}}
	if err := AssertExpectation(body, expect); err != nil {
		t.Fatalf("expected oneOf match, got %v", err)
	}
	expect["oneOf"] = []any{"inserted"}
	if err := AssertExpectation(body, expect); err == nil {
		t.Fatal("expected oneOf mismatch to fail")
	}
}

func TestAssertExpectationTypeAndBounds(t *testing.T) {
	body := map[string]any{"id": "abc", "total": float64(3)}
	if err := AssertExpectation(body, map[string]any{"path": "id", "type": "string"}); err != nil {
		t.Fatalf("type string: %v", err)
	}
	if err := AssertExpectation(body, map[string]any{"path": "total", "type": "string"}); err == nil {
		t.Fatal("expected type mismatch to fail")
	}
	if err := AssertExpectation(body, map[string]any{"path": "total", "gte": float64(1)}); err != nil {
		t.Fatalf("gte: %v", err)
	}
	if err := AssertExpectation(body, map[string]any{"path": "total", "gte": float64(10)}); err == nil {
		t.Fatal("expected gte violation to fail")
	}
}

func TestAssertExpectationAbsentOrNotContains(t *testing.T) {
	body := map[string]any{"note": "no secret here"}
	if err := AssertExpectation(body, map[string]any{"path": "note", "absentOrNotContains": "PRIVATE"}); err != nil {
		t.Fatalf("expected pass when the needle is absent, got %v", err)
	}
	body["note"] = "contains PRIVATE-SENTINEL"
	if err := AssertExpectation(body, map[string]any{"path": "note", "absentOrNotContains": "PRIVATE"}); err == nil {
		t.Fatal("expected a leaked needle to fail")
	}
	// A wholly absent path passes: "absent OR not-containing".
	if err := AssertExpectation(body, map[string]any{"path": "missing", "absentOrNotContains": "PRIVATE"}); err != nil {
		t.Fatalf("expected absent path to pass, got %v", err)
	}
}

func TestAssertExpectationRequiredPathMissing(t *testing.T) {
	body := map[string]any{}
	if err := AssertExpectation(body, map[string]any{"path": "id", "type": "string"}); err == nil {
		t.Fatal("expected a missing required path to fail")
	}
}
