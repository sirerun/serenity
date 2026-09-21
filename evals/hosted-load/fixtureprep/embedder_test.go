package main

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestHashEmbedderIsDeterministicUnitLengthAndLabeled(t *testing.T) {
	e := hashEmbedder{dim: 64}
	if got := e.ModelVersion(); got != "fixture-hash-embedder-d64@infrastructure-only-v1" || !strings.Contains(got, "infrastructure-only") {
		t.Fatalf("pin %q must carry the infrastructure-only label in the <model>@<version> shape", got)
	}
	a, err := e.Embed(context.Background(), "backup index queue backup")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := e.Embed(context.Background(), "backup index queue backup")
	if len(a) != 64 {
		t.Fatalf("dimension %d, want 64", len(a))
	}
	var norm float64
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("the same text must embed identically")
		}
		norm += float64(a[i]) * float64(a[i])
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Fatalf("squared norm %v, want 1", norm)
	}
}

func TestHashEmbedderRefusesEmptyAndCancelled(t *testing.T) {
	e := hashEmbedder{dim: 16}
	if _, err := e.Embed(context.Background(), "   "); err == nil {
		t.Error("empty text must be an error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Embed(ctx, "memory"); err == nil {
		t.Error("a cancelled context must be an error")
	}
}
