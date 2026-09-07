package disposition

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCaptureTextStagesADistillItem(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "remember to call the plumber", "", "task", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if item.ID == "" {
		t.Fatal("Capture returned an empty id")
	}
	if item.Kind != KindDistill {
		t.Fatalf("Kind = %q, want %q", item.Kind, KindDistill)
	}
	if item.State != StatePending {
		t.Fatalf("State = %q, want %q", item.State, StatePending)
	}

	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var payload CapturePayload
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Text != "remember to call the plumber" || payload.Hint != "task" {
		t.Fatalf("payload = %+v, want text/hint preserved", payload)
	}
}

func TestCaptureAudioRefStagesADistillItem(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "", "voice-notes/2026-09-06-001.m4a", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var payload CapturePayload
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.AudioRef != "voice-notes/2026-09-06-001.m4a" {
		t.Fatalf("AudioRef = %q, want the given ref", payload.AudioRef)
	}
}

func TestCaptureEmptyErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	_, err := Capture(ctx, s, "", "", "", fixedNow)
	if !errors.Is(err, ErrCaptureEmpty) {
		t.Fatalf("Capture(empty): err = %v, want ErrCaptureEmpty", err)
	}
}

// TestRouteDistillNoteAcceptsWithNoFollowOn and the three routes below are
// the acc-line clause: "dispositions claim-batch/precept-draft/note/trash
// each route the payload correctly."
func TestRouteDistillNoteAcceptsWithNoFollowOn(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "the wifi password is on the fridge", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	res, err := s.RouteDistill(ctx, item.ID, RouteNote, "human:david", "", "", fixedNow)
	if err != nil {
		t.Fatalf("RouteDistill(note): %v", err)
	}
	if res.Item.State != StateDisposed || res.Item.Verdict != VerdictAccept {
		t.Fatalf("State=%q Verdict=%q, want disposed/accept", res.Item.State, res.Item.Verdict)
	}
	if res.Item.Route != RouteNote {
		t.Fatalf("Route = %q, want %q", res.Item.Route, RouteNote)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List returned %d items, want 1 (note must not create a follow-on)", len(all))
	}
}

func TestRouteDistillTrashRejectsWithDefaultNote(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "um, never mind", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	res, err := s.RouteDistill(ctx, item.ID, RouteTrash, "human:david", "", "", fixedNow)
	if err != nil {
		t.Fatalf("RouteDistill(trash): %v", err)
	}
	if res.Item.State != StateDisposed || res.Item.Verdict != VerdictReject {
		t.Fatalf("State=%q Verdict=%q, want disposed/reject", res.Item.State, res.Item.Verdict)
	}
	if res.Item.Route != RouteTrash {
		t.Fatalf("Route = %q, want %q", res.Item.Route, RouteTrash)
	}
	if res.Item.Note == "" {
		t.Fatal("trash route left Note empty -- Dispose's reject-requires-a-note rule must still be satisfied")
	}
}

func TestRouteDistillClaimBatchAcceptsWithNoFollowOnItem(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "spent $40 on groceries at Trader Joe's", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	res, err := s.RouteDistill(ctx, item.ID, RouteClaimBatch, "human:david", "", "", fixedNow)
	if err != nil {
		t.Fatalf("RouteDistill(claim-batch): %v", err)
	}
	if res.Item.State != StateDisposed || res.Item.Verdict != VerdictAccept {
		t.Fatalf("State=%q Verdict=%q, want disposed/accept", res.Item.State, res.Item.Verdict)
	}
	if res.Item.Route != RouteClaimBatch {
		t.Fatalf("Route = %q, want %q", res.Item.Route, RouteClaimBatch)
	}
	// claim-batch's downstream extraction wiring is explicitly out of
	// scope (see RouteClaimBatch's doc) -- no follow-on item exists yet.
	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List returned %d items, want 1", len(all))
	}
}

// TestRouteDistillPreceptDraftCreatesDispositionItemNeverDira is the
// acc-line's explicit parenthetical: "precept-draft creates a disposition
// item, never a .dira entry."
func TestRouteDistillPreceptDraftCreatesDispositionItemNeverDira(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "I've decided: never accept meetings before 10am", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	res, err := s.RouteDistill(ctx, item.ID, RoutePreceptDraft, "human:david", "", "", fixedNow)
	if err != nil {
		t.Fatalf("RouteDistill(precept-draft): %v", err)
	}
	if res.Item.State != StateDisposed || res.Item.Verdict != VerdictAccept {
		t.Fatalf("State=%q Verdict=%q, want disposed/accept", res.Item.State, res.Item.Verdict)
	}
	if res.Item.Route != RoutePreceptDraft {
		t.Fatalf("Route = %q, want %q", res.Item.Route, RoutePreceptDraft)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d items, want 2 (the original distill item plus a new precept_draft item)", len(all))
	}
	var draft *Item
	for i := range all {
		if all[i].ID != item.ID {
			draft = &all[i]
		}
	}
	if draft == nil {
		t.Fatal("no follow-on item found")
	}
	if draft.Kind != KindPreceptDraft {
		t.Fatalf("follow-on Kind = %q, want %q", draft.Kind, KindPreceptDraft)
	}
	if draft.State != StatePending {
		t.Fatalf("follow-on State = %q, want %q (a disposition item, awaiting its OWN disposal)", draft.State, StatePending)
	}
	if string(draft.Payload) != string(item.Payload) {
		t.Fatalf("follow-on Payload = %q, want the original capture's payload %q", draft.Payload, item.Payload)
	}
}

func TestRouteDistillInvalidRouteErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "text", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	_, err = s.RouteDistill(ctx, item.ID, DistillRoute("archive"), "human:david", "", "", fixedNow)
	if !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("RouteDistill(bad route): err = %v, want ErrInvalidRoute", err)
	}
}

func TestRouteDistillOnNonDistillItemErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindEffect, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = s.RouteDistill(ctx, item.ID, RouteNote, "human:david", "", "", fixedNow)
	if !errors.Is(err, ErrNotDistill) {
		t.Fatalf("RouteDistill on a non-distill item: err = %v, want ErrNotDistill", err)
	}
}

// TestRouteDistillIdempotentRetryDoesNotDuplicatePreceptDraft proves the
// precept-draft follow-on is created exactly once even under a retried
// idempotency_key, inheriting Dispose's own replay guarantee.
func TestRouteDistillIdempotentRetryDoesNotDuplicatePreceptDraft(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := Capture(ctx, s, "decision text", "", "", fixedNow)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := s.RouteDistill(ctx, item.ID, RoutePreceptDraft, "human:david", "", "idem-key", fixedNow); err != nil {
		t.Fatalf("RouteDistill (first): %v", err)
	}
	res, err := s.RouteDistill(ctx, item.ID, RoutePreceptDraft, "human:david", "", "idem-key", fixedNow)
	if err != nil {
		t.Fatalf("RouteDistill (retry): %v", err)
	}
	if !res.Replayed {
		t.Fatal("retry with the same idempotency_key: Replayed = false, want true")
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d items, want 2 (retry must not create a second precept_draft)", len(all))
	}
}
