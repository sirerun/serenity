package check

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/router"
)

// fakeStore is a minimal in-memory ledger.Store for this package's tests.
// Create/Put/Delete are unused by Match (which only Lists and Gets) and
// exist solely to satisfy the interface.
type fakeStore struct {
	entries map[string]*ledger.Entry
}

func newFakeStore(entries ...*ledger.Entry) *fakeStore {
	s := &fakeStore{entries: make(map[string]*ledger.Entry, len(entries))}
	for _, e := range entries {
		s.entries[e.ID] = e
	}
	return s
}

func (s *fakeStore) Get(_ context.Context, id string) (*ledger.Entry, error) {
	e, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ledger.ErrNotFound, id)
	}
	return e, nil
}

func (s *fakeStore) List(_ context.Context) ([]ledger.EntryInfo, error) {
	infos := make([]ledger.EntryInfo, 0, len(s.entries))
	for id := range s.entries {
		infos = append(infos, ledger.EntryInfo{ID: id})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })
	return infos, nil
}

func (s *fakeStore) Create(_ context.Context, _ *ledger.Entry) error {
	return errors.New("fakeStore: Create not supported")
}

func (s *fakeStore) Put(_ context.Context, _ *ledger.Entry) error {
	return errors.New("fakeStore: Put not supported")
}

func (s *fakeStore) Delete(_ context.Context, _ string) error {
	return errors.New("fakeStore: Delete not supported")
}

var _ ledger.Store = (*fakeStore)(nil)

// constraintEntry builds a fixture constraint entry directly (bypassing
// ledger.Encode/Decode, which T3.2/T3.3 already cover): Match reads Kind,
// State, Body and Alternatives straight off the struct.
func constraintEntry(id string, state ledger.State, body string, alts ...ledger.Alternative) *ledger.Entry {
	return &ledger.Entry{
		ID:           id,
		Kind:         ledger.KindConstraint,
		Title:        "fixture constraint " + id,
		State:        state,
		Created:      "2026-08-28T00:00:00Z",
		Alternatives: alts,
		Body:         body,
	}
}

func appliesWhenBody(prose, action, params string) string {
	body := prose + "\n\n```serenity:applies_when\naction: " + action + "\n"
	if params != "" {
		body += "params: " + params + "\n"
	}
	body += "```\n"
	return body
}

const spendCeilingWhyNot = "Unbounded spend risk: 'no ceiling' was rejected outright."
const spendCeilingRevisit = "quarterly budget review"

func spendCeilingConstraint(id string, state ledger.State) *ledger.Entry {
	body := appliesWhenBody("Spend above the ceiling requires asking first.",
		"spend_over", "{amount: {gte: 200}}")
	return constraintEntry(id, state, body, ledger.Alternative{
		Option:    "no ceiling",
		WhyNot:    spendCeilingWhyNot,
		RevisitIf: spendCeilingRevisit,
	})
}

func deployFreezeConstraint(id string, state ledger.State) *ledger.Entry {
	body := appliesWhenBody("Deploys to prod are blocked outright this quarter.",
		"deploy_to_prod", "")
	return constraintEntry(id, state, body, ledger.Alternative{
		Option: "deploy freely",
		WhyNot: "prod incidents doubled last quarter",
	})
}

func noClauseConstraint(id string, state ledger.State) *ledger.Entry {
	return constraintEntry(id, state, "This constraint has no machine-checkable clause yet.\n")
}

// --- acc line 1: violated(precept_id, why_not, revisit_if), why_not verbatim ---

func TestMatch_Violated(t *testing.T) {
	store := newFakeStore(
		spendCeilingConstraint("cst-0001", ledger.StateActive),
		deployFreezeConstraint("cst-0002", ledger.StateActive),
		noClauseConstraint("cst-0004", ledger.StateActive),
		spendCeilingConstraint("cst-0003", ledger.StateSuperseded), // must be ignored
	)
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 500}},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q", result.Status, StatusViolated)
	}
	if len(result.Constraints) != 1 {
		t.Fatalf("Constraints = %+v, want exactly one verdict", result.Constraints)
	}
	got := result.Constraints[0]
	if got.PreceptID != "cst-0001" {
		t.Errorf("PreceptID = %q, want cst-0001", got.PreceptID)
	}
	if got.Outcome != OutcomeViolated {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeViolated)
	}
	if got.WhyNot != spendCeilingWhyNot {
		t.Errorf("WhyNot = %q, want verbatim %q", got.WhyNot, spendCeilingWhyNot)
	}
	if got.RevisitIf != spendCeilingRevisit {
		t.Errorf("RevisitIf = %q, want %q", got.RevisitIf, spendCeilingRevisit)
	}
	// Active, non-applicable constraints (deploy_to_prod, no-clause) and the
	// superseded spend ceiling all count toward ConsideredCount but never
	// appear in Constraints.
	if result.ConsideredCount != 3 {
		t.Errorf("ConsideredCount = %d, want 3 (cst-0001, cst-0002, cst-0004; cst-0003 is superseded)", result.ConsideredCount)
	}
}

// --- acc line 2: an allowed action yields pass ---

func TestMatch_AllowedActionPasses(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 100}},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want %q", result.Status, StatusPass)
	}
	if len(result.Constraints) != 1 || result.Constraints[0].Outcome != OutcomePass {
		t.Fatalf("Constraints = %+v, want one pass verdict", result.Constraints)
	}
	if result.Constraints[0].WhyNot != "" || result.Constraints[0].RevisitIf != "" {
		t.Errorf("a pass verdict must not carry why_not/revisit_if, got %+v", result.Constraints[0])
	}
}

// --- acc line 3: a plan matching no constraint yields no_applicable_constraints ---

func TestMatch_NoApplicableConstraints(t *testing.T) {
	store := newFakeStore(
		spendCeilingConstraint("cst-0001", ledger.StateActive),
		deployFreezeConstraint("cst-0002", ledger.StateActive),
	)
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{
		{Action: "start_project", Params: nil},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusNoApplicableConstraints {
		t.Fatalf("Status = %q, want %q", result.Status, StatusNoApplicableConstraints)
	}
	if len(result.Constraints) != 0 {
		t.Errorf("Constraints = %+v, want none", result.Constraints)
	}
	if result.ConsideredCount != 2 {
		t.Errorf("ConsideredCount = %d, want 2", result.ConsideredCount)
	}
}

func TestMatch_NoActionsYieldsNoApplicableConstraints(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	result, err := m.Match(context.Background(), nil)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusNoApplicableConstraints {
		t.Fatalf("Status = %q, want %q", result.Status, StatusNoApplicableConstraints)
	}
	if result.ConsideredCount != 1 {
		t.Errorf("ConsideredCount = %d, want 1", result.ConsideredCount)
	}
}

// --- acc line 4: a fake router that panics proves zero model calls ---

type panicProvider struct{}

func (panicProvider) Name() string         { return "panic-provider" }
func (panicProvider) ModelVersion() string { return "panic@v0" }
func (panicProvider) Send(context.Context, string) (router.Response, error) {
	panic("check: stage 1 must never call a model")
}

type panicSpendLedger struct{}

func (panicSpendLedger) Record(context.Context, router.SpendEntry) error {
	panic("check: stage 1 must never call a model")
}

func TestMatch_NeverCallsRouter(t *testing.T) {
	rtr := router.New(map[router.Tier]router.Provider{
		router.TierLocalCheap: panicProvider{},
		router.TierJudgment:   panicProvider{},
	}, panicSpendLedger{})

	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, rtr)

	// Exercise a genuine violation -- the path most tempting to "double
	// check" against a model -- through a Matcher that holds a router
	// wired to panic on Send or on a spend-ledger write. If Match ever
	// called rtr.Complete, this test would fail via an uncaught panic
	// rather than a soft assertion.
	result, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 500}},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q", result.Status, StatusViolated)
	}
	if got := m.Router(); got != rtr {
		t.Errorf("Router() = %p, want the router passed to New (%p)", got, rtr)
	}
}

// --- input validation: an action outside domain.ActionSet is rejected ---

func TestMatch_UnknownActionRejected(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	_, err := m.Match(context.Background(), []Action{{Action: "launch_the_missiles"}})
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
	if !errors.Is(err, direction.ErrUnknownAction) {
		t.Fatalf("err = %v, want it to also satisfy direction.ErrUnknownAction", err)
	}
}

// --- params matching semantics beyond the acc line's headline case ---

func TestMatch_MissingActionParamDoesNotTrigger(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	// spend_over with no amount at all: the constraint is applicable (the
	// action type matches) but its condition cannot be evaluated as true,
	// so it must not silently become a violation.
	result, err := m.Match(context.Background(), []Action{{Action: "spend_over", Params: nil}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want %q", result.Status, StatusPass)
	}
}

func TestMatch_LiteralParamEquality(t *testing.T) {
	body := appliesWhenBody("Only USD spend needs asking.", "spend_over", "{currency: usd}")
	entry := constraintEntry("cst-0005", ledger.StateActive, body, ledger.Alternative{
		Option: "any currency", WhyNot: "USD is the only ledger currency we reconcile",
	})
	store := newFakeStore(entry)
	m := New(store, nil)

	violates, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"currency": "usd"}},
	})
	if err != nil {
		t.Fatalf("Match(usd): %v", err)
	}
	if violates.Status != StatusViolated {
		t.Errorf("Status(usd) = %q, want %q", violates.Status, StatusViolated)
	}

	passes, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"currency": "eur"}},
	})
	if err != nil {
		t.Fatalf("Match(eur): %v", err)
	}
	if passes.Status != StatusPass {
		t.Errorf("Status(eur) = %q, want %q", passes.Status, StatusPass)
	}
}

func TestMatch_MultipleActionsOneTriggersOverallViolated(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 50}},
		{Action: "spend_over", Params: map[string]any{"amount": 9000}},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q", result.Status, StatusViolated)
	}
	if len(result.Constraints) != 1 || result.Constraints[0].Outcome != OutcomeViolated {
		t.Fatalf("Constraints = %+v, want a single violated verdict", result.Constraints)
	}
}

func TestMatch_ViolatedWithoutAlternativesLeavesWhyNotEmpty(t *testing.T) {
	body := appliesWhenBody("Blocked, no alternative recorded yet.", "deploy_to_prod", "")
	entry := constraintEntry("cst-0006", ledger.StateActive, body) // no alternatives
	store := newFakeStore(entry)
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusViolated {
		t.Fatalf("Status = %q, want %q", result.Status, StatusViolated)
	}
	got := result.Constraints[0]
	if got.WhyNot != "" || got.RevisitIf != "" {
		t.Errorf("expected empty WhyNot/RevisitIf with no alternatives recorded, got %+v", got)
	}
}

// --- unevaluable conditions on an active constraint error rather than pass ---

func TestMatch_UnknownComparisonOperatorErrors(t *testing.T) {
	body := appliesWhenBody("Malformed clause.", "spend_over", "{amount: {bogus_op: 200}}")
	entry := constraintEntry("cst-0007", ledger.StateActive, body)
	store := newFakeStore(entry)
	m := New(store, nil)

	_, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 500}},
	})
	if err == nil {
		t.Fatal("Match: want an error for an unknown comparison operator, got nil")
	}
}

func TestMatch_NumericOperatorOnNonNumericActionValueErrors(t *testing.T) {
	store := newFakeStore(spendCeilingConstraint("cst-0001", ledger.StateActive))
	m := New(store, nil)

	_, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": "a great deal of money"}},
	})
	if err == nil {
		t.Fatal("Match: want an error for a non-numeric action value against a numeric operator, got nil")
	}
}

// --- entries this stage must never surface as constraints ---

func TestMatch_IgnoresNonConstraintAndInactiveEntries(t *testing.T) {
	intent := &ledger.Entry{
		ID:      "int-0001",
		Kind:    ledger.KindIntent,
		Title:   "an intent, not a constraint",
		State:   ledger.StateActive,
		Created: "2026-08-28T00:00:00Z",
		Body: appliesWhenBody("Intents are never machine-checkable constraints.",
			"spend_over", "{amount: {gte: 1}}"),
	}
	store := newFakeStore(intent, spendCeilingConstraint("cst-0001", ledger.StateSuperseded))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{
		{Action: "spend_over", Params: map[string]any{"amount": 500}},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result.Status != StatusNoApplicableConstraints {
		t.Fatalf("Status = %q, want %q", result.Status, StatusNoApplicableConstraints)
	}
	if result.ConsideredCount != 0 {
		t.Errorf("ConsideredCount = %d, want 0 (no active constraint entries)", result.ConsideredCount)
	}
}
