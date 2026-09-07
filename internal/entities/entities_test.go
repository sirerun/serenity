package entities

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

// fakeEmbedder is a test double implementing embed.Embedder (zero-stub
// policy: test doubles live in _test.go files only, embed.go's own
// convention). It answers with a fixed vector per input text, set up by
// each test via vectors.
type fakeEmbedder struct {
	vectors map[string][]float32
	calls   int
}

func (f *fakeEmbedder) ModelVersion() string { return "fake-embed@v1" }

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.calls++
	v, ok := f.vectors[text]
	if !ok {
		return nil, fmt.Errorf("fakeEmbedder: no fixture vector for %q", text)
	}
	return v, nil
}

// unitVec2 returns a 2D unit vector [s, sqrt(1-s^2)] -- cosine([1,0],
// unitVec2(s)) == s exactly (up to float rounding), which lets tests pick
// an exact target similarity score instead of reverse-engineering one.
func unitVec2(s float64) []float32 {
	return []float32{float32(s), float32(math.Sqrt(1 - s*s))}
}

func alice() domain.Entity {
	return domain.Entity{Type: "person", Slug: "alice-tan", Aliases: []string{"Alice"}}
}

func aliceB() domain.Entity {
	return domain.Entity{Type: "person", Slug: "a-tan", Aliases: []string{"alice-tan"}}
}

func TestIsAliasMatchExactSlugOverlap(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "alice-tan"}
	b := domain.Entity{Type: "person", Slug: "alice-tan", Aliases: []string{"Alice"}}
	if !isAliasMatch(a, b) {
		t.Fatal("identical slugs must alias-match")
	}
}

func TestIsAliasMatchViaAliasField(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "alice-tan", Aliases: []string{"a-tan", "Alice"}}
	b := domain.Entity{Type: "person", Slug: "a-tan"} // b's own slug equals one of a's aliases
	if !isAliasMatch(a, b) {
		t.Fatal("b.Slug matching an entry in a.Aliases must alias-match")
	}
}

func TestIsAliasMatchNormalizesCaseAndWhitespace(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "alice-tan", Aliases: []string{"  Alice Tan  "}}
	b := domain.Entity{Type: "person", Slug: "bob-lee", Aliases: []string{"alice tan"}}
	if !isAliasMatch(a, b) {
		t.Fatal("alias comparison must normalize case and surrounding whitespace")
	}
}

func TestIsAliasMatchFalseForUnrelatedEntities(t *testing.T) {
	if isAliasMatch(alice(), domain.Entity{Type: "person", Slug: "bob-lee"}) {
		t.Fatal("unrelated entities must not alias-match")
	}
}

func TestEvaluateAutoMergeViaAliasNeverCallsEmbedder(t *testing.T) {
	fe := &fakeEmbedder{}
	d, err := Evaluate(context.Background(), fe, alice(), aliceB(), DefaultThresholds())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Verdict != VerdictAutoMerge || d.Stage != StageAlias || d.Score != 1.0 {
		t.Fatalf("got %+v, want AutoMerge/alias/1.0", d)
	}
	if fe.calls != 0 {
		t.Fatalf("alias match must short-circuit before any embedder call, got %d calls", fe.calls)
	}
}

func TestEvaluateEmbeddingAutoMerge(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-corporation"}
	fe := &fakeEmbedder{vectors: map[string][]float32{
		embeddingText(a): {1, 0},
		embeddingText(b): unitVec2(0.95),
	}}
	d, err := Evaluate(context.Background(), fe, a, b, DefaultThresholds())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Verdict != VerdictAutoMerge || d.Stage != StageEmbedding {
		t.Fatalf("got %+v, want AutoMerge/embedding", d)
	}
	if math.Abs(d.Score-0.95) > 1e-4 {
		t.Fatalf("score = %v, want ~0.95", d.Score)
	}
}

func TestEvaluateEmbeddingAmbiguous(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-holdings"}
	fe := &fakeEmbedder{vectors: map[string][]float32{
		embeddingText(a): {1, 0},
		embeddingText(b): unitVec2(0.80),
	}}
	d, err := Evaluate(context.Background(), fe, a, b, DefaultThresholds())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Verdict != VerdictAmbiguous || d.Stage != StageEmbedding {
		t.Fatalf("got %+v, want Ambiguous/embedding", d)
	}
}

func TestEvaluateEmbeddingNoMatch(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "unrelated-widgets"}
	fe := &fakeEmbedder{vectors: map[string][]float32{
		embeddingText(a): {1, 0},
		embeddingText(b): unitVec2(0.30),
	}}
	d, err := Evaluate(context.Background(), fe, a, b, DefaultThresholds())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Verdict != VerdictNoMatch {
		t.Fatalf("got %+v, want NoMatch", d)
	}
}

func TestEvaluateRefusesTypeMismatch(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "company", Slug: "acme-corp-inc"}
	_, err := Evaluate(context.Background(), &fakeEmbedder{}, a, b, DefaultThresholds())
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("err = %v, want ErrTypeMismatch", err)
	}
}

func TestEvaluateRefusesSameSlug(t *testing.T) {
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	_, err := Evaluate(context.Background(), &fakeEmbedder{}, a, a, DefaultThresholds())
	if err == nil {
		t.Fatal("want an error comparing an entity to itself")
	}
}

func TestCosineKnownValues(t *testing.T) {
	if s, err := cosine([]float32{1, 0}, []float32{1, 0}); err != nil || math.Abs(s-1) > 1e-6 {
		t.Fatalf("identical vectors: score=%v err=%v, want 1", s, err)
	}
	if s, err := cosine([]float32{1, 0}, []float32{0, 1}); err != nil || math.Abs(s) > 1e-6 {
		t.Fatalf("orthogonal vectors: score=%v err=%v, want 0", s, err)
	}
	if _, err := cosine([]float32{1, 0}, []float32{1, 0, 0}); err == nil {
		t.Fatal("mismatched dimensions must error")
	}
}
