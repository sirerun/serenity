package eval

// Prediction is one fact an extractor produced for a span -- the thing
// Score checks against the golden Labels. Family is Predicate (1:1 with
// the seed vocabulary's family names, see Label).
type Prediction struct {
	Span      string
	Predicate string
	Object    string
}

// PRF1 is a precision/recall/F1 result together with the raw confusion
// counts it was derived from.
type PRF1 struct {
	TP, FP, FN            int
	Precision, Recall, F1 float64
}

// precisionOf, recallOf, and f1Of use the standard definitions, with the
// conventional 0 (never NaN) when a ratio's denominator is zero: a family
// with zero predictions has undefined precision in the strict sense, but
// reporting 0 keeps every family's PRF1 usable in an aggregate report
// without a special case at the call site.
func precisionOf(tp, fp int) float64 {
	if tp+fp == 0 {
		return 0
	}
	return float64(tp) / float64(tp+fp)
}

func recallOf(tp, fn int) float64 {
	if tp+fn == 0 {
		return 0
	}
	return float64(tp) / float64(tp+fn)
}

func f1Of(precision, recall float64) float64 {
	if precision+recall == 0 {
		return 0
	}
	return 2 * precision * recall / (precision + recall)
}

func prf1(tp, fp, fn int) PRF1 {
	precision := precisionOf(tp, fp)
	recall := recallOf(tp, fn)
	return PRF1{TP: tp, FP: fp, FN: fn, Precision: precision, Recall: recall, F1: f1Of(precision, recall)}
}

// Score computes precision/recall/F1 per family (== predicate; see Label)
// by matching each Prediction against the golden Labels on the exact
// triple (Span, Predicate, Object) -- no fuzzy or partial-credit matching.
// A prediction with no matching label is a false positive for its own
// (predicted) family; a label with no matching prediction is a false
// negative for its own (expected) family; everything else is a true
// positive. The result is keyed by every family that appears in either
// input -- a family absent from both labels and predictions is simply not
// reported, rather than appearing with a manufactured all-zero row.
func Score(labels []Label, predictions []Prediction) map[string]PRF1 {
	type key struct{ span, predicate, object string }

	labelSet := make(map[key]bool, len(labels))
	for _, l := range labels {
		labelSet[key{l.Span, l.Expected.Predicate, l.Expected.Object}] = true
	}
	predSet := make(map[key]bool, len(predictions))
	for _, p := range predictions {
		predSet[key{p.Span, p.Predicate, p.Object}] = true
	}

	tp := make(map[string]int)
	fp := make(map[string]int)
	fn := make(map[string]int)

	for _, p := range predictions {
		k := key{p.Span, p.Predicate, p.Object}
		if labelSet[k] {
			tp[p.Predicate]++
		} else {
			fp[p.Predicate]++
		}
	}
	for _, l := range labels {
		k := key{l.Span, l.Expected.Predicate, l.Expected.Object}
		if !predSet[k] {
			fn[l.Expected.Predicate]++
		}
	}

	families := make(map[string]bool)
	for f := range tp {
		families[f] = true
	}
	for f := range fp {
		families[f] = true
	}
	for f := range fn {
		families[f] = true
	}

	out := make(map[string]PRF1, len(families))
	for f := range families {
		out[f] = prf1(tp[f], fp[f], fn[f])
	}
	return out
}
