package router

import (
	"context"
	"errors"
	"strings"
	"testing"
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

func TestBudgetExceededFlagsResultButStillRecordsSpend(t *testing.T) {
	fp := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok", Usage: Usage{CostUSD: 5.00}}}
	ledger := &fakeLedger{}
	r := New(map[Tier]Provider{TierLocalCheap: fp}, ledger)

	result, err := r.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 1.00})
	if err != nil {
		t.Fatal(err)
	}
	if !result.BudgetExceeded {
		t.Fatal("BudgetExceeded = false, want true")
	}
	if len(ledger.entries) != 1 || ledger.entries[0].CostUSD != 5.00 {
		t.Fatalf("spend ledger did not record the full cost of an over-budget call: %+v", ledger.entries)
	}

	fpUnder := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok", Usage: Usage{CostUSD: 0.50}}}
	ledgerUnder := &fakeLedger{}
	rUnder := New(map[Tier]Provider{TierLocalCheap: fpUnder}, ledgerUnder)
	resultUnder, err := rUnder.Complete(context.Background(), TaskClassSummarization, Prompt{Text: "x"}, Budget{MaxUSD: 1.00})
	if err != nil {
		t.Fatal(err)
	}
	if resultUnder.BudgetExceeded {
		t.Fatal("BudgetExceeded = true, want false (under budget)")
	}

	fpUnlimited := &fakeProvider{name: "fake", modelVersion: "fake@v1", resp: Response{Text: "ok", Usage: Usage{CostUSD: 100.00}}}
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
