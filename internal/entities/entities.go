// Package entities implements RFC 0001 §10.5's entity-resolution pipeline
// (T2.13): "Staged: exact/alias match -> embedding similarity within type
// (auto-merge with undoable merge event + audit trail) -> ambiguous cases
// as low-priority disposition items. Splits supported from the client."
//
// This file (entities.go) covers stage classification -- Evaluate decides,
// for a pair of same-type entities, whether they are an exact/alias match,
// an embedding-similarity match, ambiguous, or unrelated. It performs no
// writes. merge.go applies an AutoMerge verdict (Merge, with an undoable
// MergeEvent); split.go partitions one entity's claims into two pages;
// engine.go ties classification to the write paths and to T2.1's
// disposition queue for the Ambiguous verdict, mirroring
// internal/reconcile's Detect/Engine.Process split between pure
// classification and disposition staging.
package entities

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/embed"
)

// ErrTypeMismatch is returned whenever two entities passed to Evaluate,
// Merge, or Split do not share domain.Entity.Type -- RFC §10.5's "embedding
// similarity within type" applies to every stage of the pipeline, not only
// the embedding one: a person and a company are never resolution
// candidates for each other, however close their names look.
var ErrTypeMismatch = errors.New("entities: entities must share the same type")

// Verdict is Evaluate's routing decision for one candidate pair.
type Verdict string

const (
	// VerdictNoMatch: neither an alias match nor embedding-similar enough
	// to be worth a human's attention. Nothing happens.
	VerdictNoMatch Verdict = "no_match"
	// VerdictAutoMerge: an exact/alias match, or embedding similarity at or
	// above the auto-merge threshold -- RFC §10.5's "auto-merge with
	// undoable merge event + audit trail". Engine.Resolve applies Merge for
	// this verdict; Evaluate itself never merges anything.
	VerdictAutoMerge Verdict = "auto_merge"
	// VerdictAmbiguous: embedding similarity between the ambiguous and
	// auto-merge thresholds -- RFC §10.5's "ambiguous cases as low-priority
	// disposition items". Engine.Resolve stages a disposition item for this
	// verdict; nothing is merged.
	VerdictAmbiguous Verdict = "ambiguous"
)

// Stage names which pipeline stage produced the verdict -- useful on a
// disposition item's payload and in logs, since "why did this pair match"
// is exactly what a human reviewing an ambiguous item needs.
type Stage string

const (
	// StageAlias: matched at the exact/alias stage -- no embedding call was
	// made, so Score is always 1.0.
	StageAlias Stage = "alias"
	// StageEmbedding: matched (or not) at the embedding-similarity stage.
	StageEmbedding Stage = "embedding"
)

// Decision is Evaluate's result: the verdict, which stage produced it, the
// similarity score behind it (1.0 for an alias match), and a human-readable
// reason.
type Decision struct {
	Verdict Verdict
	Stage   Stage
	Score   float64
	Reason  string
}

// Thresholds configures the embedding-similarity stage's two cut points.
// AutoMerge must be >= Ambiguous; Evaluate does not enforce this itself
// (a caller supplying nonsensical thresholds gets nonsensical routing, not
// a defensive override) but DefaultThresholds satisfies it.
type Thresholds struct {
	// AutoMerge is the minimum cosine similarity for VerdictAutoMerge.
	AutoMerge float64
	// Ambiguous is the minimum cosine similarity for VerdictAmbiguous
	// (below AutoMerge). A score below this is VerdictNoMatch.
	Ambiguous float64
}

// DefaultThresholds mirrors the conservative-by-default posture RFC
// §10.3's earned-automation ladder takes elsewhere in this plan (T2.11's
// calibration sweep starts from a prior rather than an arbitrary guess):
// 0.92 is a high bar for a fully automatic, undoable identity merge; 0.75
// leaves real headroom below it for "worth a human's ten seconds" without
// flooding the queue with every loosely-related name. No calibration sweep
// backs these two numbers yet (T2.11's sweep covered the ladder's
// min_dispositions/min_accept/sample_rate, not entity-resolution
// thresholds) -- they are Engine's field defaults, not RFC-mandated
// constants, and a caller can override Engine.Thresholds once real
// disposition history exists to calibrate against.
func DefaultThresholds() Thresholds {
	return Thresholds{AutoMerge: 0.92, Ambiguous: 0.75}
}

// normalizeName lowercases and trims for alias comparison -- the same
// normalization class NormalizeKey (internal/store) applies to claim
// objects, applied here to entity slugs/aliases instead.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// names returns e's own comparable identity strings: its slug plus every
// alias, normalized, deduplicated.
func names(e domain.Entity) map[string]bool {
	out := map[string]bool{normalizeName(e.Slug): true}
	for _, a := range e.Aliases {
		if n := normalizeName(a); n != "" {
			out[n] = true
		}
	}
	return out
}

// isAliasMatch reports whether a and b's identity strings (slug + aliases)
// overlap at all -- RFC §10.5's exact/alias stage. Same-type is the
// caller's job (Evaluate checks it before calling this).
func isAliasMatch(a, b domain.Entity) bool {
	an := names(a)
	for n := range names(b) {
		if an[n] {
			return true
		}
	}
	return false
}

// embeddingText builds the string embedded for the similarity stage: the
// slug read as words plus every alias, so two entities named close to
// identically embed close to identically even before either has a summary
// (RFC §10.5 runs at ingest time, when a freshly created entity page may
// carry no summary yet -- consolidate, T2.14, is what regenerates
// summaries, and is not a dependency of this task).
func embeddingText(e domain.Entity) string {
	parts := make([]string, 0, len(e.Aliases)+1)
	parts = append(parts, strings.ReplaceAll(e.Slug, "-", " "))
	parts = append(parts, e.Aliases...)
	return strings.Join(parts, " | ")
}

// cosine computes cosine similarity in [-1, 1]. Duplicated from
// internal/index's own private cosine (same standard formula) rather than
// imported: that one is wired to SQLite-stored chunk vectors keyed by
// (chunk_ref, model) and unexported; this package compares two freshly
// computed entity-embedding vectors that never touch the derived index, so
// depending on internal/index for ten lines of arithmetic would pull in an
// unrelated storage layer for no real reuse.
func cosine(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("entities: cosine: vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// Evaluate classifies one candidate pair per RFC §10.5's staged pipeline.
// a and b must share Type (ErrTypeMismatch otherwise) and be distinct
// slugs. The alias stage is checked first and never calls embedder --
// RFC's own ordering ("exact/alias match -> embedding similarity") is also
// the cheap-first ordering. embedder is only consulted when the alias
// stage does not already decide AutoMerge.
func Evaluate(ctx context.Context, embedder embed.Embedder, a, b domain.Entity, th Thresholds) (Decision, error) {
	if a.Type != b.Type {
		return Decision{}, fmt.Errorf("entities: evaluate: %w: a.Type=%q b.Type=%q", ErrTypeMismatch, a.Type, b.Type)
	}
	if normalizeName(a.Slug) == normalizeName(b.Slug) {
		return Decision{}, fmt.Errorf("entities: evaluate: a and b must be distinct entities (both %q)", a.Slug)
	}

	if isAliasMatch(a, b) {
		return Decision{
			Verdict: VerdictAutoMerge,
			Stage:   StageAlias,
			Score:   1.0,
			Reason:  fmt.Sprintf("%q and %q share a slug/alias", a.Slug, b.Slug),
		}, nil
	}

	va, err := embedder.Embed(ctx, embeddingText(a))
	if err != nil {
		return Decision{}, fmt.Errorf("entities: evaluate: embed %s: %w", a.Slug, err)
	}
	vb, err := embedder.Embed(ctx, embeddingText(b))
	if err != nil {
		return Decision{}, fmt.Errorf("entities: evaluate: embed %s: %w", b.Slug, err)
	}
	score, err := cosine(va, vb)
	if err != nil {
		return Decision{}, fmt.Errorf("entities: evaluate: %s vs %s: %w", a.Slug, b.Slug, err)
	}

	switch {
	case score >= th.AutoMerge:
		return Decision{
			Verdict: VerdictAutoMerge, Stage: StageEmbedding, Score: score,
			Reason: fmt.Sprintf("embedding similarity %.4f >= auto-merge threshold %.4f", score, th.AutoMerge),
		}, nil
	case score >= th.Ambiguous:
		return Decision{
			Verdict: VerdictAmbiguous, Stage: StageEmbedding, Score: score,
			Reason: fmt.Sprintf("embedding similarity %.4f is between ambiguous threshold %.4f and auto-merge threshold %.4f", score, th.Ambiguous, th.AutoMerge),
		}, nil
	default:
		return Decision{
			Verdict: VerdictNoMatch, Stage: StageEmbedding, Score: score,
			Reason: fmt.Sprintf("embedding similarity %.4f is below ambiguous threshold %.4f", score, th.Ambiguous),
		}, nil
	}
}
