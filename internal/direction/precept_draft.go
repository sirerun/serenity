package direction

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
)

// PreceptDraftPayload is the JSON payload of one disposition.KindPreceptDraft
// item, staged by internal/direction/interview.Run from one interview
// question/answer pair and its judgment-tier draft synthesis. Nothing is
// written to the ledger until a human accepts it -- ApplyDisposedPreceptDraft
// below is the only path that turns this into a real .dira decision entry,
// mirroring DecomposePayload/ApplyDisposedDecompose's own shape (T3.11).
//
// Alternatives always carries at least the interview's "do not adopt this"
// floor (domain.RejectedAlternative, the same JSON-tagged type DIRECTION's
// wire uses for a precept's why_not/revisit_if, RFC 0001 section 8.3) --
// interview.Run seeds it from the model's own stated reasoning rather than
// leaving it for Confirm to invent, which Confirm's own doc comment
// explains it must never do.
type PreceptDraftPayload struct {
	QuestionID   string                       `json:"question_id"`
	Question     string                       `json:"question"`
	Answer       string                       `json:"answer"`
	Title        string                       `json:"title"`
	Body         string                       `json:"body"`
	Alternatives []domain.RejectedAlternative `json:"alternatives"`
}

// ApplyDisposedPreceptDraft writes one accepted KindPreceptDraft item as a
// real ledger entry -- KindDecision, carrying the payload's alternatives
// (this task's own acc line: "accepting a draft writes a valid dira entry
// with the 'do not adopt this' alternative as the floor").
//
// It goes through the same two calls a human confirming a staged decision
// by hand would: CreateDraft writes it StateStaged first, then Confirm
// (whose own doc comment names this task directly) transitions it to
// StateAccepted, stamping ConfirmedBy from item.Actor -- the identity that
// actually disposed the item (`serenity inbox`'s currentActor()), not a
// second identity this method invents. Confirm's own alternatives check
// (len > 0) is what would catch a payload bug here; this method also
// checks it itself first, so a malformed payload fails with a message
// naming the actual problem rather than Confirm's more generic one.
//
// item must carry State=StateDisposed and Verdict in {accept,
// edit_accept}, checked before any write -- a reject or defer, or an item
// of any other Kind, can never reach the ledger through this path. An
// edit_accept honors EditedPayload over Payload when present; `serenity
// inbox`'s own 'e' key does not yet offer edit_accept for this Kind (its
// own doc comment restricts 'e' to a single ungrouped KindReconcile row),
// so in practice only a future caller reaches that branch -- included now
// so this method's own contract does not silently drop an edited payload
// a later caller supplies.
func (s *Store) ApplyDisposedPreceptDraft(ctx context.Context, item disposition.Item, now time.Time) (*ledger.Entry, error) {
	if err := s.writable("apply precept draft"); err != nil {
		return nil, err
	}
	if item.Kind != disposition.KindPreceptDraft {
		return nil, fmt.Errorf("direction: apply precept draft %s: item kind is %q, want %q", item.ID, item.Kind, disposition.KindPreceptDraft)
	}
	if item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) {
		return nil, fmt.Errorf("direction: apply precept draft %s: item is not accepted (state=%q verdict=%q)", item.ID, item.State, item.Verdict)
	}

	raw := item.Payload
	if item.Verdict == disposition.VerdictEditAccept && len(item.EditedPayload) > 0 {
		raw = item.EditedPayload
	}
	var payload PreceptDraftPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("direction: apply precept draft %s: decode payload: %w", item.ID, err)
	}
	if payload.Title == "" {
		return nil, fmt.Errorf("direction: apply precept draft %s: payload carries no title", item.ID)
	}
	if len(payload.Alternatives) == 0 {
		return nil, fmt.Errorf("direction: apply precept draft %s: payload carries no alternatives; the interview always seeds the \"do not adopt this\" floor, so an empty list here is a payload bug, not something Confirm should paper over", item.ID)
	}

	alts := make([]ledger.Alternative, len(payload.Alternatives))
	for i, a := range payload.Alternatives {
		alts[i] = ledger.Alternative{Option: a.Option, WhyNot: a.WhyNot, RevisitIf: a.RevisitIf}
	}

	entry := &ledger.Entry{
		Kind:         ledger.KindDecision,
		Title:        payload.Title,
		State:        ledger.StateStaged,
		Created:      now.UTC().Format(time.RFC3339),
		Body:         payload.Body,
		Alternatives: alts,
		Source: &ledger.Source{
			Hook:    ledger.HookKaziDisposition,
			Excerpt: truncateExcerpt(payload.Question + " -- " + payload.Answer),
			Tier:    ledger.TierSemantic,
		},
	}
	if err := s.CreateDraft(ctx, entry); err != nil {
		return nil, fmt.Errorf("direction: apply precept draft %s: write staged entry: %w", item.ID, err)
	}

	confirmed, err := s.Confirm(ctx, entry.ID, item.Actor, now)
	if err != nil {
		return nil, fmt.Errorf("direction: apply precept draft %s: confirm %s: %w", item.ID, entry.ID, err)
	}
	return confirmed, nil
}

// truncateExcerpt bounds s to entry.schema.json's source.excerpt limit
// (1000 characters, dira/ledger/entry.go's Validate) -- "evidence for
// review, not a transcript archive", the same bound the schema states for
// every other excerpt this codebase writes.
func truncateExcerpt(s string) string {
	r := []rune(s)
	if len(r) <= 1000 {
		return s
	}
	return string(r[:1000])
}
