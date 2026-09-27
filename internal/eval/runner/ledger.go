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
// The cost it sums is real (T24.13, closing lore L-0008): the router
// prices every call's token counts from internal/router/prices.go, so
// TotalUSD moves with each live call and OverBudget trips at the cap. A
// call on a model the price table does not list arrives with
// SpendEntry.Unpriced = true and a finite CostUSD of 0 (a JSON-encoded
// report cannot carry the +Inf the router's Result reports); with a cap
// set, one such call is treated as already over budget -- fail closed,
// never a silent $0 -- and counted in UnpricedCalls so the report can
// say why the run stopped.
type TrackingLedger struct {
	mu sync.Mutex

	// BudgetUSD is the run-wide cap. <= 0 means unlimited.
	BudgetUSD float64
	totalUSD  float64
	calls     int
	unpriced  int
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
	l.calls++
	if e.Unpriced {
		l.unpriced++
		return nil
	}
	l.totalUSD += e.CostUSD
	return nil
}

// OverBudget reports whether the running total has already reached or
// exceeded BudgetUSD, or whether any call so far was on an unpriced
// model (its true cost is unknown, so the cap cannot be shown to hold).
// A non-positive BudgetUSD means unlimited: always false.
func (l *TrackingLedger) OverBudget() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.BudgetUSD > 0 && (l.totalUSD >= l.BudgetUSD || l.unpriced > 0)
}

// Snapshot returns the running total spend (finite; unpriced calls
// contribute nothing to it) and call count so far.
func (l *TrackingLedger) Snapshot() (totalUSD float64, calls int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.totalUSD, l.calls
}

// UnpricedCalls returns how many recorded calls were on a model the
// price table does not list.
func (l *TrackingLedger) UnpricedCalls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.unpriced
}
