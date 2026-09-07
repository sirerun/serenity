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
	// Eligible, when non-nil, filters candidates before every dedup layer
	// runs (T4.20, memory-compat-mapping.md item 5): a hit failing
	// Eligible is invisible to the caller and never occupies a per-type/
	// per-page slot an eligible hit could otherwise have used -- MEMORY_
	// VERBS's own private/expired-fact audience policy (store.
	// MemoryEligible) is the production use, but this stays a generic
	// predicate rather than importing store's own types here. Nil means
	// every hit is eligible (the pre-T4.20 behavior, byte-identical).
	Eligible func(index.Hit) bool
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

// poolWidenFactor and maxPoolWidenings bound how far Search retries with a
// larger candidate pool when Options.Eligible excludes enough hits to
// starve limit (T4.20 mapping item 5: "must not stop at the old limit*5
// pool when excluded hits occupy it. Widen or paginate until enough
// eligible results survive or the source is exhausted"). Doubling five
// times off a limit*5 base reaches limit*160 before giving up -- generous
// against a pool dominated by ineligible hits, still bounded so a genuinely
// exhausted store returns promptly rather than looping.
const poolWidenFactor = 2
const maxPoolWidenings = 5

// Search answers query by fusing the vector and full-text rankings via
// RRF (rrf.go) and running the fused list through 4 dedup layers
// (dedup.go, in order: exact-source, near-duplicate cosine, per-type cap,
// per-page cap) before truncating to limit. Options.Eligible, when set,
// runs BEFORE any dedup layer (mapping item 5: "apply eligibility before
// dedup/caps, so forbidden hits do not suppress eligible results") and, if
// it leaves fewer than limit results, Search widens its candidate pool and
// retries rather than returning a short page while the store still holds
// more to search -- exactly the "one public match remains visible below
// many private/expired matches" guarantee the mapping doc names.
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

	var pin string
	if embedder != nil {
		pin = embedder.ModelVersion()
	}

	var results []Result
	for attempt := 0; ; attempt++ {
		var vectorHits []index.Hit
		if embedder != nil {
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

		fused := fuseRRF(vectorHits, ftsHits)
		if opts.Eligible != nil {
			fused = filterEligible(fused, opts.Eligible)
		}

		fused = dedupExactSource(fused)
		fused, err = dedupNearDuplicates(ctx, store, pin, fused, opts.NearDupeCosine)
		if err != nil {
			return nil, fmt.Errorf("search: near-duplicate dedup: %w", err)
		}
		fused = capPerType(fused, opts.MaxPerType)
		fused = capPerPage(fused, opts.MaxPerPage)
		results = fused

		exhausted := len(vectorHits) < pool && len(ftsHits) < pool
		if len(results) >= limit || exhausted || attempt >= maxPoolWidenings {
			break
		}
		pool *= poolWidenFactor
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// filterEligible keeps only results eligible passes, preserving order.
func filterEligible(results []Result, eligible func(index.Hit) bool) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if eligible(r.Hit) {
			out = append(out, r)
		}
	}
	return out
}
