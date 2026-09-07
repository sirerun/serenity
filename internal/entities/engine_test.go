package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
)

func openTestDispositionStore(t *testing.T) *disposition.Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

// TestEngineResolveAmbiguousStagesDispositionNotMerge is T2.13's acc-line
// clause: "an ambiguous pair yields a disposition item, not a merge."
func TestEngineResolveAmbiguousStagesDispositionNotMerge(t *testing.T) {
	r := newTestRig(t)
	ds := openTestDispositionStore(t)
	ctx := context.Background()

	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-holdings"}
	aPage := store.NewEntityPage(a)
	aPage.Claims = []domain.Claim{fenceClaim("c-a1", a.Slug, "works_at", "acme")}
	aPath := r.seedPage(t, aPage)
	bPage := store.NewEntityPage(b)
	bPage.Claims = []domain.Claim{fenceClaim("c-b1", b.Slug, "works_at", "acme holdings")}
	bPath := r.seedPage(t, bPage)

	aBefore, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	bBefore, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}

	fe := &fakeEmbedder{vectors: map[string][]float32{
		embeddingText(a): {1, 0},
		embeddingText(b): unitVec2(0.80), // between Ambiguous (0.75) and AutoMerge (0.92)
	}}

	eng := NewEngine(r.q, r.fw, r.ss, ds, fe)
	d, ev, item, err := eng.Resolve(ctx, a, b, mergeFixedNow, "machine")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Verdict != VerdictAmbiguous {
		t.Fatalf("Verdict = %v, want Ambiguous", d.Verdict)
	}
	if ev != nil {
		t.Fatal("Resolve must not return a MergeEvent for an ambiguous verdict")
	}
	if item == nil {
		t.Fatal("Resolve must stage a disposition item for an ambiguous verdict")
	}
	if item.Kind != disposition.KindEntityMerge {
		t.Fatalf("item.Kind = %v, want KindEntityMerge", item.Kind)
	}
	if item.State != disposition.StatePending {
		t.Fatalf("item.State = %v, want StatePending", item.State)
	}

	// Nothing must have merged: both pages byte-identical to before, and
	// the item is genuinely pending in the store.
	aAfter, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(aBefore, aAfter) {
		t.Fatal("a's page changed even though the verdict was Ambiguous, not AutoMerge")
	}
	bAfter, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bBefore, bAfter) {
		t.Fatal("b's page changed even though the verdict was Ambiguous, not AutoMerge")
	}

	stored, err := ds.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get staged item: %v", err)
	}
	var payload MergeCandidatePayload
	if err := json.Unmarshal(stored.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.A.Slug != a.Slug || payload.B.Slug != b.Slug {
		t.Fatalf("payload = %+v, want A=%s B=%s", payload, a.Slug, b.Slug)
	}
	if payload.Stage != StageEmbedding {
		t.Fatalf("payload.Stage = %v, want StageEmbedding", payload.Stage)
	}
}

// TestEngineResolveAutoMergeAppliesMerge proves the AutoMerge side of the
// same dispatch: a confidently-matching pair (here, an exact alias match,
// so the test needs no embedding fixture) is actually merged, not staged.
func TestEngineResolveAutoMergeAppliesMerge(t *testing.T) {
	r := newTestRig(t)
	ds := openTestDispositionStore(t)
	ctx := context.Background()

	a := domain.Entity{Type: "person", Slug: "acme-corp", Aliases: []string{"Acme"}}
	b := domain.Entity{Type: "person", Slug: "acme-corporation", Aliases: []string{"acme"}} // aliases overlap on "acme"
	r.seedPage(t, store.NewEntityPage(a))
	bPath := r.seedPage(t, store.NewEntityPage(b))

	eng := NewEngine(r.q, r.fw, r.ss, ds, &fakeEmbedder{}) // embedder must never be called
	d, ev, item, err := eng.Resolve(ctx, a, b, mergeFixedNow, "machine")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Verdict != VerdictAutoMerge || d.Stage != StageAlias {
		t.Fatalf("Verdict/Stage = %v/%v, want AutoMerge/alias", d.Verdict, d.Stage)
	}
	if item != nil {
		t.Fatal("Resolve must not stage a disposition item for an AutoMerge verdict")
	}
	if ev == nil {
		t.Fatal("Resolve must return a MergeEvent for an AutoMerge verdict")
	}
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Fatalf("b's page must be gone after an applied AutoMerge (err=%v)", err)
	}

	items, err := ds.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("disposition store has %d items, want 0 for an AutoMerge verdict", len(items))
	}
}

// TestEngineResolveNoMatchDoesNothing proves the third leg: an unrelated
// pair triggers neither a merge nor a disposition item.
func TestEngineResolveNoMatchDoesNothing(t *testing.T) {
	r := newTestRig(t)
	ds := openTestDispositionStore(t)
	ctx := context.Background()

	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "unrelated-widgets"}
	r.seedPage(t, store.NewEntityPage(a))
	r.seedPage(t, store.NewEntityPage(b))

	fe := &fakeEmbedder{vectors: map[string][]float32{
		embeddingText(a): {1, 0},
		embeddingText(b): unitVec2(0.10),
	}}
	eng := NewEngine(r.q, r.fw, r.ss, ds, fe)
	d, ev, item, err := eng.Resolve(ctx, a, b, mergeFixedNow, "machine")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Verdict != VerdictNoMatch {
		t.Fatalf("Verdict = %v, want NoMatch", d.Verdict)
	}
	if ev != nil || item != nil {
		t.Fatal("NoMatch must return neither a MergeEvent nor a disposition item")
	}

	items, err := ds.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("disposition store has %d items, want 0", len(items))
	}
}
