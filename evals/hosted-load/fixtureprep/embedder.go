package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

// EmbedderVersion is the version half of the fixture pin. The pin is "<model>@<version>", the same shape a hosted
// service builds from its EmbeddingModel and EmbeddingVersion settings.
const EmbedderVersion = "infrastructure-only-v1"

// DefaultDim is the default vector width. A wider vector only grows the derived index.
const DefaultDim = 64

// hashEmbedder is INFRASTRUCTURE-ONLY. It hashes each whitespace-separated word into one of `dim` signed buckets
// and normalizes the result. It makes no provider call, has no cost and needs no key, and it carries no semantic
// quality: two texts sharing words share buckets, nothing more. It exists so the real index recovery code
// (index.RecoverMemorySearch) writes a real vector row for every fact. Never read a recall result from a brain built
// with it as evidence of retrieval quality.
type hashEmbedder struct{ dim int }

func (h hashEmbedder) ModelVersion() string {
	return fmt.Sprintf("fixture-hash-embedder-d%d@%s", h.dim, EmbedderVersion)
}

func (h hashEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vec := make([]float32, h.dim)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return nil, errors.New("fixtureprep: embed empty text")
	}
	for _, w := range words {
		sum := sha256.Sum256([]byte(w))
		slot := int(binary.LittleEndian.Uint32(sum[0:4]) % uint32(h.dim))
		if sum[4]&1 == 0 {
			vec[slot]++
		} else {
			vec[slot]--
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		// Every bucket cancelled. A zero vector cannot be cosine-compared, so pin one bucket instead of returning it.
		vec[0] = 1
		return vec, nil
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec, nil
}
