package reconcile

import (
	"fmt"

	"github.com/sirerun/serenity/internal/eval"
	prodreconcile "github.com/sirerun/serenity/internal/reconcile"
)

// isContradiction reports whether verdict v is contradiction-shaped -- the
// exact condition internal/reconcile.Engine.Process itself uses to decide
// whether a verdict needs to be staged to DISPOSITION at all (Conflict or
// WindowClose; see Process's own doc: "for VerdictConflict/VerdictWindowClose,
// stages exactly one... item"). Reusing that condition here, rather than
// re-deriving a separate notion of "is this a contradiction," means this
// eval's contradiction-detection recall never drifts from what the real
// routing logic actually treats as needing human review.
func isContradiction(v string) bool {
	return v == string(prodreconcile.VerdictConflict) || v == string(prodreconcile.VerdictWindowClose)
}

// Report is the reconcile section T2.18 adds to evals/report.json,
// alongside plan T3.16's Direction section (internal/eval/direction.Report)
// -- both attach to internal/eval/runner.Report the same way, one corpus
// each.
type Report struct {
	// VerdictConfusion is one-vs-rest precision/recall/F1 per Verdict
	// label (internal/eval.PRF1FromCounts -- the same per-label confusion
	// method internal/eval/direction.Report.VerdictByActionClass uses),
	// keyed by the verdict string. A verdict that never appears, expected
	// or predicted, across every row is omitted rather than reported as a
	// manufactured all-zero row (internal/eval.Score's own convention).
	VerdictConfusion map[string]eval.PRF1 `json:"verdict_confusion"`
	// Matrix is the raw expected-verdict -> predicted-verdict count table
	// VerdictConfusion's per-class tp/fp/fn figures are derived from --
	// exposed directly too so a human reading the report can see exactly
	// what is being confused with what, not just the aggregate rates (the
	// task's own name for this section, "verdict confusion matrix",
	// literally).
	Matrix map[string]map[string]int `json:"matrix"`
	// Contradiction is contradiction-detection recall/precision/F1 (RFC
	// SS16/SS17's explicitly-named metric, internal/eval.ContradictionRecall):
	// a golden row "is a contradiction" when its expected verdict is
	// Conflict or WindowClose (see isContradiction) -- the same condition
	// that decides whether Engine.Process routes to DISPOSITION at all, so
	// this measures exactly "did the detector correctly flag a pair that
	// needed human review," not a redefinition unique to this eval.
	Contradiction eval.PRF1 `json:"contradiction"`
	RowsScored    int       `json:"rows_scored"`
}

// Score runs Row.Detect (the real production Candidates+Detect call) over
// every row and compares its verdict against the row's golden
// ExpectedVerdict. It errors on a row with no ExpectedVerdict set (a
// fixture-authoring bug, not a scoring outcome to silently absorb).
func Score(rows []Row) (Report, error) {
	tp := make(map[string]int)
	fp := make(map[string]int)
	fn := make(map[string]int)
	verdicts := make(map[string]bool)
	matrix := make(map[string]map[string]int)

	var contradictionPairs []eval.ContradictionPair
	detected := make(map[string]bool)
	var contradictionFP int

	for _, r := range rows {
		if r.ExpectedVerdict == "" {
			return Report{}, fmt.Errorf("eval/reconcile: row %s has no expected_verdict", r.ID)
		}
		got := string(r.Detect().Verdict)

		verdicts[r.ExpectedVerdict] = true
		verdicts[got] = true

		if matrix[r.ExpectedVerdict] == nil {
			matrix[r.ExpectedVerdict] = make(map[string]int)
		}
		matrix[r.ExpectedVerdict][got]++

		if got == r.ExpectedVerdict {
			tp[got]++
		} else {
			fp[got]++
			fn[r.ExpectedVerdict]++
		}

		switch {
		case isContradiction(r.ExpectedVerdict):
			contradictionPairs = append(contradictionPairs, eval.ContradictionPair{ID: r.ID})
			if isContradiction(got) {
				detected[r.ID] = true
			}
		case isContradiction(got):
			// Expected a non-contradiction verdict (agree/neutral_additive/
			// scoped) but the detector flagged one anyway -- a real false
			// positive on the "does this need human review at all"
			// question, counted against ContradictionRecall's own
			// falsePositives parameter (it only receives the golden pairs,
			// per its own doc, so extra flags must be tallied by the
			// caller).
			contradictionFP++
		}
	}

	confusion := make(map[string]eval.PRF1, len(verdicts))
	for v := range verdicts {
		if tp[v] == 0 && fp[v] == 0 && fn[v] == 0 {
			continue
		}
		confusion[v] = eval.PRF1FromCounts(tp[v], fp[v], fn[v])
	}

	return Report{
		VerdictConfusion: confusion,
		Matrix:           matrix,
		Contradiction:    eval.ContradictionRecall(contradictionPairs, detected, contradictionFP),
		RowsScored:       len(rows),
	}, nil
}
