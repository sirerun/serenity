package supersede

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ApplyDisposedDirtyEdit applies a disposed KindDirtyEdit disposition
// item's accept verdict to the canonical brain repo (T2.4).
//
// T0.4's dirty-tree guard (ADR 004, internal/writer.dirtytree.go) pauses a
// machine write whenever the target fence page carries an uncommitted
// human edit rather than silently overwriting it, and records both sides
// of the conflict as a writer.PendingRecord. T2.1's Store.ImportPending
// sweeps those records into KindDirtyEdit disposition items -- this is
// the accept side ADR 004 promised but deferred: "M2 imports these
// records as dirty_edit items instead" of the pre-M2 fallback of
// hand-inspecting and deleting the file.
//
// Accepting a dirty_edit item means the human's on-disk edit -- not the
// machine's paused write -- is what should become durable. For a
// fence-tier family that is trivial: the file already IS truth (§7.2), so
// a fence-tier hand edit never even reaches the dirty-tree guard in the
// first place (there is no separate "machine" writer racing it) and this
// function skips any such row unchanged. The real divergence this
// function exists for is shard-tier (§7.2a): the shard is canonical, and
// the fence's head row for a shard family is only ever a DERIVED cache of
// it (store/fence.go's own SourceRef="shard" convention) -- a hand edit to
// that cached row does not, by itself, change what the shard resolves to.
// This function is what makes an accepted hand edit real: for each
// shard-tier row on the human's edited page (item.Payload's
// PendingRecord.Human, parsed via store.ParseEntityBytes) whose Object
// diverges from what its (SubjectSlug, Family) shard currently resolves
// to, the row is appended to that shard as a new line -- reusing Writer's
// own tier-dispatching Apply (T2.3) with the human's row as the new claim
// and the shard's current head as the claim it supersedes, so the exact
// same shard-append + head-regenerate path T2.3 and T2.7 already exercise
// runs here too, not a second implementation of it. Its Provenance is
// rebuilt from scratch (Actor=item.Actor, ObservedAt=now) rather than
// carried over from the parsed row: the fence markdown format has no
// Provenance column at all (T2.3's own finding, reconfirmed in T2.7), so
// there is nothing on the human's row to carry forward, and a human's
// direct fence edit is, like edit_accept's human correction, grounded in
// no source span or extraction model.
//
// A row whose Object already matches its family's current shard head is
// not divergent -- nothing on the human's side actually changed for that
// family, so nothing is appended. Two shapes this task's acc line does
// not exercise are deliberately left unhandled, disclosed rather than
// guessed at: a family with no existing shard entry at all (there is
// nothing for the human's row to supersede, and Apply requires an old
// claim to supersede), and a family currently resolving to more than one
// concurrent head (which one a single edited row should supersede is
// ambiguous). Both are silently skipped rather than guessed at; a caller
// that needs them is a later task.
func (w *Writer) ApplyDisposedDirtyEdit(item disposition.Item, now time.Time) ([]Result, error) {
	if item.Kind != disposition.KindDirtyEdit {
		return nil, fmt.Errorf("supersede: apply disposed dirty edit: item %s is kind %q, not %q", item.ID, item.Kind, disposition.KindDirtyEdit)
	}
	if item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictAccept {
		return nil, fmt.Errorf("supersede: apply disposed dirty edit: item %s is state=%q verdict=%q, want disposed with accept",
			item.ID, item.State, item.Verdict)
	}

	var rec writer.PendingRecord
	if err := json.Unmarshal(item.Payload, &rec); err != nil {
		return nil, fmt.Errorf("supersede: apply disposed dirty edit: decode payload: %w", err)
	}
	human, err := store.ParseEntityBytes([]byte(rec.Human))
	if err != nil {
		return nil, fmt.Errorf("supersede: apply disposed dirty edit: parse human copy: %w", err)
	}

	// The human's on-disk edit is exactly what accepting this item
	// sanctions -- commit it as its own checkpoint before anything below
	// tries to write through this same path again. Skipping this would
	// immediately re-trigger the very guard that staged this item in the
	// first place (internal/writer/dirtytree.go's dirty() has no notion
	// of "already reviewed," only "committed" vs not): the shard append
	// below is unaffected (a different file), but regenerating the fence
	// head onto this same page would find it still uncommitted and pause
	// again instead of landing.
	if _, err := writer.CommitPath(w.Fence.Root, rec.Path, fmt.Sprintf("serenity: accept dirty_edit %s (human-authored fence edit)", item.ID)); err != nil {
		return nil, fmt.Errorf("supersede: apply disposed dirty edit: commit human edit: %w", err)
	}

	var results []Result
	for _, c := range human.Claims {
		if w.Config.TierOf(c.Family) != domain.TierShard {
			continue
		}
		heads, err := w.Shard.ResolveHeads(c.SubjectSlug, c.Family)
		if err != nil {
			return results, fmt.Errorf("supersede: apply disposed dirty edit: resolve heads for %s/%s: %w", c.SubjectSlug, c.Family, err)
		}
		if len(heads) != 1 {
			// 0 heads: nothing to supersede. >1 heads: ambiguous which one
			// the human's row replaces. Both disclosed above, neither
			// exercised by this task's acc line.
			continue
		}
		var cur domain.Claim
		for _, h := range heads {
			cur = h
		}
		if c.Object == cur.Object {
			continue // matches the shard's current head -- no divergence
		}

		nc := c
		nc.Provenance = domain.Provenance{Actor: item.Actor, ObservedAt: now.UTC()}
		nc.ObjectKey = store.NormalizeKey(nc.Object)
		nc.ID = store.DerivedID(nc.SubjectSlug, nc.Predicate, nc.ObjectKey, nc.ValidFrom, nc.Provenance.SourceSHA256, store.DefaultIDWidth)

		res, err := w.Apply(nc, cur)
		if err != nil {
			return results, fmt.Errorf("supersede: apply disposed dirty edit: apply %s/%s: %w", c.SubjectSlug, c.Family, err)
		}
		results = append(results, res)
	}
	return results, nil
}
