// Package consolidate regenerates derived memory from canonical brain files.
package consolidate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ErrHeadDivergence preserves a hand-repaired shard head until the disposition
// workflow can append that intent to its canonical shard (RFC 7.2a).
var ErrHeadDivergence = errors.New("consolidate: edited shard head requires disposition")

// Result counts completed work. Nil Embedder explicitly selects FTS-only mode.
type Result struct{ Pages, Embedded int }

// Run uses the caller's single writer queue. It flushes canonical writes before
// rebuilding derived search data. Provider failure is retryable: the next run
// preserves successful vectors and fills only missing ones.
func Run(ctx context.Context, root string, cfg *config.Config, q *writer.Queue, eng *index.SQLite, embedder index.Embedder, now time.Time) (Result, error) {
	var result Result
	if cfg == nil || q == nil || eng == nil {
		return result, errors.New("consolidate: config, writer queue and index are required")
	}
	fw := store.NewFenceWriter(root)
	fw.Vocabulary = map[string]bool{}
	for name := range cfg.Families {
		fw.Vocabulary[name] = true
	}
	ss := store.NewShardStore(root)
	paths, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return result, err
	}
	pages := map[string]*store.EntityPage{}
	for _, path := range paths {
		p, err := fw.ParseEntity(path)
		if err != nil {
			return result, err
		}
		if p.Entity.Slug == "" || p.Entity.Type == "" {
			return result, fmt.Errorf("consolidate: missing entity identity in %s", path)
		}
		if filepath.Clean(fw.PathFor(p.Entity.Type, p.Entity.Slug)) != filepath.Clean(path) {
			return result, fmt.Errorf("consolidate: entity identity differs from path %s", path)
		}
		if _, exists := pages[p.Entity.Slug]; exists {
			return result, fmt.Errorf("consolidate: duplicate entity slug %s", p.Entity.Slug)
		}
		pages[p.Entity.Slug] = p
	}
	slugs, err := ss.Slugs()
	if err != nil {
		return result, err
	}
	for _, slug := range slugs {
		if pages[slug] == nil {
			pages[slug] = store.NewEntityPage(domain.Entity{Slug: slug, Type: ingest.DefaultEntityType})
		}
	}
	slugs = slugs[:0]
	for slug := range pages {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	// Prepare every page before writing, so a malformed page or diverged head
	// never leaves earlier pages half-consolidated.
	for _, slug := range slugs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		p := pages[slug]
		families, err := ss.Families(slug)
		if err != nil {
			return result, err
		}
		historical := map[string]domain.Claim{}
		var derived []domain.Claim
		for _, family := range families {
			if cfg.TierOf(family) != domain.TierShard {
				continue
			}
			lines, err := ss.Lines(slug, family)
			if err != nil {
				return result, err
			}
			for _, c := range lines {
				historical[c.ID] = c
			}
			heads, err := ss.ResolveHeads(slug, family)
			if err != nil {
				return result, err
			}
			for _, key := range store.HeadKeys(heads) {
				c := heads[key]
				c.SourceRef = "shard"
				derived = append(derived, c)
			}
		}
		var claims []domain.Claim
		for _, c := range p.Claims {
			if cfg.TierOf(c.Family) != domain.TierShard {
				claims = append(claims, c)
				continue
			}
			original, exists := historical[c.ID]
			if !exists || !sameHead(c, original) {
				return result, fmt.Errorf("%w: %s/%s", ErrHeadDivergence, slug, c.ID)
			}
		}
		p.Claims = append(claims, derived...)
		p.Summary = summary(p, now)
	}
	for _, slug := range slugs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		p := pages[slug]
		var writeErr error
		if _, statErr := os.Stat(fw.PathFor(p.Entity.Type, slug)); errors.Is(statErr, os.ErrNotExist) {
			_, _, writeErr = writer.Fence(q, fw, p)
		} else if statErr != nil {
			writeErr = statErr
		} else {
			_, _, writeErr = writer.FenceDerived(q, fw, p)
		}
		if err := writeErr; err != nil {
			// Commit earlier successful pages so their machine writes are not
			// mistaken for human edits when the paused file is retried.
			_, flushErr := writer.Flush(q, root)
			return result, errors.Join(err, flushErr)
		}
		result.Pages++
	}
	if _, err := writer.Flush(q, root); err != nil {
		return result, err
	}
	if err := index.Refresh(ctx, root, cfg, eng); err != nil {
		return result, fmt.Errorf("consolidate: refresh index: %w", err)
	}
	if embedder != nil {
		result.Embedded, err = index.ReembedMissing(ctx, eng, embedder)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func sameHead(a, b domain.Claim) bool {
	return a.Predicate == b.Predicate && a.Object == b.Object && a.ValidFrom == b.ValidFrom && a.ValidTo == b.ValidTo && a.State == b.State && a.SupersededBy == b.SupersededBy && a.SourceRef == "shard" && fmt.Sprintf("%.2f", a.Confidence) == fmt.Sprintf("%.2f", b.Confidence)
}

func summary(p *store.EntityPage, now time.Time) string {
	claims := append([]domain.Claim(nil), p.Claims...)
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Predicate != claims[j].Predicate {
			return claims[i].Predicate < claims[j].Predicate
		}
		return claims[i].ID < claims[j].ID
	})
	var lines []string
	var latest time.Time
	for _, c := range claims {
		if c.State != domain.StateActive {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", c.Predicate, c.Object))
		observed := c.Provenance.ObservedAt
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, c.ValidFrom); err == nil && t.After(observed) {
				observed = t
			}
		}
		if observed.After(latest) {
			latest = observed
		}
	}
	banner := "Freshness: no dated active evidence."
	if !latest.IsZero() {
		banner = "Freshness: latest active evidence " + latest.UTC().Format("2006-01-02") + "."
		if now.Sub(latest) >= 30*24*time.Hour {
			banner = "Freshness: nothing new on " + p.Title + " since " + latest.UTC().Format("2006-01-02") + "."
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "No active claims.")
	}
	return banner + "\n\n" + strings.Join(lines, "\n")
}
