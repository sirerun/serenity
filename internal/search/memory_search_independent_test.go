// Independent retrieval-policy regressions; prepared outside the active worker.
package search

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

type independentRankedStore struct{ hits []index.Hit }

func (s independentRankedStore) SearchFTS(_ context.Context, _ string, limit int) ([]index.Hit, error) {
	if limit > len(s.hits) {
		limit = len(s.hits)
	}
	return append([]index.Hit(nil), s.hits[:limit]...), nil
}

func (s independentRankedStore) SearchVectors(ctx context.Context, _ string, _ []float32, limit int) ([]index.Hit, error) {
	return s.SearchFTS(ctx, "", limit)
}

func (s independentRankedStore) VectorFor(context.Context, string, string) ([]float32, bool, error) {
	return nil, false, nil
}

func independentRankedFixture(forbidden int) independentRankedStore {
	s := independentRankedStore{}
	for i := 0; i < forbidden; i++ {
		s.hits = append(s.hits, index.Hit{ChunkRef: fmt.Sprintf("private-%d", i), SourceSHA256: fmt.Sprintf("private-source-%d", i), Kind: "memory_fact"})
	}
	s.hits = append(s.hits, index.Hit{ChunkRef: "public", SourceSHA256: "public-source", Kind: "memory_fact"})
	return s
}

func TestMemorySearchFindsEligibleHitBeyondDenseForbiddenPrefix(t *testing.T) {
	// A dense private/expired prefix must not turn an existing public match
	// into an empty result simply because a larger fixed pool was exhausted.
	s := independentRankedFixture(513)
	got, err := Search(context.Background(), s, nil, "shared term", 1, Options{
		Eligible: func(h index.Hit) bool { return h.ChunkRef == "public" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ChunkRef != "public" {
		t.Fatalf("eligible match hidden behind forbidden prefix: %+v", got)
	}
}

type independentCountingEmbedder struct{ calls int }

func (*independentCountingEmbedder) ModelVersion() string { return "independent@v1" }
func (e *independentCountingEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	return []float32{1}, nil
}

func TestMemorySearchEmbedsQueryOnceWhileBackfilling(t *testing.T) {
	e := &independentCountingEmbedder{}
	got, err := Search(context.Background(), independentRankedFixture(20), e, "shared term", 1, Options{
		Eligible: func(h index.Hit) bool { return h.ChunkRef == "public" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ChunkRef != "public" {
		t.Fatalf("missing eligible public result: %+v", got)
	}
	if e.calls != 1 {
		t.Fatalf("one query triggered %d embedding calls during pool expansion; want 1", e.calls)
	}
}
