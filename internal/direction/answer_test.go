package direction

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
)

// openQuestion builds a fixture question entry directly (bypassing
// CreateDraft, which is decision-only -- a question's state vocabulary is
// open/answered, never staged, so a question reaches the ledger through
// Store.Create with a fully-formed entry, exactly as an already-answered
// import would).
func openQuestion(id, title, body string) *ledger.Entry {
	return &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindQuestion,
		Title:   title,
		State:   ledger.StateOpen,
		Created: "2026-08-28T00:00:00Z",
		Body:    body,
	}
}

func TestAnswerTransitionsOpenQuestionToAnswered(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	q := openQuestion("qst-0001", "Which cloud account pays for the prod deploy?", "Because nobody wrote it down before the freeze.\n")
	if err := s.Create(ctx, q); err != nil {
		t.Fatalf("Create: %v", err)
	}

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	answered, err := s.Answer(ctx, "qst-0001", "david", now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if answered.State != ledger.StateAnswered {
		t.Fatalf("answered.State = %q, want answered", answered.State)
	}
	if answered.ConfirmedBy != "david" {
		t.Fatalf("answered.ConfirmedBy = %q, want david", answered.ConfirmedBy)
	}
	if answered.Updated != "2026-08-28T12:00:00Z" {
		t.Fatalf("answered.Updated = %q, want 2026-08-28T12:00:00Z", answered.Updated)
	}
	// Answer must not touch anything but state/updated/confirmed_by.
	if answered.Title != q.Title || answered.Body != q.Body {
		t.Fatalf("Answer changed fields it must not: %+v", answered)
	}

	path := filepath.Join(root, ".dira", "entries", "qst-0001.md")
	validateAgainstVendoredSchema(t, path)

	got, err := s.Get(ctx, "qst-0001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != ledger.StateAnswered {
		t.Fatalf("re-read State = %q, want answered", got.State)
	}
}

func TestAnswerRejectsNonQuestionKind(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	e := stagedDraft("", "not a question")
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if _, err := s.Answer(ctx, e.ID, "david", time.Now()); err == nil {
		t.Fatal("Answer on a decision entry: want error, got nil")
	}
}

func TestSecondAnswerErrors(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	q := openQuestion("qst-0001", "Do we need a second approver for wires over $10k?", "Raised during the vendor onboarding review.\n")
	if err := s.Create(ctx, q); err != nil {
		t.Fatalf("Create: %v", err)
	}

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	if _, err := s.Answer(ctx, "qst-0001", "david", now); err != nil {
		t.Fatalf("first Answer: %v", err)
	}
	if _, err := s.Answer(ctx, "qst-0001", "david", now); err == nil {
		t.Fatal("second Answer of the same id: want error, got nil")
	}
}
