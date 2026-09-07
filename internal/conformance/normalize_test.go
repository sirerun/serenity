package conformance

import (
	"strings"
	"testing"
)

func TestCompareBodiesExactMatch(t *testing.T) {
	got := CompareBodies(`{"a":1,"b":"x"}`, `{"a":1,"b":"x"}`)
	if got != nil {
		t.Fatalf("expected no mismatches, got %v", got)
	}
}

func TestCompareBodiesExcusesDynamicIDsAndTimestamps(t *testing.T) {
	expected := `{"id":"cda08aff5a5749c7f6ad9b50d90554ac","created_at":"2026-09-07T12:00:00Z","verdict":"accept"}`
	actual := `{"id":"11112222333344445555666677778888","created_at":"2027-01-01T00:00:00.123456Z","verdict":"accept"}`
	got := CompareBodies(expected, actual)
	if got != nil {
		t.Fatalf("expected dynamic id/timestamp to be excused, got mismatches: %v", got)
	}
}

func TestCompareBodiesCatchesRealMismatch(t *testing.T) {
	expected := `{"id":"cda08aff5a5749c7f6ad9b50d90554ac","verdict":"accept"}`
	actual := `{"id":"11112222333344445555666677778888","verdict":"reject"}`
	got := CompareBodies(expected, actual)
	if len(got) != 1 {
		t.Fatalf("expected exactly one mismatch (verdict), got %v", got)
	}
	if !strings.Contains(got[0].Path, "verdict") {
		t.Fatalf("mismatch path = %q, want it to name verdict", got[0].Path)
	}
}

func TestCompareBodiesDoesNotExcuseAShortNumericStringAsDynamic(t *testing.T) {
	// Cursor tokens ("2", "4") and short ids ("g1") are deterministic in
	// this codebase (positional / caller-chosen), not server-random -- a
	// real difference there must still fail.
	got := CompareBodies(`{"next_cursor":"2"}`, `{"next_cursor":"4"}`)
	if len(got) != 1 {
		t.Fatalf("expected next_cursor mismatch to be caught, got %v", got)
	}
}

func TestCompareBodiesArrayLengthMismatch(t *testing.T) {
	got := CompareBodies(`{"items":[1,2,3]}`, `{"items":[1,2]}`)
	if len(got) != 1 {
		t.Fatalf("expected one array-length mismatch, got %v", got)
	}
}

func TestCompareBodiesMissingAndExtraKeys(t *testing.T) {
	got := CompareBodies(`{"a":1}`, `{"b":1}`)
	if len(got) != 2 {
		t.Fatalf("expected two mismatches (a absent from actual, b absent from expected), got %v", got)
	}
}

func TestCompareBodiesPlainTextFallback(t *testing.T) {
	if got := CompareBodies("unauthorized\n", "unauthorized\n"); got != nil {
		t.Fatalf("expected exact plain-text match, got %v", got)
	}
	got := CompareBodies("unauthorized\n", "forbidden\n")
	if len(got) != 1 {
		t.Fatalf("expected one mismatch for differing plain-text bodies, got %v", got)
	}
}

func TestFormatMismatchesJoinsLines(t *testing.T) {
	out := FormatMismatches([]Mismatch{{Path: "$.a", Expected: "1", Actual: "2"}, {Path: "$.b", Expected: `"x"`, Actual: `"y"`}})
	want := "$.a: expected 1, got 2\n$.b: expected \"x\", got \"y\""
	if out != want {
		t.Fatalf("FormatMismatches = %q, want %q", out, want)
	}
}
