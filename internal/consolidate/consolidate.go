// Package consolidate implements the nightly + on-demand consolidate pass
// (RFC 0001 §10.4, plan T2.14): regenerate every entity's summary fence
// with a freshness banner, refresh shard-tier fence head rows from their
// shard's own resolved heads (§7.2a), then rebuild the derived index and
// re-embed whatever chunks the sweep leaves without a vector under the
// pinned embedding model.
//
// Authority rule (§7.2, §7.2a): the summary fence is always DERIVED --
// consolidate overwrites it unconditionally, exactly like a shard-tier
// fence head row. A fence-tier claims row is the opposite: for those
// families the *file* is truth, so consolidate never touches a row it
// did not itself derive from a shard -- ParseEntity reads it back
// byte-for-byte and RenderEntity re-emits it unchanged. Every write goes
// through writer.Fence, the only sanctioned entry point (RFC §7.7): a
// human edit sitting dirty on an entity page pauses that page's write
// (writer.ErrDirtyTree) rather than racing it, and the sweep continues to
// every other entity instead of aborting outright -- the same posture
// RFC §7.7 describes for every other machine writer in this repo.
package consolidate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// staleAfter is the RFC §10.4 example threshold ("nothing on X since
// March"): an entity whose most recent active claim is older than this
// gets the explicit stale-since banner instead of a plain activity count.
const staleAfter = 30 * 24 * time.Hour

// Consolidator runs one consolidate pass against a brain repo. Construct
// with New; the zero value is not usable (Queue/Fence/Shard are
// required).
type Consolidator struct {
	Queue  *writer.Queue
	Fence  *store.FenceWriter
	Shard  *store.ShardStore
	Config *config.Config
	// Now supplies the instant freshness banners render against. Nil
	// defaults to time.Now; every test supplies a fixed clock so two
	// passes at the same instant are byte-identical
	// (TestConsolidateIdempotent) and scheduled runs (internal/cron) stay
	// on the job's own injected Clock rather than calling time.Now
	// directly.
	Now func() time.Time
}

// New builds a Consolidator. q, fw, and ss must be non-nil. cfg nil
// defaults to config.Default().
func New(q *writer.Queue, fw *store.FenceWriter, ss *store.ShardStore, cfg *config.Config) *Consolidator {
	if cfg == nil {
		cfg = config.Default()
	}
	return &Consolidator{Queue: q, Fence: fw, Shard: ss, Config: cfg}
}

func (c *Consolidator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Report summarizes one Run.
type Report struct {
	// EntitiesVisited counts every (type, slug) the sweep considered,
	// whether or not its write actually landed.
	EntitiesVisited int
	// DirtySkipped lists the entity page paths whose write was paused by
	// the dirty-tree guard (writer.ErrDirtyTree) -- paused, not lost:
	// writer.PendingPath(root, slug) records both sides for later
	// resolution (RFC §7.7). Consolidate never aborts the rest of the
	// sweep over one paused page.
	DirtySkipped []string
	// Embedded counts chunks index.ReembedMissing actually embedded.
	// Zero when embedder is nil (no embedding model pinned yet -- the
	// same explicit-skip contract providers.BuildEmbeddingRouter uses).
	Embedded int
}

// Run walks every entity the brain repo knows about -- every existing
// fence page plus every slug that owns shard files but has no page yet --
// regenerates its summary and shard-tier head rows, then, when eng is
// non-nil, rebuilds the derived index and re-embeds whatever chunks come
// out of that rebuild lacking a vector under embedder's pin (embedder nil
// skips the re-embed step only).
func (c *Consolidator) Run(ctx context.Context, eng *index.SQLite, embedder index.Embedder) (Report, error) {
	var rep Report

	entities, err := c.entities()
	if err != nil {
		return rep, fmt.Errorf("consolidate: list entities: %w", err)
	}

	for _, ent := range entities {
		rep.EntitiesVisited++
		skipped, err := c.consolidateEntity(ent)
		if err != nil {
			return rep, fmt.Errorf("consolidate: entity %s/%s: %w", ent.Type, ent.Slug, err)
		}
		if skipped {
			rep.DirtySkipped = append(rep.DirtySkipped, c.Fence.PathFor(ent.Type, ent.Slug))
		}
	}

	if eng != nil {
		if err := index.Rebuild(ctx, c.Fence.Root, c.Config, eng); err != nil {
			return rep, fmt.Errorf("consolidate: rebuild index: %w", err)
		}
		if embedder != nil {
			embedded, err := index.ReembedMissing(ctx, eng, embedder)
			if err != nil {
				return rep, fmt.Errorf("consolidate: reembed: %w", err)
			}
			rep.Embedded = embedded
		}
	}

	return rep, nil
}

// entities returns every (type, slug) pair consolidate must visit, sorted
// by slug: every existing fence page under brain/entities, plus every
// slug that owns shard files but has no page yet (defaulting to
// ingest.DefaultEntityType, the same bucket every trust-0 write lands
// claims under until entity resolution assigns a real type).
func (c *Consolidator) entities() ([]domain.Entity, error) {
	seen := map[string]domain.Entity{}

	pages, err := filepath.Glob(filepath.Join(c.Fence.Root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range pages {
		p, err := c.Fence.ParseEntity(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if p.Entity.Slug == "" {
			return nil, fmt.Errorf("%s: entity page has no slug", path)
		}
		seen[p.Entity.Slug] = p.Entity
	}

	shardSlugs, err := c.Shard.Slugs()
	if err != nil {
		return nil, err
	}
	for _, slug := range shardSlugs {
		if _, ok := seen[slug]; !ok {
			seen[slug] = domain.Entity{Type: ingest.DefaultEntityType, Slug: slug}
		}
	}

	out := make([]domain.Entity, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// consolidateEntity regenerates one entity's summary and shard-tier head
// rows and writes it through writer.Fence. skipped is true when the
// dirty-tree guard paused the write -- the caller records that and moves
// on, never treating it as a hard failure.
func (c *Consolidator) consolidateEntity(ent domain.Entity) (skipped bool, err error) {
	path := c.Fence.PathFor(ent.Type, ent.Slug)
	p, err := c.Fence.ParseEntity(path)
	if err != nil {
		p = store.NewEntityPage(ent)
	}

	families, err := c.Shard.Families(ent.Slug)
	if err != nil {
		return false, fmt.Errorf("list shard families: %w", err)
	}
	for _, family := range families {
		heads, err := c.Shard.ResolveHeads(ent.Slug, family)
		if err != nil {
			return false, fmt.Errorf("resolve heads %s/%s: %w", ent.Slug, family, err)
		}
		// Shard-tier rows on the page are always derived (§7.2a): drop
		// whatever this family's rows currently say and replace them with
		// exactly the shard's own resolved heads. Fence-tier rows (every
		// other family) are untouched by this loop -- the file is truth
		// for those.
		kept := p.Claims[:0]
		for _, existing := range p.Claims {
			if existing.Family != family {
				kept = append(kept, existing)
			}
		}
		p.Claims = kept
		for _, key := range store.HeadKeys(heads) {
			head := heads[key]
			head.SourceRef = "shard"
			p.Claims = append(p.Claims, head)
		}
	}

	p.Summary = freshnessBanner(p.Claims, c.now())

	if _, _, err := writer.Fence(c.Queue, c.Fence, p); err != nil {
		if errors.Is(err, writer.ErrDirtyTree) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// freshnessBanner renders the RFC §10.4 freshness banner for an entity's
// active claims as of now: a stale-since note once the most recent active
// claim crosses staleAfter, otherwise a plain activity count. It is a
// pure function of (claims, now) so two consolidate passes at the same
// instant render an identical banner, and it never reads what a human
// last wrote in Summary -- the summary fence is always DERIVED (§7.2).
func freshnessBanner(claims []domain.Claim, now time.Time) string {
	active := 0
	var latest time.Time
	for _, c := range claims {
		if c.State != domain.StateActive {
			continue
		}
		active++
		if c.Provenance.ObservedAt.After(latest) {
			latest = c.Provenance.ObservedAt
		}
	}
	if active == 0 {
		return "No active claims."
	}
	if now.Sub(latest) >= staleAfter {
		return fmt.Sprintf("Nothing new since %s (%d active claim(s)).", latest.Format("January 2, 2006"), active)
	}
	return fmt.Sprintf("%d active claim(s), most recent %s.", active, latest.Format("January 2, 2006"))
}
