// Package supersede implements RFC 0001's supersession write path (T2.3):
// applying an accepted reconcile A/B disposition item (internal/reconcile,
// T2.2) to the canonical brain repo.
//
// "Accept" on a reconcile item means the newly-proposed claim (A) is
// correct and the prior active claim (B) it conflicted or was temporally
// superseding wins: RFC §7.2's two authority rules govern how that lands
// on disk, split by storage tier (§7.2a):
//
//   - fence-tier: the file is truth. B's row gets the strikethrough +
//     forward-pointer encoding (§7.2: "supersession keeps the row --
//     strikethrough + pointer") in place, A is added as a new active row,
//     and the whole page re-renders through the deterministic writer.
//   - shard-tier: the shard is canonical, nothing rewrites history in
//     place (§7.2a). A lands as one new appended line carrying
//     Supersedes=B.ID -- B's own line is never touched -- and the
//     entity page's derived fence head row for that (subject, family) is
//     regenerated to reflect the new resolved head, marked src: shard.
//     T2.14 (consolidate) owns the full nightly sweep across every
//     entity/family; Apply only refreshes the one head this specific
//     accept just changed, so a human reading the page next doesn't see
//     a stale head until the next full consolidate run.
package supersede

import (
	"fmt"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Writer applies accepted reconcile decisions to the canonical brain
// through the writer queue (RFC §7.7) -- the only sanctioned entry point
// for the FenceWriter/ShardStore write-throughs a supersession touches.
// Construct with New; the zero value is not usable (Queue/Fence/Shard are
// required).
type Writer struct {
	Queue *writer.Queue
	Fence *store.FenceWriter
	Shard *store.ShardStore
	// Config supplies the tier assignment (config.TierOf) that decides
	// whether a claim's family lands in a fence or a shard (§7.2a). Nil
	// falls back to config.Default().
	Config *config.Config
	// EntityType resolves the fence-tier folder for a subject slug. Nil
	// defaults every slug to ingest.DefaultEntityType, the same bucket
	// T1.9's write path uses -- a supersession's B side was written
	// through that same convention, so Apply must resolve the identical
	// path to find it.
	EntityType func(subjectSlug string) string
}

// New builds a Writer. q, fw, and ss must be non-nil. cfg nil defaults to
// config.Default().
func New(q *writer.Queue, fw *store.FenceWriter, ss *store.ShardStore, cfg *config.Config) *Writer {
	if cfg == nil {
		cfg = config.Default()
	}
	return &Writer{Queue: q, Fence: fw, Shard: ss, Config: cfg}
}

func (w *Writer) entityType(subjectSlug string) string {
	if w.EntityType != nil {
		if t := w.EntityType(subjectSlug); t != "" {
			return t
		}
	}
	return ingest.DefaultEntityType
}

// Result summarizes what Apply wrote.
type Result struct {
	Tier domain.Tier
	// FencePath is always set: fence-tier writes the entity page
	// directly; shard-tier writes the same page's regenerated head row.
	FencePath string
	// ShardPath is set only for a shard-tier apply.
	ShardPath string
}

// Apply supersedes b with a: a is written as the new active claim,
// carrying Supersedes=b.ID, and b's own prior state is retired per its
// storage tier (see the package doc). a and b must already share
// (SubjectSlug, Predicate) -- Apply does not re-validate that beyond what
// a caller building the pair from a real internal/reconcile
// (T2.2) ReconcilePayload's A/B fields already guarantees; a mismatched
// pair reaching Apply is a programmer error, reported rather than
// silently reinterpreted.
//
// Out of scope, disclosed: edit_accept (a human-edited payload replacing
// a) needs no special case here -- RFC §8.2 treats edit_accept as
// generically "accept with a substituted payload" for any item kind, so
// a caller wanting that behavior passes the edited claim as a. A human
// disposition choosing b over a (rejecting the machine's proposed
// winner) is a different verb entirely (reject with a note, or a future
// explicit reverse-supersession) -- RFC's dispose() operation does not
// model "accept, but the other way," and this task's acc line does not
// exercise it.
func (w *Writer) Apply(a, b domain.Claim) (Result, error) {
	if a.SubjectSlug != b.SubjectSlug || a.Predicate != b.Predicate {
		return Result{}, fmt.Errorf("supersede: apply: a and b must share (subject, predicate): a=%s/%s b=%s/%s",
			a.SubjectSlug, a.Predicate, b.SubjectSlug, b.Predicate)
	}

	a.State = domain.StateActive
	a.Supersedes = b.ID
	a.SupersededBy = ""

	if w.Config.TierOf(a.Family) == domain.TierShard {
		return w.applyShard(a, b)
	}
	return w.applyFence(a, b)
}

// applyFence handles the fence-tier path: strikethrough + pointer on b's
// existing row, a added as a new row, the whole page re-rendered through
// the deterministic writer (§7.2's FenceWriter.RenderEntity already knows
// how to render a StateSuperseded row with SupersededBy set -- Apply only
// needs to set those two fields correctly, not reimplement the encoding).
func (w *Writer) applyFence(a, b domain.Claim) (Result, error) {
	entityType := w.entityType(a.SubjectSlug)
	path := w.Fence.PathFor(entityType, a.SubjectSlug)

	p, err := w.Fence.ParseEntity(path)
	if err != nil {
		// Unlike ingest's first-write tolerance, a supersession's b side
		// must already exist on disk -- reconcile only proposes a
		// conflict against a genuinely active claim (T2.2's Candidates
		// reads active.State), so a missing page here means the caller
		// handed Apply a b that current disk state disagrees with. That
		// is a real inconsistency to surface, not paper over with a
		// fresh empty page.
		return Result{}, fmt.Errorf("supersede: apply: read entity page %s: %w", path, err)
	}

	found := false
	for i := range p.Claims {
		if p.Claims[i].ID == b.ID {
			p.Claims[i].State = domain.StateSuperseded
			p.Claims[i].SupersededBy = a.ID
			found = true
			break
		}
	}
	if !found {
		return Result{}, fmt.Errorf("supersede: apply: claim %s not found on entity page %s -- b must already be a row on the page Apply is asked to supersede it on", b.ID, path)
	}
	p.Claims = append(p.Claims, a)

	fencePath, _, err := writer.Fence(w.Queue, w.Fence, p)
	if err != nil {
		return Result{}, fmt.Errorf("supersede: apply: fence write: %w", err)
	}
	return Result{Tier: domain.TierFence, FencePath: fencePath}, nil
}

// applyShard handles the shard-tier path: a lands as one new appended
// shard line (b's own line is never touched -- append-only, §7.2a), then
// the entity page's derived head row for (a.SubjectSlug, a.Family) is
// regenerated from the shard's own resolved heads.
func (w *Writer) applyShard(a, b domain.Claim) (Result, error) {
	shardPath, _, err := writer.Shard(w.Queue, w.Shard, a)
	if err != nil {
		return Result{}, fmt.Errorf("supersede: apply: shard append: %w", err)
	}

	fencePath, err := w.regenerateShardHead(a.SubjectSlug, a.Family)
	if err != nil {
		return Result{}, err
	}
	return Result{Tier: domain.TierShard, FencePath: fencePath, ShardPath: shardPath}, nil
}

// regenerateShardHead re-derives the entity page's claims-fence rows for
// exactly one (slug, family) from the shard's own resolved heads (§7.2a:
// "the entity page's claims fence holds only the current resolved head of
// each shard family ... regenerated by consolidate exactly like the
// summary fence, marked DERIVED"), leaving every other row on the page
// (other families, and every fence-tier claim) untouched.
func (w *Writer) regenerateShardHead(slug, family string) (string, error) {
	heads, err := w.Shard.ResolveHeads(slug, family)
	if err != nil {
		return "", fmt.Errorf("supersede: regenerate shard head: resolve heads for %s/%s: %w", slug, family, err)
	}

	entityType := w.entityType(slug)
	path := w.Fence.PathFor(entityType, slug)
	p, err := w.Fence.ParseEntity(path)
	if err != nil {
		// First-ever fence touch for this entity (every one of its
		// claims lives in shards so far): start a fresh page, the same
		// fallback internal/ingest's Writer uses for its own first
		// fence write.
		p = store.NewEntityPage(domain.Entity{Type: entityType, Slug: slug})
	}

	kept := p.Claims[:0]
	for _, c := range p.Claims {
		if c.Family != family {
			kept = append(kept, c)
		}
	}
	p.Claims = kept
	for _, key := range store.HeadKeys(heads) {
		head := heads[key]
		head.SourceRef = "shard"
		p.Claims = append(p.Claims, head)
	}

	fencePath, _, err := writer.Fence(w.Queue, w.Fence, p)
	if err != nil {
		return "", fmt.Errorf("supersede: regenerate shard head: fence write: %w", err)
	}
	return fencePath, nil
}
