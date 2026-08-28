package embed

import (
	"context"
	"fmt"

	"github.com/sirerun/serenity/internal/index"
)

// Store is what Search needs from the derived index (RFC §7.5).
// index.Engine (and *index.SQLite) satisfies it.
type Store interface {
	SearchVectors(ctx context.Context, model string, query []float32, limit int) ([]index.Hit, error)
	SearchFTS(ctx context.Context, query string, limit int) ([]index.Hit, error)
	HasVector(ctx context.Context, chunkRef, model string) (bool, error)
}

// Search answers query under exactly one embedding pin: pin's own vectors
// rank by cosine similarity, and any chunk the FTS index would otherwise
// surface but that has no vector under pin is included as a fallback hit
// rather than silently dropped or compared against a wrong-pin vector
// (RFC §10.1, §7.5 "not-yet-re-embedded chunks are served by FTS").
//
// A chunk lacking pin's vector but ranked by FTS is never re-scored
// against pin -- its FTS rank stands, appended after the vector hits, up
// to limit total results. This is the T1.10 acceptance behavior; the
// fuller RRF fusion of vector and lexical rank (T1.11) builds on top of
// this primitive rather than replacing it.
func Search(ctx context.Context, store Store, embedder Embedder, query string, limit int) ([]index.Hit, error) {
	if limit <= 0 {
		return nil, nil
	}
	pin := embedder.ModelVersion()

	qvec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: search: embed query: %w", err)
	}

	vectorHits, err := store.SearchVectors(ctx, pin, qvec, limit)
	if err != nil {
		return nil, fmt.Errorf("embed: search: vector scan: %w", err)
	}

	hits := make([]index.Hit, 0, limit)
	seen := make(map[string]bool, len(vectorHits))
	for _, h := range vectorHits {
		hits = append(hits, h)
		seen[h.ChunkRef] = true
	}
	if len(hits) >= limit {
		return hits[:limit], nil
	}

	ftsHits, err := store.SearchFTS(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("embed: search: fts scan: %w", err)
	}
	for _, h := range ftsHits {
		if seen[h.ChunkRef] {
			continue // already ranked via its own pin's vector -- FTS never re-adds it
		}
		hasPinVector, err := store.HasVector(ctx, h.ChunkRef, pin)
		if err != nil {
			return nil, fmt.Errorf("embed: search: has vector for %s: %w", h.ChunkRef, err)
		}
		if hasPinVector {
			// It has a pin vector but SearchVectors' top-limit cut it --
			// still never compared under FTS as a substitute ranking.
			continue
		}
		hits = append(hits, h)
		seen[h.ChunkRef] = true
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}
