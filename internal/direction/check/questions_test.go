package check

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/writer"
)

// questionEntry builds a fixture question entry directly, mirroring
// constraintEntry above.
func questionEntry(id string, state ledger.State, body string) *ledger.Entry {
	return &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindQuestion,
		Title:   "fixture question " + id,
		State:   state,
		Created: "2026-08-28T00:00:00Z",
		Body:    body,
	}
}

// --- T3.8 acc: a question entry targeting deploy_to_prod makes check_plan
// return a blocking warning for that action ---

func TestMatchQuestions_OpenQuestionBlocksTargetAction(t *testing.T) {
	body := appliesWhenBody("Who owns the rollback runbook for this service?", "deploy_to_prod", "")
	store := newFakeStore(questionEntry("qst-0001", ledger.StateOpen, body))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	// A blocking question is a warning, never a Status change: no
	// constraint is in play here, so this must still read
	// no_applicable_constraints, not pass or violated.
	if result.Status != StatusNoApplicableConstraints {
		t.Fatalf("Status = %q, want %q (a question warning must not change the schema verdict)", result.Status, StatusNoApplicableConstraints)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want exactly one", result.Warnings)
	}
	got := result.Warnings[0]
	if got.PreceptID != "qst-0001" {
		t.Errorf("PreceptID = %q, want qst-0001", got.PreceptID)
	}
	if got.Action != "deploy_to_prod" {
		t.Errorf("Action = %q, want deploy_to_prod", got.Action)
	}
	if got.Title != "fixture question qst-0001" {
		t.Errorf("Title = %q, want the entry's title verbatim", got.Title)
	}
}

func TestMatchQuestions_AnsweredQuestionDoesNotBlock(t *testing.T) {
	body := appliesWhenBody("Who owns the rollback runbook for this service?", "deploy_to_prod", "")
	store := newFakeStore(questionEntry("qst-0001", ledger.StateAnswered, body))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none once the question is answered", result.Warnings)
	}
}

func TestMatchQuestions_ActionMismatchNoWarning(t *testing.T) {
	body := appliesWhenBody("Who owns the rollback runbook for this service?", "deploy_to_prod", "")
	store := newFakeStore(questionEntry("qst-0001", ledger.StateOpen, body))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{{Action: "start_project"}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none for an unrelated action", result.Warnings)
	}
}

func TestMatchQuestions_NoAppliesWhenBlockIsSkipped(t *testing.T) {
	store := newFakeStore(questionEntry("qst-0001", ledger.StateOpen, "Not yet machine-checkable.\n"))
	m := New(store, nil)

	result, err := m.Match(context.Background(), []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none for a question with no applies_when block", result.Warnings)
	}
}

func TestMatchQuestions_ParamsClauseScopesTheBlock(t *testing.T) {
	body := appliesWhenBody("Do we need a second approver for this wire?", "spend_over", "{amount: {gte: 1000}}")
	store := newFakeStore(questionEntry("qst-0001", ledger.StateOpen, body))
	m := New(store, nil)

	below, err := m.Match(context.Background(), []Action{{Action: "spend_over", Params: map[string]any{"amount": 500}}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(below.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none below the question's own threshold", below.Warnings)
	}

	above, err := m.Match(context.Background(), []Action{{Action: "spend_over", Params: map[string]any{"amount": 5000}}})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(above.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want exactly one above the question's own threshold", above.Warnings)
	}
}

func TestMatchQuestions_MalformedAppliesWhenErrors(t *testing.T) {
	body := appliesWhenBody("Broken clause.", "not_a_real_action", "")
	store := newFakeStore(questionEntry("qst-0001", ledger.StateOpen, body))
	m := New(store, nil)

	_, err := m.Match(context.Background(), []Action{{Action: "deploy_to_prod"}})
	if err == nil {
		t.Fatal("Match with a question carrying an unparseable applies_when block: want error, got nil")
	}
	if !errors.Is(err, direction.ErrUnknownAction) {
		t.Errorf("err = %v, want wrapping direction.ErrUnknownAction", err)
	}
}

// --- T3.8 acc: answering the question clears the block ---
//
// This exercises the real T3.3 Store and T3.8's Store.Answer end to end
// (not fakeStore), so the acc's "answering... clears the block" is proven
// against the actual writer-queue-backed lifecycle a caller would use, not
// just against a struct field flipped by hand in the test.
func TestMatchQuestions_AnsweringClearsTheBlock(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	store := direction.NewStore(root, q)

	body := appliesWhenBody("Who signs off on this prod deploy?", "deploy_to_prod", "")
	entry := &ledger.Entry{
		ID:      "qst-0001",
		Kind:    ledger.KindQuestion,
		Title:   "Who signs off on this prod deploy?",
		State:   ledger.StateOpen,
		Created: "2026-08-28T00:00:00Z",
		Body:    body,
	}
	if err := store.Create(ctx, entry); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m := New(store, nil)
	before, err := m.Match(ctx, []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match before answering: %v", err)
	}
	if len(before.Warnings) != 1 || before.Warnings[0].PreceptID != "qst-0001" {
		t.Fatalf("before.Warnings = %+v, want one warning for qst-0001", before.Warnings)
	}

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	if _, err := store.Answer(ctx, "qst-0001", "david", now); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	after, err := m.Match(ctx, []Action{{Action: "deploy_to_prod"}})
	if err != nil {
		t.Fatalf("Match after answering: %v", err)
	}
	if len(after.Warnings) != 0 {
		t.Fatalf("after.Warnings = %+v, want none once the question is answered", after.Warnings)
	}
}
