package embed

import (
	"context"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

// fakeStore is a test double implementing Store, backed by plain maps so
// Search's orchestration logic (pin scoping, FTS fallback, dedup) can be
// exercised without a real SQLite engine.
type fakeStore struct {
	vectorHits map[string][]index.Hit // model -> hits SearchVectors returns
	ftsHits    []index.Hit
	hasVector  map[string]bool // "chunkRef|model" -> bool
}

func vecKey(chunkRef, model string) string { return chunkRef + "|" + model }

func (s *fakeStore) SearchVectors(_ context.Context, model string, _ []float32, limit int) ([]index.Hit, error) {
	hits := s.vectorHits[model]
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (s *fakeStore) SearchFTS(_ context.Context, _ string, limit int) ([]index.Hit, error) {
	hits := s.ftsHits
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (s *fakeStore) HasVector(_ context.Context, chunkRef, model string) (bool, error) {
	return s.hasVector[vecKey(chunkRef, model)], nil
}

// fakeEmbedder is a test double implementing Embedder for Search tests.
type fakeEmbedder struct {
	pin string
	vec []float32
}

func (f *fakeEmbedder) ModelVersion() string { return f.pin }
func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return f.vec, nil
}

func TestSearchReturnsOnlyPinVectorHitsPlusFallback(t *testing.T) {
	store := &fakeStore{
		vectorHits: map[string][]index.Hit{
			"pinA@v1": {{ChunkRef: "c1", Text: "alpha", Score: 0.95}},
			// A different pin's vectors exist in the store but must
			// never surface from a pinA search.
			"pinB@v1": {{ChunkRef: "c2", Text: "beta", Score: 0.99}},
		},
		ftsHits: []index.Hit{
			{ChunkRef: "c3", Text: "gamma lexical match", Score: 1.2},
		},
		hasVector: map[string]bool{
			// c3 has no vector under pinA -- must be served by FTS.
		},
	}
	embedder := &fakeEmbedder{pin: "pinA@v1", vec: []float32{1, 0}}

	hits, err := Search(context.Background(), store, embedder, "query", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2 (c1 via vector, c3 via FTS fallback): %+v", len(hits), hits)
	}
	if hits[0].ChunkRef != "c1" {
		t.Fatalf("hits[0] = %+v, want c1 (pinA's own vector hit ranks first)", hits[0])
	}
	if hits[1].ChunkRef != "c3" {
		t.Fatalf("hits[1] = %+v, want c3 (FTS fallback for a chunk lacking pinA's vector)", hits[1])
	}
	for _, h := range hits {
		if h.ChunkRef == "c2" {
			t.Fatalf("pinB's vector hit (c2) leaked into a pinA search: %+v", hits)
		}
	}
}

func TestSearchDoesNotFTSFallbackAChunkThatHasThePinVector(t *testing.T) {
	// c1 has a pinA vector but didn't make SearchVectors' top-K cut (the
	// fake store just never lists it there). It also happens to lexically
	// match via FTS. It must NOT be re-surfaced through the FTS fallback
	// path -- fallback is reserved for chunks that lack the pin's vector
	// entirely, never as a second ranking channel for ones that have it.
	store := &fakeStore{
		vectorHits: map[string][]index.Hit{
			"pinA@v1": {}, // c1 didn't rank in the top-K vector scan
		},
		ftsHits: []index.Hit{
			{ChunkRef: "c1", Text: "alpha", Score: 1.0},
		},
		hasVector: map[string]bool{
			vecKey("c1", "pinA@v1"): true,
		},
	}
	embedder := &fakeEmbedder{pin: "pinA@v1", vec: []float32{1, 0}}

	hits, err := Search(context.Background(), store, embedder, "query", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("got %d hits, want 0 -- c1 has a pinA vector so FTS must not substitute for it: %+v", len(hits), hits)
	}
}

func TestSearchNeverDoubleCountsAChunkPresentInBothChannels(t *testing.T) {
	store := &fakeStore{
		vectorHits: map[string][]index.Hit{
			"pinA@v1": {{ChunkRef: "c1", Text: "alpha", Score: 0.9}},
		},
		ftsHits: []index.Hit{
			{ChunkRef: "c1", Text: "alpha", Score: 1.0},
		},
	}
	embedder := &fakeEmbedder{pin: "pinA@v1", vec: []float32{1, 0}}

	hits, err := Search(context.Background(), store, embedder, "query", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1 (c1 counted once): %+v", len(hits), hits)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	store := &fakeStore{
		vectorHits: map[string][]index.Hit{
			"pinA@v1": {
				{ChunkRef: "c1", Score: 0.9},
				{ChunkRef: "c2", Score: 0.8},
			},
		},
		ftsHits: []index.Hit{
			{ChunkRef: "c3", Score: 1.0},
			{ChunkRef: "c4", Score: 0.5},
		},
	}
	embedder := &fakeEmbedder{pin: "pinA@v1", vec: []float32{1, 0}}

	hits, err := Search(context.Background(), store, embedder, "query", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3 (limit respected): %+v", len(hits), hits)
	}
}

func TestSearchZeroLimitReturnsNoHits(t *testing.T) {
	store := &fakeStore{}
	embedder := &fakeEmbedder{pin: "pinA@v1", vec: []float32{1, 0}}

	hits, err := Search(context.Background(), store, embedder, "query", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("got %d hits, want 0", len(hits))
	}
}
