package ladder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// repoRoot resolves the repo root relative to this test file's own path,
// so evals/calibration/report.json resolves regardless of the working
// directory a test runner uses. internal/ladder is one directory
// shallower than internal/eval/runner (whose repoRoot helper this
// mirrors), hence two "..", not three.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func rfcPrior() GridPoint {
	return GridPoint(DefaultConfig().Default)
}

func TestSweepEveryScenarioMatchesItsWantPromoteAtTheRFCPrior(t *testing.T) {
	prior := rfcPrior()
	cfg := &Config{Default: prior.policy(), CorrelationGuards: sweepGuards, NeverAutomate: sweepNeverAutomate}
	eng := NewEngine(cfg)

	for _, sc := range DefaultScenarios() {
		got := eng.Evaluate(sc.Cell, sc.History)
		if got != sc.WantPromote {
			t.Errorf("scenario %q at RFC prior (min_dispositions=%d, min_accept=%.2f): Evaluate = %v, want %v (%s)",
				sc.Name, prior.MinDispositions, prior.MinAccept, got, sc.WantPromote, sc.Why)
		}
	}
}

func TestSweepFindsRFCPriorAmongCorrectCandidates(t *testing.T) {
	prior := rfcPrior()
	candidates := Sweep(DefaultGrid(), sweepGuards, DefaultScenarios())

	found := false
	for _, c := range candidates {
		if c.MinDispositions == prior.MinDispositions && c.MinAccept == prior.MinAccept && c.SampleRate == prior.SampleRate {
			found = true
			if !c.AllCorrect {
				t.Fatalf("RFC prior grid point is present but AllCorrect=false: %+v", c.Scenarios)
			}
		}
	}
	if !found {
		t.Fatal("RFC prior (min_dispositions=50, min_accept=0.98, sample_rate=0.05) is not itself a point in DefaultGrid() -- the grid must include the shipped prior so Calibrate can affirm or reject it")
	}
}

func TestSweepThinEvidenceEliminatesLowMinDispositions(t *testing.T) {
	var thin Scenario
	for _, sc := range DefaultScenarios() {
		if sc.Name == "thin_evidence_looks_perfect" {
			thin = sc
		}
	}
	if thin.Name == "" {
		t.Fatal("thin_evidence_looks_perfect scenario not found")
	}

	cfg := &Config{
		Default:           CellPolicy{MinDispositions: 20, MinAccept: 0.90, SampleRate: 0.05},
		CorrelationGuards: sweepGuards,
		NeverAutomate:     sweepNeverAutomate,
	}
	eng := NewEngine(cfg)
	if got := eng.Evaluate(thin.Cell, thin.History); got != true {
		t.Fatalf("sanity check: expected a permissive min_dispositions=20 policy to promote the 25-disposition thin-evidence scenario, got %v", got)
	}
	// thin.WantPromote is false -- so this permissive candidate is
	// correctly excluded from the AllCorrect set by Sweep.
	if thin.WantPromote {
		t.Fatal("test assumes thin_evidence_looks_perfect.WantPromote is false")
	}
}

func TestCalibrateKeepsRFCPriorWhenItSatisfiesEveryScenario(t *testing.T) {
	prior := rfcPrior()
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)

	report, err := Calibrate(DefaultGrid(), sweepGuards, DefaultScenarios(), prior, now)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if report.Chosen != prior {
		t.Fatalf("Calibrate chose %+v, want the RFC prior %+v unchanged -- the prior satisfies every scenario in DefaultScenarios(), so calibration should affirm it rather than substitute a different grid point", report.Chosen, prior)
	}
	if report.Rationale == "" {
		t.Fatal("Report.Rationale is empty")
	}
	if len(report.Scenarios) != len(DefaultScenarios()) {
		t.Fatalf("Report.Scenarios has %d entries, want %d", len(report.Scenarios), len(DefaultScenarios()))
	}
	for _, s := range report.Scenarios {
		if s.Why == "" {
			t.Fatalf("scenario %q has no rationale (Why)", s.Name)
		}
	}
}

func TestCalibrateFallsBackToMostPermissiveWhenPriorFails(t *testing.T) {
	// A deliberately-wrong "prior" (way outside the grid, and not itself a
	// correct candidate) exercises the fallback branch: Calibrate must
	// choose the most-permissive AllCorrect grid point instead of the
	// (rejected) prior.
	badPrior := GridPoint{MinDispositions: 9999, MinAccept: 0.5, SampleRate: 0.05}
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)

	report, err := Calibrate(DefaultGrid(), sweepGuards, DefaultScenarios(), badPrior, now)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if report.Chosen == badPrior {
		t.Fatal("Calibrate chose the bad prior verbatim -- it is not in DefaultGrid() and cannot itself be a swept candidate")
	}

	// The chosen point must itself be AllCorrect, and must be the
	// lexicographically smallest (min_dispositions asc, then min_accept
	// asc) among the AllCorrect set.
	var chosenCandidate *CandidateResult
	smallest := true
	for i := range report.Candidates {
		c := &report.Candidates[i]
		if !c.AllCorrect {
			continue
		}
		if c.MinDispositions == report.Chosen.MinDispositions && c.MinAccept == report.Chosen.MinAccept {
			chosenCandidate = c
		}
		if c.MinDispositions < report.Chosen.MinDispositions ||
			(c.MinDispositions == report.Chosen.MinDispositions && c.MinAccept < report.Chosen.MinAccept) {
			smallest = false
		}
	}
	if chosenCandidate == nil {
		t.Fatalf("Chosen grid point %+v is not itself an AllCorrect candidate", report.Chosen)
	}
	if !smallest {
		t.Fatalf("Chosen grid point %+v is not the most-permissive AllCorrect candidate", report.Chosen)
	}
	// sample_rate is not discriminated by the correctness sweep -- Calibrate
	// must still retain the (bad) prior's sample_rate rather than inventing
	// one, per Calibrate's documented behavior.
	if report.Chosen.SampleRate != badPrior.SampleRate {
		t.Fatalf("Chosen.SampleRate = %v, want the prior's sample_rate %v retained", report.Chosen.SampleRate, badPrior.SampleRate)
	}
}

func TestCalibrateErrorsWhenNoGridPointSatisfiesEveryScenario(t *testing.T) {
	impossible := Scenario{
		Name:        "impossible",
		Cell:        "email/anything",
		History:     syntheticHistory(1, 1.0, 60, 10, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		WantPromote: true, // 1 disposition can never satisfy any grid point's min_dispositions (smallest is 15)
	}
	_, err := Calibrate(DefaultGrid(), sweepGuards, []Scenario{impossible}, rfcPrior(), time.Now())
	if err != ErrNoCorrectCandidate {
		t.Fatalf("Calibrate error = %v, want ErrNoCorrectCandidate", err)
	}
}

func TestSyntheticHistoryProducesRequestedAcceptRateAndSourceSpread(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hist := syntheticHistory(50, 0.98, 20, 8, start)
	if len(hist) != 50 {
		t.Fatalf("len = %d, want 50", len(hist))
	}
	accepted := 0
	sources := make(map[string]bool)
	for _, d := range hist {
		if d.Accepted {
			accepted++
		}
		sources[d.SourceID] = true
	}
	if accepted != 49 {
		t.Fatalf("accepted = %d, want 49 (50 * 0.98)", accepted)
	}
	if len(sources) != 8 {
		t.Fatalf("distinct sources = %d, want 8", len(sources))
	}
	span := hist[len(hist)-1].OccurredAt.Sub(hist[0].OccurredAt)
	if span > 20*24*time.Hour || span < 19*24*time.Hour {
		t.Fatalf("span = %v, want ~20 days", span)
	}
}

// TestDefaultConfigMatchesCommittedCalibrationReport is T2.11's acc-line
// test: "a test asserts config.Default ladder values equal the report's
// chosen values." It reads the checked-in
// evals/calibration/report.json (generated by
// evals/calibration/gen_calibration.go) rather than regenerating it, so a
// change to DefaultConfig with no matching regeneration of the report
// fails CI loudly instead of silently drifting.
func TestDefaultConfigMatchesCommittedCalibrationReport(t *testing.T) {
	path := filepath.Join(repoRoot(t), "evals", "calibration", "report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run evals/calibration/gen_calibration.go to generate it)", path, err)
	}

	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}

	def := DefaultConfig().Default
	if report.Chosen.MinDispositions != def.MinDispositions {
		t.Errorf("report.Chosen.MinDispositions = %d, DefaultConfig().Default.MinDispositions = %d", report.Chosen.MinDispositions, def.MinDispositions)
	}
	if report.Chosen.MinAccept != def.MinAccept {
		t.Errorf("report.Chosen.MinAccept = %v, DefaultConfig().Default.MinAccept = %v", report.Chosen.MinAccept, def.MinAccept)
	}
	if report.Chosen.SampleRate != def.SampleRate {
		t.Errorf("report.Chosen.SampleRate = %v, DefaultConfig().Default.SampleRate = %v", report.Chosen.SampleRate, def.SampleRate)
	}
	if report.Rationale == "" {
		t.Error("report.Rationale is empty")
	}
	if len(report.Scenarios) == 0 {
		t.Error("report.Scenarios is empty")
	}
	minMinDispositions := report.Grid.MinDispositions[0]
	for _, md := range report.Grid.MinDispositions {
		if md < minMinDispositions {
			minMinDispositions = md
		}
	}
	if minMinDispositions >= 50 {
		t.Errorf("report.Grid.MinDispositions has no value < 50 (smallest is %d) -- acc line requires cells with < 50 dispositions in the swept grid", minMinDispositions)
	}
}
