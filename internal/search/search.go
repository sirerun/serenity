// Package search implements Serenity's hybrid search (RFC 0001 §10.1,
// §16): the vector and full-text rankings internal/index already exposes
// are fused into one score per chunk via Reciprocal Rank Fusion (RRF,
// rrf.go), then passed through 4 independent dedup layers (dedup.go)
// before being truncated to the caller's limit.
//
// This builds on top of internal/embed's Search primitive (T1.10) rather
// than replacing it: that function answers "rank by my pin's vectors,
// fall back to FTS for chunks the pin hasn't reached yet" -- a single
// primary ranking with a gap-filler. Hybrid search here asks a different
// question -- "how do these two independent rankings agree?" -- so it
// calls Store's vector and lexical scans directly and fuses both
// rankings, rather than treating one as primary and the other as a
// fallback.
package search

import (
	"context"
	"fmt"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/index"
)

// Store is what Search needs from the derived index: independent vector
// and lexical rankings, plus a per-chunk vector lookup for near-duplicate
// dedup. *index.SQLite (index.Engine) satisfies it.
type Store interface {
	SearchVectors(ctx context.Context, model string, query []float32, limit int) ([]index.Hit, error)
	SearchFTS(ctx context.Context, query string, limit int) ([]index.Hit, error)
	VectorFor(ctx context.Context, chunkRef, model string) ([]float32, bool, error)
}

// Result is one ranked, deduplicated hit: the underlying index.Hit plus
// the fused RRF score it carries through the dedup pipeline.
type Result struct {
	index.Hit
	RRFScore float64
}

// Options configures the dedup layers (dedup.go). The zero value uses the
// pinned defaults (DefaultMaxPerType, DefaultMaxPerPage,
// DefaultNearDupeCosine).
type Options struct {
	MaxPerType     int
	MaxPerPage     int
	NearDupeCosine float64
}

func (o Options) withDefaults() Options {
	if o.MaxPerType <= 0 {
		o.MaxPerType = DefaultMaxPerType
	}
	if o.MaxPerPage <= 0 {
		o.MaxPerPage = DefaultMaxPerPage
	}
	if o.NearDupeCosine <= 0 {
		o.NearDupeCosine = DefaultNearDupeCosine
	}
	return o
}

// candidatePoolMultiplier widens each channel's own request beyond limit
// so the dedup layers still have enough surviving candidates to fill
// limit results after collapsing duplicates and applying the per-type and
// per-page caps.
const candidatePoolMultiplier = 5

// Search answers query by fusing the vector and full-text rankings via
// RRF (rrf.go) and running the fused list through 4 dedup layers
// (dedup.go, in order: exact-source, near-duplicate cosine, per-type cap,
// per-page cap) before truncating to limit.
//
// embedder may be nil: Search then skips the vector channel entirely and
// ranks on the FTS channel alone, through the same fusion and dedup
// pipeline. This is the honest degraded mode for a brain repo with no
// embedding model pinned (or no live provider wired yet) rather than
// erroring or fabricating a vector ranking.
func Search(ctx context.Context, store Store, embedder embed.Embedder, query string, limit int, opts Options) ([]Result, error) {
	if limit <= 0 {
		return nil, nil
	}
	opts = opts.withDefaults()
	pool := limit * candidatePoolMultiplier

	var vectorHits []index.Hit
	var pin string
	if embedder != nil {
		pin = embedder.ModelVersion()
		qvec, err := embedder.Embed(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search: embed query: %w", err)
		}
		vectorHits, err = store.SearchVectors(ctx, pin, qvec, pool)
		if err != nil {
			return nil, fmt.Errorf("search: vector scan: %w", err)
		}
	}

	ftsHits, err := store.SearchFTS(ctx, query, pool)
	if err != nil {
		return nil, fmt.Errorf("search: fts scan: %w", err)
	}

	results := fuseRRF(vectorHits, ftsHits)

	results = dedupExactSource(results)
	results, err = dedupNearDuplicates(ctx, store, pin, results, opts.NearDupeCosine)
	if err != nil {
		return nil, fmt.Errorf("search: near-duplicate dedup: %w", err)
	}
	results = capPerType(results, opts.MaxPerType)
	results = capPerPage(results, opts.MaxPerPage)

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
