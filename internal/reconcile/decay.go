// Package reconcile implements Serenity's conflict-detection and
// weekly-sweep machinery (RFC 0001 §10.2). This file (decay.go, T2.10 --
// T2.12 in docs/plan.md) covers the sweep's read-time pieces: confidence
// decay for ranking, alias-merge candidate detection, and flagging
// low-confidence claims for the distill queue. The reconcile engine
// itself (new-claim-vs-active-claims conflict detection, six-verdict
// temporal enum, A/B disposition items) is T2.2, a separate task landing
// in this same package.
package reconcile

import (
	"math"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// DecayedConfidence returns confidence decayed by elapsed time since
// observedAt, using an exponential half-life of halfLifeDays (RFC §10.2:
// "confidence decay: half-life per predicate family ... decay affects
// ranking and staleness banners; it never by itself authorizes
// supersession"). halfLifeDays <= 0 disables decay entirely (confidence
// is returned unchanged) — a predicate family with no half-life
// configured (absent from serenity.yml's families map) does not silently
// decay to zero. A non-positive elapsed age (a claim observed in the
// future, or exactly now) also returns confidence unchanged.
func DecayedConfidence(confidence float64, observedAt, now time.Time, halfLifeDays int) float64 {
	if halfLifeDays <= 0 {
		return confidence
	}
	ageDays := now.Sub(observedAt).Hours() / 24
	if ageDays <= 0 {
		return confidence
	}
	return confidence * math.Pow(0.5, ageDays/float64(halfLifeDays))
}

// Ranked pairs a claim with its read-time decayed confidence. The Claim
// field is a copy — Rank and every function in this file only ever reads
// domain.Claim values; nothing here mutates a caller's slice or writes
// anything to disk (RFC's own "decay never writes a confidence value back
// to a file" invariant, T2.12's acc line).
type Ranked struct {
	Claim             domain.Claim
	DecayedConfidence float64
}

// Rank orders claims by decayed confidence descending — ties broken by
// claim ID for determinism — without mutating any claim's stored
// Confidence or State. halfLives maps predicate family name to its
// configured half-life in days (internal/config.Family.HalfLifeDays,
// adapted by the caller); a family absent from halfLives decays nothing
// (see DecayedConfidence).
func Rank(claims []domain.Claim, halfLives map[string]int, now time.Time) []Ranked {
	ranked := make([]Ranked, len(claims))
	for i, c := range claims {
		ranked[i] = Ranked{
			Claim:             c,
			DecayedConfidence: DecayedConfidence(c.Confidence, c.Provenance.ObservedAt, now, halfLives[c.Family]),
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].DecayedConfidence != ranked[j].DecayedConfidence {
			return ranked[i].DecayedConfidence > ranked[j].DecayedConfidence
		}
		return ranked[i].Claim.ID < ranked[j].Claim.ID
	})
	return ranked
}

// DefaultDistillThreshold mirrors the extraction pipeline's own
// low-confidence bar (RFC §10.1: "below threshold 0.6 the item goes to
// the distill queue instead") — reused here so an active claim whose
// confidence has decayed below the same bar the pipeline already applies
// to fresh observations is flagged the same way, not a separately
// invented number.
const DefaultDistillThreshold = 0.6

// DistillCandidate names one active claim whose decayed confidence has
// fallen below the sweep's distill threshold. Nothing here writes a
// disposition item or mutates the claim — internal/disposition's queue
// (T2.1, a concurrent sibling task, not a dependency of this one) is
// where a real distill item eventually lands; a weekly-sweep caller feeds
// this list into that queue once it exists.
type DistillCandidate struct {
	Claim             domain.Claim
	DecayedConfidence float64
}

// DistillCandidates reports every domain.StateActive claim in claims
// whose decayed confidence (per halfLives, evaluated at now) falls below
// threshold. threshold <= 0 selects DefaultDistillThreshold. A superseded
// or retracted claim is never a distill candidate — there is nothing left
// to distill once a claim is no longer the active belief.
func DistillCandidates(claims []domain.Claim, halfLives map[string]int, now time.Time, threshold float64) []DistillCandidate {
	if threshold <= 0 {
		threshold = DefaultDistillThreshold
	}
	var out []DistillCandidate
	for _, c := range claims {
		if c.State != domain.StateActive {
			continue
		}
		dc := DecayedConfidence(c.Confidence, c.Provenance.ObservedAt, now, halfLives[c.Family])
		if dc < threshold {
			out = append(out, DistillCandidate{Claim: c, DecayedConfidence: dc})
		}
	}
	return out
}

// AliasCandidate is a pair of distinct subject slugs the weekly sweep
// flags as plausibly the same entity spelled two different ways — a
// candidate for a human (or T2.13's embedding-similarity merge, once that
// lands: deps [T2.1, T1.10, T0.3], not this task) to confirm. Nothing
// here merges anything.
type AliasCandidate struct {
	A, B string
}

// AliasCandidates scans claims' subject slugs for near-identical
// spellings (RFC §10.2's "alias-merge candidates"): a lightweight lexical
// signal — Levenshtein distance 1 on the hyphen-joined slug, or one
// slug's hyphen-separated token set fully contained in the other's — not
// the embedding-similarity-within-type comparison T2.13 later builds.
// Output is sorted and deterministic; a slug never pairs with itself.
func AliasCandidates(claims []domain.Claim) []AliasCandidate {
	slugSet := make(map[string]bool)
	for _, c := range claims {
		if c.SubjectSlug != "" {
			slugSet[c.SubjectSlug] = true
		}
	}
	slugs := make([]string, 0, len(slugSet))
	for s := range slugSet {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	var out []AliasCandidate
	for i := 0; i < len(slugs); i++ {
		for j := i + 1; j < len(slugs); j++ {
			if isAliasPair(slugs[i], slugs[j]) {
				out = append(out, AliasCandidate{A: slugs[i], B: slugs[j]})
			}
		}
	}
	return out
}

func isAliasPair(a, b string) bool {
	if a == b {
		return false
	}
	if levenshtein(a, b) <= 1 {
		return true
	}
	return tokenSubset(a, b) || tokenSubset(b, a)
}

// tokenSubset reports whether every hyphen-separated token of a appears
// among b's tokens (e.g. "acme" is a token-subset of "acme-corp").
func tokenSubset(a, b string) bool {
	bTokens := make(map[string]bool)
	for _, t := range splitTokens(b) {
		bTokens[t] = true
	}
	aTokens := splitTokens(a)
	if len(aTokens) == 0 || len(aTokens) >= len(bTokens) {
		return false
	}
	for _, t := range aTokens {
		if !bTokens[t] {
			return false
		}
	}
	return true
}

func splitTokens(s string) []string {
	var tokens []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '-' {
			if i > start {
				tokens = append(tokens, s[start:i])
			}
			start = i + 1
		}
	}
	return tokens
}

// levenshtein computes the classic edit distance between a and b (byte-
// wise — subject slugs are ASCII by construction, RFC §7.2). Used only
// for the small candidate-pair check above, not a general-purpose Unicode
// text-distance utility.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// SweepResult bundles the weekly sweep's three read-time outputs (RFC
// §10.2 / §10.3, UC-044). Nothing in Sweep writes to disk or mutates its
// input claims.
type SweepResult struct {
	Ranked            []Ranked
	AliasCandidates   []AliasCandidate
	DistillCandidates []DistillCandidate
}

// Sweep runs the weekly sweep's read-time pieces over claims: decay-based
// ranking, alias-merge candidate detection, and low-confidence-to-distill
// flagging. now is an injected clock (T2.19's `serenity cron sweep` calls
// this with the real clock; tests call it with a fixed one).
// distillThreshold <= 0 selects DefaultDistillThreshold.
func Sweep(claims []domain.Claim, halfLives map[string]int, now time.Time, distillThreshold float64) SweepResult {
	return SweepResult{
		Ranked:            Rank(claims, halfLives, now),
		AliasCandidates:   AliasCandidates(claims),
		DistillCandidates: DistillCandidates(claims, halfLives, now, distillThreshold),
	}
}
