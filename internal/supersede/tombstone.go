package supersede

// TombstoneCascade and ApplyDisposedTombstone implement T2.21's half of RFC
// 0001's right-to-forget chain (§14, §7.6, docs/threat-model.md's "Right-
// to-forget: the deletion chain"): "source tombstone -> retraction
// proposals -> accept rewrites fences/shards and rebuilds." T1.2's
// store.SourceStore.Tombstone(sha, shardStore) is the read-side "which
// claims cite this source" lookup (its own doc comment names turning that
// into retraction proposals as later work) -- this file is that later
// work, split the same way T2.1/T2.4 split ImportPending (stage) from
// ApplyDisposedDirtyEdit (accept), and the way internal/reconcile's Engine
// (T2.2) splits Detect (pure) from Process (stages a disposition item):
//
//   - TombstoneCascade (the "propose" side): for every shard-tier claim
//     citing the tombstoned source, decide sole-provenance vs
//     multi-provenance by checking whether another still-live claim
//     shares the same (subject, predicate, object_key) from a different,
//     non-tombstoned source. A sole-provenance claim stages one
//     KindTombstone disposition item -- a human must accept it before
//     anything is retracted (ladder.go's own never_automate: "tombstones",
//     RFC's "cascades retraction proposals"). A multi-provenance claim is
//     demoted immediately, through the existing Apply supersession path,
//     with no human gate -- demotion only ever lowers confidence, never
//     removes a row, so it does not carry the same risk retraction does.
//   - ApplyDisposedTombstone (the "accept" side): once a human accepts a
//     staged item, the named claim is retracted in its shard via Retract,
//     and the shard's derived head row regenerates -- RFC's "accept
//     rewrites fences/shards and rebuilds": rebuild (internal/index)
//     already drops a retracted claim from the index for free, since
//     store.ResolveHeadLines treats a State=Retracted line as dead and
//     Rebuild only ever indexes resolved shard heads.
//
// Disclosed scope: this cascade only ever sees shard-tier claims.
// SourceStore.Tombstone's own doc comment names why -- fence-tier rows
// render provenance down to a short human-readable src cell (RFC §7.2's
// table has no sha256 column), so a parsed entity page cannot answer
// "does this cite sha" at all. A fence-tier retraction would be a smaller,
// different operation (an in-place row edit, since the file is already
// truth there) that this task does not build or test.
import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// demoteFactor is this implementation's own policy choice for how much a
// claim's confidence drops when it loses one corroborating source but
// keeps others (RFC §14/§7.6 name "demoted... confidence reflects the lost
// corroboration" as the required *behavior*, not a number -- the RFC
// mirrors this same gap on the corroboration-gain side: internal/reconcile's
// VerdictAgree doc comment names "a real confidence-bump write-through" as
// explicitly future, unbuilt work). 0.7 is a disclosed, deliberately
// conservative starting point (30% reduction), not derived from any eval;
// this task's acc line does not test a specific value, only that a
// multi-provenance claim ends up demoted rather than retracted.
const demoteFactor = 0.7

// TombstonePayload is a KindTombstone disposition item's payload: the one
// shard-tier claim a human is being asked to retract, plus the source sha
// that triggered the cascade (review context; ApplyDisposedTombstone does
// not re-derive it, it only needs Claim).
type TombstonePayload struct {
	Claim        domain.Claim `json:"claim"`
	SourceSHA256 string       `json:"source_sha256"`
}

// TombstoneCascade walks every shard-tier claim citing sha (via
// src.Tombstone) and either stages a retraction proposal or demotes the
// claim in place, per the package doc comment above. retracted and demoted
// count how many claims took each path; a citing claim that is no longer
// live (already superseded or retracted by something else since) takes
// neither path and is skipped -- there is nothing left to cascade.
func (w *Writer) TombstoneCascade(ctx context.Context, ds *disposition.Store, src *store.SourceStore, sha string, now time.Time) (retracted, demoted int, err error) {
	citing, err := src.Tombstone(sha, w.Shard)
	if err != nil {
		return 0, 0, fmt.Errorf("supersede: tombstone cascade: %w", err)
	}

	for _, c := range citing {
		if w.Config.TierOf(c.Family) != domain.TierShard {
			continue // fence-tier: disclosed out of scope, see package doc
		}
		lines, err := w.Shard.Lines(c.SubjectSlug, c.Family)
		if err != nil {
			return retracted, demoted, fmt.Errorf("supersede: tombstone cascade: read lines %s/%s: %w", c.SubjectSlug, c.Family, err)
		}
		live, dead := liveByKey(lines)
		if c.State != domain.StateActive || dead[c.ID] {
			continue // already superseded or retracted elsewhere -- nothing to cascade
		}

		key := c.ObjectKey
		if key == "" {
			key = store.NormalizeKey(c.Object)
		}
		corroborated := false
		for _, d := range live[key] {
			if d.ID == c.ID {
				continue
			}
			if d.Provenance.SourceSHA256 != sha {
				corroborated = true
				break
			}
		}

		if corroborated {
			if _, err := w.demoteClaim(c, now); err != nil {
				return retracted, demoted, fmt.Errorf("supersede: tombstone cascade: demote %s: %w", c.ID, err)
			}
			demoted++
			continue
		}

		payload := TombstonePayload{Claim: c, SourceSHA256: sha}
		raw, err := json.Marshal(payload)
		if err != nil {
			return retracted, demoted, fmt.Errorf("supersede: tombstone cascade: marshal payload: %w", err)
		}
		if _, err := ds.Create(ctx, disposition.KindTombstone, raw, "", now); err != nil {
			return retracted, demoted, fmt.Errorf("supersede: tombstone cascade: create disposition item: %w", err)
		}
		retracted++
	}
	return retracted, demoted, nil
}

// liveByKey groups every ACTIVE, non-dead shard line by object key --
// the same "dead" resolution store.ResolveHeadLines uses (a claim is dead
// once some other line's Supersedes names it, or it is itself
// State=Retracted), but keeping every live claim per key rather than
// collapsing to a single head. A source-tombstone cascade needs to know
// whether OTHER independent corroboration exists for a fact, not just
// which one line currently wins ResolveHeadLines' tie-break -- two
// claims can both be live for the same key (neither supersedes the
// other) when two sources independently corroborate the same fact,
// which is exactly the shape TombstoneCascade is looking for.
func liveByKey(lines []domain.Claim) (byKey map[string][]domain.Claim, dead map[string]bool) {
	dead = map[string]bool{}
	for _, c := range lines {
		if c.Supersedes != "" {
			dead[c.Supersedes] = true
		}
		if c.State == domain.StateRetracted {
			dead[c.ID] = true
		}
	}
	byKey = map[string][]domain.Claim{}
	for _, c := range lines {
		if c.State != domain.StateActive || dead[c.ID] {
			continue
		}
		key := c.ObjectKey
		if key == "" {
			key = store.NormalizeKey(c.Object)
		}
		byKey[key] = append(byKey[key], c)
	}
	return byKey, dead
}

// demoteClaim reuses Apply (T2.3) exactly like every other write in this
// package: a fresh, lower-confidence copy of c supersedes c, carrying the
// same object/object key (it is the same fact, just less corroborated)
// and a freshly rebuilt Provenance (Actor: "machine", ObservedAt: now) --
// this is a machine-computed confidence recompute, not a new observation
// tied to any one source, so SourceSHA256 is left empty rather than
// copying c's own (now partially invalidated) source.
func (w *Writer) demoteClaim(c domain.Claim, now time.Time) (Result, error) {
	nc := c
	nc.Confidence = c.Confidence * demoteFactor
	nc.Provenance = domain.Provenance{Actor: "machine", ObservedAt: now.UTC()}
	nc.ID = store.DerivedID(nc.SubjectSlug, nc.Predicate, nc.ObjectKey, nc.ValidFrom, nc.Provenance.SourceSHA256, store.DefaultIDWidth)
	return w.Apply(nc, c)
}

// ApplyDisposedTombstone applies a disposed KindTombstone item's accept
// verdict: the claim named in its payload is retracted via Retract. Like
// T2.4/T2.7's own accept-side functions, it requires the item to already
// be disposed with an accept verdict -- edit_accept is not supported here
// (there is no reviewer-editable payload shape to accept-with-changes for
// a retraction: either the claim is retracted or it is not).
func (w *Writer) ApplyDisposedTombstone(item disposition.Item, now time.Time) (Result, error) {
	if item.Kind != disposition.KindTombstone {
		return Result{}, fmt.Errorf("supersede: apply disposed tombstone: item %s is kind %q, not %q", item.ID, item.Kind, disposition.KindTombstone)
	}
	if item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictAccept {
		return Result{}, fmt.Errorf("supersede: apply disposed tombstone: item %s is state=%q verdict=%q, want disposed with accept",
			item.ID, item.State, item.Verdict)
	}

	var payload TombstonePayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return Result{}, fmt.Errorf("supersede: apply disposed tombstone: decode payload: %w", err)
	}
	return w.Retract(payload.Claim, item.Actor, now)
}

// Retract appends a retraction line for target -- reusing target's own id
// rather than deriving a new one (§7.2a: "a retraction is a lifecycle
// tombstone that deliberately reuses its target claim's id";
// store.ShardStore.Append exempts State=Retracted rows from its id
// collision check for exactly this reason, and ResolveHeadLines keys
// retraction off that same id match) -- and regenerates the shard-tier
// head, so a rebuild picks up the retraction for free (ResolveHeadLines
// marks a retracted id dead, and index.Rebuild only ever indexes resolved
// shard heads). Shard-tier only; see the package doc comment for why
// fence-tier is out of scope here.
func (w *Writer) Retract(target domain.Claim, actor string, now time.Time) (Result, error) {
	if w.Config.TierOf(target.Family) != domain.TierShard {
		return Result{}, fmt.Errorf("supersede: retract: family %q is fence-tier -- Retract only supports shard-tier families", target.Family)
	}

	tomb := domain.Claim{
		SubjectSlug: target.SubjectSlug,
		Predicate:   target.Predicate,
		Family:      target.Family,
		ID:          target.ID,
		ObjectKey:   target.ObjectKey,
		State:       domain.StateRetracted,
		Provenance:  domain.Provenance{Actor: actor, ObservedAt: now.UTC()},
	}
	shardPath, _, err := writer.Shard(w.Queue, w.Shard, tomb)
	if err != nil {
		return Result{}, fmt.Errorf("supersede: retract: shard append: %w", err)
	}

	fencePath, err := w.regenerateShardHead(target.SubjectSlug, target.Family)
	if err != nil {
		return Result{}, err
	}
	return Result{Tier: domain.TierShard, FencePath: fencePath, ShardPath: shardPath}, nil
}
