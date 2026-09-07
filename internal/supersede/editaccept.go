package supersede

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
)

// ApplyDisposedReconcile applies a disposed KindReconcile disposition item
// (T2.2's own staged shape) to the canonical brain repo, and records the
// resulting claim's id back onto the disposition item via
// disposition.Store.RecordResultClaimID -- "the disposition row
// references the new claim id" (T2.7's acc line).
//
// verdict accept writes the staged A claim exactly as T2.2 proposed it.
// verdict edit_accept substitutes item.EditedPayload's domain.Claim for A
// first -- this package's own doc comment (T2.3) already names this as
// the correct reading of RFC 0001 section 8.2's edit_accept: "accept, but
// with a substituted payload." Both verdicts then write through the exact
// same tier-dispatching Apply this package already has (T2.3); there is
// no edit_accept-specific write path, only a different A.
//
// "Human-tier provenance" (this task's own title): the resulting claim's
// Provenance.Actor is set to item.Actor -- the actor who actually
// disposed the item (Store.Dispose records it at dispose time), so
// provenance reflects the real decision-maker even if applying the write
// happens later or in a different process, not whoever happens to call
// this function. For edit_accept specifically, SourceSHA256/Span/Model
// are also cleared: a human-typed correction is grounded in no source
// span and no extraction model, so carrying the pre-edit A's stale values
// forward would misrepresent the new value's real provenance; ObservedAt
// becomes now, the moment of the edit. A plain accept's A is left
// otherwise byte-for-byte as T2.2 staged it -- nothing about the value
// changed, so nothing about its provenance should either, beyond Actor.
//
// Either verdict recomputes the resulting claim's id (RFC section 7.2:
// shorthash(subject_slug, predicate, normalized_object_key, valid_from,
// source_ref)) rather than reusing A's staged id -- required for
// edit_accept (the edited object changes every id input downstream of
// ObjectKey); a no-op for plain accept, recomputing the exact id A
// already carried, kept unconditional so both verdicts share one path.
//
// item must be StateDisposed with Verdict accept or edit_accept, and Kind
// KindReconcile; any other combination is a programmer error -- Process
// (internal/reconcile, T2.2) only ever stages KindReconcile items for a
// human to accept/edit_accept/reject/defer, so a caller handing this
// anything else has already taken a wrong turn upstream, reported rather
// than silently reinterpreted (the same convention Apply's own
// subject/predicate mismatch check uses).
func (w *Writer) ApplyDisposedReconcile(ctx context.Context, dispStore *disposition.Store, item disposition.Item, now time.Time) (Result, error) {
	if item.Kind != disposition.KindReconcile {
		return Result{}, fmt.Errorf("supersede: apply disposed reconcile: item %s is kind %q, not %q", item.ID, item.Kind, disposition.KindReconcile)
	}
	if item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) {
		return Result{}, fmt.Errorf("supersede: apply disposed reconcile: item %s is state=%q verdict=%q, want disposed with accept or edit_accept",
			item.ID, item.State, item.Verdict)
	}

	var payload reconcile.ReconcilePayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return Result{}, fmt.Errorf("supersede: apply disposed reconcile: decode payload: %w", err)
	}
	a := payload.A

	if item.Verdict == disposition.VerdictEditAccept {
		var edited domain.Claim
		if err := json.Unmarshal(item.EditedPayload, &edited); err != nil {
			return Result{}, fmt.Errorf("supersede: apply disposed reconcile: decode edited payload: %w", err)
		}
		edited.Provenance.SourceSHA256 = ""
		edited.Provenance.Span = ""
		edited.Provenance.Model = ""
		edited.Provenance.ObservedAt = now.UTC()
		a = edited
	}
	if item.Actor != "" {
		a.Provenance.Actor = item.Actor
	}

	// Recompute id and ObjectKey from whatever Object ended up in a
	// (edited or original) -- see the doc comment above for why this is
	// unconditional rather than edit_accept-only.
	a.ObjectKey = store.NormalizeKey(a.Object)
	a.ID = store.DerivedID(a.SubjectSlug, a.Predicate, a.ObjectKey, a.ValidFrom, a.Provenance.SourceSHA256, store.DefaultIDWidth)

	res, err := w.Apply(a, payload.B)
	if err != nil {
		return Result{}, err
	}

	if err := dispStore.RecordResultClaimID(ctx, item.ID, a.ID, now); err != nil {
		return Result{}, fmt.Errorf("supersede: apply disposed reconcile: record result claim id: %w", err)
	}
	return res, nil
}
