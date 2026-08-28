package runner

import (
	"context"
	"sync"

	"github.com/sirerun/serenity/internal/router"
)

// TrackingLedger is a router.SpendLedger that sums CostUSD across every
// recorded call and reports whether the running total has reached its
// cap. This is an AGGREGATE, whole-run cap -- distinct from
// router.Budget.MaxUSD, which bounds a single Router.Complete call.
// internal/router/ledger.go's own doc comment says a real SpendLedger is
// wired by "whichever subsystem first holds a live index.Engine handle
// ... or T4.10's spend ceiling" -- neither exists yet, so this type
// exists purely to give plan T1.22's nightly eval workflow's
// SERENITY_EVAL_BUDGET_USD env var something to be enforced against.
//
// KNOWN LIMITATION (disclosed deliberately, not hidden): neither
// router.AnthropicProvider nor router.OpenAICompatibleProvider computes
// Usage.CostUSD today (internal/router/anthropic.go,
// openai_compatible.go both leave it at its zero value) -- so against
// those two providers as they stand, TotalUSD never moves and OverBudget
// never trips, no matter how many real calls a live run makes. The
// mechanism below is correctly wired and starts enforcing the moment a
// provider actually reports a cost; Record counts every call and reports
// the (possibly zero) total honestly rather than estimating a number the
// provider never gave it.
type TrackingLedger struct {
	mu sync.Mutex

	// BudgetUSD is the run-wide cap. <= 0 means unlimited.
	BudgetUSD float64
	totalUSD  float64
	calls     int
}

var _ router.SpendLedger = (*TrackingLedger)(nil)

// NewTrackingLedger builds a TrackingLedger capped at budgetUSD (<= 0:
// unlimited).
func NewTrackingLedger(budgetUSD float64) *TrackingLedger {
	return &TrackingLedger{BudgetUSD: budgetUSD}
}

// Record implements router.SpendLedger.
func (l *TrackingLedger) Record(_ context.Context, e router.SpendEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.totalUSD += e.CostUSD
	l.calls++
	return nil
}

// OverBudget reports whether the running total has already reached or
// exceeded BudgetUSD. A non-positive BudgetUSD means unlimited: always
// false.
func (l *TrackingLedger) OverBudget() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.BudgetUSD > 0 && l.totalUSD >= l.BudgetUSD
}

// Snapshot returns the running total spend and call count so far.
func (l *TrackingLedger) Snapshot() (totalUSD float64, calls int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.totalUSD, l.calls
}
