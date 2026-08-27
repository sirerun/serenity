package index

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
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
		if err := eng.InsertChunk(ctx, "page:"+p.Entity.Slug, p.Entity.Slug, text); err != nil {
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
