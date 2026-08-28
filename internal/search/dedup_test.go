package search

import (
	"context"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

func result(ref, sha, kind, text string, score float64) Result {
	return Result{Hit: index.Hit{ChunkRef: ref, SourceSHA256: sha, Kind: kind, Text: text}, RRFScore: score}
}

// --- layer 1: exact source dedup ---

func TestDedupExactSourceCollapsesIdenticalTextFromSameSource(t *testing.T) {
	in := []Result{
		result("c1", "sha-a", "file", "identical text", 0.9),
		result("c2", "sha-a", "file", "identical text", 0.5), // same source+text, lower score: dropped
		result("c3", "sha-a", "file", "different text", 0.4), // same source, different text: survives
	}
	out := dedupExactSource(in)
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(out), out)
	}
	if out[0].ChunkRef != "c1" || out[1].ChunkRef != "c3" {
		t.Fatalf("unexpected survivors: %+v", out)
	}
}

func TestDedupExactSourceIgnoresResultsWithoutASourceSHA(t *testing.T) {
	in := []Result{
		result("c1", "", "entity_page", "same text", 0.9),
		result("c2", "", "entity_page", "same text", 0.5),
	}
	out := dedupExactSource(in)
	if len(out) != 2 {
		t.Fatalf("results with no SourceSHA256 must never collapse against each other, got %d: %+v", len(out), out)
	}
}

// --- layer 2: near-duplicate cosine collapse ---

type fakeVectorStore struct {
	vecs map[string][]float32
}

func (s *fakeVectorStore) SearchVectors(context.Context, string, []float32, int) ([]index.Hit, error) {
	return nil, nil
}
func (s *fakeVectorStore) SearchFTS(context.Context, string, int) ([]index.Hit, error) {
	return nil, nil
}
func (s *fakeVectorStore) VectorFor(_ context.Context, chunkRef, _ string) ([]float32, bool, error) {
	v, ok := s.vecs[chunkRef]
	return v, ok, nil
}

func TestDedupNearDuplicatesCollapsesAboveThreshold(t *testing.T) {
	store := &fakeVectorStore{vecs: map[string][]float32{
		"c1": {1, 0},
		"c2": {0.99, 0.01}, // cosine ~0.9998 with c1: near-duplicate
		"c3": {0, 1},       // orthogonal to c1: distinct
	}}
	in := []Result{
		result("c1", "sha-a", "file", "a", 0.9),
		result("c2", "sha-b", "file", "b", 0.7),
		result("c3", "sha-c", "file", "c", 0.6),
	}
	out, err := dedupNearDuplicates(context.Background(), store, "pin@v1", in, 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2 (c2 collapsed into c1): %+v", len(out), out)
	}
	if out[0].ChunkRef != "c1" || out[1].ChunkRef != "c3" {
		t.Fatalf("unexpected survivors: %+v", out)
	}
}

func TestDedupNearDuplicatesNeverDropsAResultLackingAVector(t *testing.T) {
	store := &fakeVectorStore{vecs: map[string][]float32{
		"c1": {1, 0},
		// c2 has no vector under this pin at all.
	}}
	in := []Result{
		result("c1", "sha-a", "file", "a", 0.9),
		result("c2", "sha-b", "file", "b", 0.7),
	}
	out, err := dedupNearDuplicates(context.Background(), store, "pin@v1", in, 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("a result lacking a vector must never be dropped by this layer, got %d: %+v", len(out), out)
	}
}

func TestDedupNearDuplicatesNoOpsWithoutAPin(t *testing.T) {
	in := []Result{
		result("c1", "sha-a", "file", "a", 0.9),
		result("c2", "sha-a", "file", "b", 0.8),
	}
	out, err := dedupNearDuplicates(context.Background(), &fakeVectorStore{}, "", in, 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2 (no pin configured: layer no-ops)", len(out))
	}
}

// --- layer 3: per-type cap ---

func TestCapPerTypeLimitsResultsPerKind(t *testing.T) {
	in := []Result{
		result("c1", "s1", "email", "a", 0.9),
		result("c2", "s2", "email", "b", 0.8),
		result("c3", "s3", "file", "c", 0.7),
	}
	out := capPerType(in, 1)
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2 (1 email + 1 file): %+v", len(out), out)
	}
	if out[0].ChunkRef != "c1" || out[1].ChunkRef != "c3" {
		t.Fatalf("must keep the higher-scored result per type: %+v", out)
	}
}

func TestCapPerTypeIgnoresResultsWithoutAKind(t *testing.T) {
	in := []Result{
		result("c1", "s1", "", "a", 0.9),
		result("c2", "s2", "", "b", 0.8),
	}
	out := capPerType(in, 1)
	if len(out) != 2 {
		t.Fatalf("results with no Kind must never be capped against each other, got %d: %+v", len(out), out)
	}
}

// --- layer 4: per-page/document cap ---

func TestCapPerPageLimitsResultsPerSource(t *testing.T) {
	in := []Result{
		result("c1", "sha-a", "file", "a", 0.9),
		result("c2", "sha-a", "file", "b", 0.8),
		result("c3", "sha-a", "file", "c", 0.7),
	}
	out := capPerPage(in, 2)
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2 (max 2 per SourceSHA256): %+v", len(out), out)
	}
	if out[0].ChunkRef != "c1" || out[1].ChunkRef != "c2" {
		t.Fatalf("must keep the higher-scored results: %+v", out)
	}
}

func TestCapPerPageIgnoresResultsWithoutASourceSHA(t *testing.T) {
	in := []Result{
		result("c1", "", "entity_page", "a", 0.9),
		result("c2", "", "entity_page", "b", 0.8),
	}
	out := capPerPage(in, 1)
	if len(out) != 2 {
		t.Fatalf("results with no SourceSHA256 must never be capped against each other, got %d: %+v", len(out), out)
	}
}
