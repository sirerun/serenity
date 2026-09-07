package disposition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CapturePayload is a distill item's Payload shape when it was staged by
// Capture (as opposed to a distill item arriving from another source, e.g.
// T2.12's low-confidence decay sweep, which defines its own shape).
type CapturePayload struct {
	Text     string `json:"text,omitempty"`
	AudioRef string `json:"audio_ref,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

// ErrCaptureEmpty is returned by Capture when neither text nor audioRef is
// given -- there is nothing to stage.
var ErrCaptureEmpty = errors.New("disposition: capture: text or audio_ref required")

// Capture stages a zero-friction ingress item in the distill queue (RFC
// 0001 §8.2/§10.1: "capture(text | audio_ref, hint?) -- zero-friction
// ingress; returns a staged distill item id"). Exactly one of text/audioRef
// is normally given; both may be given (e.g. an audio capture with a typed
// caption), but at least one is required.
func Capture(ctx context.Context, s *Store, text, audioRef, hint string, now time.Time) (Item, error) {
	if text == "" && audioRef == "" {
		return Item{}, ErrCaptureEmpty
	}
	payload, err := json.Marshal(CapturePayload{Text: text, AudioRef: audioRef, Hint: hint})
	if err != nil {
		return Item{}, fmt.Errorf("disposition: capture: marshal payload: %w", err)
	}
	return s.Create(ctx, KindDistill, payload, "", now)
}

// DistillRoute is the destination a disposition of a KindDistill item
// chooses (RFC 0001 §8.2 capture doc, UC-034: "dispositions
// claim-batch/precept-draft/note/trash"). This is distinct from the
// generic accept/edit_accept/reject/defer verdict vocabulary Dispose uses
// for reconcile/effect/tombstone items -- a distilled capture's
// disposition IS which of these four destinations it goes to, so
// RouteDistill maps each route onto Dispose's verdict machinery
// underneath (recording which route was chosen in the item, not just its
// accept/reject outcome) rather than inventing a second, parallel
// disposal path.
type DistillRoute string

const (
	// RouteClaimBatch accepts the capture as raw material bound for the
	// claim-extraction pipeline (RFC §10.1's chunk->extract->write
	// pipeline). Wiring an accepted claim-batch item into that pipeline
	// automatically is a later, still-unowned task -- this package only
	// records the routing decision durably; T1.8/T1.9's already-shipped
	// extraction and write path is the real machinery it would eventually
	// feed.
	RouteClaimBatch DistillRoute = "claim-batch"
	// RoutePreceptDraft accepts the capture and additionally creates a
	// NEW disposition item of kind precept_draft carrying the same
	// payload -- never a .dira entry (only internal/direction may write
	// there, T3.12's AST gate, and only after its own separate human
	// confirmation step, T3.3/T3.4).
	RoutePreceptDraft DistillRoute = "precept-draft"
	// RouteNote accepts the capture as a standalone note. Nothing beyond
	// the disposition item itself is written -- rendering or indexing
	// captured notes as a first-class surface is out of scope (RFC 0001
	// §6: "note-taking app" is a stated non-goal).
	RouteNote DistillRoute = "note"
	// RouteTrash discards the capture (Dispose's reject verdict, which
	// requires a note; RouteDistill supplies a default one if the caller
	// gives none).
	RouteTrash DistillRoute = "trash"
)

func (r DistillRoute) valid() bool {
	switch r {
	case RouteClaimBatch, RoutePreceptDraft, RouteNote, RouteTrash:
		return true
	}
	return false
}

// ErrInvalidRoute is returned by RouteDistill for a route outside
// {claim-batch, precept-draft, note, trash}.
var ErrInvalidRoute = errors.New("disposition: invalid distill route")

// ErrNotDistill is returned by RouteDistill when id does not name a
// KindDistill item -- routes are a distill-only concept.
var ErrNotDistill = errors.New("disposition: item is not a distill item")

// RouteDistill disposes of the KindDistill item id along one of the four
// capture routes (RFC 0001 §8.2, UC-034). It records the chosen route on
// the item (Item.Route) so "note" and "claim-batch" -- which both accept
// the item -- remain distinguishable after the fact, then applies the
// route's specific effect:
//
//   - trash: rejected (a note is required by Dispose's own rule; a default
//     is supplied when the caller gives none).
//   - note, claim-batch: accepted, with no further write beyond the item
//     itself (claim-batch's downstream extraction wiring is explicitly
//     out of scope here -- see RouteClaimBatch's doc).
//   - precept-draft: accepted, and a new disposition item of kind
//     precept_draft is created carrying the distill item's own payload
//     (never a .dira entry).
//
// Idempotency and the already_disposed cross-client rule are inherited
// unchanged from Dispose -- RouteDistill is a thin routing layer over it,
// not a second disposal mechanism.
func (s *Store) RouteDistill(ctx context.Context, id string, route DistillRoute, actor, note, idempotencyKey string, now time.Time) (Result, error) {
	if !route.valid() {
		return Result{}, fmt.Errorf("%w: %q", ErrInvalidRoute, route)
	}
	item, err := s.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if item.Kind != KindDistill {
		return Result{}, fmt.Errorf("%w: %s is kind %q", ErrNotDistill, id, item.Kind)
	}

	verdict := VerdictAccept
	if route == RouteTrash {
		verdict = VerdictReject
		if note == "" {
			note = "discarded via capture route: trash"
		}
	}

	res, err := s.disposeWithRoute(ctx, id, verdict, route, note, actor, idempotencyKey, now)
	if err != nil {
		return Result{}, err
	}
	// A replay or a cross-client already_disposed hit must not create a
	// second precept-draft follow-on item -- only a genuinely NEW
	// disposition (res.Item.Route freshly set by this very call) does.
	if res.AlreadyDisposed || res.Replayed {
		return res, nil
	}

	if route == RoutePreceptDraft {
		if _, err := s.Create(ctx, KindPreceptDraft, item.Payload, "", now); err != nil {
			return Result{}, fmt.Errorf("disposition: route distill: stage precept draft for %s: %w", id, err)
		}
	}
	return res, nil
}

// disposeWithRoute is Dispose plus recording route on the item before it
// is written back -- Dispose itself knows nothing about distill routes, so
// this small wrapper re-implements just enough of Dispose's write step to
// stamp Route on the same item/history transaction rather than issuing a
// second write.
func (s *Store) disposeWithRoute(ctx context.Context, id string, verdict Verdict, route DistillRoute, note, actor, idempotencyKey string, now time.Time) (Result, error) {
	res, err := s.Dispose(ctx, id, verdict, nil, note, actor, idempotencyKey, now)
	if err != nil {
		return Result{}, err
	}
	if res.AlreadyDisposed || res.Replayed {
		return res, nil
	}
	res.Item.Route = route
	if err := s.put(ctx, res.Item); err != nil {
		return Result{}, fmt.Errorf("disposition: route distill: record route for %s: %w", id, err)
	}
	return res, nil
}
