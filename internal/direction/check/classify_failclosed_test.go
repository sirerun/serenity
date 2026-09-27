package check

import (
	"context"
	"strings"
	"testing"

	"github.com/kazi-org/dira/ledger"
	"github.com/sirerun/serenity/internal/router"
)

// These tests pin AI-06 (deep review 001): the free-text classifier must
// fail closed. Every case below is a classifier output that previously
// fell through to a stage-1 verdict (no_applicable_constraints or pass)
// and must now be StatusUnverified.

// classifyOnce runs one MatchFreeText call against the spend-ceiling
// fixture with the given canned model response.
func classifyOnce(t *testing.T, planText, modelResponse string) FreeTextResult {
	t.Helper()
	fp := &fakeClassifyProvider{modelVersion: "fake-classifier@v1", resp: router.Response{Text: modelResponse}}
	rtr := newClassifyTestRouter(fp, &countingLedger{})
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	c := NewClassifier(New(store, rtr), "fake-classifier@v1", nil)
	result, err := c.MatchFreeText(context.Background(), planText, router.Budget{})
	if err != nil {
		t.Fatalf("MatchFreeText: %v", err)
	}
	return result
}

// assertUnverified fails unless result is StatusUnverified, carries a
// Reason containing wantReason, and stage 1 never ran.
func assertUnverified(t *testing.T, result FreeTextResult, wantReason string) {
	t.Helper()
	if result.Status != StatusUnverified {
		t.Fatalf("Status = %q, want %q", result.Status, StatusUnverified)
	}
	if !strings.Contains(result.Reason, wantReason) {
		t.Errorf("Reason = %q, want it to contain %q", result.Reason, wantReason)
	}
	if len(result.Constraints) != 0 || result.ConsideredCount != 0 {
		t.Errorf("Result = %+v, want stage 1 never to have run", result.Result)
	}
}

func TestMatchFreeText_EmptyActionListIsUnverified(t *testing.T) {
	result := classifyOnce(t, wireVendorText, `{"confidence":0.9,"actions":[]}`)
	assertUnverified(t, result, `no actions`)
}

func TestMatchFreeText_BelowThresholdConfidenceIsUnverified(t *testing.T) {
	resp := `{"confidence":0.79,"actions":[{"action":"spend_over","params":{"amount":800},"evidence":"wire $800"}]}`
	result := classifyOnce(t, wireVendorText, resp)
	assertUnverified(t, result, `below the 0.80 floor`)
}

func TestMatchFreeText_OutOfSetActionIsUnverified(t *testing.T) {
	resp := `{"confidence":0.9,"actions":[{"action":"launch_the_missiles","params":{},"evidence":"wire $800"}]}`
	result := classifyOnce(t, wireVendorText, resp)
	assertUnverified(t, result, `outside the closed action set`)
	if result.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", result.Rejected)
	}
}

func TestMatchFreeText_OutOfSetActionAlongsideValidOneIsUnverified(t *testing.T) {
	// A single dropped action poisons the whole classification: the model
	// reported something the closed set cannot express, so the remaining
	// actions are not a complete account of the plan.
	resp := `{"confidence":0.9,"actions":[` +
		`{"action":"launch_the_missiles","params":{},"evidence":"the new vendor"},` +
		`{"action":"spend_over","params":{"amount":100},"evidence":"wire $800"}` +
		`]}`
	result := classifyOnce(t, wireVendorText, resp)
	assertUnverified(t, result, `outside the closed action set`)
}

func TestMatchFreeText_EvidenceAbsentFromPlanIsUnverified(t *testing.T) {
	resp := `{"confidence":0.9,"actions":[{"action":"spend_over","params":{"amount":800},"evidence":"a phrase that never appears"}]}`
	result := classifyOnce(t, wireVendorText, resp)
	assertUnverified(t, result, `"a phrase that never appears"`)
	if len(result.MissingEvidence) != 1 || result.MissingEvidence[0].Action.Action != "spend_over" {
		t.Errorf("MissingEvidence = %+v, want the spend_over action named", result.MissingEvidence)
	}
}

func TestMatchFreeText_EmptyEvidenceIsUnverified(t *testing.T) {
	resp := `{"confidence":0.9,"actions":[{"action":"spend_over","params":{"amount":800},"evidence":""}]}`
	result := classifyOnce(t, wireVendorText, resp)
	assertUnverified(t, result, `spend_over (no evidence cited)`)
}

// TestMatchFreeText_ForgedAmountReproductionIsUnverified is the deep
// review's reproduction: the plan wires $800 (over the 200 ceiling), the
// model reports spend_over{amount:50} citing fabricated evidence
// "wire $50". Before AI-06 this yielded "pass".
func TestMatchFreeText_ForgedAmountReproductionIsUnverified(t *testing.T) {
	resp := `{"confidence":0.95,"actions":[{"action":"spend_over","params":{"amount":50},"evidence":"wire $50"}]}`
	result := classifyOnce(t, wireVendorText, resp)
	if result.Status == StatusPass {
		t.Fatalf("Status = pass: fabricated evidence with a forged amount must never pass")
	}
	assertUnverified(t, result, `spend_over "wire $50"`)
}

// --- golden cases that must keep producing a stage-1 verdict ---

func TestMatchFreeText_GoldenPassWithVerbatimEvidence(t *testing.T) {
	const text = "wire $100 to the new vendor tomorrow"
	resp := `{"confidence":0.9,"actions":[{"action":"spend_over","params":{"amount":100},"evidence":"wire $100"}]}`
	result := classifyOnce(t, text, resp)
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want %q", result.Status, StatusPass)
	}
	if len(result.Constraints) != 1 || result.Constraints[0].Outcome != OutcomePass {
		t.Errorf("Constraints = %+v, want one passing spend-ceiling verdict", result.Constraints)
	}
}

func TestMatchFreeText_EvidenceMatchesUnderNormalizedWhitespace(t *testing.T) {
	// The model collapsed a line break and a double space when quoting;
	// the words are the plan's own, so the evidence is verified.
	const text = "wire  $800\nto the new vendor tomorrow"
	resp := `{"confidence":0.9,"actions":[{"action":"spend_over","params":{"amount":800},"evidence":"wire $800 to the new vendor"}]}`
	result := classifyOnce(t, text, resp)
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q (reason %q)", result.Status, StatusViolated, result.Reason)
	}
	if result.Reason != "" || len(result.MissingEvidence) != 0 {
		t.Errorf("Reason = %q, MissingEvidence = %+v, want both empty on a verified classification", result.Reason, result.MissingEvidence)
	}
}
