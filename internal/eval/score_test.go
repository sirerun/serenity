package eval

import "testing"

// fixtureLabelsAndPredictions builds a small, self-contained, hand-checkable
// fixture over three families from config.Default()'s seed vocabulary
// (internal/config/config.go): works_at, has_role, prefers. It deliberately
// does NOT depend on the Ava Standardo corpus (a separate, later task) --
// see the per-family math in each predictions block below and in
// TestScoreExactPerFamily.
//
// The "fake extractor" here is nothing but this hardcoded []Prediction
// slice, defined in a _test.go file only (zero-stub policy: fakes belong in
// tests, never in production code).
func fixtureLabelsAndPredictions() ([]Label, []Prediction) {
	labels := []Label{
		// works_at: 3 labels.
		{Span: "w1", Expected: ExpectedFact{Predicate: "works_at", Object: "acme"}},
		{Span: "w2", Expected: ExpectedFact{Predicate: "works_at", Object: "beta"}},
		{Span: "w3", Expected: ExpectedFact{Predicate: "works_at", Object: "gamma"}},
		// has_role: 2 labels.
		{Span: "h1", Expected: ExpectedFact{Predicate: "has_role", Object: "engineer"}},
		{Span: "h2", Expected: ExpectedFact{Predicate: "has_role", Object: "manager"}},
		// prefers: 2 labels.
		{Span: "p1", Expected: ExpectedFact{Predicate: "prefers", Object: "tea"}},
		{Span: "p2", Expected: ExpectedFact{Predicate: "prefers", Object: "coffee"}},
	}

	predictions := []Prediction{
		// works_at: w1, w2 correct (TP=2). w3 gets the WRONG object -- a
		// miss that is simultaneously a false negative for w3's label and
		// a false positive for this wrong prediction -- plus one more
		// prediction on a span with no works_at label at all (another false
		// positive). => TP=2 FP=2 FN=1.
		{Span: "w1", Predicate: "works_at", Object: "acme"},
		{Span: "w2", Predicate: "works_at", Object: "beta"},
		{Span: "w3", Predicate: "works_at", Object: "WRONG-gamma"},
		{Span: "w9", Predicate: "works_at", Object: "extra"},

		// has_role: both labels correct (TP=2) plus one extra false
		// prediction on an unlabeled span (FP=1), and every label matched
		// (FN=0). => TP=2 FP=1 FN=0.
		{Span: "h1", Predicate: "has_role", Object: "engineer"},
		{Span: "h2", Predicate: "has_role", Object: "manager"},
		{Span: "h9", Predicate: "has_role", Object: "extra"},

		// prefers: the extractor produces NO predictions for this family
		// at all. => TP=0 FP=0 FN=2 (both labels missed).
	}
	return labels, predictions
}

func TestScoreExactPerFamily(t *testing.T) {
	labels, predictions := fixtureLabelsAndPredictions()
	got := Score(labels, predictions)

	if len(got) != 3 {
		t.Fatalf("Score returned %d families, want 3: %+v", len(got), got)
	}

	// works_at: TP=2, FP=2, FN=1.
	//   P = 2/(2+2) = 0.5
	//   R = 2/(2+1) = 2/3 ≈ 0.6667
	//   F1 = 2*P*R/(P+R) ≈ 0.5714
	wantWorksAt := PRF1{TP: 2, FP: 2, FN: 1}
	wantWorksAt.Precision = float64(wantWorksAt.TP) / float64(wantWorksAt.TP+wantWorksAt.FP)
	wantWorksAt.Recall = float64(wantWorksAt.TP) / float64(wantWorksAt.TP+wantWorksAt.FN)
	wantWorksAt.F1 = 2 * wantWorksAt.Precision * wantWorksAt.Recall / (wantWorksAt.Precision + wantWorksAt.Recall)

	// has_role: TP=2, FP=1, FN=0.
	//   P = 2/(2+1) = 2/3 ≈ 0.6667
	//   R = 2/(2+0) = 1.0
	//   F1 = 2*(2/3)*1 / ((2/3)+1) = (4/3)/(5/3) = 4/5 = 0.8
	wantHasRole := PRF1{TP: 2, FP: 1, FN: 0}
	wantHasRole.Precision = float64(wantHasRole.TP) / float64(wantHasRole.TP+wantHasRole.FP)
	wantHasRole.Recall = float64(wantHasRole.TP) / float64(wantHasRole.TP+wantHasRole.FN)
	wantHasRole.F1 = 2 * wantHasRole.Precision * wantHasRole.Recall / (wantHasRole.Precision + wantHasRole.Recall)

	// prefers: TP=0, FP=0, FN=2. No predictions at all for this family, so
	// precision is 0 by the documented TP+FP==0 convention (never NaN);
	// recall is 0/2=0; F1 is 0.
	wantPrefers := PRF1{TP: 0, FP: 0, FN: 2, Precision: 0, Recall: 0, F1: 0}

	cases := map[string]PRF1{
		"works_at": wantWorksAt,
		"has_role": wantHasRole,
		"prefers":  wantPrefers,
	}
	for family, want := range cases {
		gotPRF1, ok := got[family]
		if !ok {
			t.Fatalf("Score result missing family %q; got families %+v", family, got)
		}
		if gotPRF1 != want {
			t.Errorf("family %q: Score = %+v, want %+v", family, gotPRF1, want)
		}
	}
}

// TestScorePrecisionRecallConventionsAtZero exercises the TP+FP==0 and
// TP+FN==0 conventions directly, and confirms Score only reports families
// that appear in at least one of its inputs.
func TestScorePrecisionRecallConventionsAtZero(t *testing.T) {
	labels := []Label{{Span: "s1", Expected: ExpectedFact{Predicate: "committed_to", Object: "x"}}}
	got := Score(labels, nil)

	want := PRF1{TP: 0, FP: 0, FN: 1, Precision: 0, Recall: 0, F1: 0}
	if got["committed_to"] != want {
		t.Errorf("got %+v, want %+v", got["committed_to"], want)
	}
	if _, ok := got["deadline_on"]; ok {
		t.Errorf("Score reported family %q, which has no labels and no predictions", "deadline_on")
	}
	if len(got) != 1 {
		t.Fatalf("Score returned %d families, want 1: %+v", len(got), got)
	}
}

func TestScorePerfectMatch(t *testing.T) {
	labels := []Label{
		{Span: "s1", Expected: ExpectedFact{Predicate: "owns_account", Object: "acct-1"}},
		{Span: "s2", Expected: ExpectedFact{Predicate: "owns_account", Object: "acct-2"}},
	}
	predictions := []Prediction{
		{Span: "s1", Predicate: "owns_account", Object: "acct-1"},
		{Span: "s2", Predicate: "owns_account", Object: "acct-2"},
	}
	got := Score(labels, predictions)
	want := PRF1{TP: 2, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if got["owns_account"] != want {
		t.Errorf("got %+v, want %+v", got["owns_account"], want)
	}
}
