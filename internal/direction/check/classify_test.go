package check

import (
	"context"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/router"
)

// fakeClassifyProvider is a test double implementing router.Provider --
// the same pattern check_test.go and internal/extract/extract_test.go
// use. Test-file only, per the zero-stub policy: no production code path
// constructs one.
type fakeClassifyProvider struct {
	modelVersion string
	resp         router.Response
	err          error
	calls        int
}

func (f *fakeClassifyProvider) Name() string         { return "fake-classify" }
func (f *fakeClassifyProvider) ModelVersion() string { return f.modelVersion }
func (f *fakeClassifyProvider) Send(_ context.Context, _ string) (router.Response, error) {
	f.calls++
	return f.resp, f.err
}

// countingLedger is a test double implementing router.SpendLedger that
// records every entry, so a test can assert exactly how many spend rows
// a sequence of calls produced -- the acc line's "writes no spend row" on
// a cache hit.
type countingLedger struct{ entries []router.SpendEntry }

func (l *countingLedger) Record(_ context.Context, e router.SpendEntry) error {
	l.entries = append(l.entries, e)
	return nil
}

func newClassifyTestRouter(fp *fakeClassifyProvider, l router.SpendLedger) *router.Router {
	return router.New(map[router.Tier]router.Provider{router.TierLocalCheap: fp}, l)
}

const wireVendorText = "wire $800 to the new vendor tomorrow"

const wireVendorResponse = `{"confidence":0.9,"actions":[` +
	`{"action":"spend_over","params":{"amount":800},"evidence":"wire $800"},` +
	`{"action":"contact_new_party","params":{},"evidence":"the new vendor"}` +
	`]}`

// --- acc line 1: classifies to [spend_over{amount:800}, contact_new_party]
// with spans, then stage 1 runs ---

func TestMatchFreeText_ClassifiesWithSpansThenRunsStage1(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: wireVendorResponse}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}

	if len(result.MatchedActions) != 2 {
		t.Fatalf("MatchedActions = %+v, want 2", result.MatchedActions)
	}

	spend := result.MatchedActions[0]
	if spend.Action.Action != "spend_over" {
		t.Errorf("MatchedActions[0].Action.Action = %q, want spend_over", spend.Action.Action)
	}
	if amt, _ := spend.Action.Params["amount"].(float64); amt != 800 {
		t.Errorf("MatchedActions[0].Action.Params[amount] = %v, want 800", spend.Action.Params["amount"])
	}
	if spend.Span.Text != "wire $800" || spend.Span.Start != 0 || spend.Span.End != len("wire $800") {
		t.Errorf("MatchedActions[0].Span = %+v, want a located span over %q", spend.Span, "wire $800")
	}

	contact := result.MatchedActions[1]
	if contact.Action.Action != "contact_new_party" {
		t.Errorf("MatchedActions[1].Action.Action = %q, want contact_new_party", contact.Action.Action)
	}
	wantStart := len("wire $800 to ")
	if contact.Span.Text != "the new vendor" || contact.Span.Start != wantStart {
		t.Errorf("MatchedActions[1].Span = %+v, want a located span over %q at %d", contact.Span, "the new vendor", wantStart)
	}

	// Stage 1 actually ran: the classified spend_over{amount:800} trips
	// the fixture's spend-ceiling constraint (gte 200), so the overall
	// verdict must be StatusViolated, not merely "classification
	// succeeded".
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q (stage 1 must have run against the classified actions)", result.Status, StatusViolated)
	}
	if len(result.Constraints) != 1 || result.Constraints[0].PreceptID != "cst-0001" {
		t.Fatalf("Constraints = %+v, want the spend ceiling constraint's verdict", result.Constraints)
	}
	if result.Constraints[0].WhyNot != spendCeilingWhyNot {
		t.Errorf("WhyNot = %q, want verbatim %q", result.Constraints[0].WhyNot, spendCeilingWhyNot)
	}
	if fp.calls != 1 {
		t.Errorf("provider calls = %d, want 1", fp.calls)
	}
}

// --- acc line 2: a second run hits the cache and writes no spend row ---

func TestMatchFreeText_SecondRunHitsCacheWritesNoSpendRow(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: wireVendorResponse}}
	ledgerDouble := &countingLedger{}
	rtr := newClassifyTestRouter(fp, ledgerDouble)
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	cache := NewMemoryClassifyCache()
	c := NewClassifier(m, "fake-classifier@v1", cache)

	first, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("first MatchFreeText: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider calls after first run = %d, want 1", fp.calls)
	}
	if len(ledgerDouble.entries) != 1 {
		t.Fatalf("spend rows after first run = %d, want 1", len(ledgerDouble.entries))
	}

	second, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("second MatchFreeText: %v", err)
	}
	if fp.calls != 1 {
		t.Errorf("provider calls after second (cached) run = %d, want unchanged at 1", fp.calls)
	}
	if len(ledgerDouble.entries) != 1 {
		t.Errorf("spend rows after second (cached) run = %d, want unchanged at 1 -- a cache hit must write no spend row", len(ledgerDouble.entries))
	}
	if second.Status != first.Status || len(second.MatchedActions) != len(first.MatchedActions) {
		t.Errorf("second run = %+v, want the same verdict/actions as the first (%+v)", second, first)
	}
}

// --- acc line 3: confidence < 0.80 returns verdict unverified ---

func TestMatchFreeText_LowConfidenceReturnsUnverified(t *testing.T) {
	resp := `{"confidence":0.5,"actions":[{"action":"spend_over","params":{"amount":800},"evidence":"wire $800"}]}`
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: resp}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	if result.Status != StatusUnverified {
		t.Fatalf("Status = %q, want %q", result.Status, StatusUnverified)
	}
	if result.Confidence != 0.5 {
		t.Errorf("Confidence = %v, want 0.5", result.Confidence)
	}
	// The low-confidence classification is still surfaced for audit, even
	// though stage 1 never ran on it.
	if len(result.MatchedActions) != 1 {
		t.Errorf("MatchedActions = %+v, want the classified (but untrusted) action preserved", result.MatchedActions)
	}
	if len(result.Constraints) != 0 || result.ConsideredCount != 0 {
		t.Errorf("Result = %+v, want stage 1 to never have run", result.Result)
	}
}

// --- acc line 3: no model available returns verdict unverified ---

func TestMatchFreeText_NilRouterReturnsUnverified(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil) // no router at all, mirrors check_test.go's stage-1-only construction
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	if result.Status != StatusUnverified {
		t.Fatalf("Status = %q, want %q", result.Status, StatusUnverified)
	}
	if len(result.MatchedActions) != 0 {
		t.Errorf("MatchedActions = %+v, want none -- no model was ever consulted", result.MatchedActions)
	}
}

func TestMatchFreeText_NoLocalCheapProviderReturnsUnverified(t *testing.T) {
	// A router that exists but has no provider registered for the
	// classification task class's tier -- distinct from a nil router,
	// exercising router.ErrTierUnavailable rather than the m.Router()==nil
	// short-circuit.
	rtr := router.New(map[router.Tier]router.Provider{}, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	if result.Status != StatusUnverified {
		t.Fatalf("Status = %q, want %q", result.Status, StatusUnverified)
	}
}

// --- closed-action-set enforcement: an action outside domain.ActionSet
// is dropped, not passed through to stage 1 ---

func TestMatchFreeText_RejectsActionOutsideClosedSet(t *testing.T) {
	resp := `{"confidence":0.9,"actions":[` +
		`{"action":"launch_the_missiles","params":{},"evidence":"wire $800"},` +
		`{"action":"spend_over","params":{"amount":800},"evidence":"wire $800"}` +
		`]}`
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: resp}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	if result.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", result.Rejected)
	}
	if len(result.MatchedActions) != 1 || result.MatchedActions[0].Action.Action != "spend_over" {
		t.Fatalf("MatchedActions = %+v, want only the in-vocabulary spend_over action", result.MatchedActions)
	}
	for _, ma := range result.MatchedActions {
		if ma.Action.Action == "launch_the_missiles" {
			t.Fatalf("found out-of-vocabulary action %q -- prompt injection defeated the closed action set", ma.Action.Action)
		}
	}
}

// --- span audit: unlocatable evidence is kept, not silently dropped ---

func TestLocateSpan_UnfoundEvidenceKeepsTextWithZeroOffsets(t *testing.T) {
	span := locateSpan(wireVendorText, "a phrase that never appears")
	if span.Text != "a phrase that never appears" {
		t.Errorf("Text = %q, want the unlocated evidence preserved", span.Text)
	}
	if span.Start != 0 || span.End != 0 {
		t.Errorf("Span = %+v, want zero offsets for unlocated evidence", span)
	}
}

func TestLocateSpan_EmptyEvidenceYieldsZeroSpan(t *testing.T) {
	if got := locateSpan(wireVendorText, "   "); got != (Span{}) {
		t.Errorf("locateSpan(empty) = %+v, want the zero Span", got)
	}
}

// --- model version pinning mismatch is an error, never silently
// overwritten (mirrors internal/extract's identical guarantee) ---

func TestMatchFreeText_ModelVersionMismatchErrors(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v2", resp: router.Response{Text: wireVendorResponse}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	_, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err == nil {
		t.Fatal("MatchFreeText: want an error for a model-version mismatch, got nil")
	}
}

// --- a malformed/unparseable model response fails closed, not open ---

func TestMatchFreeText_UnparseableResponseYieldsNoActionsNotAnError(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: "not json at all"}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	result, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	// confidence 0 (nothing decoded) is below the floor, so this must
	// still surface as unverified, not a bare "pass" hiding the failure.
	if result.Status != StatusUnverified {
		t.Fatalf("Status = %q, want %q", result.Status, StatusUnverified)
	}
	if len(result.MatchedActions) != 0 {
		t.Errorf("MatchedActions = %+v, want none", result.MatchedActions)
	}
}

// --- a router error unrelated to tier availability propagates, not
// silently swallowed into unverified ---

func TestMatchFreeText_GenuineRouterErrorPropagates(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", err: errors.New("boom")}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	c := NewClassifier(m, "fake-classifier@v1", nil)

	_, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{})
	if err == nil {
		t.Fatal("MatchFreeText: want a genuine provider error to propagate, got nil")
	}
	if errors.Is(err, router.ErrTierUnavailable) {
		t.Fatalf("err = %v, must not be mistaken for ErrTierUnavailable", err)
	}
}

// --- cache key sensitivity: a different input text is a fresh
// classification, not a false-positive cache hit ---

func TestMatchFreeText_DifferentTextIsNotACacheHit(t *testing.T) {
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: wireVendorResponse}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)
	cache := NewMemoryClassifyCache()
	c := NewClassifier(m, "fake-classifier@v1", cache)

	if _, err := c.MatchFreeText(context.Background(), wireVendorText, router.Budget{}); err != nil {
		t.Fatalf("first MatchFreeText: %v", err)
	}
	if _, err := c.MatchFreeText(context.Background(), wireVendorText+" please", router.Budget{}); err != nil {
		t.Fatalf("second MatchFreeText: %v", err)
	}
	if fp.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (different text must not share a cache entry)", fp.calls)
	}
}
