// Package runner is plan T1.22's eval-workflow engine. It turns a T1.14-
// shaped corpus (labels/, split.yaml, a checksum manifest) plus a source
// of Predictions -- either a frozen fixture (ModeCached, scored on every
// push, zero network calls) or a real internal/extract.Extractor call
// per held-out span (ModeLive, real cost, meant for the nightly scheduled
// workflow only) -- into the evals/report.json shape RFC 0001 SS16/SS17
// asks for: per-family precision/recall/F1 on the corpus's held-out
// split, plus a contradiction-detection section.
//
// Contradiction detection has no production implementation yet: T1.9's
// acc line explicitly deferred semantic reconciliation (including
// contradiction detection) to E2. Report.Contradiction therefore always
// carries a "not_implemented" status rather than a fabricated recall
// number -- there is no detector anywhere in this codebase for Run to
// call, and reporting a manufactured 0 or 1 here would misrepresent a
// capability gap as a measurement.
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/eval"
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

	// Now stubs time.Now for deterministic tests; nil means time.Now.
	Now func() time.Time
}

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
		var skipped int
		predictions, skipped, err = runLive(ctx, cfg, heldOut)
		if err != nil {
			return Report{}, err
		}
		total, calls := cfg.Ledger.Snapshot()
		report.ModelVersion = cfg.ModelVersion
		report.SpansSkipped = skipped
		report.SpansScored = len(heldOut) - skipped
		report.Spend = &SpendSection{
			BudgetUSD:       cfg.BudgetUSD,
			SpentUSD:        total,
			Calls:           calls,
			StoppedOnBudget: skipped > 0,
		}

	default:
		return Report{}, fmt.Errorf("runner: unknown mode %q", cfg.Mode)
	}

	report.Families = eval.Score(heldOut, predictions)
	return report, nil
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
func runLive(ctx context.Context, cfg Config, heldOut []eval.Label) ([]eval.Prediction, int, error) {
	var predictions []eval.Prediction
	var skipped int
	budget := router.Budget{MaxUSD: cfg.BudgetUSD}

	for _, lbl := range heldOut {
		if cfg.Ledger.OverBudget() {
			skipped++
			continue
		}

		c := chunk.Chunk{Span: chunk.Span{Start: 0, End: len(lbl.Span)}, Text: lbl.Span}
		res, err := cfg.Extractor.ExtractChunk(ctx, spanSourceID(lbl.Span), c, budget)
		if err != nil {
			return nil, skipped, fmt.Errorf("runner: live extraction on span %q: %w", lbl.Span, err)
		}

		for _, obs := range res.Ready {
			predictions = append(predictions, eval.Prediction{Span: lbl.Span, Predicate: obs.Predicate, Object: obs.Object})
		}
		for _, obs := range res.Distill {
			predictions = append(predictions, eval.Prediction{Span: lbl.Span, Predicate: obs.Predicate, Object: obs.Object})
		}
	}
	return predictions, skipped, nil
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
