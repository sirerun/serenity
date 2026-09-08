package index

import (
	"fmt"
	"path/filepath"
	"sync"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/store"
)

// sourceChunkAuthority compares derived rows with the same canonical projection
// Rebuild produces. The cache belongs to one request, never to the index lifetime.
func sourceChunkAuthority(root string, proj *store.MemoryProjection, restricted map[string]bool) (func(Hit) bool, error) {
	pages := make(map[string]Hit)
	paths, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		p, err := store.NewFenceWriter(root).ParseEntity(path)
		if err != nil {
			return nil, err
		}
		text := p.Title
		if !restricted[p.Entity.Slug] {
			text += "\n" + p.Summary
		}
		ref := "page:" + p.Entity.Slug
		pages[ref] = Hit{ChunkRef: ref, EntitySlug: p.Entity.Slug, Text: text, Kind: "entity_page"}
	}
	ss := store.NewSourceStore(root)
	var mu sync.Mutex
	cache := make(map[string]map[string]Hit)
	return func(hit Hit) bool {
		matches := func(expected Hit, exists bool) bool {
			return exists && hit.ChunkRef == expected.ChunkRef && hit.Text == expected.Text && hit.EntitySlug == expected.EntitySlug && hit.SourceSHA256 == expected.SourceSHA256 && hit.Kind == expected.Kind
		}
		if hit.Kind == "entity_page" {
			expected, ok := pages[hit.ChunkRef]
			return matches(expected, ok)
		}
		if !proj.SourceKnown(hit.SourceSHA256) || proj.IsLifecycle(hit.SourceSHA256) {
			return false
		}
		if rec, ok := proj.Get(hit.SourceSHA256); ok {
			return matches(Hit{ChunkRef: "fact:" + rec.SHA256, EntitySlug: rec.Payload.EntitySlug, Text: rec.Payload.Fact, SourceSHA256: rec.SHA256, Kind: store.SourceKindMemoryFact}, true)
		}
		mu.Lock()
		defer mu.Unlock()
		expected, loaded := cache[hit.SourceSHA256]
		if !loaded {
			expected = make(map[string]Hit)
			data, src, err := ss.Read(hit.SourceSHA256)
			if err == nil && utf8.Valid(data) {
				for _, part := range chunk.Split(string(data), chunk.DefaultConfig) {
					ref := fmt.Sprintf("src:%s:%d-%d", src.SHA256, part.Span.Start, part.Span.End)
					expected[ref] = Hit{ChunkRef: ref, Text: part.Text, SourceSHA256: src.SHA256, Kind: src.Kind}
				}
			}
			cache[hit.SourceSHA256] = expected
		}
		canonical, ok := expected[hit.ChunkRef]
		return matches(canonical, ok)
	}, nil
}
