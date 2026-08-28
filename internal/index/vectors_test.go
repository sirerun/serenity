package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
)

func openTestEngine(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	eng, err := Open(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

// TestSearchVectorsNeverMixesPins proves the T1.10 core invariant: a
// search under pin A only ever returns A's own vectors, even when a B
// vector for a different chunk is a closer cosine match to the query.
func TestSearchVectorsNeverMixesPins(t *testing.T) {
	ctx := context.Background()
	eng := openTestEngine(t)

	if err := eng.InsertChunk(ctx, "c1", "acme", "alpha content", "sha-alpha", "file"); err != nil {
		t.Fatal(err)
	}
	if err := eng.InsertChunk(ctx, "c2", "acme", "beta content", "sha-beta", "file"); err != nil {
		t.Fatal(err)
	}

	// c1 gets an "A" vector pointing exactly at the query direction.
	if err := eng.UpsertVector(ctx, "c1", "modelA@v1", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	// c2 gets a "B" vector that is an even better match for the same
	// query -- if pins ever mixed, c2 would outrank or replace c1.
	if err := eng.UpsertVector(ctx, "c2", "modelB@v1", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}

	hits, err := eng.SearchVectors(ctx, "modelA@v1", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("SearchVectors(modelA) returned %d hits, want 1 (c2's B vector must never surface): %+v", len(hits), hits)
	}
	if hits[0].ChunkRef != "c1" {
		t.Fatalf("SearchVectors(modelA) hit = %+v, want chunk_ref c1", hits[0])
	}

	hitsB, err := eng.SearchVectors(ctx, "modelB@v1", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hitsB) != 1 || hitsB[0].ChunkRef != "c2" {
		t.Fatalf("SearchVectors(modelB) = %+v, want exactly c2", hitsB)
	}
}

// TestSearchVectorsRanksByCosineDescending checks the exact-cosine-scan
// ordering within a single pin.
func TestSearchVectorsRanksByCosineDescending(t *testing.T) {
	ctx := context.Background()
	eng := openTestEngine(t)

	for _, c := range []struct {
		ref string
		vec []float32
	}{
		{"near", []float32{0.9, 0.1, 0}},
		{"far", []float32{0, 0, 1}},
		{"exact", []float32{1, 0, 0}},
	} {
		if err := eng.InsertChunk(ctx, c.ref, "e", c.ref, "sha-"+c.ref, "file"); err != nil {
			t.Fatal(err)
		}
		if err := eng.UpsertVector(ctx, c.ref, "m@v1", c.vec); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := eng.SearchVectors(ctx, "m@v1", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(hits))
	}
	want := []string{"exact", "near", "far"}
	for i, w := range want {
		if hits[i].ChunkRef != w {
			t.Fatalf("hits[%d].ChunkRef = %q, want %q (order: %+v)", i, hits[i].ChunkRef, w, hits)
		}
	}
}

// TestHasVectorIsPinScoped proves HasVector answers for one pin only --
// the primitive callers use to route a chunk to FTS instead of comparing
// it against the wrong pin's vector.
func TestHasVectorIsPinScoped(t *testing.T) {
	ctx := context.Background()
	eng := openTestEngine(t)

	if err := eng.InsertChunk(ctx, "c1", "e", "text", "sha-c1", "file"); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "c1", "modelA@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}

	has, err := eng.HasVector(ctx, "c1", "modelA@v1")
	if err != nil || !has {
		t.Fatalf("HasVector(c1, modelA@v1) = %v, %v, want true, nil", has, err)
	}
	has, err = eng.HasVector(ctx, "c1", "modelB@v1")
	if err != nil || has {
		t.Fatalf("HasVector(c1, modelB@v1) = %v, %v, want false, nil -- c1 has no B vector", has, err)
	}
	has, err = eng.HasVector(ctx, "nonexistent", "modelA@v1")
	if err != nil || has {
		t.Fatalf("HasVector(unknown chunk) = %v, %v, want false, nil", has, err)
	}
}

// TestUpsertVectorIsIdempotentPerPin proves writing the same pin twice
// replaces in place (one row survives), while writing a second pin for
// the same chunk adds a row rather than clobbering the first.
func TestUpsertVectorIsIdempotentPerPin(t *testing.T) {
	ctx := context.Background()
	eng := openTestEngine(t)

	if err := eng.InsertChunk(ctx, "c1", "e", "text", "sha-c1", "file"); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "c1", "modelA@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "c1", "modelA@v1", []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "c1", "modelB@v1", []float32{0, 1}); err != nil {
		t.Fatal(err)
	}

	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["vectors"] != 2 {
		t.Fatalf("stats[vectors] = %d, want 2 (one row per pin, second modelA write replaced not appended)", stats["vectors"])
	}

	hitsA, err := eng.SearchVectors(ctx, "modelA@v1", []float32{0, 1}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hitsA) != 1 || hitsA[0].Score < 0.99 {
		t.Fatalf("modelA@v1 vector was not updated in place: %+v", hitsA)
	}
}

// TestVectorsParticipateInRebuildIdentity extends the M0 wipe-and-rebuild
// invariant (rebuild_test.go's TestWipeAndRebuildInvariant) to the vectors
// table: the same canonical inputs plus the same (fake, deterministic)
// embedding pass, dumped before and after a full wipe, must be byte for
// byte identical.
func TestVectorsParticipateInRebuildIdentity(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	cfg := config.Default()

	// deterministicEmbed stands in for a real embedding call: it is a
	// pure function of the chunk text, which is what a real provider's
	// embedding endpoint is expected to be for a fixed model version
	// (no sampling/temperature applies to embeddings).
	deterministicEmbed := func(text string) []float32 {
		var v [3]float32
		for i, r := range text {
			v[i%3] += float32(r)
		}
		return v[:]
	}

	populate := func(dbPath string) string {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			t.Fatal(err)
		}
		eng, err := Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = eng.Close() }()
		if err := Rebuild(ctx, root, cfg, eng); err != nil {
			t.Fatal(err)
		}
		// Embed every indexed chunk under a single pinned model -- the
		// same deterministic step a real ingest pipeline would perform
		// via internal/embed after Rebuild populates the chunks table.
		hits, err := eng.SearchFTS(ctx, "checking", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("fixture produced no chunks to embed")
		}
		for _, h := range hits {
			if err := eng.UpsertVector(ctx, h.ChunkRef, "fake-embed@v1", deterministicEmbed(h.Text)); err != nil {
				t.Fatal(err)
			}
		}
		dump, err := DumpString(ctx, eng)
		if err != nil {
			t.Fatal(err)
		}
		return dump
	}

	dbPath := filepath.Join(root, ".serenity", "index.db")
	dump1 := populate(dbPath)

	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatal(err)
	}
	dump2 := populate(dbPath)

	if dump1 != dump2 {
		t.Fatalf("rebuild + re-embed under an unchanged pin not byte-identical\n--- before ---\n%s\n--- after ---\n%s", dump1, dump2)
	}
	hasVectorRow := false
	for _, line := range strings.Split(dump1, "\n") {
		if strings.HasPrefix(line, "vectors\t") {
			hasVectorRow = true
			break
		}
	}
	if !hasVectorRow {
		t.Fatalf("dump has no vectors rows -- test did not exercise the vectors table:\n%s", dump1)
	}
}
