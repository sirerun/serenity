package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/hosted/pool"
)

// firstBrainID returns the directory name of one brain in a prepared fixture copy.
func firstBrainID(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "data", "brains"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no brains in the fixture: %v", err)
	}
	return entries[0].Name()
}

// These two tests exercise the read side of the hosted service in process, with no listener and no provider: the real
// pool open path, its embedding-pin check and the real recall tool over a prepared brain. They are infrastructure
// mechanics only. The hash embedder carries no semantic quality, so nothing here says anything about recall quality.

// The pooled runtime opens a prepared brain under the fixture pin, and the production recall tool returns exactly the
// canonical facts the plan regenerates for the fixture, from the derived state the verifier also counts.
func TestProductionRecallToolReadsThePreparedBrainUnderTheFixturePin(t *testing.T) {
	dir := mutable(t)
	id := firstBrainID(t, dir)
	p, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: filepath.Join(dir, "data", "brains"), Embedder: hashEmbedder{dim: 16}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	rt, release, err := p.Acquire(context.Background(), id)
	if err != nil {
		t.Fatalf("the pooled runtime must open a prepared brain under the fixture pin: %v", err)
	}
	defer release()

	// Every fact the plan regenerates for any brain in the fixture (2 per brain in the test fixture).
	wl, err := LoadWorkload(frozenWorkload)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(wl, ProfileSmoke, smokeFactsInTests)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	for _, a := range plan.Accounts {
		for _, b := range a.Brains {
			for i := 0; i < b.Facts; i++ {
				expected[factFor(a.Label, b.Index, i, wl.FactTokens.Min, wl.FactTokens.Max).Payload.Fact] = true
			}
		}
	}

	var recall func(context.Context, json.RawMessage) (string, error)
	for _, tool := range rt.Tools {
		if tool.Name != "recall" {
			continue
		}
		recall = func(ctx context.Context, args json.RawMessage) (string, error) {
			res, e := tool.Handler(ctx, args)
			if e != nil {
				return "", e
			}
			if res.IsError {
				return "", errors.New("recall returned a tool error")
			}
			data, e := json.Marshal(res)
			return string(data), e
		}
	}
	if recall == nil {
		t.Fatal("the pooled runtime has no recall tool")
	}
	raw, err := recall(context.Background(), json.RawMessage(`{"limit":50}`))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err = json.Unmarshal([]byte(raw), &envelope); err != nil || len(envelope.Content) != 1 {
		t.Fatalf("unexpected recall envelope %q: %v", raw, err)
	}
	var body struct {
		Total int `json:"total"`
		Facts []struct {
			FactID     string `json:"fact_id"`
			Fact       string `json:"fact"`
			Provenance string `json:"provenance"`
		} `json:"facts"`
	}
	if err = json.Unmarshal([]byte(envelope.Content[0].Text), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != smokeFactsInTests || len(body.Facts) != smokeFactsInTests {
		t.Fatalf("recall returned total %d and %d facts, want %d", body.Total, len(body.Facts), smokeFactsInTests)
	}
	seen := map[string]bool{}
	for _, f := range body.Facts {
		if !expected[f.Fact] {
			t.Errorf("recall returned a fact the plan does not regenerate: %.60q", f.Fact)
		}
		if seen[f.FactID] {
			t.Errorf("recall returned fact %s twice", f.FactID)
		}
		seen[f.FactID] = true
		if f.Provenance != Provenance {
			t.Errorf("provenance %q, want %q", f.Provenance, Provenance)
		}
	}
}

// A service whose embedder carries another pin cannot open a prepared brain: the pool refuses it with the pin
// mismatch error and never rewrites the brain's pin. This is the fact the service-seams request depends on.
func TestPooledRuntimeRefusesAnotherEmbeddingPin(t *testing.T) {
	dir := mutable(t)
	id := firstBrainID(t, dir)
	before, err := os.ReadFile(filepath.Join(dir, "data", "brains", id, "serenity.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []hashEmbedder{{dim: 8}, {dim: 32}} {
		p, e := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: filepath.Join(dir, "data", "brains"), Embedder: other})
		if e != nil {
			t.Fatal(e)
		}
		_, release, e := p.Acquire(context.Background(), id)
		if release != nil {
			release()
		}
		if !errors.Is(e, embed.ErrPinMismatch) {
			t.Errorf("a %s service opened a brain pinned to another embedder: %v", other.ModelVersion(), e)
		}
		if e = p.Close(); e != nil {
			t.Error(e)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "data", "brains", id, "serenity.yml"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("a refused open must not rewrite the brain's config: %v", err)
	}
}
