package ladder

import (
	"fmt"
	"sort"
	"time"
)

// Grid is the T2.11 calibration sweep's parameter search space (RFC 0001
// §10.3: "the numbers above are priors, replaced by evidence before
// launch"). CorrelationGuards is deliberately NOT part of this grid: T2.10
// already covers its own mechanics and tests, and every Scenario below is
// built with fixed guard-relevant fields (span, distinct sources) chosen
// specifically to pass or fail sweepGuards independent of which grid point
// is under test -- so the sweep isolates exactly the three dimensions
// T2.11's acc line names: min_dispositions, min_accept, sample_rate.
type Grid struct {
	MinDispositions []int     `json:"min_dispositions"`
	MinAccept       []float64 `json:"min_accept"`
	SampleRate      []float64 `json:"sample_rate"`
}

// DefaultGrid is the search space evals/calibration/report.json (this
// sweep's checked-in output) was generated from -- exported so
// evals/calibration/gen_calibration.go and calibrate_test.go both sweep
// exactly the same space the committed report used.
func DefaultGrid() Grid {
	return Grid{
		MinDispositions: []int{15, 20, 30, 50, 75, 100},
		MinAccept:       []float64{0.90, 0.93, 0.95, 0.97, 0.98, 0.99},
		SampleRate:      []float64{0.02, 0.05, 0.10, 0.15, 0.20},
	}
}

// sweepGuards are the fixed correlation guards every Scenario is evaluated
// under -- the RFC §10.3 prior, unchanged by this sweep (T2.10's own scope
// covers calibrating these, not T2.11's).
var sweepGuards = CorrelationGuards{MinSpanDays: 14, MinDistinctSources: 5}

// sweepNeverAutomate mirrors DefaultConfig's never_automate seed list --
// fixed, like sweepGuards, so the never_automate_family scenario below
// actually exercises the NeverAutomate override rather than silently
// falling through to an empty list, which would make every grid point
// fail that scenario (NeverAutomate is a distinct backstop from the
// min_dispositions/min_accept/sample_rate grid this sweep searches; T2.11's
// acc line does not ask this sweep to calibrate the seed list itself).
var sweepNeverAutomate = DefaultConfig().NeverAutomate

// Scenario is one named synthetic cell this sweep grades every grid point
// against. RFC §10.3 provides no real production disposition history to
// calibrate against -- E2 has not shipped, and no cell has ever actually
// earned automation yet. These are disclosed, hand-constructed archetypes
// with a judgment-call WantPromote ground truth stated in Why, not a
// measured real-world outcome. A grid point is a correctness candidate
// only if Evaluate agrees with WantPromote on every scenario.
type Scenario struct {
	Name        string
	Cell        string
	History     []Disposition
	WantPromote bool
	Why         string
}

// syntheticHistory builds n Disposition rows: the first n*acceptRate
// (rounded down) accepted, the rest rejected, spread evenly across
// spanDays starting at start, cycling through distinctSources source ids.
// Deterministic (no RNG) so the sweep and its report are exactly
// reproducible run to run.
func syntheticHistory(n int, acceptRate float64, spanDays, distinctSources int, start time.Time) []Disposition {
	hist := make([]Disposition, n)
	accepted := int(float64(n)*acceptRate + 0.5) // round to nearest
	var step time.Duration
	if n > 1 {
		step = time.Duration(spanDays) * 24 * time.Hour / time.Duration(n-1)
	}
	for i := 0; i < n; i++ {
		hist[i] = Disposition{
			Accepted:   i < accepted,
			OccurredAt: start.Add(time.Duration(i) * step),
			SourceID:   fmt.Sprintf("source-%d", i%distinctSources),
		}
	}
	return hist
}

// DefaultScenarios is the synthetic battery evals/calibration/report.json
// was generated from. See each Scenario's Why for its rationale.
func DefaultScenarios() []Scenario {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []Scenario{
		{
			Name:        "reliable_high_volume",
			Cell:        "email/has_role",
			History:     syntheticHistory(200, 0.995, 60, 25, start),
			WantPromote: true,
			Why:         "200 dispositions, 99.5% accept, spanning 60 days across 25 sources: abundant, diverse, high-quality evidence -- should promote under any reasonable threshold.",
		},
		{
			Name:        "thin_evidence_looks_perfect",
			Cell:        "email/has_balance",
			History:     syntheticHistory(25, 1.0, 45, 15, start),
			WantPromote: false,
			Why:         "25 dispositions, 100% accept: a perfect small sample is exactly the shape that fools a low min_dispositions bar -- a short window is more sensitive to a single lucky streak than a longer one, so this must not promote even at a flawless observed rate. Eliminates any min_dispositions candidate <= 25.",
		},
		{
			Name:        "exactly_the_rfc_prior_bar",
			Cell:        "email/works_at",
			History:     syntheticHistory(50, 0.98, 20, 8, start),
			WantPromote: true,
			Why:         "Exactly RFC 0001 §10.3's stated prior bar (50 dispositions, 98% accept): a cell meeting the bar precisely must actually clear it, not be silently rejected by an off-by-one in the comparison. Confirms min_dispositions <= 50 and min_accept <= 0.98 are viable.",
		},
		{
			Name:        "noisy_moderate_volume",
			Cell:        "email/lives_in",
			History:     syntheticHistory(150, 0.92, 40, 18, start),
			WantPromote: false,
			Why:         "150 dispositions but only 92% accept (an 8% error rate): too risky to fully automate regardless of volume -- humans should keep reviewing this cell. Eliminates any min_accept candidate <= 0.92.",
		},
		{
			Name:        "correlated_error_risk",
			Cell:        "email/has_title",
			History:     syntheticHistory(100, 0.99, 5, 2, start),
			WantPromote: false,
			Why:         "100 dispositions at 99% accept looks excellent in isolation, but all from a 5-day window across only 2 sources -- exactly the correlated-error shape RFC §10.3's mandatory guards exist to catch (one bad connector day or one labeling mood). Validates that sweepGuards (min_span_days=14, min_distinct_sources=5) block this regardless of the swept min_dispositions/min_accept grid point; does not itself discriminate among grid candidates.",
		},
		{
			Name:        "never_automate_family",
			Cell:        "precept-touching",
			History:     syntheticHistory(300, 1.0, 90, 30, start),
			WantPromote: false,
			Why:         "Even a flawless, abundant, diverse history must never promote a never_automate cell (precept-touching conflicts are structurally unsafe to auto-resolve). Validates NeverAutomate overrides any grid point; does not itself discriminate among grid candidates.",
		},
	}
}

// GridPoint is one (min_dispositions, min_accept, sample_rate) candidate
// under evaluation.
type GridPoint struct {
	MinDispositions int     `json:"min_dispositions"`
	MinAccept       float64 `json:"min_accept"`
	SampleRate      float64 `json:"sample_rate"`
}

func (g GridPoint) policy() CellPolicy {
	return CellPolicy(g)
}

// ScenarioResult is one scenario's outcome under one grid point.
type ScenarioResult struct {
	Scenario string `json:"scenario"`
	Promoted bool   `json:"promoted"`
	Correct  bool   `json:"correct"`
}

// CandidateResult is one grid point's full sweep outcome.
type CandidateResult struct {
	GridPoint
	AllCorrect bool             `json:"all_correct"`
	Scenarios  []ScenarioResult `json:"scenarios"`
}

// ScenarioSummary is a Scenario's report-facing metadata (its synthetic
// History is an implementation detail of the sweep, not reported).
type ScenarioSummary struct {
	Name        string `json:"name"`
	Cell        string `json:"cell"`
	WantPromote bool   `json:"want_promote"`
	Why         string `json:"why"`
}

// Report is the sweep's complete, JSON-serializable output --
// evals/calibration/report.json.
type Report struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Grid        Grid              `json:"grid"`
	Guards      CorrelationGuards `json:"correlation_guards"`
	Scenarios   []ScenarioSummary `json:"scenarios"`
	Candidates  []CandidateResult `json:"candidates"`
	Chosen      GridPoint         `json:"chosen"`
	Rationale   string            `json:"rationale"`
}

// Sweep evaluates every point in grid against every scenario using guards
// as the fixed correlation-guard config (never itself swept -- see the
// package doc above), and returns one CandidateResult per grid point. It
// does not choose defaults -- Calibrate does that on top of Sweep's
// output, so a caller wanting only the raw grid results (e.g. a re-sweep
// against a revised scenario battery) can call Sweep directly.
func Sweep(grid Grid, guards CorrelationGuards, scenarios []Scenario) []CandidateResult {
	out := make([]CandidateResult, 0, len(grid.MinDispositions)*len(grid.MinAccept)*len(grid.SampleRate))
	for _, md := range grid.MinDispositions {
		for _, ma := range grid.MinAccept {
			for _, sr := range grid.SampleRate {
				gp := GridPoint{MinDispositions: md, MinAccept: ma, SampleRate: sr}
				cfg := &Config{Default: gp.policy(), CorrelationGuards: guards, NeverAutomate: sweepNeverAutomate}
				eng := NewEngine(cfg)

				allCorrect := true
				results := make([]ScenarioResult, 0, len(scenarios))
				for _, sc := range scenarios {
					promoted := eng.Evaluate(sc.Cell, sc.History)
					correct := promoted == sc.WantPromote
					if !correct {
						allCorrect = false
					}
					results = append(results, ScenarioResult{Scenario: sc.Name, Promoted: promoted, Correct: correct})
				}
				out = append(out, CandidateResult{GridPoint: gp, AllCorrect: allCorrect, Scenarios: results})
			}
		}
	}
	return out
}

// ErrNoCorrectCandidate is returned by Calibrate when no grid point
// satisfies every scenario's WantPromote -- the scenario battery or grid
// needs revision, not something a caller should silently paper over.
var ErrNoCorrectCandidate = fmt.Errorf("ladder: calibrate: no grid point satisfies every scenario")

// Calibrate runs Sweep and selects the chosen defaults from the grid
// points where AllCorrect (every scenario's WantPromote is matched):
//
//   - If prior (the RFC §10.3 shipped values) is itself among the correct
//     set, Calibrate chooses it, unchanged. This sweep's synthetic
//     scenario battery is disclosed evidence this session authored, not
//     real field evidence -- E2 has not shipped, so no cell has ever
//     actually earned automation. Where the prior already checks out
//     against the battery, this deliberately does not use a synthetic
//     battery it wrote itself to justify loosening a bar the RFC council
//     already set.
//   - Otherwise, Calibrate falls back to the most-permissive correct
//     candidate (lowest min_dispositions, then lowest min_accept) --
//     earning automation faster is the actual product goal (RFC §3:
//     "hygiene must earn automation"), and NeverAutomate/correlation
//     guards/mandatory audit-sampling remain independent backstops
//     regardless of this choice.
//
// sample_rate never affects Evaluate's promote/no-promote outcome (see
// AutoAction.Sampled's doc: it only selects an already-Trust1 cell's
// actions for human audit) -- this synthetic correctness sweep has no
// mechanism to discriminate among sample_rate candidates at all, so
// Calibrate always keeps prior.SampleRate, disclosed here rather than
// silently arbitrary.
func Calibrate(grid Grid, guards CorrelationGuards, scenarios []Scenario, prior GridPoint, now time.Time) (Report, error) {
	candidates := Sweep(grid, guards, scenarios)

	correct := make([]CandidateResult, 0, len(candidates))
	for _, c := range candidates {
		if c.AllCorrect {
			correct = append(correct, c)
		}
	}
	if len(correct) == 0 {
		return Report{}, ErrNoCorrectCandidate
	}

	var chosen GridPoint
	var rationale string
	priorIsCorrect := false
	for _, c := range correct {
		if c.MinDispositions == prior.MinDispositions && c.MinAccept == prior.MinAccept {
			priorIsCorrect = true
			break
		}
	}
	if priorIsCorrect {
		chosen = GridPoint{MinDispositions: prior.MinDispositions, MinAccept: prior.MinAccept, SampleRate: prior.SampleRate}
		rationale = fmt.Sprintf(
			"the RFC 0001 §10.3 prior (min_dispositions=%d, min_accept=%.2f, sample_rate=%.2f) is among the grid points that correctly classify every synthetic scenario below; this sweep found no evidence to loosen it, and -- with no real production disposition history yet (E2 has not shipped) -- deliberately does not use a synthetic scenario battery this session authored to justify moving away from a value the RFC council already considered. sample_rate does not affect Evaluate's promote/no-promote outcome at all (see AutoAction.Sampled's doc); it is retained at the prior's value here, not independently discriminated by this correctness sweep.",
			prior.MinDispositions, prior.MinAccept, prior.SampleRate,
		)
	} else {
		sort.Slice(correct, func(i, k int) bool {
			if correct[i].MinDispositions != correct[k].MinDispositions {
				return correct[i].MinDispositions < correct[k].MinDispositions
			}
			return correct[i].MinAccept < correct[k].MinAccept
		})
		best := correct[0]
		chosen = GridPoint{MinDispositions: best.MinDispositions, MinAccept: best.MinAccept, SampleRate: prior.SampleRate}
		rationale = fmt.Sprintf(
			"the RFC 0001 §10.3 prior (min_dispositions=%d, min_accept=%.2f) does NOT satisfy every scenario in this sweep, so the most-permissive grid point that does was chosen instead: min_dispositions=%d, min_accept=%.2f (earning automation faster serves RFC §3's \"hygiene must earn automation\" while NeverAutomate, correlation guards, and mandatory audit sampling remain independent backstops). sample_rate is retained at the prior's value (%.2f) -- this correctness sweep has no mechanism to discriminate among sample_rate candidates (see AutoAction.Sampled's doc).",
			prior.MinDispositions, prior.MinAccept, chosen.MinDispositions, chosen.MinAccept, prior.SampleRate,
		)
	}

	summaries := make([]ScenarioSummary, 0, len(scenarios))
	for _, sc := range scenarios {
		summaries = append(summaries, ScenarioSummary{Name: sc.Name, Cell: sc.Cell, WantPromote: sc.WantPromote, Why: sc.Why})
	}

	return Report{
		GeneratedAt: now.UTC(),
		Grid:        grid,
		Guards:      guards,
		Scenarios:   summaries,
		Candidates:  candidates,
		Chosen:      chosen,
		Rationale:   rationale,
	}, nil
}
