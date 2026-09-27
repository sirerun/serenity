package router

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeProvider is a test double implementing Provider. Test-file only,
// per the zero-stub policy.
type fakeProvider struct {
	name         string
	modelVersion string
	resp         Response
	err          error
	calls        int
}

func (f *fakeProvider) Name() string         { return f.name }
func (f *fakeProvider) ModelVersion() string { return f.modelVersion }
func (f *fakeProvider) Send(_ context.Context, _ string) (Response, error) {
	f.calls++
	return f.resp, f.err
}

// retryFakeProvider is a test double implementing Provider whose Send
// fails with a configured error for the first failCount calls, then
// succeeds. failCount left at or above the router's retryAttempts makes
// it fail every attempt Complete makes. Test-file only, per the zero-stub
// policy.
type retryFakeProvider struct {
	name         string
	modelVersion string
	failWith     error
	failCount    int
	resp         Response
	calls        int
}

func (f *retryFakeProvider) Name() string         { return f.name }
func (f *retryFakeProvider) ModelVersion() string { return f.modelVersion }
func (f *retryFakeProvider) Send(_ context.Context, _ string) (Response, error) {
	f.calls++
	if f.calls <= f.failCount {
		return Response{}, f.failWith
	}
	return f.resp, nil
}

// fakeLedger is a test double implementing SpendLedger.
type fakeLedger struct {
	entries []SpendEntry
	err     error
}

func (f *fakeLedger) Record(_ context.Context, e SpendEntry) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, e)
	return nil
}

func TestTierUnavailableErrorsForJudgmentOnLocalCheap(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	_, err := r.Complete(context.Background(), TaskClassComposerSynthesis, Prompt{Text: "plan this"}, Budget{})
	if err == nil {
		t.Fatal("expected an error routing a judgment task class with only a local-cheap provider registered")
	}
	if !errors.Is(err, ErrTierUnavailable) {
		t.Fatalf("expected ErrTierUnavailable, got %v", err)
	}
	if fp.calls != 0 {
		t.Fatalf("provider Send was called %d times, want 0 -- never silently downgrade to a cheaper tier", fp.calls)
	}
	if len(ledger.entries) != 0 {
		t.Fatalf("spend ledger has %d entries, want 0 -- an unrouted call is never billed", len(ledger.entries))
	}
}

func TestConfidenceClampsToLocalCheapCap(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok", Confidence: 0.97}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Confidence.Value != CapLocalCheap {
		t.Fatalf("Confidence.Value = %v, want %v", result.Confidence.Value, CapLocalCheap)
	}
	if !result.Confidence.Clamped {
		t.Fatal("Confidence.Clamped = false, want true")
	}
}

func TestConfidenceClampsToJudgmentCap(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok", Confidence: 0.97}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierJudgment: fp}, ledger)

	result, err := r.Complete(context.Background(), TaskClassComposerSynthesis, Prompt{Text: "x"}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Confidence.Value != CapJudgment {
		t.Fatalf("Confidence.Value = %v, want %v", result.Confidence.Value, CapJudgment)
	}
	if !result.Confidence.Clamped {
		t.Fatal("Confidence.Clamped = false, want true")
	}
}

func TestConfidenceNotClampedAtOrUnderCap(t *testing.T) {
	cases := []struct {
		raw  float64
		tier Tier
	}{
		{0.5, TierLocalCheap},
		{CapLocalCheap, TierLocalCheap}, // exactly at cap
		{CapJudgment, TierJudgment},     // exactly at cap
		{0.0, TierJudgment},
	}
	for _, c := range cases {
		got := NewConfidence(c.raw, c.tier)
		if got.Clamped {
			t.Fatalf("NewConfidence(%v, %v).Clamped = true, want false", c.raw, c.tier)
		}
		if got.Value != c.raw {
			t.Fatalf("NewConfidence(%v, %v).Value = %v, want %v", c.raw, c.tier, got.Value, c.raw)
		}
	}
}

func TestCompleteAppendsOneSpendRowPerCall(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	for i := 0; i < 2; i++ {
		if _, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(ledger.entries) != 2 {
		t.Fatalf("spend ledger has %d entries, want 2 (one per call)", len(ledger.entries))
	}
}

func TestSpendRowCarriesProviderAndModelVersion(t *testing.T) {
	fp := &fakeProvider{name: "anthropic", modelVersion: "claude-haiku-4-5@20251001", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	if _, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{}); err != nil {
		t.Fatal(err)
	}
	if len(ledger.entries) != 1 {
		t.Fatalf("spend ledger has %d entries, want 1", len(ledger.entries))
	}
	entry := ledger.entries[0]
	if entry.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want %q", entry.Provider, "anthropic")
	}
	if entry.ModelVersion != "claude-haiku-4-5@20251001" {
		t.Fatalf("ModelVersion = %q, want %q", entry.ModelVersion, "claude-haiku-4-5@20251001")
	}
	if strings.Count(entry.ModelVersion, "@") != 1 {
		t.Fatalf("ModelVersion %q does not have exactly one %q", entry.ModelVersion, "@")
	}
}

func TestIndexOnlyPromptRefusedBeforeEgress(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	_, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "sensitive", IndexOnly: true}, Budget{})
	if !errors.Is(err, ErrIndexOnlyEgress) {
		t.Fatalf("expected ErrIndexOnlyEgress, got %v", err)
	}
	if fp.calls != 0 {
		t.Fatalf("provider Send was called %d times, want 0 -- index_only content must never reach egress", fp.calls)
	}
	if len(ledger.entries) != 0 {
		t.Fatalf("spend ledger has %d entries, want 0 -- a refused call is never billed", len(ledger.entries))
	}
}

func TestNonIndexOnlyPromptReachesProvider(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	_, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "not sensitive", IndexOnly: false}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider Send was called %d times, want 1", fp.calls)
	}
}

// pricedModelVersion is a model id that prices.go lists ($1.00 per
// million input tokens, $5.00 per million output tokens), pinned the
// way a real provider's ModelVersion() reports it. The fake providers
// below report token counts ONLY -- no Usage.CostUSD -- exactly like
// AnthropicProvider/OpenAICompatibleProvider did before T24.13 (lore
// L-0008), so the budget checks here prove the router prices the call
// itself rather than trusting an injected number.
const pricedModelVersion = "claude-haiku-4-5-20251001@v1"

func TestBudgetExceededFlagsResultButStillRecordsSpend(t *testing.T) {
	// 5,000,000 input tokens at $1.00/M = $5.00 against a $1.00 cap.
	fp := &fakeProvider{name: "fake", modelVersion: pricedModelVersion, resp: Response{Text: "ok", Usage: Usage{InputTokens: 5_000_000}}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 1.00})
	if err != nil {
		t.Fatal(err)
	}
	if !result.BudgetExceeded {
		t.Fatalf("BudgetExceeded = false, want true: 5M input tokens on %s is $5.00 against a $1.00 cap (Usage = %+v)", pricedModelVersion, result.Usage)
	}
	if math.Abs(result.Usage.CostUSD-5.00) > 1e-9 {
		t.Fatalf("Result.Usage.CostUSD = %v, want 5.00 priced from the token counts", result.Usage.CostUSD)
	}
	if len(ledger.entries) != 1 || math.Abs(ledger.entries[0].CostUSD-5.00) > 1e-9 {
		t.Fatalf("spend ledger did not record the full cost of an over-budget call: %+v", ledger.entries)
	}
	if ledger.entries[0].Unpriced {
		t.Fatal("spend ledger row marked Unpriced for a model the price table lists")
	}

	// 500,000 input tokens = $0.50, under the $1.00 cap.
	fpUnder := &fakeProvider{name: "fake", modelVersion: pricedModelVersion, resp: Response{Text: "ok", Usage: Usage{InputTokens: 500_000}}}
	ledgerUnder := &fakeLedger{}
	rUnder := New(map[Tier]Provider{TierLocalCheap: fpUnder}, ledgerUnder)
	resultUnder, err := rUnder.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 1.00})
	if err != nil {
		t.Fatal(err)
	}
	if resultUnder.BudgetExceeded {
		t.Fatalf("BudgetExceeded = true, want false (under budget: Usage = %+v)", resultUnder.Usage)
	}

	// 100,000,000 input tokens = $100.00, but Budget{} means unlimited.
	fpUnlimited := &fakeProvider{name: "fake", modelVersion: pricedModelVersion, resp: Response{Text: "ok", Usage: Usage{InputTokens: 100_000_000}}}
	ledgerUnlimited := &fakeLedger{}
	rUnlimited := New(map[Tier]Provider{TierLocalCheap: fpUnlimited}, ledgerUnlimited)
	resultUnlimited, err := rUnlimited.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if resultUnlimited.BudgetExceeded {
		t.Fatal("BudgetExceeded = true, want false (Budget{} with MaxUSD 0 means unlimited)")
	}
}

// TestBudgetMaxUSDTripsFromTokenCountsAlone is T24.13's genuine-red test:
// a provider that reports token counts and nothing else (the shape both
// real adapters had before this task) must still trip Budget.MaxUSD,
// because the router prices the call from prices.go. Against the
// pre-T24.13 router this fails with BudgetExceeded = false and CostUSD = 0.
func TestBudgetMaxUSDTripsFromTokenCountsAlone(t *testing.T) {
	// 200,000 output tokens at $5.00/M = $1.00 against a $0.50 cap.
	fp := &fakeProvider{name: "fake", modelVersion: pricedModelVersion, resp: Response{Text: "ok", Usage: Usage{InputTokens: 10, OutputTokens: 200_000}}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 0.50})
	if err != nil {
		t.Fatal(err)
	}
	if !result.BudgetExceeded {
		t.Fatalf("BudgetExceeded = false, want true: the provider reported 200,000 output tokens on %s ($1.00) against MaxUSD 0.50, but the router saw CostUSD = %v", pricedModelVersion, result.Usage.CostUSD)
	}
	if len(ledger.entries) != 1 || ledger.entries[0].CostUSD <= 0 {
		t.Fatalf("spend ledger row carries CostUSD %v, want the real priced cost (> 0): %+v", ledger.entries[0].CostUSD, ledger.entries)
	}
}

// TestUnlistedModelCostIsInfiniteAndTripsBudget: a model the price table
// does not list fails closed -- its cost is +Inf, so any positive MaxUSD
// trips; the ledger row stays finite (a JSON-persisted ledger cannot carry
// +Inf) but is marked Unpriced; and the router warns exactly once per
// model id, not once per call.
func TestUnlistedModelCostIsInfiniteAndTripsBudget(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "model-nobody-priced@v9", resp: Response{Text: "ok", Usage: Usage{InputTokens: 1, OutputTokens: 1}}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	var warnings []string
	r.warn = func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	for i := 0; i < 2; i++ {
		result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 1_000_000})
		if err != nil {
			t.Fatal(err)
		}
		if !result.BudgetExceeded {
			t.Fatalf("call %d: BudgetExceeded = false, want true -- an unlisted model must fail closed against any MaxUSD (CostUSD = %v)", i, result.Usage.CostUSD)
		}
		if !math.IsInf(result.Usage.CostUSD, 1) {
			t.Fatalf("call %d: Result.Usage.CostUSD = %v, want +Inf for an unlisted model", i, result.Usage.CostUSD)
		}
	}
	if len(ledger.entries) != 2 {
		t.Fatalf("spend ledger has %d entries, want 2 -- an unpriced call is still recorded", len(ledger.entries))
	}
	for _, e := range ledger.entries {
		if !e.Unpriced {
			t.Fatalf("spend ledger row not marked Unpriced: %+v", e)
		}
		if math.IsInf(e.CostUSD, 0) || math.IsNaN(e.CostUSD) {
			t.Fatalf("spend ledger row CostUSD = %v, want a finite value (the index ledger JSON-encodes rows)", e.CostUSD)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("router warned %d times for one unlisted model over two calls, want exactly 1: %q", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "model-nobody-priced") {
		t.Fatalf("warning does not name the unlisted model id: %q", warnings[0])
	}
}

// TestUnlistedModelWithoutMaxUSDIsNotExceeded: Budget{} is unlimited, so
// an unpriced model does not flag BudgetExceeded -- fail closed applies to
// a ceiling that exists, it does not invent one.
func TestUnlistedModelWithoutMaxUSDIsNotExceeded(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "model-nobody-priced@v9", resp: Response{Text: "ok", Usage: Usage{InputTokens: 1, OutputTokens: 1}}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.warn = func(string, ...any) {}

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	if result.BudgetExceeded {
		t.Fatal("BudgetExceeded = true with Budget{} (unlimited), want false")
	}
}

// TestCompleteReturnsCtxErrImmediatelyWhenCancelled: a context that is
// already done never reaches the provider and never sleeps -- Complete
// returns ctx.Err() at once, with nothing billed.
func TestCompleteReturnsCtxErrImmediatelyWhenCancelled(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: pricedModelVersion, resp: Response{Text: "ok"}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.wait = func(context.Context, time.Duration) error {
		t.Fatal("retry backoff wait was entered for a cancelled context")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Complete(ctx, TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete on a cancelled context returned %v, want context.Canceled", err)
	}
	if fp.calls != 0 {
		t.Fatalf("provider Send was called %d times on a cancelled context, want 0", fp.calls)
	}
	if len(ledger.entries) != 0 {
		t.Fatalf("spend ledger has %d entries, want 0", len(ledger.entries))
	}
}

// TestRetryStopsWhenContextCancelledDuringBackoff: cancellation that
// lands while the router is waiting between attempts ends the retry loop
// with ctx.Err() -- the backoff is a ctx-aware wait, not a bare sleep.
func TestRetryStopsWhenContextCancelledDuringBackoff(t *testing.T) {
	fp := &retryFakeProvider{
		name:         "fake",
		modelVersion: pricedModelVersion,
		failWith:     &fakeNetError{msg: "connection reset by peer"},
		failCount:    1000,
	}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	r.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		cancel() // the caller gives up mid-backoff
		return ctx.Err()
	}

	_, err := r.Complete(ctx, TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete returned %v, want context.Canceled from the backoff wait", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider Send was called %d times, want 1 -- no retry after the context was cancelled", fp.calls)
	}
	if waits != 1 {
		t.Fatalf("backoff wait entered %d times, want 1", waits)
	}
}

// TestRetryDoesNotRetryContextCanceledFromProvider: a provider error that
// wraps context.Canceled (net/http returns it as a *url.Error, which is
// also a net.Error) is the caller's own cancellation, never a transient
// network fault to retry.
func TestRetryDoesNotRetryContextCanceledFromProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fp := &cancellingFakeProvider{modelVersion: pricedModelVersion, cancel: cancel}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.wait = func(context.Context, time.Duration) error {
		t.Fatal("retry backoff wait was entered after the provider reported the caller's own cancellation")
		return nil
	}

	_, err := r.Complete(ctx, TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete returned %v, want context.Canceled", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider Send was called %d times, want 1", fp.calls)
	}
}

// cancellingFakeProvider cancels the caller's context from inside Send
// and returns the net/http-shaped error a cancelled request produces.
// Test-file only, per the zero-stub policy.
type cancellingFakeProvider struct {
	modelVersion string
	cancel       context.CancelFunc
	calls        int
}

func (f *cancellingFakeProvider) Name() string         { return "fake" }
func (f *cancellingFakeProvider) ModelVersion() string { return f.modelVersion }
func (f *cancellingFakeProvider) Send(_ context.Context, _ string) (Response, error) {
	f.calls++
	f.cancel()
	return Response{}, &url.Error{Op: "Post", URL: "http://127.0.0.1:9/v1/chat/completions", Err: context.Canceled}
}

func TestTierForUnknownTaskClass(t *testing.T) {
	if _, ok := TierFor(TaskClass("does_not_exist")); ok {
		t.Fatal("TierFor(unknown) ok = true, want false")
	}
}

func TestCompleteFailsWhenLedgerAppendFails(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok"}}
	ledger := &fakeLedger{err: errors.New("disk full")}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	_, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err == nil {
		t.Fatal("expected Complete to fail when the spend ledger append fails -- an unrecorded call must not report success")
	}
}

// noWait replaces Router.wait in tests so retry backoff never actually
// waits real wall-clock time, regardless of the production base/max delay
// constants.
func noWait(context.Context, time.Duration) error { return nil }

func TestCompleteRetriesTransientErrorThenSucceeds(t *testing.T) {
	fp := &retryFakeProvider{
		name:         "fake",
		modelVersion: "fake@v1",
		failWith:     &fakeNetError{msg: "connection reset by peer", timeout: false},
		failCount:    1, // fails once, then succeeds
		resp:         Response{Text: "ok after retry"},
	}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.wait = noWait

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err != nil {
		t.Fatalf("Complete returned an error after a single transient failure followed by success: %v", err)
	}
	if result.Text != "ok after retry" {
		t.Fatalf("result.Text = %q, want %q", result.Text, "ok after retry")
	}
	if fp.calls != 2 {
		t.Fatalf("provider Send was called %d times, want 2 (one failure, one retry that succeeded)", fp.calls)
	}
	if len(ledger.entries) != 1 {
		t.Fatalf("spend ledger has %d entries, want 1 -- exactly the successful call is billed", len(ledger.entries))
	}
}

func TestCompleteFailsClearlyAfterBoundedRetriesWhenAlwaysFailing(t *testing.T) {
	fp := &retryFakeProvider{
		name:         "fake",
		modelVersion: "fake@v1",
		failWith:     &fakeNetError{msg: "connection refused", timeout: false},
		failCount:    1000, // never succeeds
	}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.wait = noWait

	_, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err == nil {
		t.Fatal("expected Complete to return a clear error when every attempt fails, not a success")
	}
	if fp.calls != defaultRetryAttempts {
		t.Fatalf("provider Send was called %d times, want exactly %d (bounded retry count, not an unbounded hang)", fp.calls, defaultRetryAttempts)
	}
	if len(ledger.entries) != 0 {
		t.Fatalf("spend ledger has %d entries, want 0 -- a call that never succeeded is never billed", len(ledger.entries))
	}
}

func TestCompleteDoesNotRetryNonTransientError(t *testing.T) {
	fp := &retryFakeProvider{
		name:         "fake",
		modelVersion: "fake@v1",
		failWith:     errors.New("openai_compatible: status 400: bad request"),
		failCount:    1000,
	}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)
	r.wait = noWait

	_, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{})
	if err == nil {
		t.Fatal("expected Complete to return an error for a non-transient application-level failure")
	}
	if fp.calls != 1 {
		t.Fatalf("provider Send was called %d times, want 1 -- an application-level error (e.g. a 4xx status) is never retried", fp.calls)
	}
}
