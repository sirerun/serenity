package direction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
)

var preceptDraftFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func disposedPreceptDraftItem(t *testing.T, verdict disposition.Verdict, payload, editedPayload PreceptDraftPayload) disposition.Item {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	item := disposition.Item{
		ID:      "item-1",
		Kind:    disposition.KindPreceptDraft,
		State:   disposition.StateDisposed,
		Verdict: verdict,
		Actor:   "human:test",
		Payload: raw,
	}
	if verdict == disposition.VerdictEditAccept {
		editedRaw, err := json.Marshal(editedPayload)
		if err != nil {
			t.Fatalf("marshal edited payload: %v", err)
		}
		item.EditedPayload = editedRaw
	}
	return item
}

func fixturePayload() PreceptDraftPayload {
	return PreceptDraftPayload{
		QuestionID: "spend-threshold",
		Question:   "Above what dollar amount do you want to be asked before money is spent on your behalf?",
		Answer:     "Ask me before spending more than 200 dollars on anything not already budgeted.",
		Title:      "Ask before any unbudgeted spend over $200",
		Body:       "The interview answer named a $200 threshold for unbudgeted spending; anything above it needs a disposition before it happens.",
		Alternatives: []domain.RejectedAlternative{
			{Option: "Do not adopt this precept", WhyNot: "Leaving spend unbounded risks silent overruns, which the interview answer explicitly wanted to avoid.", RevisitIf: "the $200 threshold stops matching actual spend patterns"},
		},
	}
}

// TestApplyDisposedPreceptDraftWritesAcceptedEntryWithFloorAlternative is
// this task's own acc line: "accepting a draft writes a valid dira entry
// with the 'do not adopt this' alternative as the floor". It checks both
// halves -- the entry Confirm returns, and that the file on disk
// validates against the real vendored entry.schema.json, not just this
// package's own Go-side mirror of it.
func TestApplyDisposedPreceptDraftWritesAcceptedEntryWithFloorAlternative(t *testing.T) {
	s, _ := newTestStore(t)
	item := disposedPreceptDraftItem(t, disposition.VerdictAccept, fixturePayload(), PreceptDraftPayload{})

	entry, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedPreceptDraft: %v", err)
	}

	if entry.Kind != ledger.KindDecision {
		t.Errorf("entry.Kind = %q, want %q", entry.Kind, ledger.KindDecision)
	}
	if entry.State != ledger.StateAccepted {
		t.Errorf("entry.State = %q, want %q", entry.State, ledger.StateAccepted)
	}
	if entry.Title != "Ask before any unbudgeted spend over $200" {
		t.Errorf("entry.Title = %q, want the payload's title", entry.Title)
	}
	if entry.ConfirmedBy != "human:test" {
		t.Errorf("entry.ConfirmedBy = %q, want item.Actor (%q)", entry.ConfirmedBy, "human:test")
	}
	if len(entry.Alternatives) != 1 {
		t.Fatalf("len(entry.Alternatives) = %d, want 1", len(entry.Alternatives))
	}
	alt := entry.Alternatives[0]
	if alt.Option != "Do not adopt this precept" {
		t.Errorf("Alternatives[0].Option = %q, want the do-not-adopt floor", alt.Option)
	}
	if alt.WhyNot == "" {
		t.Error("Alternatives[0].WhyNot is empty, want the model's reasoning carried through")
	}

	// The entry was written twice on disk (CreateDraft, then Confirm) --
	// confirm what's actually there now is the accepted version, and
	// that it validates against the real vendored schema.
	got, err := s.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get %s: %v", entry.ID, err)
	}
	if got.State != ledger.StateAccepted {
		t.Errorf("re-read entry.State = %q, want %q", got.State, ledger.StateAccepted)
	}
	validateAgainstVendoredSchema(t, s.PathFor(entry.ID))
}

func TestApplyDisposedPreceptDraftRejectsWrongKind(t *testing.T) {
	s, _ := newTestStore(t)
	item := disposedPreceptDraftItem(t, disposition.VerdictAccept, fixturePayload(), PreceptDraftPayload{})
	item.Kind = disposition.KindReconcile

	if _, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow); err == nil {
		t.Fatal("ApplyDisposedPreceptDraft: want error for wrong Kind, got nil")
	}
}

func TestApplyDisposedPreceptDraftRejectsNotYetDisposed(t *testing.T) {
	s, _ := newTestStore(t)
	item := disposedPreceptDraftItem(t, disposition.VerdictAccept, fixturePayload(), PreceptDraftPayload{})
	item.State = disposition.StatePending
	item.Verdict = ""

	if _, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow); err == nil {
		t.Fatal("ApplyDisposedPreceptDraft: want error for a not-yet-disposed item, got nil")
	}
}

func TestApplyDisposedPreceptDraftRejectsRejectedVerdict(t *testing.T) {
	s, _ := newTestStore(t)
	item := disposedPreceptDraftItem(t, disposition.VerdictReject, fixturePayload(), PreceptDraftPayload{})

	if _, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow); err == nil {
		t.Fatal("ApplyDisposedPreceptDraft: want error for verdict=reject, got nil")
	}
	if infos, err := s.List(context.Background()); err != nil || len(infos) != 0 {
		t.Fatalf("s.List = %v (err %v), want no entries written for a rejected item", infos, err)
	}
}

func TestApplyDisposedPreceptDraftRejectsEmptyAlternatives(t *testing.T) {
	s, _ := newTestStore(t)
	payload := fixturePayload()
	payload.Alternatives = nil
	item := disposedPreceptDraftItem(t, disposition.VerdictAccept, payload, PreceptDraftPayload{})

	_, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow)
	if err == nil {
		t.Fatal("ApplyDisposedPreceptDraft: want error for a payload with no alternatives, got nil")
	}
}

// TestApplyDisposedPreceptDraftHonorsEditedPayloadOnEditAccept proves
// edit_accept, when it reaches this method, uses EditedPayload over
// Payload -- `serenity inbox` does not yet offer edit_accept for
// KindPreceptDraft (its 'e' key is reconcile-only today), so this
// exercises this method's own documented contract for a future caller,
// not current CLI behavior.
func TestApplyDisposedPreceptDraftHonorsEditedPayloadOnEditAccept(t *testing.T) {
	s, _ := newTestStore(t)
	original := fixturePayload()
	edited := fixturePayload()
	edited.Title = "A human-edited title"
	edited.Alternatives = []domain.RejectedAlternative{{Option: "Do not adopt this precept", WhyNot: "edited reasoning"}}

	item := disposedPreceptDraftItem(t, disposition.VerdictEditAccept, original, edited)

	entry, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedPreceptDraft: %v", err)
	}
	if entry.Title != "A human-edited title" {
		t.Errorf("entry.Title = %q, want the edited payload's title", entry.Title)
	}
}

func TestApplyDisposedPreceptDraftReturnsErrorOnReadOnlyStore(t *testing.T) {
	s := NewStore(t.TempDir(), nil)
	item := disposedPreceptDraftItem(t, disposition.VerdictAccept, fixturePayload(), PreceptDraftPayload{})

	_, err := s.ApplyDisposedPreceptDraft(context.Background(), item, preceptDraftFixedNow)
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("ApplyDisposedPreceptDraft: err = %v, want it to wrap ErrReadOnly", err)
	}
}
