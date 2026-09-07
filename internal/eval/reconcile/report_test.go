package reconcile

import (
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/eval"
)

func newClaimRow(id, expected, object, existingObject string) Row {
	return Row{
		ID:              id,
		NewClaim:        ClaimFixture{ID: "claim-a", Subject: "acme-corp", Predicate: "has_balance", Object: object},
		Active:          []ClaimFixture{{ID: "claim-b", Subject: "acme-corp", Predicate: "has_balance", Object: existingObject}},
		ExpectedVerdict: expected,
	}
}

// TestScorePerfectRowsYieldAllTruePositivesAndFullContradictionRecall is
// T2.18's acc-line clause "report has a reconcile section with per-verdict
// P/R/F1": every row's expected verdict matches what Detect actually
// returns, so every verdict class scores a clean 1/1/1 and contradiction
// recall is a clean 1/1/1 too.
func TestScorePerfectRowsYieldAllTruePositivesAndFullContradictionRecall(t *testing.T) {
	rows := []Row{
		newClaimRow("R-conflict", "conflict", "$700", "$500"),
		newClaimRow("R-agree", "agree", "$500", "$500"),
	}
	report, err := Score(rows)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	want := eval.PRF1{TP: 1, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if got := report.VerdictConfusion["conflict"]; got != want {
		t.Errorf("VerdictConfusion[conflict] = %+v, want %+v", got, want)
	}
	if got := report.VerdictConfusion["agree"]; got != want {
		t.Errorf("VerdictConfusion[agree] = %+v, want %+v", got, want)
	}

	wantContradiction := eval.PRF1{TP: 1, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if report.Contradiction != wantContradiction {
		t.Errorf("Contradiction = %+v, want %+v", report.Contradiction, wantContradiction)
	}
	if report.RowsScored != 2 {
		t.Errorf("RowsScored = %d, want 2", report.RowsScored)
	}
	if report.Matrix["conflict"]["conflict"] != 1 {
		t.Errorf("Matrix[conflict][conflict] = %d, want 1", report.Matrix["conflict"]["conflict"])
	}
}

// TestScoreMisclassifiedRowAppearsAsFalseNegative pins the exact shape
// T2.18's acc line names: "a deliberately missed temporal fixture appears
// as a false negative." A row genuinely expects window_close (two claims
// with distinct, determinable validity windows) but is deliberately
// constructed so the real production detectPair/parseValidFrom cannot
// parse ValidFrom -- the row falls through to VerdictConflict instead,
// which must show up as report.VerdictConfusion["window_close"].FN == 1
// (a real classification miss, not swept under an inflated pass rate) and
// as a contradiction-recall true positive still (conflict is also
// contradiction-shaped) -- this test isolates the verdict-confusion
// consequence specifically, independent of any specific corpus fixture.
//
// Uses a garbage date string, not RFC3339: T2.18 originally used an
// RFC3339 ValidFrom here because that was, at the time, a real gap
// parseValidFrom rejected (see git history / docs/roadmap.md's T2.18
// entry). T2.23 closed that gap -- parseValidFrom now accepts RFC3339
// (validDateLayouts) -- so this test was updated to a format neither
// layout parses, to keep testing Score's FN-detection machinery itself
// rather than accidentally re-testing (and now contradicting) the fixed
// bug.
func TestScoreMisclassifiedRowAppearsAsFalseNegative(t *testing.T) {
	rows := []Row{
		{
			ID: "R-unparseable-window",
			NewClaim: ClaimFixture{
				ID: "claim-a", Subject: "alice-tan", Predicate: "works_at", Object: "acme-corp",
				ValidFrom: "06/01/2025", // garbage format -- neither validDateLayout nor RFC3339 -- parseValidFrom rejects this
			},
			Active: []ClaimFixture{{
				ID: "claim-b", Subject: "alice-tan", Predicate: "works_at", Object: "initech",
				ValidFrom: "01/01/2023",
			}},
			ExpectedVerdict: "window_close",
		},
	}
	report, err := Score(rows)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	got := report.VerdictConfusion["window_close"]
	if got.FN != 1 {
		t.Fatalf("VerdictConfusion[window_close].FN = %d, want 1 (this row's expected window_close verdict must be missed given the unparseable date format)", got.FN)
	}
	if got.TP != 0 {
		t.Fatalf("VerdictConfusion[window_close].TP = %d, want 0", got.TP)
	}
	if gotConflict := report.VerdictConfusion["conflict"]; gotConflict.FP != 1 {
		t.Fatalf("VerdictConfusion[conflict].FP = %d, want 1 (Detect actually returned conflict)", gotConflict.FP)
	}
	if report.Matrix["window_close"]["conflict"] != 1 {
		t.Fatalf("Matrix[window_close][conflict] = %d, want 1", report.Matrix["window_close"]["conflict"])
	}
}

// TestScoreCommittedCorpusHasReconcileSectionAndNoFalseNegatives is
// T2.18's acc line run against the actual shipped corpus (not a synthetic
// fixture): "report has a reconcile section with per-verdict P/R/F1."
// Loads evals/corpora/reconcile/labels/ for real and confirms every row,
// including R-014, now scores correctly.
//
// R-014 (RFC3339 valid_from) was deliberately included by T2.18 as a real,
// disclosed false negative: parseValidFrom rejected RFC3339 outright, so
// Detect fell through to VerdictConflict instead of the temporally-correct
// VerdictWindowClose. T2.23 fixed parseValidFrom to accept RFC3339
// (validDateLayouts) -- this test was updated accordingly, per T2.23's own
// acc line ("T2.18's eval-runner re-run shows window_close.FN == 0"), and
// now guards the fix as a regression test: R-014 must stay a true
// positive, not silently regress back to a false negative.
func TestScoreCommittedCorpusHasReconcileSectionAndNoFalseNegatives(t *testing.T) {
	rows, err := LoadRows(filepath.Join(corpusDir(t), "labels"))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	report, err := Score(rows)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if report.RowsScored != len(rows) {
		t.Fatalf("RowsScored = %d, want %d", report.RowsScored, len(rows))
	}
	if len(report.VerdictConfusion) == 0 {
		t.Fatal("VerdictConfusion is empty, want per-verdict P/R/F1 for the corpus's verdict classes")
	}

	wc := report.VerdictConfusion["window_close"]
	if wc.FN != 0 {
		t.Fatalf("VerdictConfusion[window_close].FN = %d, want 0 -- R-014's RFC3339 valid_from must parse correctly post-T2.23, not regress to a false negative", wc.FN)
	}
	if report.Matrix["window_close"]["conflict"] != 0 {
		t.Fatalf("Matrix[window_close][conflict] = %d, want 0 -- no window_close row should be confused as conflict", report.Matrix["window_close"]["conflict"])
	}
}

func TestScoreEmptyExpectedVerdictErrors(t *testing.T) {
	rows := []Row{{ID: "R-bad", NewClaim: ClaimFixture{ID: "a", Subject: "x", Predicate: "y", Object: "z"}}}
	if _, err := Score(rows); err == nil {
		t.Fatal("Score with empty ExpectedVerdict returned nil error, want an error")
	}
}

// TestScoreNonContradictionRowFalselyFlaggedCountsAsContradictionFalsePositive
// covers the other direction from the temporal-miss test above: a row
// whose golden verdict is NOT contradiction-shaped (scoped) but where
// Detect is deliberately fed fixtures that instead land on conflict --
// this must count against Contradiction's false-positive rate, not be
// silently absorbed into only the per-verdict confusion counts.
func TestScoreNonContradictionRowFalselyFlaggedCountsAsContradictionFalsePositive(t *testing.T) {
	rows := []Row{
		newClaimRow("R-should-be-scoped-but-conflicts", "scoped", "$700", "$500"),
	}
	report, err := Score(rows)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if report.Contradiction.FP != 1 {
		t.Fatalf("Contradiction.FP = %d, want 1", report.Contradiction.FP)
	}
}
