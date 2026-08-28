package search

import (
	"sort"

	"github.com/sirerun/serenity/internal/index"
)

// RRFK is Reciprocal Rank Fusion's smoothing constant: a chunk ranked r
// (1-indexed) in a channel contributes 1/(RRFK+r) to its fused score. 60
// is the standard constant from Cormack, Clarke & Buettcher 2009
// ("Reciprocal Rank Fusion Outperforms Condorcet and Individual Rank
// Learning Methods") -- large enough that a channel's long tail still
// contributes something, small enough that rank 1 vs rank 2 within a
// channel still matters. Pinned as a named constant rather than inlined
// so changing it is a deliberate, reviewed edit (T1.11 acc: "RRF k is a
// pinned constant") -- see TestRRFKIsPinned.
const RRFK = 60

// fuseRRF combines vectorHits and ftsHits -- each already ranked
// best-first by its own channel -- into one Result per distinct
// chunk_ref: a chunk present in both channels sums both rank
// contributions. Output is ordered by fused score descending, ChunkRef
// ascending on ties (deterministic; ties are otherwise arbitrary since
// map iteration order is not).
func fuseRRF(vectorHits, ftsHits []index.Hit) []Result {
	scores := make(map[string]float64)
	hits := make(map[string]index.Hit)
	add := func(ranked []index.Hit) {
		for i, h := range ranked {
			scores[h.ChunkRef] += 1.0 / float64(RRFK+i+1)
			if _, ok := hits[h.ChunkRef]; !ok {
				hits[h.ChunkRef] = h
			}
		}
	}
	add(vectorHits)
	add(ftsHits)

	results := make([]Result, 0, len(hits))
	for ref, h := range hits {
		results = append(results, Result{Hit: h, RRFScore: scores[ref]})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].RRFScore != results[j].RRFScore {
			return results[i].RRFScore > results[j].RRFScore
		}
		return results[i].ChunkRef < results[j].ChunkRef
	})
	return results
}
