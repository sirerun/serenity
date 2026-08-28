package index

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/store"
)

// Rebuild reconstructs the entire derived index from canonical repo bytes
// (RFC §7 preamble: "there are no database backups — the index rebuilds
// from the repo"). Deterministic: files are walked in sorted order and
// every derived row comes from the file contents alone.
//
// Authority rule (§7.2a): for shard-tier families the shard is canonical
// and the fence head row is derived — so fence rows belonging to
// shard-tier families are skipped here and the shard resolution is
// indexed instead. A hand-edited (diverged) fence head therefore never
// leaks into the index; the reconciler (M2) turns such divergence into a
// disposition item.
func Rebuild(ctx context.Context, root string, cfg *config.Config, eng Engine) error {
	if err := eng.ResetAll(ctx); err != nil {
		return err
	}

	// Entity pages: entities, fence-tier claims, page chunks.
	pages, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(pages)
	fw := store.NewFenceWriter(root)
	for _, path := range pages {
		p, err := fw.ParseEntity(path)
		if err != nil {
			return fmt.Errorf("rebuild %s: %w", path, err)
		}
		if p.Entity.Slug == "" {
			return fmt.Errorf("rebuild %s: page has no slug", path)
		}
		if err := eng.UpsertEntity(ctx, p.Entity); err != nil {
			return err
		}
		for _, c := range p.Claims {
			if cfg.TierOf(c.Family) == domain.TierShard {
				continue // derived head row; the shard below is canonical
			}
			if c.ObjectKey == "" {
				c.ObjectKey = store.NormalizeKey(c.Object)
			}
			if err := eng.UpsertClaim(ctx, c); err != nil {
				return err
			}
		}
		text := p.Title + "\n" + p.Summary
		// Entity-page chunks are derived summaries, not raw Source
		// material -- no SourceSHA256 to carry, "entity_page" as their
		// own Kind bucket for internal/search's per-type cap (T1.11).
		if err := eng.InsertChunk(ctx, "page:"+p.Entity.Slug, p.Entity.Slug, text, "", "entity_page"); err != nil {
			return err
		}
	}

	// Shards: resolved heads are the indexed truth for shard-tier families.
	ss := store.NewShardStore(root)
	slugs, err := ss.Slugs()
	if err != nil {
		return err
	}
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return err
		}
		for _, family := range families {
			heads, err := ss.ResolveHeads(slug, family)
			if err != nil {
				return err
			}
			for _, key := range store.HeadKeys(heads) {
				c := heads[key]
				c.SourceRef = "shard"
				if err := eng.UpsertClaim(ctx, c); err != nil {
					return err
				}
			}
		}
	}

	// Raw sources: index every stored source's own text for full-text
	// search (RFC §10.1's "index" pipeline stage) -- searchable evidence
	// distinct from the entity-page/claim-derived chunks above, available
	// even before extraction has run over it (T1.15's `serenity sync`).
	// Sorted by SHA256 (SourceStore.All's own order) for the same
	// reproducibility guarantee the entity-page glob above gets from its
	// sort.Strings. Binary/non-UTF8 sources (PDFs, images -- no
	// text-extraction pipeline exists yet in this codebase, a disclosed
	// v1 gap) are stored and git-committed by `serenity sync` like any
	// other source; they are simply never chunked into the FTS index here.
	srcStore := store.NewSourceStore(root)
	sources, err := srcStore.All()
	if err != nil {
		return err
	}
	for _, src := range sources {
		data, _, err := srcStore.Read(src.SHA256)
		if err != nil {
			return fmt.Errorf("rebuild: read source %s: %w", src.SHA256, err)
		}
		if !utf8.Valid(data) {
			continue
		}
		for _, ch := range chunk.Split(string(data), chunk.DefaultConfig) {
			ref := fmt.Sprintf("src:%s:%d-%d", src.SHA256, ch.Span.Start, ch.Span.End)
			if err := eng.InsertChunk(ctx, ref, "", ch.Text, src.SHA256, src.Kind); err != nil {
				return err
			}
		}
	}
	return nil
}

// DumpString renders the deterministic dump as a string (test helper and
// doctor output).
func DumpString(ctx context.Context, eng Engine) (string, error) {
	var b strings.Builder
	if err := eng.Dump(ctx, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Embedder is the subset of internal/embed.Embedder that ReembedMissing
// needs, declared locally so this package never has to import
// internal/embed -- the same asymmetric-dependency shape jobs.go uses for
// connector.JobStore and ledger.go's doc comment describes for
// router.SpendLedger. *embed.RouterEmbedder satisfies this structurally.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	ModelVersion() string
}

// ReembedMissing fills in every indexed chunk's vector under embedder's
// pin (RFC §10.1's "embed" pipeline stage), skipping chunks that already
// have one under that pin. It lives here, not in the CLI, because
// UpsertVector is an index-write primitive the file-first CI gate
// (internal/gate) restricts to this exact file plus internal/writer/ --
// the same rule Rebuild itself is allowlisted under. Called after Rebuild
// (which wipes and fully reconstructs the vectors table via ResetAll):
// the wipe-and-rebuild invariant requires every vector to be reproducible
// purely from repo bytes plus the pinned model, never carried forward as
// separate state, so a real embedding pass -- a pure function of (pin,
// text), per T1.10's own TestVectorsParticipateInRebuildIdentity -- runs
// fresh after every Rebuild, not only for newly added chunks.
func ReembedMissing(ctx context.Context, eng *SQLite, embedder Embedder) (embedded int, err error) {
	chunks, err := eng.AllChunks(ctx)
	if err != nil {
		return 0, err
	}
	pin := embedder.ModelVersion()
	for _, c := range chunks {
		has, err := eng.HasVector(ctx, c.ChunkRef, pin)
		if err != nil {
			return embedded, err
		}
		if has {
			continue
		}
		vec, err := embedder.Embed(ctx, c.Text)
		if err != nil {
			return embedded, fmt.Errorf("index: reembed chunk %s: %w", c.ChunkRef, err)
		}
		if err := eng.UpsertVector(ctx, c.ChunkRef, pin, vec); err != nil {
			return embedded, err
		}
		embedded++
	}
	return embedded, nil
}
