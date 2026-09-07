package eval

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestBootstrapProportionPointEstimateMatchesPlainMean(t *testing.T) {
	outcomes := []bool{true, true, true, false, true, false, true, true} // 6/8 = 0.75
	ci := bootstrapProportion(outcomes, 2000, 0.90, rand.New(rand.NewSource(1)))
	if got, want := ci.Point, 0.75; got != want {
		t.Fatalf("Point = %v, want %v", got, want)
	}
	if ci.N != 8 {
		t.Fatalf("N = %d, want 8", ci.N)
	}
}

func TestBootstrapProportionEmptyOutcomesReturnsZeroValue(t *testing.T) {
	ci := bootstrapProportion(nil, 2000, 0.90, rand.New(rand.NewSource(1)))
	if ci != (BootstrapCI{Resamples: 2000, Level: 0.90}) {
		t.Fatalf("empty outcomes: got %+v, want zero Point/Lower/Upper/N", ci)
	}
}

func TestBootstrapProportionAllTrueGivesDegenerateCIAtOne(t *testing.T) {
	outcomes := []bool{true, true, true, true, true}
	ci := bootstrapProportion(outcomes, 2000, 0.90, rand.New(rand.NewSource(1)))
	if ci.Point != 1 || ci.Lower != 1 || ci.Upper != 1 {
		t.Fatalf("all-true outcomes: got Point=%v Lower=%v Upper=%v, want all 1 (every resample is also all-true)", ci.Point, ci.Lower, ci.Upper)
	}
}

func TestBootstrapProportionAllFalseGivesDegenerateCIAtZero(t *testing.T) {
	outcomes := []bool{false, false, false, false, false}
	ci := bootstrapProportion(outcomes, 2000, 0.90, rand.New(rand.NewSource(1)))
	if ci.Point != 0 || ci.Lower != 0 || ci.Upper != 0 {
		t.Fatalf("all-false outcomes: got Point=%v Lower=%v Upper=%v, want all 0", ci.Point, ci.Lower, ci.Upper)
	}
}

// TestBootstrapProportionDeterministicForFixedSeed pins down the property
// DefaultBootstrapOptions relies on: identical (outcomes, resamples,
// level, seed) always yields an identical CI, so a report is reproducible
// without a real live-model call.
func TestBootstrapProportionDeterministicForFixedSeed(t *testing.T) {
	outcomes := []bool{true, false, true, true, false, true, true, true, false, true}
	a := bootstrapProportion(outcomes, 5000, 0.90, rand.New(rand.NewSource(42)))
	b := bootstrapProportion(outcomes, 5000, 0.90, rand.New(rand.NewSource(42)))
	if a != b {
		t.Fatalf("same seed produced different CIs: %+v vs %+v", a, b)
	}
}

// TestBootstrapProportionWidensAtSmallN is the actual statistical
// property T1.32 exists to surface: at the same true proportion, a
// smaller sample's CI is wider (more sampling uncertainty) than a larger
// sample's -- directly reproducing chief-architect's finding in miniature
// (n=4 vs n=20 at the same 0.75 proportion).
func TestBootstrapProportionWidensAtSmallN(t *testing.T) {
	small := []bool{true, true, true, false} // 3/4 = 0.75
	large := []bool{
		true, true, true, false, true, true, true, false, true, true,
		true, false, true, true, true, false, true, true, true, false,
	} // 15/20 = 0.75
	smallCI := bootstrapProportion(small, 20000, 0.90, rand.New(rand.NewSource(7)))
	largeCI := bootstrapProportion(large, 20000, 0.90, rand.New(rand.NewSource(7)))

	smallWidth := smallCI.Upper - smallCI.Lower
	largeWidth := largeCI.Upper - largeCI.Lower
	if smallWidth <= largeWidth {
		t.Fatalf("expected the n=4 CI (width %.3f) to be wider than the n=20 CI (width %.3f) at the same 0.75 point estimate", smallWidth, largeWidth)
	}
}

// TestBootstrapProportionLowerBoundCanMissAFloorEvenWhenPointClearsIt is
// T1.32's headline scenario spelled out as a test: a family whose point
// estimate clears 0.80 recall can still have a bootstrap lower bound
// below 0.70 at small n -- the exact gap the old point-estimate-only bar
// couldn't see.
func TestBootstrapProportionLowerBoundCanMissAFloorEvenWhenPointClearsIt(t *testing.T) {
	// n=4, 4/4 = 1.0 point recall (clears the old 0.80 bar outright), but
	// the bootstrap can never see anything except "the sample I have" --
	// the 90% CI collapses to [1,1] here (every resample of an all-true
	// vector is all-true), which itself demonstrates the point estimate's
	// blindness at n=4 differently: it is maximally overconfident, not
	// appropriately uncertain. Use a near-miss (3/4) instead, which is
	// exactly T1.29's own recurring shape (F1 0.75, one flipped span).
	nearMiss := []bool{true, true, true, false} // 3/4 = 0.75, misses the old point-estimate 0.80 bar already
	ci := bootstrapProportion(nearMiss, 20000, 0.90, rand.New(rand.NewSource(3)))
	if ci.Lower >= 0.70 {
		t.Fatalf("n=4 near-miss (3/4=0.75) CI lower bound = %v, want < 0.70 -- small-n sampling noise should make this family fail the T1.32 floor even though it might have looked marginal under a raw point estimate", ci.Lower)
	}
}

func TestScoreWithCIPRF1MatchesPlainScore(t *testing.T) {
	labels := []Label{
		{Span: "s1", Expected: ExpectedFact{Predicate: "works_at", Object: "acme-corp"}},
		{Span: "s2", Expected: ExpectedFact{Predicate: "works_at", Object: "beta-llc"}},
		{Span: "s3", Expected: ExpectedFact{Predicate: "has_role", Object: "staff-engineer"}},
	}
	predictions := []Prediction{
		{Span: "s1", Predicate: "works_at", Object: "Acme Corp"},  // TP (normalized match)
		{Span: "s2", Predicate: "works_at", Object: "beta-llc"},   // TP
		{Span: "s3", Predicate: "has_role", Object: "qa-analyst"}, // FP (wrong object) + s3's label is FN
		{Span: "s4", Predicate: "prefers", Object: "oat-milk"},    // FP, no matching label at all
	}

	plain := Score(labels, predictions)
	withCI := ScoreWithCI(labels, predictions, DefaultBootstrapOptions())

	if len(plain) != len(withCI) {
		t.Fatalf("family count mismatch: Score=%d ScoreWithCI=%d", len(plain), len(withCI))
	}
	for family, want := range plain {
		got, ok := withCI[family]
		if !ok {
			t.Fatalf("family %q present in Score but missing from ScoreWithCI", family)
		}
		if got.PRF1 != want {
			t.Errorf("family %q: PRF1 = %+v, want %+v (must match plain Score exactly)", family, got.PRF1, want)
		}
	}
}

// TestScoreWithCIIsIndependentOfMapIterationOrder guards the "fresh rng
// per family, same seed" design: Go's map iteration order is randomized,
// so if ScoreWithCI's per-family CI depended on WHEN in iteration order a
// family's resampling ran (a single rng shared across families, advanced
// as iteration goes), two calls could disagree. Calling twice and
// requiring byte-identical output pins this down without needing to
// force a specific iteration order.
func TestScoreWithCIIsIndependentOfMapIterationOrder(t *testing.T) {
	labels := []Label{
		{Span: "a1", Expected: ExpectedFact{Predicate: "works_at", Object: "acme-corp"}},
		{Span: "a2", Expected: ExpectedFact{Predicate: "works_at", Object: "beta-llc"}},
		{Span: "b1", Expected: ExpectedFact{Predicate: "has_role", Object: "staff-engineer"}},
		{Span: "b2", Expected: ExpectedFact{Predicate: "has_role", Object: "qa-analyst"}},
		{Span: "c1", Expected: ExpectedFact{Predicate: "prefers", Object: "oat-milk"}},
	}
	predictions := []Prediction{
		{Span: "a1", Predicate: "works_at", Object: "acme-corp"},
		{Span: "b1", Predicate: "has_role", Object: "staff-engineer"},
		{Span: "b2", Predicate: "has_role", Object: "wrong-object"},
		{Span: "c1", Predicate: "prefers", Object: "oat-milk"},
	}
	opts := DefaultBootstrapOptions()

	first := ScoreWithCI(labels, predictions, opts)
	for i := 0; i < 5; i++ {
		again := ScoreWithCI(labels, predictions, opts)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d: ScoreWithCI output differs between calls with identical input -- got %+v, want %+v", i, again, first)
		}
	}
}
