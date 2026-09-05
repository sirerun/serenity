// Package router is the single chokepoint for model calls (RFC 0001
// section 9): Router.Complete(taskClass, prompt, budget). It enforces the
// two-tier model: local-cheap for cheap/bulk task classes, judgment for
// hard reasoning, and never silently substitutes one for the other. It
// clamps machine-reported confidence at the type boundary (Confidence,
// confidence.go), refuses egress for index_only content (section 7.4,
// section 14) before any provider is invoked, and records a spend ledger
// row for every completed call (section 16).
//
// Core files in this package (router.go, confidence.go, provider.go,
// ledger.go) use the standard library only, per ADR 003. Provider
// adapters (anthropic.go, openai_compatible.go) are the "connector and
// provider edge" ADR 003 permits to reach out over net/http directly.
package router

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// TaskClass names one unit of routed work. The fixed set below mirrors
// RFC 0001 section 9's task-class table; TierFor is the only place the
// class-to-tier mapping is decided.
type TaskClass string

const (
	// Local-cheap task classes (RFC section 9).
	TaskClassEmbedding            TaskClass = "embedding"
	TaskClassTranscription        TaskClass = "transcription"
	TaskClassClassification       TaskClass = "classification"
	TaskClassExtractionCandidates TaskClass = "extraction_candidates"
	TaskClassSummarization        TaskClass = "summarization"
	TaskClassConsolidation        TaskClass = "consolidation"

	// Judgment task classes (RFC section 9).
	TaskClassDecompositionProposals     TaskClass = "decomposition_proposals"
	TaskClassHardConflictReconciliation TaskClass = "hard_conflict_reconciliation"
	TaskClassPlanVsPreceptAnalysis      TaskClass = "plan_vs_precept_analysis"
	TaskClassComposerSynthesis          TaskClass = "composer_synthesis"
)

// Tier is a model routing tier (RFC section 9).
type Tier string

const (
	TierLocalCheap Tier = "local-cheap"
	TierJudgment   Tier = "judgment"
)

// taskClassTiers is the closed mapping from RFC section 9's table. It is
// the ONLY place a task class resolves to a tier -- callers never pick a
// tier directly, so a judgment-shaped task can never be routed to the
// cheaper tier by convention or caller mistake.
var taskClassTiers = map[TaskClass]Tier{
	TaskClassEmbedding:            TierLocalCheap,
	TaskClassTranscription:        TierLocalCheap,
	TaskClassClassification:       TierLocalCheap,
	TaskClassExtractionCandidates: TierLocalCheap,
	TaskClassSummarization:        TierLocalCheap,
	TaskClassConsolidation:        TierLocalCheap,

	TaskClassDecompositionProposals:     TierJudgment,
	TaskClassHardConflictReconciliation: TierJudgment,
	TaskClassPlanVsPreceptAnalysis:      TierJudgment,
	TaskClassComposerSynthesis:          TierJudgment,
}

// TierFor resolves a task class to its routing tier. ok is false for a
// task class outside the closed set.
func TierFor(tc TaskClass) (Tier, bool) {
	tier, ok := taskClassTiers[tc]
	return tier, ok
}

// Prompt is one call's input. IndexOnly mirrors RFC section 7.4/14: it is
// true when any composed segment originates from a source marked
// index_only in serenity.yml. index_only bytes must never leave the
// machine (section 14), so Complete refuses to send such a prompt to any
// provider, unconditionally -- this is a stricter, self-contained
// superset of "refuse when redaction is disabled but index_only content
// is present" (T1.19's egress acceptance line): the RFC states the
// index_only rule without a redaction-enabled precondition, so Complete
// enforces it that way regardless of any future redaction pass. This
// package deliberately has no dependency on a redaction implementation.
type Prompt struct {
	Text      string
	IndexOnly bool
}

// Budget bounds one call's cost. MaxUSD <= 0 means unlimited for this
// call (the monthly ceiling in RFC section 9 is a separate, later
// subsystem -- T4.10 -- that aggregates spend ledger rows this package
// writes; Budget here is a per-call advisory bound, not that ceiling).
type Budget struct {
	MaxUSD float64
}

// Result is what Complete returns on success.
type Result struct {
	Text         string
	Confidence   Confidence
	Provider     string
	ModelVersion string
	Usage        Usage
	Tier         Tier
	// BudgetExceeded is true when Budget.MaxUSD was positive and the
	// call's actual cost exceeded it. The call still happened and its
	// real cost is still recorded in the spend ledger -- refusing to
	// report a call that already spent money would only hide the spend,
	// not prevent it.
	BudgetExceeded bool
}

// ErrTierUnavailable means the tier a task class requires has no
// registered provider. Complete never falls back to a cheaper tier.
var ErrTierUnavailable = errors.New("router: no provider configured for required tier")

// ErrIndexOnlyEgress means the prompt carried index_only content and was
// refused before any provider call was made.
var ErrIndexOnlyEgress = errors.New("router: prompt carries index_only content, refused before egress")

// ErrUnknownTaskClass means the task class is outside RFC section 9's
// closed set.
var ErrUnknownTaskClass = errors.New("router: unknown task class")

// Router is the model-routing chokepoint. Construct with New; the zero
// value is not usable (providers/ledger are required).
type Router struct {
	providers map[Tier]Provider
	ledger    SpendLedger
	now       func() time.Time
	newID     func() string
}

// New builds a Router over the given per-tier providers and spend
// ledger. providers need not cover every tier -- a tier with no provider
// simply errors when a task class requiring it is routed (never silently
// substitutes another tier).
func New(providers map[Tier]Provider, ledger SpendLedger) *Router {
	return &Router{
		providers: providers,
		ledger:    ledger,
		now:       time.Now,
		newID:     newSpendID,
	}
}

// Provider returns the provider registered for tier -- for callers that
// need to inspect which concrete adapter a wiring path built (e.g.
// internal/providers' provider-selection tests, ADR 013) rather than
// route a call through it. ok is false when tier has no provider.
func (r *Router) Provider(tier Tier) (Provider, bool) {
	p, ok := r.providers[tier]
	return p, ok
}

func newSpendID() string {
	var b [16]byte
	// crypto/rand.Read on the package-level Reader never returns a
	// short read without an error on supported platforms; a failure
	// here means the platform's CSPRNG is broken, which every caller
	// of this process already depends on elsewhere, so a panic here
	// buys no additional safety and a time-based fallback would be
	// misleading as an "id" -- read again is not an option unlike
	// claim ids (there is no derivation contract for spend rows, RFC
	// section 16 only requires a per-row identity), so the practical
	// choice is: this line does not fail in this codebase's target
	// platforms (darwin/linux), full stop.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Complete routes taskClass to its tier, refuses egress for index_only
// prompts before any network call, invokes the tier's provider, clamps
// the reported confidence at the tier's cap, and appends exactly one
// spend ledger row -- success or not recorded is not a valid outcome, so
// a ledger append failure fails the call even though the provider already
// answered.
func (r *Router) Complete(ctx context.Context, tc TaskClass, p Prompt, b Budget) (Result, error) {
	tier, ok := TierFor(tc)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownTaskClass, tc)
	}

	if p.IndexOnly {
		return Result{}, fmt.Errorf("%w: task class %q", ErrIndexOnlyEgress, tc)
	}

	provider, ok := r.providers[tier]
	if !ok {
		return Result{}, fmt.Errorf("%w: task class %q requires %s tier", ErrTierUnavailable, tc, tier)
	}

	resp, err := provider.Send(ctx, p.Text)
	if err != nil {
		return Result{}, fmt.Errorf("router: %s provider: %w", provider.Name(), err)
	}

	conf := NewConfidence(resp.Confidence, tier)
	exceeded := b.MaxUSD > 0 && resp.Usage.CostUSD > b.MaxUSD

	entry := SpendEntry{
		ID:                r.newID(),
		TaskClass:         tc,
		Tier:              tier,
		Provider:          provider.Name(),
		ModelVersion:      provider.ModelVersion(),
		InputTokens:       resp.Usage.InputTokens,
		OutputTokens:      resp.Usage.OutputTokens,
		CostUSD:           resp.Usage.CostUSD,
		Confidence:        conf.Value,
		ConfidenceClamped: conf.Clamped,
		OccurredAt:        r.now(),
	}
	if err := r.ledger.Record(ctx, entry); err != nil {
		return Result{}, fmt.Errorf("router: spend ledger record: %w", err)
	}

	return Result{
		Text:           resp.Text,
		Confidence:     conf,
		Provider:       provider.Name(),
		ModelVersion:   provider.ModelVersion(),
		Usage:          resp.Usage,
		Tier:           tier,
		BudgetExceeded: exceeded,
	}, nil
}
