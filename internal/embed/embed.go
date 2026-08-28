// Package embed generates per-chunk vector embeddings under an explicit
// model pin and answers pin-scoped nearest-neighbor queries over
// internal/index's vector store (RFC 0001 §7.5, §10.1).
//
// The governing invariant is "never mix pins": a vector produced under one
// (provider, model, version) is never compared against, or substituted
// for, a vector produced under a different one. Storage-side enforcement
// (the composite chunk_ref+model key, the pin-scoped cosine scan) lives in
// internal/index; this package adds the generation side (Embedder) and the
// orchestration that lets a caller search one pin while a chunk missing
// that pin's vector still surfaces via FTS instead of being silently
// dropped (Search, in search.go).
package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sirerun/serenity/internal/router"
)

// Embedder produces a vector for one chunk's text under exactly one model
// pin (ModelVersion). Real callers use RouterEmbedder; fakes live in
// _test.go files only, per the zero-stub policy.
type Embedder interface {
	// Embed returns text's embedding vector. Every call must use the
	// same pin as ModelVersion() -- RouterEmbedder verifies this itself
	// rather than trusting the caller.
	Embed(ctx context.Context, text string) ([]float32, error)
	// ModelVersion is the pin this Embedder produces vectors under,
	// shaped "<model>@<version>" (router.Provider.ModelVersion()).
	ModelVersion() string
}

// ErrPinMismatch means the provider actually serving TaskClassEmbedding
// reported a different model@version than the pin RouterEmbedder was
// configured with. This is the same "never mix pins" invariant applied to
// provider configuration rather than storage: a config drift (someone
// repoints the local-cheap tier at a different embedding model without
// updating serenity.yml's pin) must fail loudly, never silently write a
// vector under the wrong key.
var ErrPinMismatch = errors.New("embed: provider's model@version does not match the configured pin")

// RouterEmbedder is the production Embedder: it issues one
// TaskClassEmbedding call per Embed through the shared router chokepoint
// (RFC §9), so embedding calls are budgeted, spend-ledgered, and
// index_only-refused exactly like every other model call.
//
// router.Provider.Send is text-in/text-out (ADR 003's provider-adapter
// boundary is deliberately generic across chat-completion-shaped and
// embedding-shaped APIs). The interop convention this package relies on:
// an embedding-tier provider's Response.Text is a JSON array of numbers,
// e.g. "[0.0123,-0.0456,...]". This keeps router itself embedding-agnostic
// while giving this package a single, testable decode step (decodeVector).
type RouterEmbedder struct {
	Router *router.Router
	Budget router.Budget
	// Pin is the expected "<model>@<version>" for embedding-tier calls,
	// normally read from serenity.yml's pinned model set (RFC §7.5).
	Pin string
}

var _ Embedder = (*RouterEmbedder)(nil)

func (e *RouterEmbedder) ModelVersion() string { return e.Pin }

func (e *RouterEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := e.Router.Complete(ctx, router.TaskClassEmbedding, router.Prompt{Text: text}, e.Budget)
	if err != nil {
		return nil, fmt.Errorf("embed: router complete: %w", err)
	}
	if result.ModelVersion != e.Pin {
		return nil, fmt.Errorf("%w: provider reports %q, configured pin is %q",
			ErrPinMismatch, result.ModelVersion, e.Pin)
	}
	vec, err := decodeVector(result.Text)
	if err != nil {
		return nil, fmt.Errorf("embed: decode vector from provider response: %w", err)
	}
	return vec, nil
}

// decodeVector parses the JSON-array convention described on RouterEmbedder.
func decodeVector(text string) ([]float32, error) {
	var vec []float32
	if err := json.Unmarshal([]byte(text), &vec); err != nil {
		return nil, err
	}
	if len(vec) == 0 {
		return nil, errors.New("embed: decoded vector is empty")
	}
	return vec, nil
}
