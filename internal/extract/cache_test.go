package extract

import (
	"context"
	"testing"

	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/router"
)

// TestFileCacheRoundTrips proves FileCache persists a CachedOutput
// through a real file and reads it back unchanged -- the disk-backed
// path a fresh `serenity extract` CLI process relies on, unlike
// MemoryCache which only covers one process's lifetime.
func TestFileCacheRoundTrips(t *testing.T) {
	c := NewFileCache(t.TempDir())
	key := CacheKey{ChunkSHA256: "abc123", ModelVersion: "fake-extractor@v1", PromptVersion: PromptVersion}

	if _, hit, err := c.Get(context.Background(), key); err != nil {
		t.Fatalf("Get before Put: %v", err)
	} else if hit {
		t.Fatal("Get before Put: hit = true, want false")
	}

	want := CachedOutput{
		Accepted: []Candidate{{Subject: "acme-corp", Predicate: "works_at", Object: "Acme Corp", Confidence: 0.8}},
		Rejected: 1,
	}
	if err := c.Put(context.Background(), key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, hit, err := c.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if !hit {
		t.Fatal("Get after Put: hit = false, want true")
	}
	if len(got.Accepted) != 1 || got.Accepted[0] != want.Accepted[0] || got.Rejected != want.Rejected {
		t.Fatalf("Get after Put = %+v, want %+v", got, want)
	}

	// A different key must never collide with the one just written.
	other := CacheKey{ChunkSHA256: "def456", ModelVersion: "fake-extractor@v1", PromptVersion: PromptVersion}
	if _, hit, err := c.Get(context.Background(), other); err != nil {
		t.Fatalf("Get other key: %v", err)
	} else if hit {
		t.Fatal("Get other key: hit = true, want false")
	}
}

// TestExtractChunkWorksWithFileCache proves ExtractChunk's caching path
// is agnostic to which Cache implementation backs it -- FileCache
// deduplicates a repeated call exactly like MemoryCache does.
func TestExtractChunkWorksWithFileCache(t *testing.T) {
	respText := `{"observations":[{"subject":"acme-corp","predicate":"works_at","object":"Acme Corp","confidence":0.8}]}`
	fp := &fakeProvider{name: "fake", modelVersion: "fake-extractor@v1", resp: router.Response{Text: respText}}
	cache := NewFileCache(t.TempDir())
	ex := New(newTestRouter(fp), "fake-extractor@v1", nil, cache)

	ch := chunk.Chunk{Span: chunk.Span{Start: 0, End: 20}, Text: "Jane works at Acme."}
	if _, err := ex.ExtractChunk(context.Background(), "src-a", ch, router.Budget{}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := ex.ExtractChunk(context.Background(), "src-a", ch, router.Budget{}); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (FileCache should have served the second call)", fp.calls)
	}
}
