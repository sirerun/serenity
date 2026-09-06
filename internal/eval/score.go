package eval

import (
	"regexp"

	"github.com/sirerun/serenity/internal/store"
)

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

// PRF1FromCounts derives a PRF1 from raw true/false positive/negative
// counts, exported so a caller scoring a different confusion shape (e.g.
// internal/eval/direction's one-verdict-per-row, per-action-class
// matching, T3.16) reuses the same precision/recall/F1 math instead of
// reimplementing it -- ContradictionRecall uses the unexported prf1 the
// same way from inside this package.
func PRF1FromCounts(tp, fp, fn int) PRF1 {
	return prf1(tp, fp, fn)
}

// objectHyphenRx treats a run of hyphens as a word separator, same as
// whitespace, before objectMatchKey hands off to store.NormalizeKey. This
// closes a real scoring gap found running T1.23's first-ever live eval:
// the ava corpus's golden objects are hand-authored slugs ("contoso-systems")
// representing a normalized entity identity, but nothing in the real
// system ever asks a model to slug its object output (buildPrompt only
// slugs the subject) and store.NormalizeKey itself never hyphenates -- so
// a live model's correct, natural-language object ("Contoso Systems")
// could never exact-match the slug and scored as a false positive/false
// negative pair instead of a true positive, for every family whose golden
// objects use this convention. -mode cached never exposed this because
// its hand-authored fixture predictions already used the corpus's own
// slug spelling.
var objectHyphenRx = regexp.MustCompile(`-+`)

// objectMatchKey is the comparison key Score uses for the Object field of
// both a golden Label and a Prediction: hyphens folded to spaces first
// (so "contoso-systems" and "Contoso Systems" compare equal), then
// store.NormalizeKey's existing production rule (RFC SS7.2: lowercase,
// collapse whitespace, canonical date/number forms) -- the same
// normalizer real claim-id derivation and semantic dedup already rely on,
// not a new eval-only convention.
func objectMatchKey(object string) string {
	return store.NormalizeKey(objectHyphenRx.ReplaceAllString(object, " "))
}

// Score computes precision/recall/F1 per family (== predicate; see Label)
// by matching each Prediction against the golden Labels on the triple
// (Span, Predicate, objectMatchKey(Object)) -- no partial credit, but
// Object is compared after normalization (see objectMatchKey), not as a
// raw exact string. A prediction with no matching label is a false
// positive for its own (predicted) family; a label with no matching
// prediction is a false negative for its own (expected) family;
// everything else is a true positive. The result is keyed by every family
// that appears in either input -- a family absent from both labels and
// predictions is simply not reported, rather than appearing with a
// manufactured all-zero row.
func Score(labels []Label, predictions []Prediction) map[string]PRF1 {
	type key struct{ span, predicate, object string }

	labelSet := make(map[key]bool, len(labels))
	for _, l := range labels {
		labelSet[key{l.Span, l.Expected.Predicate, objectMatchKey(l.Expected.Object)}] = true
	}
	predSet := make(map[key]bool, len(predictions))
	for _, p := range predictions {
		predSet[key{p.Span, p.Predicate, objectMatchKey(p.Object)}] = true
	}

	tp := make(map[string]int)
	fp := make(map[string]int)
	fn := make(map[string]int)

	for _, p := range predictions {
		k := key{p.Span, p.Predicate, objectMatchKey(p.Object)}
		if labelSet[k] {
			tp[p.Predicate]++
		} else {
			fp[p.Predicate]++
		}
	}
	for _, l := range labels {
		k := key{l.Span, l.Expected.Predicate, objectMatchKey(l.Expected.Object)}
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
