package briefing

import "testing"

// countEstimator is a trivial Estimator: one unit per item, regardless of
// text -- lets these tests reason about "how many items fit" directly by
// count rather than by word math, isolating Pack's own section-inclusion
// logic from WordEstimator's separate word-counting concern (that gets
// its own test below).
func countEstimator(string) int { return 1 }

// TestPackIncludesEverySectionUnderBudget is the trivial baseline: every
// section's own cost fits inside a generous budget, so Pack returns every
// section whole, in order, with Omitted == 0 for each.
func TestPackIncludesEverySectionUnderBudget(t *testing.T) {
	sections := []Section{
		{Name: SectionBlocked, Items: []Item{{Text: "a"}}},
		{Name: SectionNeedsYou, Items: []Item{{Text: "b"}, {Text: "c"}}},
		{Name: SectionDrift, Items: nil},
	}
	got := Pack(sections, 10, countEstimator)
	if len(got.Sections) != 3 {
		t.Fatalf("got %d sections, want 3", len(got.Sections))
	}
	want := []struct {
		name    SectionName
		items   int
		omitted int
	}{
		{SectionBlocked, 1, 0},
		{SectionNeedsYou, 2, 0},
		{SectionDrift, 0, 0},
	}
	for i, w := range want {
		ps := got.Sections[i]
		if ps.Name != w.name || len(ps.Items) != w.items || ps.Omitted != w.omitted {
			t.Fatalf("section %d = %+v, want name=%s items=%d omitted=%d", i, ps, w.name, w.items, w.omitted)
		}
	}
}

// TestPackDropsSectionOverBudgetWhole is plan T2.17's own acc-line
// clause verbatim: "a section over budget is dropped whole with
// omitted: N." A 3-item section against a budget of 2 must vanish
// entirely (Items == nil), not truncate to 2 items, and Omitted must
// equal the section's full original item count (3), not the shortfall.
func TestPackDropsSectionOverBudgetWhole(t *testing.T) {
	sections := []Section{
		{Name: SectionBlocked, Items: []Item{{Text: "a"}, {Text: "b"}, {Text: "c"}}},
	}
	got := Pack(sections, 2, countEstimator)
	if len(got.Sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(got.Sections))
	}
	ps := got.Sections[0]
	if ps.Items != nil {
		t.Fatalf("dropped section kept items: %+v, want nil", ps.Items)
	}
	if ps.Omitted != 3 {
		t.Fatalf("Omitted = %d, want 3 (the section's full item count, not the shortfall)", ps.Omitted)
	}
}

// TestPackDroppingOneSectionDoesNotStarveALaterOne proves Pack's own
// documented greedy-by-priority behavior: dropping a higher-priority
// section whole does not consume the budget it would have used, so a
// later, lower-priority section that fits on its own is still included.
// Budget 3: Blocked costs 5 (dropped whole), NeedsYou costs 2 (fits in
// the full, untouched budget of 3).
func TestPackDroppingOneSectionDoesNotStarveALaterOne(t *testing.T) {
	sections := []Section{
		{Name: SectionBlocked, Items: []Item{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}}},
		{Name: SectionNeedsYou, Items: []Item{{Text: "x"}, {Text: "y"}}},
	}
	got := Pack(sections, 3, countEstimator)
	if got.Sections[0].Omitted != 5 || got.Sections[0].Items != nil {
		t.Fatalf("Blocked = %+v, want dropped whole (omitted 5)", got.Sections[0])
	}
	if got.Sections[1].Omitted != 0 || len(got.Sections[1].Items) != 2 {
		t.Fatalf("NeedsYou = %+v, want included whole (2 items, omitted 0)", got.Sections[1])
	}
}

// TestPackEmptySectionIsIncludedNotDropped: a section with zero items
// costs 0, always fits, and must render as included-but-empty (Items ==
// non-nil empty or nil is fine, but Omitted must stay 0) -- Render (see
// TestRenderEmptyIncludedSectionShowsNothing below) is what turns that
// into "(nothing)" rather than "(omitted: 0)".
func TestPackEmptySectionIsIncludedNotDropped(t *testing.T) {
	got := Pack([]Section{{Name: SectionDrift, Items: nil}}, 0, countEstimator)
	ps := got.Sections[0]
	if ps.Omitted != 0 {
		t.Fatalf("Omitted = %d, want 0 for an empty section (included, not dropped)", ps.Omitted)
	}
}

// TestWordEstimatorCountsWhitespaceSeparatedWords pins RFC 0001 §7's own
// unit ("hard cap 800 words") to the obvious implementation.
func TestWordEstimatorCountsWhitespaceSeparatedWords(t *testing.T) {
	cases := map[string]int{
		"":                 0,
		"one":              1,
		"one two three":    3,
		"  leading spaces": 2,
		"trailing   ":      1,
	}
	for text, want := range cases {
		if got := WordEstimator(text); got != want {
			t.Errorf("WordEstimator(%q) = %d, want %d", text, got, want)
		}
	}
}

// TestRenderEmptyIncludedSectionShowsNothing proves Render distinguishes
// "included but nothing to show" ("(nothing)") from "dropped whole"
// ("(omitted: N)") -- the two zero-item cases Pack can produce read
// differently to a human, which is the whole point of carrying Omitted
// as a separate field rather than collapsing both into len(Items)==0.
func TestRenderEmptyIncludedSectionShowsNothing(t *testing.T) {
	b := Briefing{Sections: []PackedSection{
		{Name: SectionDrift, Items: nil, Omitted: 0},
		{Name: SectionBlocked, Items: nil, Omitted: 4},
		{Name: SectionNeedsYou, Items: []Item{{Text: "reconcile abc123"}}},
	}}
	got := Render(b)
	want := "## Drift\n(nothing)\n\n## Blocked\n(omitted: 4)\n\n## Needs you\n- reconcile abc123\n"
	if got != want {
		t.Fatalf("Render mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
