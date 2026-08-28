package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

// fakeEmbedder is a test double implementing embed.Embedder: it returns a
// fixed vector regardless of the query text, so tests control the vector
// ranking exactly (a real provider call is out of scope here -- T1.10's
// own tests cover RouterEmbedder).
type fakeEmbedder struct {
	pin string
	vec []float32
}

func (f *fakeEmbedder) ModelVersion() string { return f.pin }
func (f *fakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return f.vec, nil
}

// TestSearchGoldenRankingOnFixtureCorpus is T1.11's acc "golden ranking
// test on the fixture corpus": a small, hand-computable corpus over a
// real SQLite index, asserting the exact fused order Search must produce.
//
// Fixture: 3 chunks from 3 distinct sources/kinds.
//   - c1 "quarterly budget review"        vector [1,0,0]  (email)
//   - c2 "unrelated gardening document"   vector [0,1,0]  (file)
//   - c3 "budget review notes in repo"    vector [.7,.7,0](git_repo)
//
// Query vector is [1,0,0] (exact match to c1, ~0.707 to c3, 0 to c2), and
// the query text "quarterly" lexically matches only c1 via FTS5.
//
// Vector ranking: c1 (cos=1.0), c3 (cos=~0.707), c2 (cos=0).
// FTS ranking:    c1 only.
// Fused (RRF, k=60): c1 = 1/61+1/61 ≈ .03279; c3 = 1/62 ≈ .01613;
// c2 = 1/63 ≈ .01587 -- so c1 > c3 > c2, with c1 pulling ahead of c3
// specifically because it also won the lexical channel.
func TestSearchGoldenRankingOnFixtureCorpus(t *testing.T) {
	ctx := context.Background()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()

	const pin = "fake-embed@v1"
	fixture := []struct {
		ref, sha, kind, text string
		vec                  []float32
	}{
		{"c1", "sha-1", "email", "quarterly budget review", []float32{1, 0, 0}},
		{"c2", "sha-2", "file", "unrelated document about gardening", []float32{0, 1, 0}},
		{"c3", "sha-3", "git_repo", "budget review notes in repo", []float32{0.7, 0.7, 0}},
	}
	for _, f := range fixture {
		if err := eng.InsertChunk(ctx, f.ref, "acme", f.text, f.sha, f.kind); err != nil {
			t.Fatal(err)
		}
		if err := eng.UpsertVector(ctx, f.ref, pin, f.vec); err != nil {
			t.Fatal(err)
		}
	}

	embedder := &fakeEmbedder{pin: pin, vec: []float32{1, 0, 0}}
	results, err := Search(ctx, eng, embedder, "quarterly", 10, Options{})
	if err != nil {
		t.Fatal(err)
	}

	wantOrder := []string{"c1", "c3", "c2"}
	if len(results) != len(wantOrder) {
		t.Fatalf("got %d results, want %d: %+v", len(results), len(wantOrder), results)
	}
	for i, ref := range wantOrder {
		if results[i].ChunkRef != ref {
			t.Fatalf("results[%d].ChunkRef = %q, want %q (full ranking: %+v)", i, results[i].ChunkRef, ref, results)
		}
	}
	if results[0].SourceSHA256 != "sha-1" || results[0].Kind != "email" {
		t.Fatalf("source metadata not hydrated through the pipeline: %+v", results[0])
	}
}

// TestSearchDegradesToFTSOnlyWithNoEmbedder proves the honest degraded
// mode: a nil embedder still returns results, ranked purely on the FTS
// channel, rather than erroring or fabricating a vector ranking.
func TestSearchDegradesToFTSOnlyWithNoEmbedder(t *testing.T) {
	ctx := context.Background()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()

	if err := eng.InsertChunk(ctx, "c1", "acme", "quarterly budget review", "sha-1", "email"); err != nil {
		t.Fatal(err)
	}

	results, err := Search(ctx, eng, nil, "quarterly", 10, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ChunkRef != "c1" {
		t.Fatalf("got %+v, want exactly c1 via the FTS channel alone", results)
	}
}

// TestSearchAppliesPerPageCapAcrossManyChunksFromOneSource exercises the
// full pipeline's layer 4 (per-page/document max): a single source
// contributing more distinct on-topic chunks than DefaultMaxPerPage must
// not fill the results list on its own.
func TestSearchAppliesPerPageCapAcrossManyChunksFromOneSource(t *testing.T) {
	ctx := context.Background()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()

	// 5 distinct chunks (distinct text, so layer 1's exact-source dedup
	// never fires here), same source -- more than DefaultMaxPerPage (3).
	sections := []string{"intro", "install", "config", "usage", "faq"}
	for i, section := range sections {
		ref := "c" + string(rune('1'+i))
		text := "widget release notes: " + section
		if err := eng.InsertChunk(ctx, ref, "acme", text, "sha-widget", "file"); err != nil {
			t.Fatal(err)
		}
	}

	results, err := Search(ctx, eng, nil, "widget", 10, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != DefaultMaxPerPage {
		t.Fatalf("got %d results, want %d (per-page cap enforced end to end): %+v", len(results), DefaultMaxPerPage, results)
	}
}
