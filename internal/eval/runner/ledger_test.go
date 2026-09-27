package runner

import (
	"context"
	"math"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

// tokensOnlyProvider reports token counts and NOTHING else -- the exact
// shape both real adapters had before T24.13 (lore L-0008) -- so the
// aggregate cap below is proven to trip on the router's own pricing, not
// on a test-injected CostUSD. Test-file only, per the zero-stub policy.
type tokensOnlyProvider struct {
	modelVersion string
	in, out      int
	calls        int
}

func (p *tokensOnlyProvider) Name() string         { return "fake" }
func (p *tokensOnlyProvider) ModelVersion() string { return p.modelVersion }
func (p *tokensOnlyProvider) Send(_ context.Context, _ string) (router.Response, error) {
	p.calls++
	return router.Response{Text: "ok", Usage: router.Usage{InputTokens: p.in, OutputTokens: p.out}}, nil
}

// TestTrackingLedgerTripsOnRealPricedCost: SERENITY_EVAL_BUDGET_USD's
// aggregate cap is real once the router prices token counts. Two calls of
// 300,000 input tokens on claude-haiku-4-5 ($1.00/M) are $0.60 against a
// $0.50 cap -- OverBudget must be true after the second call and false
// after the first.
func TestTrackingLedgerTripsOnRealPricedCost(t *testing.T) {
	ledger := NewTrackingLedger(0.50)
	p := &tokensOnlyProvider{modelVersion: "claude-haiku-4-5-20251001@v1", in: 300_000}
	rt := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p}, ledger)

	if _, err := rt.Complete(context.Background(), router.TaskClassExtractionCandidates, router.Prompt{Text: "x"}, router.Budget{MaxUSD: 0.50}); err != nil {
		t.Fatal(err)
	}
	if ledger.OverBudget() {
		total, _ := ledger.Snapshot()
		t.Fatalf("OverBudget = true after one $0.30 call against a $0.50 cap (total %.4f)", total)
	}
	if _, err := rt.Complete(context.Background(), router.TaskClassExtractionCandidates, router.Prompt{Text: "x"}, router.Budget{MaxUSD: 0.50}); err != nil {
		t.Fatal(err)
	}
	total, calls := ledger.Snapshot()
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if math.Abs(total-0.60) > 1e-9 {
		t.Fatalf("Snapshot total = %.4f, want 0.60 priced from token counts (was the cost ever computed?)", total)
	}
	if !ledger.OverBudget() {
		t.Fatalf("OverBudget = false after $%.2f of spend against a $0.50 cap, want true", total)
	}
}

// TestTrackingLedgerUnpricedCallFailsClosed: a call on a model the price
// table does not list has no finite cost. With a cap set the ledger must
// treat that as over budget immediately (fail closed) while keeping its
// running total finite (the report JSON-encodes it); with no cap set it
// stays unlimited, as before.
func TestTrackingLedgerUnpricedCallFailsClosed(t *testing.T) {
	capped := NewTrackingLedger(100)
	if err := capped.Record(context.Background(), router.SpendEntry{ModelVersion: "model-nobody-priced@v1", Unpriced: true}); err != nil {
		t.Fatal(err)
	}
	if !capped.OverBudget() {
		t.Fatal("OverBudget = false after an unpriced call under a $100 cap, want true (fail closed)")
	}
	total, calls := capped.Snapshot()
	if math.IsInf(total, 0) || math.IsNaN(total) {
		t.Fatalf("Snapshot total = %v, want finite (report.json cannot encode +Inf)", total)
	}
	if calls != 1 || capped.UnpricedCalls() != 1 {
		t.Fatalf("calls = %d, unpriced = %d, want 1 and 1", calls, capped.UnpricedCalls())
	}

	unlimited := NewTrackingLedger(0)
	if err := unlimited.Record(context.Background(), router.SpendEntry{ModelVersion: "model-nobody-priced@v1", Unpriced: true}); err != nil {
		t.Fatal(err)
	}
	if unlimited.OverBudget() {
		t.Fatal("OverBudget = true with no cap set, want false (unlimited stays unlimited)")
	}
}
