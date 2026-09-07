package eval

import (
	"math/rand"
	"sort"
)

// BootstrapCI is a percentile bootstrap confidence interval around a
// precision or recall point estimate, computed by resampling the
// per-unit correctness outcomes underlying that estimate with
// replacement -- T1.32's mechanism for the P>=0.90/R>=0.80 bar's known
// small-sample brittleness (RFC 0001 §16 / docs/plans/E1-m1-ingest.md
// T1.29/T1.32): a single-draw point estimate on a handful of held-out
// spans can clear or miss a fixed bar by sampling noise alone, and a
// confidence interval makes that uncertainty visible instead of hiding
// it behind one number.
type BootstrapCI struct {
	// Point is the plain point estimate (TP/(TP+FN) for recall,
	// TP/(TP+FP) for precision) -- identical to PRF1.Recall/.Precision,
	// repeated here so a BootstrapCI is self-contained.
	Point float64 `json:"point"`
	// Lower and Upper are the bootstrap distribution's Level-confidence
	// interval bounds (e.g. for Level=0.90, the 5th and 95th
	// percentiles of the resampled statistic). Lower doubles as a
	// one-sided (1+Level)/2-confidence lower bound -- the number a
	// "recall lower bound clears 0.70" pass rule reads.
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	// N is the number of scored units resampled (recall: golden labels
	// for this family; precision: predictions for this family) -- T1.32's
	// acc line calls this a "scored unit".
	N int `json:"n"`
	// Resamples and Level record how Lower/Upper were computed, so a
	// reader of a serialized report never has to guess the method.
	Resamples int     `json:"resamples"`
	Level     float64 `json:"level"`
}

// BootstrapOptions controls ScoreWithCI's resampling.
type BootstrapOptions struct {
	// Resamples is how many bootstrap draws to take per family per
	// metric (precision, recall).
	Resamples int
	// Level is the two-sided confidence level, e.g. 0.90 for a 90% CI
	// (5th/95th percentile bounds) -- Lower then also reads as a 95%
	// one-sided lower confidence bound, which is what "recall lower
	// bound clears 0.70" (T1.32's acc line) means in this package.
	Level float64
	// Seed makes the resampling deterministic: the same (outcomes,
	// Resamples, Level, Seed) always produces the same CI, so a report
	// is reproducible and testable without needing a real live-model
	// call to exercise the bootstrap math itself.
	Seed int64
}

// DefaultBootstrapOptions is T1.32's chosen configuration: 10,000
// resamples (stable percentile estimates at this scale -- more would cost
// CPU with no visible precision gain, per a quick sanity check comparing
// 10k vs 50k resamples against the same outcome vector during
// development), a 90% two-sided confidence level (so Lower is a 95%
// one-sided lower bound, a conventional and defensible choice absent a
// specific level named in the acc line), and a fixed seed so eval-runner's
// own output is reproducible run to run for identical underlying
// predictions -- unlike the P/R point estimates, which vary with the live
// model's real sampling (docs/lore.md L-0011), the CI *math* itself must
// not add its own nondeterminism on top.
func DefaultBootstrapOptions() BootstrapOptions {
	return BootstrapOptions{Resamples: 10000, Level: 0.90, Seed: 1}
}

// FamilyScore is one family's PRF1 point estimate plus bootstrap
// confidence intervals on precision and recall.
type FamilyScore struct {
	PRF1
	RecallCI    BootstrapCI `json:"recall_ci"`
	PrecisionCI BootstrapCI `json:"precision_ci"`
}

// ScoreWithCI is Score plus a bootstrap confidence interval on precision
// and recall for every family, built from the same per-unit matching
// Score performs -- it does not change Score's own matching semantics or
// its TP/FP/FN counts, only additionally records, per family, which
// individual golden labels were recalled (for the recall CI) and which
// individual predictions were correct (for the precision CI), then
// resamples those per-unit outcome vectors. Score itself is left
// unmodified (still used directly by any caller that doesn't need a CI,
// and by ScoreWithCI's own tests as a cross-check that the point
// estimates agree).
func ScoreWithCI(labels []Label, predictions []Prediction, opts BootstrapOptions) map[string]FamilyScore {
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
	// recallOutcomes[family] has one entry per golden label of that
	// family: true if a matching prediction exists (a TP, i.e. recalled),
	// false if not (an FN, i.e. missed).
	recallOutcomes := make(map[string][]bool)
	// precisionOutcomes[family] has one entry per prediction of that
	// family: true if it matches a golden label (a TP), false if not (an
	// FP).
	precisionOutcomes := make(map[string][]bool)

	for _, p := range predictions {
		k := key{p.Span, p.Predicate, objectMatchKey(p.Object)}
		hit := labelSet[k]
		if hit {
			tp[p.Predicate]++
		} else {
			fp[p.Predicate]++
		}
		precisionOutcomes[p.Predicate] = append(precisionOutcomes[p.Predicate], hit)
	}
	for _, l := range labels {
		k := key{l.Span, l.Expected.Predicate, objectMatchKey(l.Expected.Object)}
		hit := predSet[k]
		if !hit {
			fn[l.Expected.Predicate]++
		}
		recallOutcomes[l.Expected.Predicate] = append(recallOutcomes[l.Expected.Predicate], hit)
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

	out := make(map[string]FamilyScore, len(families))
	for f := range families {
		// A fresh rng per family (same seed) keeps each family's CI
		// independent of map-iteration order -- iterating families in a
		// different order must not change any individual family's CI.
		rng := rand.New(rand.NewSource(opts.Seed))
		recallCI := bootstrapProportion(recallOutcomes[f], opts.Resamples, opts.Level, rng)
		precisionCI := bootstrapProportion(precisionOutcomes[f], opts.Resamples, opts.Level, rng)
		out[f] = FamilyScore{
			PRF1:        prf1(tp[f], fp[f], fn[f]),
			RecallCI:    recallCI,
			PrecisionCI: precisionCI,
		}
	}
	return out
}

// bootstrapProportion computes a percentile bootstrap CI for the mean of
// a boolean outcome vector (a proportion, e.g. recall or precision).
// Returns the zero value when outcomes is empty -- an undefined CI,
// matching precisionOf/recallOf's own zero-denominator convention (never
// NaN, always a usable zero value).
func bootstrapProportion(outcomes []bool, resamples int, level float64, rng *rand.Rand) BootstrapCI {
	n := len(outcomes)
	if n == 0 || resamples <= 0 {
		return BootstrapCI{Resamples: resamples, Level: level}
	}

	point := meanBool(outcomes)

	means := make([]float64, resamples)
	for i := 0; i < resamples; i++ {
		var sum int
		for j := 0; j < n; j++ {
			if outcomes[rng.Intn(n)] {
				sum++
			}
		}
		means[i] = float64(sum) / float64(n)
	}
	sort.Float64s(means)

	tail := (1 - level) / 2
	lowerIdx := int(tail * float64(resamples))
	upperIdx := int((1 - tail) * float64(resamples))
	if lowerIdx < 0 {
		lowerIdx = 0
	}
	if upperIdx >= resamples {
		upperIdx = resamples - 1
	}

	return BootstrapCI{
		Point:     point,
		Lower:     means[lowerIdx],
		Upper:     means[upperIdx],
		N:         n,
		Resamples: resamples,
		Level:     level,
	}
}

func meanBool(outcomes []bool) float64 {
	if len(outcomes) == 0 {
		return 0
	}
	var sum int
	for _, o := range outcomes {
		if o {
			sum++
		}
	}
	return float64(sum) / float64(len(outcomes))
}
