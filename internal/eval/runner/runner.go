// Package runner is plan T1.22's eval-workflow engine. It turns a T1.14-
// shaped corpus (labels/, split.yaml, a checksum manifest) plus a source
// of Predictions -- either a frozen fixture (ModeCached, scored on every
// push, zero network calls) or a real internal/extract.Extractor call
// per held-out span (ModeLive, real cost, meant for the nightly scheduled
// workflow only) -- into the evals/report.json shape RFC 0001 SS16/SS17
// asks for: per-family precision/recall/F1 on the corpus's held-out
// split, plus a contradiction-detection section.
//
// Contradiction detection against THIS package's primary corpus (ava, an
// extraction-accuracy corpus of golden (span, predicate, object) facts)
// has no golden claim-pair fixtures to score a detector against, so
// Report.Contradiction always carries a "not_implemented" status rather
// than a fabricated recall number here -- T1.9's acc line deferred
// semantic reconciliation to E2, and even now that E2 has landed a real
// detector (internal/reconcile.Detect, T2.2), nothing in the ava corpus's
// own (span, predicate, object) label shape represents a pair of claims
// that should or shouldn't contradict. Plan T2.18 measures that detector
// for real, but against its own dedicated corpus (evals/corpora/reconcile)
// with its own golden verdict labels -- see Report.Reconcile below, which
// carries a genuine (not placeholder) contradiction-detection recall
// number, populated whenever Config.ReconcileCorpusDir is set.
//
// Plan T3.16 layers a second, independent corpus onto the same Run/Report
// shape: Config.DirectionCorpusDir scores the DIRECTION plan-check corpus
// (T3.13) via internal/eval/direction.Score, attaching the result as
// Report.Direction alongside the primary corpus's Families -- one
// evals/report.json, two corpora, reusing the ModeCached fixture
// convention rather than a second reporting pipeline.
//
// Plan T2.18 layers a third, independent corpus the same way:
// Config.ReconcileCorpusDir scores the reconcile eval corpus
// (evals/corpora/reconcile) via internal/eval/reconcile.Score, attaching
// the result as Report.Reconcile. Unlike Direction, this corpus needs no
// cached predictions fixture and no ModeCached restriction: internal/
// reconcile.Detect is a pure deterministic function (T2.2's own doc), so
// Score calls the real production function directly in every mode.
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/eval"
	"github.com/sirerun/serenity/internal/eval/direction"
	reconcileeval "github.com/sirerun/serenity/internal/eval/reconcile"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/router"
)

// Mode selects where Run's Predictions come from.
type Mode string

const (
	// ModeCached scores a frozen fixture file (Config.FixturePath) --
	// zero network calls, fast and free enough to run on every push
	// (plan T1.22's per-push cached eval gate).
	ModeCached Mode = "cached"
	// ModeLive runs Config.Extractor (a real internal/extract.Extractor
	// over a real internal/router.Router provider) against the corpus's
	// held-out spans -- real cost, meant for the nightly scheduled
	// workflow only.
	ModeLive Mode = "live"
)

// Corpus layout constants, matching evals/corpora/ava's layout
// (evals/corpora/ava/README.md): labels live under labels/, the checksum
// manifest lives one directory above it, and the held-out split file
// sits alongside the manifest. A future second corpus with a different
// layout would need its own Config fields, not a hardcoded assumption
// here.
const (
	labelsSubdir    = "labels"
	manifestSubpath = "checksums.yaml"
	splitSubpath    = "split.yaml"
)

// Config controls one Run.
type Config struct {
	// CorpusDir is a corpus root, e.g. "evals/corpora/ava".
	CorpusDir string
	Mode      Mode

	// FixturePath is required in ModeCached: an eval.LoadPredictions file.
	FixturePath string

	// Extractor and Ledger are required in ModeLive; the caller builds
	// them (wiring a real router.Provider from CLI flags/env, see
	// cmd/eval-runner) since Run itself has no opinion on which provider
	// or model is pinned.
	Extractor *extract.Extractor
	Ledger    *TrackingLedger
	// BudgetUSD is this run's aggregate USD cap, used both as the
	// per-call router.Budget passed to every extraction call (a single
	// held-out span's call should never alone exceed the whole run's
	// intended cap) and, via Ledger.OverBudget, as the run-wide stop
	// condition checked before each new call. <= 0 means unlimited.
	BudgetUSD float64
	// ModelVersion is recorded in the report (ModeLive only).
	ModelVersion string

	// DirectionCorpusDir, when set, additionally scores plan T3.16's
	// DIRECTION corpus (evals/corpora/direction) and attaches the result
	// as Report.Direction -- independent of the primary corpus scored
	// above: a DIRECTION row carries a verdict, not a (predicate, object)
	// fact, so it is scored by internal/eval/direction.Score's own
	// one-verdict-per-row matching rather than eval.Score's set matching,
	// and the corpus has no held-out split (T3.13 built all of its rows
	// as one golden set, not a train/held-out partition). Requires
	// DirectionFixturePath and cfg.Mode == ModeCached: a real production
	// check_plan exists (internal/direction/check, T3.5/T3.6/T3.7), but
	// driving it here would mean adapting this corpus's own applies_when
	// representation (T3.13) into a real ledger.Store per row -- future
	// work, not this section's scope -- so DIRECTION is cached-fixture-only
	// for now.
	DirectionCorpusDir string
	// DirectionFixturePath is a direction.LoadPredictions cached-verdicts
	// fixture, required whenever DirectionCorpusDir is set.
	DirectionFixturePath string

	// ReconcileCorpusDir, when set, additionally scores plan T2.18's
	// reconcile eval corpus (evals/corpora/reconcile) and attaches the
	// result as Report.Reconcile -- independent of the primary corpus and
	// of DirectionCorpusDir. No fixture path is needed (see the package
	// doc): internal/reconcile.Detect is called directly, in every Mode,
	// since it needs no model and produces no cost.
	ReconcileCorpusDir string

	// Now stubs time.Now for deterministic tests; nil means time.Now.
	Now func() time.Time

	// OnProgress, when non-nil, is called after every held-out span
	// ModeLive attempts (scored, skipped, or errored) with the running
	// totals -- done is always len(heldOut) on the final call. A
	// multi-hour live run (T1.32's expanded 312-span held-out corpus
	// runs ~4-5x longer than the original 52-span split) previously gave
	// no signal at all until it either finished or died; a caller (see
	// cmd/eval-runner) wires this to a stderr progress line so a
	// background run's own log file says something before the end. Left
	// nil in every test and in ModeCached (progress reporting is a
	// ModeLive-only, real-network-calls concern).
	OnProgress func(done, total, skipped, errored int)
}

// RecallFloor is T1.32's pass/fail rule (chief's disposition ruling on
// T1.29, statistical floor set by chief-architect): a family counts as
// passing when its bootstrap recall confidence interval's LOWER bound
// clears this floor, not when a single-draw point estimate clears the
// old 0.80 bar. Precision is still reported (Report.Families,
// Report.PrecisionCI) but no longer gates pass/fail under this rule --
// see docs/plans/E1-m1-ingest.md T1.29's disposition note and T1.32.
const RecallFloor = 0.70

// ContradictionSection reports contradiction-detection recall, or
// explains why it isn't reported (see package doc).
type ContradictionSection struct {
	Status string     `json:"status"`
	Result *eval.PRF1 `json:"result,omitempty"`
}

// SpendSection reports ModeLive's aggregate spend against its cap.
type SpendSection struct {
	BudgetUSD       float64 `json:"budget_usd"`
	SpentUSD        float64 `json:"spent_usd"`
	Calls           int     `json:"calls"`
	StoppedOnBudget bool    `json:"stopped_on_budget"`
}

// Report is the evals/report.json shape.
type Report struct {
	GeneratedAt   time.Time             `json:"generated_at"`
	Mode          Mode                  `json:"mode"`
	Corpus        string                `json:"corpus"`
	ModelVersion  string                `json:"model_version,omitempty"`
	Families      map[string]eval.PRF1  `json:"families"`
	Contradiction *ContradictionSection `json:"contradiction"`
	Spend         *SpendSection         `json:"spend,omitempty"`
	SpansScored   int                   `json:"spans_scored"`
	SpansSkipped  int                   `json:"spans_skipped_on_budget,omitempty"`
	// SpansErrored and ErrorSamples are T1.32's live-run resilience
	// fields: a single held-out span whose extraction call fails (even
	// after internal/router's T1.30 bounded retry is exhausted -- e.g. a
	// transient "context deadline exceeded" against the DGX endpoint) no
	// longer aborts the entire run (see runLive's doc comment). The span
	// is excluded from SpansScored the same way a budget-skipped span is
	// (its golden label still counts as a miss in Families/RecallCI --
	// unchanged scoring semantics, just no longer fatal to collect).
	// ErrorSamples carries up to maxErrorSamples "span: error" strings so
	// a report.json from a mostly-broken endpoint is loudly diagnosable
	// rather than silently indistinguishable from genuinely low recall.
	SpansErrored int      `json:"spans_errored,omitempty"`
	ErrorSamples []string `json:"error_samples,omitempty"`
	// RecallCI and PrecisionCI are T1.32's bootstrap confidence intervals
	// per family, built from the same per-unit matching that produced
	// Families -- Families itself is unchanged (still the plain point
	// estimate) so any existing consumer of the old shape keeps working.
	RecallCI    map[string]eval.BootstrapCI `json:"recall_ci,omitempty"`
	PrecisionCI map[string]eval.BootstrapCI `json:"precision_ci,omitempty"`
	// RecallFloorPassed is T1.32's literal pass/fail rule per family:
	// true only when RecallCI[family].Lower >= RecallFloor. A family
	// absent from this map had zero held-out scored units (an empty
	// bootstrap, no floor to clear).
	RecallFloorPassed map[string]bool `json:"recall_floor_passed,omitempty"`
	// Direction is plan T3.16's DIRECTION eval section, present whenever
	// Config.DirectionCorpusDir was set.
	Direction *direction.Report `json:"direction,omitempty"`
	// Reconcile is plan T2.18's reconcile eval section (verdict confusion
	// matrix + a real, non-placeholder contradiction-detection recall),
	// present whenever Config.ReconcileCorpusDir was set.
	Reconcile *reconcileeval.Report `json:"reconcile,omitempty"`
}

// notImplementedContradiction is the honest placeholder every Report
// carries until a real contradiction detector exists (see package doc).
func notImplementedContradiction() *ContradictionSection {
	return &ContradictionSection{
		Status: "not_implemented: no production contradiction detector exists yet (T1.9 deferred semantic reconciliation to E2)",
	}
}

// Run scores cfg's corpus per cfg.Mode and returns the resulting Report.
// Scoring is always against the corpus's held-out split (split.yaml),
// per RFC 0001 SS16/SS17's "published per predicate family on a held-out
// set" -- never the full corpus, which would include spans a milestone's
// extractor work may have been developed against.
func Run(ctx context.Context, cfg Config) (Report, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	labelsDir := filepath.Join(cfg.CorpusDir, labelsSubdir)
	manifestPath := filepath.Join(cfg.CorpusDir, manifestSubpath)
	if err := eval.VerifyManifest(labelsDir, manifestPath); err != nil {
		return Report{}, fmt.Errorf("runner: corpus %s failed manifest verification: %w", cfg.CorpusDir, err)
	}

	labels, err := eval.LoadLabels(labelsDir)
	if err != nil {
		return Report{}, err
	}

	split, err := eval.LoadSplit(filepath.Join(cfg.CorpusDir, splitSubpath))
	if err != nil {
		return Report{}, err
	}
	heldOut, _ := split.Filter(labels)
	if len(heldOut) == 0 {
		return Report{}, fmt.Errorf("runner: corpus %s: held-out split is empty", cfg.CorpusDir)
	}

	report := Report{
		GeneratedAt:   now(),
		Mode:          cfg.Mode,
		Corpus:        cfg.CorpusDir,
		Contradiction: notImplementedContradiction(),
	}

	var predictions []eval.Prediction
	switch cfg.Mode {
	case ModeCached:
		predictions, err = eval.LoadPredictions(cfg.FixturePath)
		if err != nil {
			return Report{}, err
		}
		report.SpansScored = len(heldOut)

	case ModeLive:
		if cfg.Extractor == nil || cfg.Ledger == nil {
			return Report{}, fmt.Errorf("runner: mode live requires a non-nil Extractor and Ledger")
		}
		var skipped, errored int
		var errSamples []string
		predictions, skipped, errored, errSamples = runLive(ctx, cfg, heldOut)
		total, calls := cfg.Ledger.Snapshot()
		report.ModelVersion = cfg.ModelVersion
		report.SpansSkipped = skipped
		report.SpansErrored = errored
		report.ErrorSamples = errSamples
		report.SpansScored = len(heldOut) - skipped - errored
		report.Spend = &SpendSection{
			BudgetUSD:       cfg.BudgetUSD,
			SpentUSD:        total,
			Calls:           calls,
			StoppedOnBudget: skipped > 0,
		}

	default:
		return Report{}, fmt.Errorf("runner: unknown mode %q", cfg.Mode)
	}

	scored := eval.ScoreWithCI(heldOut, predictions, eval.DefaultBootstrapOptions())
	report.Families = make(map[string]eval.PRF1, len(scored))
	report.RecallCI = make(map[string]eval.BootstrapCI, len(scored))
	report.PrecisionCI = make(map[string]eval.BootstrapCI, len(scored))
	report.RecallFloorPassed = make(map[string]bool, len(scored))
	for family, fs := range scored {
		report.Families[family] = fs.PRF1
		report.RecallCI[family] = fs.RecallCI
		report.PrecisionCI[family] = fs.PrecisionCI
		report.RecallFloorPassed[family] = fs.RecallCI.N > 0 && fs.RecallCI.Lower >= RecallFloor
	}

	if cfg.DirectionCorpusDir != "" {
		if cfg.Mode != ModeCached {
			return Report{}, fmt.Errorf("runner: direction corpus scoring only supports mode cached (a live DIRECTION run would need a real ledger.Store adapter over this corpus, not built yet)")
		}
		dirReport, err := scoreDirection(cfg.DirectionCorpusDir, cfg.DirectionFixturePath)
		if err != nil {
			return Report{}, err
		}
		report.Direction = &dirReport
	}

	if cfg.ReconcileCorpusDir != "" {
		reconcileReport, err := scoreReconcile(cfg.ReconcileCorpusDir)
		if err != nil {
			return Report{}, err
		}
		report.Reconcile = &reconcileReport
	}

	return report, nil
}

// scoreDirection loads plan T3.16's DIRECTION corpus and its cached
// predictions fixture and scores them. Unlike the primary corpus above,
// its checksum manifest lives inside labels/ itself
// (direction.ManifestName), matching internal/eval/direction's own layout
// convention rather than evals/corpora/ava's manifest-at-corpus-root
// layout -- the two corpora predate a shared layout decision, and T3.16
// reads direction's real one rather than imposing ava's.
func scoreDirection(corpusDir, fixturePath string) (direction.Report, error) {
	labelsDir := filepath.Join(corpusDir, labelsSubdir)
	manifestPath := filepath.Join(labelsDir, direction.ManifestName)
	if err := eval.VerifyManifest(labelsDir, manifestPath); err != nil {
		return direction.Report{}, fmt.Errorf("runner: direction corpus %s failed manifest verification: %w", corpusDir, err)
	}

	rows, err := direction.LoadRows(labelsDir)
	if err != nil {
		return direction.Report{}, err
	}
	if len(rows) == 0 {
		return direction.Report{}, fmt.Errorf("runner: direction corpus %s has zero rows", corpusDir)
	}

	predictions, err := direction.LoadPredictions(fixturePath)
	if err != nil {
		return direction.Report{}, err
	}

	return direction.Score(rows, predictions)
}

// scoreReconcile loads plan T2.18's reconcile eval corpus and scores it
// directly against the real production internal/reconcile.Detect. Its
// checksum manifest lives inside labels/ itself
// (reconcileeval.ManifestName), the same convention scoreDirection's own
// comment names for the DIRECTION corpus.
func scoreReconcile(corpusDir string) (reconcileeval.Report, error) {
	labelsDir := filepath.Join(corpusDir, labelsSubdir)
	manifestPath := filepath.Join(labelsDir, reconcileeval.ManifestName)
	if err := eval.VerifyManifest(labelsDir, manifestPath); err != nil {
		return reconcileeval.Report{}, fmt.Errorf("runner: reconcile corpus %s failed manifest verification: %w", corpusDir, err)
	}

	rows, err := reconcileeval.LoadRows(labelsDir)
	if err != nil {
		return reconcileeval.Report{}, err
	}
	if len(rows) == 0 {
		return reconcileeval.Report{}, fmt.Errorf("runner: reconcile corpus %s has zero rows", corpusDir)
	}

	return reconcileeval.Score(rows)
}

// runLive extracts one Prediction set per held-out label by calling
// cfg.Extractor on the label's span text as a single one-chunk "document"
// -- the corpus's unit of evaluation is a span, not a source file, so
// there is no chunking to do beyond wrapping the whole span in one
// chunk.Chunk. Before each call it checks cfg.Ledger.OverBudget and, once
// tripped, stops issuing further calls (skipped spans are reported, never
// silently dropped from the report). Both Ready and Distill observations
// are scored: this measures raw extraction accuracy against the golden
// set, not reconciliation eligibility (DistillThreshold gates the latter,
// a separate concern from whether the model got the fact right at all).
// maxErrorSamples bounds Report.ErrorSamples so a fully-broken endpoint
// (every one of a large held-out set erroring) doesn't inflate
// report.json with hundreds of near-identical lines -- the count
// (Report.SpansErrored) already carries the magnitude; the samples exist
// to show what kind of error, not to enumerate every occurrence.
const maxErrorSamples = 20

// runLive drives cfg.Extractor over every held-out span. A single span's
// extraction error (T1.32 finding: T1.30's bounded retry -- 3 attempts,
// backoff capped 30s -- exhausted by one persistent "context deadline
// exceeded" against the DGX endpoint) no longer aborts the whole run and
// discards every other span's already-completed result; it is recorded
// (errored count + a bounded sample of "span: error" strings) and
// skipped like a budget skip, and the loop continues. This matters at
// T1.32's corpus scale specifically: a 4-5 hour, ~312-call live run has
// real exposure to a single transient network blip, and losing the
// entire run's results to one is a genuine operability gap the smaller
// 52-span corpus never surfaced. A caller wanting the OLD hard-fail
// behavior (e.g. detecting a fully-broken endpoint fast) can inspect
// Report.SpansErrored == len(heldOut) after Run returns instead.
func runLive(ctx context.Context, cfg Config, heldOut []eval.Label) ([]eval.Prediction, int, int, []string) {
	var predictions []eval.Prediction
	var skipped, errored int
	var errorSamples []string
	budget := router.Budget{MaxUSD: cfg.BudgetUSD}

	for i, lbl := range heldOut {
		if cfg.Ledger.OverBudget() {
			skipped++
			if cfg.OnProgress != nil {
				cfg.OnProgress(i+1, len(heldOut), skipped, errored)
			}
			continue
		}

		c := chunk.Chunk{Span: chunk.Span{Start: 0, End: len(lbl.Span)}, Text: lbl.Span}
		// false: golden-set spans are synthetic eval fixtures, never a
		// real index_only source.
		res, err := cfg.Extractor.ExtractChunk(ctx, spanSourceID(lbl.Span), false, c, budget)
		if err != nil {
			errored++
			if len(errorSamples) < maxErrorSamples {
				errorSamples = append(errorSamples, fmt.Sprintf("%q: %v", lbl.Span, err))
			}
			if cfg.OnProgress != nil {
				cfg.OnProgress(i+1, len(heldOut), skipped, errored)
			}
			continue
		}

		for _, obs := range res.Ready {
			predictions = append(predictions, eval.Prediction{Span: lbl.Span, Predicate: obs.Predicate, Object: obs.Object})
		}
		for _, obs := range res.Distill {
			predictions = append(predictions, eval.Prediction{Span: lbl.Span, Predicate: obs.Predicate, Object: obs.Object})
		}
		if cfg.OnProgress != nil {
			cfg.OnProgress(i+1, len(heldOut), skipped, errored)
		}
	}
	return predictions, skipped, errored, errorSamples
}

// spanSourceID stands in for a source sha256 in ExtractChunk's cache-key
// and provenance-stamping arguments: a golden span has no real source
// file in this evaluation, so its own text's identity is used instead.
// domain.Observation.Span (byte-offset provenance) is discarded entirely
// by runLive -- eval.Prediction.Span is always the label's own span text,
// which is what eval.Score keys on.
func spanSourceID(span string) string {
	return "eval-span:" + span
}
