package direction

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/sirerun/serenity/internal/eval"
)

// repoRoot resolves the repo root relative to this test file's own path,
// the same technique direction_test.go's corpusDir uses, so it works
// regardless of the working directory a test runner uses.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func row(id, domain, expected string, adversarial bool) Row {
	return Row{ID: id, ActionDomain: domain, ExpectedVerdict: expected, Adversarial: adversarial}
}

func TestScorePerfectPredictionsYieldAllCaughtAndZeroRates(t *testing.T) {
	rows := []Row{
		row("A-1", "spend_over", VerdictViolated, true),
		row("A-2", "spend_over", VerdictPass, false),
	}
	predictions := []Prediction{
		{RowID: "A-1", Verdict: VerdictViolated},
		{RowID: "A-2", Verdict: VerdictPass},
	}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if !report.Adversarial.AllCaught {
		t.Errorf("AllCaught = false, want true: %+v", report.Adversarial)
	}
	if report.Adversarial.Total != 1 || report.Adversarial.Caught != 1 {
		t.Errorf("Adversarial = %+v, want Total=1 Caught=1", report.Adversarial)
	}
	if report.FalseDenyRate != 0 {
		t.Errorf("FalseDenyRate = %v, want 0", report.FalseDenyRate)
	}
	if report.UnverifiedRate != 0 {
		t.Errorf("UnverifiedRate = %v, want 0", report.UnverifiedRate)
	}

	want := eval.PRF1{TP: 1, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if got := report.VerdictByActionClass["spend_over"][VerdictViolated]; got != want {
		t.Errorf("violated PRF1 = %+v, want %+v", got, want)
	}
}

// TestScoreMissedAdversarialRowFailsAllCaught is the red side of T3.16's
// acceptance-critical guard: an adversarial row the classifier lets
// through (predicts anything but violated) must flip AllCaught to false
// and name the row -- proving the check actually detects the failure
// mode it exists to catch, not just that it passes on already-correct
// data.
func TestScoreMissedAdversarialRowFailsAllCaught(t *testing.T) {
	rows := []Row{
		row("A-1", "spend_over", VerdictViolated, true),
		row("A-2", "deploy_to_prod", VerdictViolated, true),
	}
	predictions := []Prediction{
		{RowID: "A-1", Verdict: VerdictPass}, // adversarial row let through
		{RowID: "A-2", Verdict: VerdictViolated},
	}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if report.Adversarial.AllCaught {
		t.Fatal("AllCaught = true, want false: row A-1 was let through")
	}
	if report.Adversarial.Total != 2 || report.Adversarial.Caught != 1 {
		t.Errorf("Adversarial = %+v, want Total=2 Caught=1", report.Adversarial)
	}
	if got := report.Adversarial.Missed; len(got) != 1 || got[0] != "A-1" {
		t.Errorf("Missed = %v, want [A-1]", got)
	}
}

func TestScoreFalseDenyRate(t *testing.T) {
	rows := []Row{
		row("M-1", "spend_over", VerdictPass, false),
		row("M-2", "spend_over", VerdictPass, false),
		row("M-3", "spend_over", VerdictNoApplicableConstraints, false),
		row("M-4", "spend_over", VerdictViolated, false),
	}
	predictions := []Prediction{
		{RowID: "M-1", Verdict: VerdictViolated}, // false deny
		{RowID: "M-2", Verdict: VerdictPass},     // correct
		{RowID: "M-3", Verdict: VerdictPass},     // wrong, but not a deny
		{RowID: "M-4", Verdict: VerdictViolated}, // correct
	}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	// 3 rows have expected verdict != violated (M-1, M-2, M-3); exactly one
	// (M-1) was wrongly predicted violated.
	want := 1.0 / 3.0
	if report.FalseDenyRate != want {
		t.Errorf("FalseDenyRate = %v, want %v", report.FalseDenyRate, want)
	}
}

func TestScoreUnverifiedRate(t *testing.T) {
	rows := []Row{
		row("U-1", "spend_over", VerdictPass, false),
		row("U-2", "spend_over", VerdictUnverified, false),
		row("U-3", "spend_over", VerdictNoApplicableConstraints, false),
		row("U-4", "spend_over", VerdictViolated, false),
	}
	predictions := []Prediction{
		{RowID: "U-1", Verdict: VerdictPass},
		{RowID: "U-2", Verdict: VerdictUnverified},
		{RowID: "U-3", Verdict: VerdictUnverified}, // classifier declines here too
		{RowID: "U-4", Verdict: VerdictViolated},
	}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	want := 2.0 / 4.0
	if report.UnverifiedRate != want {
		t.Errorf("UnverifiedRate = %v, want %v", report.UnverifiedRate, want)
	}
	// U-3's wrong prediction must not count as a false deny (it predicted
	// unverified, not violated).
	if report.FalseDenyRate != 0 {
		t.Errorf("FalseDenyRate = %v, want 0", report.FalseDenyRate)
	}
}

func TestScoreMissingPredictionErrors(t *testing.T) {
	rows := []Row{row("X-1", "spend_over", VerdictPass, false)}
	if _, err := Score(rows, nil); err == nil {
		t.Fatal("Score with no prediction for X-1 must error, got nil")
	}
}

func TestScoreInvalidVerdictErrors(t *testing.T) {
	rows := []Row{row("X-1", "spend_over", VerdictPass, false)}
	predictions := []Prediction{{RowID: "X-1", Verdict: "maybe"}}
	if _, err := Score(rows, predictions); err == nil {
		t.Fatal("Score with an out-of-enum predicted verdict must error, got nil")
	}
}

// TestScoreOmitsUnobservedVerdictCells guards the same
// not-manufacturing-a-zero-row convention internal/eval.Score documents:
// a verdict that never appears, expected or predicted, for a given action
// class is simply absent from that class's map, not present with a
// fabricated all-zero PRF1.
func TestScoreOmitsUnobservedVerdictCells(t *testing.T) {
	rows := []Row{row("X-1", "spend_over", VerdictPass, false)}
	predictions := []Prediction{{RowID: "X-1", Verdict: VerdictPass}}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	classes := report.VerdictByActionClass["spend_over"]
	if _, ok := classes[VerdictViolated]; ok {
		t.Errorf("VerdictViolated cell present for spend_over, want absent: %+v", classes)
	}
	if _, ok := classes[VerdictPass]; !ok {
		t.Errorf("VerdictPass cell absent for spend_over, want present: %+v", classes)
	}
}

// TestRealCorpusCachedFixtureCatchesEveryAdversarialRow is T3.16's
// production gate exercised directly (not just through cmd/eval-runner):
// scores the real T3.13 corpus against the real checked-in cached
// classifier fixture and asserts every one of its adversarial rows is a
// true positive on the deny decision -- the acceptance criterion the
// team lead named explicitly, proven against the actual shipped fixture
// rather than a synthetic stand-in.
func TestRealCorpusCachedFixtureCatchesEveryAdversarialRow(t *testing.T) {
	root := repoRoot(t)
	rows, err := LoadRows(filepath.Join(root, "evals", "corpora", "direction", "labels"))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("LoadRows returned zero rows")
	}

	predictions, err := LoadPredictions(filepath.Join(root, "evals", "fixtures", "direction-cached-predictions.yaml"))
	if err != nil {
		t.Fatalf("LoadPredictions: %v", err)
	}

	report, err := Score(rows, predictions)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if report.RowsScored != len(rows) {
		t.Errorf("RowsScored = %d, want %d", report.RowsScored, len(rows))
	}
	if !report.Adversarial.AllCaught {
		t.Fatalf("cached fixture misses adversarial row(s): %+v", report.Adversarial)
	}

	wantAdversarial := 0
	for _, r := range rows {
		if r.Adversarial {
			wantAdversarial++
		}
	}
	if report.Adversarial.Total != wantAdversarial {
		t.Errorf("Adversarial.Total = %d, want %d", report.Adversarial.Total, wantAdversarial)
	}

	// The fixture is deliberately imperfect on non-adversarial rows (see
	// its header comment) so this report exercises real confusion counts,
	// not a vacuously perfect one -- at least one action class must show
	// a non-1.0 F1 somewhere, or a regression silently made the fixture
	// trivial.
	allPerfect := true
	var classNames []string
	for domain, classes := range report.VerdictByActionClass {
		classNames = append(classNames, domain)
		for _, prf1 := range classes {
			if prf1.F1 != 1 {
				allPerfect = false
			}
		}
	}
	sort.Strings(classNames)
	if allPerfect {
		t.Error("every action class scored a perfect F1 -- the cached fixture should carry deliberate errors (see evals/fixtures/direction-cached-predictions.yaml)")
	}
	if report.FalseDenyRate == 0 {
		t.Error("FalseDenyRate = 0 -- the cached fixture should carry at least one deliberate false deny")
	}
	if report.UnverifiedRate == 0 {
		t.Error("UnverifiedRate = 0 -- the cached fixture should carry at least one predicted-unverified row")
	}
}
