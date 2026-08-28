package search

import (
	"math"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

// TestRRFKIsPinned proves T1.11's acc line ("RRF k is a pinned constant")
// is a fact about the code, not just a comment: a change to the constant
// must be a deliberate, reviewed edit to this test too.
func TestRRFKIsPinned(t *testing.T) {
	if RRFK != 60 {
		t.Fatalf("RRFK = %d, want the pinned value 60", RRFK)
	}
}

func TestFuseRRFSumsContributionsAcrossBothChannels(t *testing.T) {
	vectorHits := []index.Hit{
		{ChunkRef: "c1", Text: "alpha"}, // vector rank 1
		{ChunkRef: "c2", Text: "beta"},  // vector rank 2
	}
	ftsHits := []index.Hit{
		{ChunkRef: "c2", Text: "beta"},  // fts rank 1
		{ChunkRef: "c3", Text: "gamma"}, // fts rank 2
	}
	results := fuseRRF(vectorHits, ftsHits)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3: %+v", len(results), results)
	}

	want := map[string]float64{
		"c1": 1.0 / (RRFK + 1),
		"c2": 1.0/(RRFK+2) + 1.0/(RRFK+1), // ranked in both channels
		"c3": 1.0 / (RRFK + 2),
	}
	const epsilon = 1e-9
	for _, r := range results {
		if got, wantScore := r.RRFScore, want[r.ChunkRef]; math.Abs(got-wantScore) > epsilon {
			t.Fatalf("chunk %s: RRFScore = %v, want %v", r.ChunkRef, got, wantScore)
		}
	}
	// c2's fused score (present in both channels) beats c1's, even though
	// c1 was ranked ahead of c2 in the vector channel alone.
	if results[0].ChunkRef != "c2" {
		t.Fatalf("results[0] = %+v, want c2 (highest fused score)", results[0])
	}
}

func TestFuseRRFDeterministicTiebreakOnEqualScore(t *testing.T) {
	// "z" and "a" each rank 1 in their own single channel -- equal fused
	// scores. The tie must break by ChunkRef ascending, not map order.
	results := fuseRRF([]index.Hit{{ChunkRef: "z"}}, []index.Hit{{ChunkRef: "a"}})
	if len(results) != 2 || results[0].ChunkRef != "a" || results[1].ChunkRef != "z" {
		t.Fatalf("equal-score ties must break by chunk_ref ascending, got %+v", results)
	}
}

func TestFuseRRFEmptyChannelsYieldNoResults(t *testing.T) {
	if results := fuseRRF(nil, nil); len(results) != 0 {
		t.Fatalf("got %d results from two empty channels, want 0: %+v", len(results), results)
	}
}
