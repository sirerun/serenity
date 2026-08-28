package search

import (
	"context"
	"fmt"
	"math"
)

// Pinned defaults for the dedup layers below (T1.11 acc line). Small
// enough that one prolific source kind, or one long document, cannot fill
// a results page on its own; large enough that a genuinely dominant,
// on-topic source still shows up more than once. DefaultNearDupeCosine is
// the RFC-specified near-duplicate threshold.
const (
	DefaultMaxPerType     = 5
	DefaultMaxPerPage     = 3
	DefaultNearDupeCosine = 0.85
)

// dedupExactSource is dedup layer 1: two results sharing both
// SourceSHA256 and Text are the same evidence unit surfaced twice (e.g.
// once via the vector channel, once via a distinct chunk_ref from a
// reindex) -- only the higher-fused-score one survives. Distinct spans
// from the same source are untouched here; capping how many of those
// appear is layer 4's job. A result with no SourceSHA256 (no backing
// Source, e.g. a rebuild-derived entity-page chunk) never collapses
// against another -- there is nothing to prove they are the same source.
func dedupExactSource(results []Result) []Result {
	type key struct{ sha, text string }
	seen := make(map[key]bool, len(results))
	out := make([]Result, 0, len(results))
	for _, r := range results { // fuseRRF already sorted best-first
		if r.SourceSHA256 == "" {
			out = append(out, r)
			continue
		}
		k := key{r.SourceSHA256, r.Text}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// dedupNearDuplicates is dedup layer 2: when two results' own embedding
// vectors under pin are more than threshold cosine-similar, they are
// near-duplicate evidence -- only the higher-fused-score one survives.
// Comparison is chunk-vs-chunk (via Store.VectorFor), never chunk-vs-
// query. A result with no vector under pin (no embedder configured, or
// the chunk predates the pin) never participates: it is neither dropped
// nor used to drop another, since there is no vector to compare.
func dedupNearDuplicates(ctx context.Context, store Store, pin string, results []Result, threshold float64) ([]Result, error) {
	if pin == "" {
		return results, nil
	}
	vecs := make([][]float32, len(results))
	for i, r := range results {
		vec, ok, err := store.VectorFor(ctx, r.ChunkRef, pin)
		if err != nil {
			return nil, fmt.Errorf("vector for %s: %w", r.ChunkRef, err)
		}
		if ok {
			vecs[i] = vec
		}
	}

	dropped := make([]bool, len(results))
	out := make([]Result, 0, len(results))
	for i, r := range results { // best-first: i is never dropped by a lower-scored j
		if dropped[i] {
			continue
		}
		out = append(out, r)
		if vecs[i] == nil {
			continue
		}
		for j := i + 1; j < len(results); j++ {
			if dropped[j] || vecs[j] == nil {
				continue
			}
			sim, err := cosineSimilarity(vecs[i], vecs[j])
			if err != nil {
				return nil, err
			}
			if sim > threshold {
				dropped[j] = true
			}
		}
	}
	return out, nil
}

// capPerType is dedup layer 3: at most max results share one Kind (source
// kind -- email/file/git_repo/...), preserving fused-score order. A
// result with no Kind is never capped against another -- there is no
// declared type to enforce the cap on.
func capPerType(results []Result, max int) []Result {
	counts := make(map[string]int, len(results))
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.Kind == "" {
			out = append(out, r)
			continue
		}
		if counts[r.Kind] >= max {
			continue
		}
		counts[r.Kind]++
		out = append(out, r)
	}
	return out
}

// capPerPage is dedup layer 4: at most max results share one
// SourceSHA256 -- the "one long document dominates the results" case,
// where a single source produced many genuinely distinct on-topic chunks
// (layer 1 already removed literal duplicates; this caps distinct ones).
func capPerPage(results []Result, max int) []Result {
	counts := make(map[string]int, len(results))
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.SourceSHA256 == "" {
			out = append(out, r)
			continue
		}
		if counts[r.SourceSHA256] >= max {
			continue
		}
		counts[r.SourceSHA256]++
		out = append(out, r)
	}
	return out
}

// cosineSimilarity computes cosine similarity in [-1, 1] between two
// chunks' own embedding vectors under the same pin (dedupNearDuplicates
// only ever compares vectors it fetched under one model, so a dimension
// mismatch here means the store returned mixed-pin data -- a defensive
// error, not an expected path).
func cosineSimilarity(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
