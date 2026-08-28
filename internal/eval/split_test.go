package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitFilter(t *testing.T) {
	dir := t.TempDir()
	splitPath := filepath.Join(dir, "split.yaml")
	if err := os.WriteFile(splitPath, []byte(`
held_out:
  - "span-b"
  - "span-d"
`), 0o644); err != nil {
		t.Fatalf("write split fixture: %v", err)
	}

	split, err := LoadSplit(splitPath)
	if err != nil {
		t.Fatalf("LoadSplit: %v", err)
	}
	if len(split.HeldOut) != 2 || split.HeldOut[0] != "span-b" || split.HeldOut[1] != "span-d" {
		t.Fatalf("LoadSplit = %+v, want held_out [span-b span-d]", split)
	}

	labels := []Label{
		{Span: "span-a", Expected: ExpectedFact{Predicate: "works_at", Object: "x"}},
		{Span: "span-b", Expected: ExpectedFact{Predicate: "works_at", Object: "y"}},
		{Span: "span-c", Expected: ExpectedFact{Predicate: "prefers", Object: "z"}},
		{Span: "span-d", Expected: ExpectedFact{Predicate: "prefers", Object: "w"}},
	}
	heldOut, rest := split.Filter(labels)

	if len(heldOut) != 2 || heldOut[0].Span != "span-b" || heldOut[1].Span != "span-d" {
		t.Fatalf("heldOut = %+v, want span-b then span-d", heldOut)
	}
	if len(rest) != 2 || rest[0].Span != "span-a" || rest[1].Span != "span-c" {
		t.Fatalf("rest = %+v, want span-a then span-c", rest)
	}
}

// TestSplitFilterUnmatchedSpanIgnored pins the documented behavior: a split
// file naming a span absent from the labels slice must not panic or error
// -- the split file is allowed to name a superset (e.g. spans reserved for
// a future corpus revision that have not been labeled yet).
func TestSplitFilterUnmatchedSpanIgnored(t *testing.T) {
	split := Split{HeldOut: []string{"span-does-not-exist"}}
	labels := []Label{{Span: "span-a", Expected: ExpectedFact{Predicate: "works_at", Object: "x"}}}

	heldOut, rest := split.Filter(labels)
	if len(heldOut) != 0 {
		t.Fatalf("heldOut = %+v, want empty", heldOut)
	}
	if len(rest) != 1 || rest[0].Span != "span-a" {
		t.Fatalf("rest = %+v, want [span-a]", rest)
	}
}

func TestSplitFilterEmptySplit(t *testing.T) {
	var split Split
	labels := []Label{{Span: "span-a"}, {Span: "span-b"}}
	heldOut, rest := split.Filter(labels)
	if len(heldOut) != 0 {
		t.Fatalf("heldOut = %+v, want empty", heldOut)
	}
	if len(rest) != 2 {
		t.Fatalf("rest = %+v, want both labels", rest)
	}
}
