// This file (reconcile.go, T2.2) is the reconcile engine itself:
// new-claim-vs-active-claims conflict detection, the routing verdict,
// and staging A/B disposition items -- the piece decay.go's package doc
// names as landing here separately from the weekly-sweep read-time
// pieces (T2.10-T2.12) it already covers.
package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
)

// Verdict is the reconcile engine's routing decision for a new claim
// against its (subject, predicate) neighbors (RFC 0001 §10.2). Serenity
// adapts gbrain's six-verdict shape (dndungu/gbrain's
// eval-contradictions.Verdict, the six-member enum docs/plan.md's gbrain
// integration notes name) to RFC §10.2's own four named routing outcomes
// (Agree / Neutral additive / Conflict-facts / Conflict-touches-a-precept)
// plus the two first-class special cases the very next paragraph names
// (window-close, scoped) -- six members, values renamed to match
// §10.2's own vocabulary rather than reusing gbrain's TypeScript names
// verbatim, since this engine routes into DISPOSITION item kinds
// gbrain's contradiction probe never needed to.
type Verdict string

const (
	// VerdictAgree: the new claim corroborates an existing active claim
	// (identical normalized object). RFC: "corroborate; weighted
	// confidence bump." Detect is a pure function and writes nothing
	// (see its own doc) -- wiring a real confidence-bump write-through
	// is left to a future task; it is not exercised by this task's acc
	// line, which only tests the Conflict/WindowClose/Scoped outcomes
	// plus the general no-mutation invariant.
	VerdictAgree Verdict = "agree"
	// VerdictNeutralAdditive: no active claim shares the new claim's
	// (subject, predicate) at all -- nothing to agree or conflict with.
	VerdictNeutralAdditive Verdict = "neutral_additive"
	// VerdictConflict: a genuine same-time contradiction -- same
	// (subject, predicate), differing objects, no distinguishing scope
	// or determinable validity-window ordering. Routes to a DISPOSITION
	// reconcile item, A/B with both provenances; never auto-superseded
	// at trust 0 (RFC §10.3: "No automatic supersession at trust level
	// 0").
	VerdictConflict Verdict = "conflict"
	// VerdictWindowClose: the detector recognizes a temporal-
	// supersession shape (both claims' validity windows parse and
	// differ) rather than a flat contradiction -- RFC §10.2: "proposes
	// window-close (temporal supersession) as distinct from refutation."
	// Still routed through DISPOSITION, never auto-applied at trust 0 --
	// "proposes" is the operative word.
	VerdictWindowClose Verdict = "window_close"
	// VerdictScoped: the claims differ only because they carry distinct,
	// explicit scope qualifiers (RFC's own example: "true for project
	// X") -- both true, both stay, no DISPOSITION item.
	VerdictScoped Verdict = "scoped"
	// VerdictPreceptConflict is reserved for RFC §10.2's "Conflict
	// (touches a precept)" routing outcome -- never auto-resolved,
	// routes to a precept-draft/question item rather than a reconcile
	// item. Not detected by this task: recognizing that a claim "touches
	// a precept" needs a precept-matching lookup this task has no
	// dependency on (T2.2's own deps: [T1.9, T1.10, T2.1] -- no
	// internal/dira dependency). Disclosed gap, left to a later task;
	// the value exists purely so Verdict's shape is forward-compatible
	// with RFC §10.2's full four-way routing once that lookup exists.
	// Detect never returns it.
	VerdictPreceptConflict Verdict = "precept_conflict"
)

// Candidates returns every domain.StateActive claim in active sharing
// newClaim's (SubjectSlug, Predicate) -- RFC §10.2's "active claims
// sharing (subject, predicate)" -- excluding newClaim itself by ID.
//
// "plus high-similarity neighbors" (RFC §10.2) -- embedding-similarity
// matches beyond an exact (subject, predicate) match -- is deliberately
// not implemented here: no per-claim embedding index exists yet (T1.10
// built the chunk-level vector store for retrieval, internal/embed, not
// a claim-level index), and full embedding-similarity comparison is
// explicitly named as T2.13's job (entity resolution) elsewhere in this
// plan and in internal/reconcile/decay.go's own AliasCandidates doc
// comment ("not the embedding-similarity-within-type comparison T2.13
// later builds"). Exact (subject, predicate) matching covers every
// scenario this task's acc line exercises.
func Candidates(newClaim domain.Claim, active []domain.Claim) []domain.Claim {
	out := make([]domain.Claim, 0, len(active))
	for _, c := range active {
		if c.ID == newClaim.ID {
			continue
		}
		if c.State != domain.StateActive {
			continue
		}
		if c.SubjectSlug == newClaim.SubjectSlug && c.Predicate == newClaim.Predicate {
			out = append(out, c)
		}
	}
	return out
}

// scopePattern extracts an explicit scope qualifier from a claim's
// object text -- RFC §10.2's own example phrasing ("true for project
// X"): "for project <ident>" or "(project: <ident>)", case-insensitive.
// This is a narrow, disclosed lexical heuristic (the same class of
// simplification decay.go's AliasCandidates uses for alias detection),
// not a general scope-extraction system: no field in domain.Claim or
// the persisted fence/shard row format (RFC §7.2's claims-table columns
// are id/predicate/object/conf/valid/src/state -- no scope column)
// carries a structured scope value today, so scope lives inside Object
// text exactly as RFC's own example writes it, and this reads it back
// out the same way. A structured Scope field is a plausible future
// migration (RFC §7.5: "changing a pin is a migration"); not this
// task's job, and not required by its acc line.
var scopePattern = regexp.MustCompile(`(?i)\bfor project ([a-z0-9][a-z0-9_-]*)|\(project:\s*([a-z0-9][a-z0-9_-]*)\)`)

func scopeOf(c domain.Claim) string {
	m := scopePattern.FindStringSubmatch(c.Object)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return strings.ToLower(m[1])
	}
	return strings.ToLower(m[2])
}

// validDateLayout is the canonical ValidFrom/ValidTo string format this
// engine parses (calendar-day granularity, matching RFC §7.2's table
// example dates like "2025-06-.."). internal/ingest's ClaimFromObservation
// leaves ValidFrom empty today ("observations carry no validity window
// yet ... temporal claims are E2 work") -- this task is that E2 work:
// the first reader to give ValidFrom/ValidTo real temporal meaning.
const validDateLayout = "2006-01-02"

func parseValidFrom(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(validDateLayout, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// Detection is Detect's outcome: the routing verdict plus, for every
// verdict except NeutralAdditive, the specific candidate claim it was
// decided against (Candidate's provenance is the B side; the caller's
// newClaim is the A side).
type Detection struct {
	Verdict   Verdict
	Candidate domain.Claim
	Reason    string
}

// detectPair classifies newClaim against exactly one candidate already
// known to share (subject, predicate) -- see Detect for the ordering
// this implements and why.
func detectPair(newClaim, candidate domain.Claim) Detection {
	if newClaim.ObjectKey != "" && newClaim.ObjectKey == candidate.ObjectKey {
		return Detection{
			Verdict:   VerdictAgree,
			Candidate: candidate,
			Reason:    "identical normalized object -- corroboration",
		}
	}

	newScope, candScope := scopeOf(newClaim), scopeOf(candidate)
	if newScope != "" && candScope != "" && newScope != candScope {
		return Detection{
			Verdict:   VerdictScoped,
			Candidate: candidate,
			Reason:    fmt.Sprintf("both true under different scope qualifiers (%q vs %q)", newScope, candScope),
		}
	}

	newFrom, newOK := parseValidFrom(newClaim.ValidFrom)
	candFrom, candOK := parseValidFrom(candidate.ValidFrom)
	if newOK && candOK && !newFrom.Equal(candFrom) {
		return Detection{
			Verdict:   VerdictWindowClose,
			Candidate: candidate,
			Reason: fmt.Sprintf(
				"temporally distinct validity windows (new claim valid_from=%s, existing claim valid_from=%s) -- propose closing the earlier claim's window, not refutation",
				newClaim.ValidFrom, candidate.ValidFrom,
			),
		}
	}

	return Detection{
		Verdict:   VerdictConflict,
		Candidate: candidate,
		Reason:    "same (subject, predicate), differing objects, no distinguishing scope qualifier or determinable validity-window ordering -- genuine contradiction",
	}
}

// verdictRank orders Verdict values for Detect's candidate-priority
// resolution (see Detect): a real contradiction against any active
// neighbor must never be masked by a different neighbor's agreement.
func verdictRank(v Verdict) int {
	switch v {
	case VerdictConflict:
		return 4
	case VerdictWindowClose:
		return 3
	case VerdictScoped:
		return 2
	case VerdictAgree:
		return 1
	default: // VerdictNeutralAdditive, VerdictPreceptConflict (unused here)
		return 0
	}
}

// Detect evaluates newClaim against every active claim in candidates
// (RFC §10.2, T2.2's core job) and returns exactly one overall
// Detection -- RFC's "new claim vs. active claims sharing (subject,
// predicate)" is evaluated as one decision per new claim, not one per
// candidate pair, so a claim with several active neighbors never
// produces several contradictory verdicts. When candidates disagree
// (e.g. one candidate would agree, another would conflict), the
// highest-priority verdict wins per verdictRank: Conflict > WindowClose
// > Scoped > Agree > NeutralAdditive.
//
// Pure: reads only, mutates nothing, performs no I/O -- provable by a
// before/after dump diff of every domain.Claim passed in (reconcile_test.go).
// candidates with zero elements (nothing shares the subject/predicate)
// returns VerdictNeutralAdditive with a zero-value Candidate.
func Detect(newClaim domain.Claim, candidates []domain.Claim) Detection {
	if len(candidates) == 0 {
		return Detection{Verdict: VerdictNeutralAdditive}
	}
	best := Detection{Verdict: VerdictNeutralAdditive}
	bestRank := -1
	for _, c := range candidates {
		d := detectPair(newClaim, c)
		if r := verdictRank(d.Verdict); r > bestRank {
			best, bestRank = d, r
		}
	}
	return best
}

// ReconcilePayload is the JSON payload of a disposition.KindReconcile
// item this package creates -- both provenances, per T2.2's acc line
// ("exactly one A/B item with both provenances"): A is the new claim
// under evaluation, B is the existing active claim Detect matched it
// against; each domain.Claim already carries its own Provenance field,
// so both sides' provenance is present without any extra plumbing.
type ReconcilePayload struct {
	Verdict Verdict      `json:"verdict"`
	Reason  string       `json:"reason"`
	A       domain.Claim `json:"a"`
	B       domain.Claim `json:"b"`
}

// Engine wires Detect's pure decision to real DISPOSITION staging
// (T2.1's Store) for the two verdicts that need human review (Conflict,
// WindowClose) -- RFC §10.2: conflicts get "No automatic supersession at
// trust level 0", and window-close is explicitly "proposed", never
// auto-applied, so both route through the same staging path. Agree /
// NeutralAdditive / Scoped never stage anything (RFC: "both stay" /
// "corroborate" -- nothing for a human to review); VerdictPreceptConflict
// is never returned by Detect (see its own doc), so Engine never needs
// to route to a precept-draft item.
type Engine struct {
	Store *disposition.Store
}

// NewEngine builds an Engine over an already-open disposition store
// (T2.1). store must be non-nil.
func NewEngine(store *disposition.Store) *Engine {
	return &Engine{Store: store}
}

// Process runs Detect(newClaim, Candidates(newClaim, active)) and, for
// VerdictConflict/VerdictWindowClose, stages exactly one
// disposition.KindReconcile item via Store.Create -- satisfying "exactly
// one A/B item" even when several candidates share the (subject,
// predicate): Detect already resolves multiple candidates down to one
// overall Detection (see its own doc), so Process stages at most one
// item per call, never one per candidate.
//
// Process never mutates newClaim, active, or any claim's State --
// Store.Create only writes to the disposition_items/disposition_history
// runtime tables (never canonical claim state, RFC §7.5's "DB-only by
// design" runtime allowlist), so calling it for Conflict/WindowClose is
// DISPOSITION routing, not a claim state change; the returned
// *disposition.Item is nil for every other verdict.
func (e *Engine) Process(ctx context.Context, newClaim domain.Claim, active []domain.Claim, now time.Time) (Detection, *disposition.Item, error) {
	detection := Detect(newClaim, Candidates(newClaim, active))

	if detection.Verdict != VerdictConflict && detection.Verdict != VerdictWindowClose {
		return detection, nil, nil
	}

	payload := ReconcilePayload{
		Verdict: detection.Verdict,
		Reason:  detection.Reason,
		A:       newClaim,
		B:       detection.Candidate,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return detection, nil, fmt.Errorf("reconcile: marshal payload: %w", err)
	}
	item, err := e.Store.Create(ctx, disposition.KindReconcile, raw, "", now)
	if err != nil {
		return detection, nil, fmt.Errorf("reconcile: stage disposition item: %w", err)
	}
	return detection, &item, nil
}
