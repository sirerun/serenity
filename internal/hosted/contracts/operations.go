package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Operation identity/accounting (interfaces.md "Operation identity/
// accounting", owner task44, design review owner task41).
//
// STATUS: design approved by chief-architect on PR236 revision
// 218d7234d9abea5964f9d4640d1bfffe5c9f8087; task41 reconciled the approved
// narrow commit-section scope in interfaces.md. Migration version 4 is owned
// by task41. Production ledger behavior is task44's implementation scope and
// remains subject to ordinary review. The executable specification for every
// rule below is contractstest.RunLedgerSuite.
//
// One OperationRecord is one durable row representing exactly one logical
// mutation, however many counters it moves (a "remember" moves both "writes"
// and "input_tokens"). Reserve and Finalize are called exactly once per
// logical mutation, never once per metric, so every delta commits or none
// does in one SQL transaction.
//
// Quiescence rule (added after coordinator review). A lease that has expired
// proves only that time passed, not that the writer stopped: a writer paused
// inside the canonical write can resume after a checker has observed the
// operation absent and released its reservation, leaving a landed mutation
// with no charge. Therefore an absence-based release is reachable ONLY
// through OperationLedger.ReconcilePending, which must hold the brain's
// exclusive BrainFence across both the canonical absence inspection and the
// ledger transition, and a writer must call OperationLedger.EnterCanonical,
// inside a BrainFence commit section, before its first canonical byte. A
// writer that finds its row no longer reserved is stopped before it writes.
// Lease expiry stays as a liveness filter (do not fence healthy in-flight
// work) and is never sufficient on its own. See commitfence.go.

// OperationPhase is the operation state machine:
//
//	reserved ──► committed            (final)
//	reserved ──► released             (final)
//	reserved ──► pending_review       (holding; NOT final)
//	pending_review ──► committed      (final; operator only)
//	pending_review ──► released       (final; operator only)
//
// Committed and Released are final: no further transition exists. Pending
// review is a holding phase, not a terminal one: it keeps every delta held
// against the quota window (never freed, never charged) until a human
// operator resolves it. ValidateTransition is the single source of truth for
// which (from, to, actor, evidence) combinations are legal.
type OperationPhase string

const (
	OperationReserved      OperationPhase = "reserved"
	OperationCommitted     OperationPhase = "committed"
	OperationReleased      OperationPhase = "released"
	OperationPendingReview OperationPhase = "pending_review"
)

// Valid reports whether p is one of the four defined phases.
func (p OperationPhase) Valid() bool {
	switch p {
	case OperationReserved, OperationCommitted, OperationReleased, OperationPendingReview:
		return true
	}
	return false
}

// Final reports whether p admits no further transition.
func (p OperationPhase) Final() bool { return p == OperationCommitted || p == OperationReleased }

// HoldsCapacity reports whether a row in phase p keeps its deltas held
// against the quota window: reserved and pending_review rows do, so an
// unresolved operation can never let a customer exceed an allowance;
// committed rows are counted as used; released rows count as nothing.
func (p OperationPhase) HoldsCapacity() bool {
	return p == OperationReserved || p == OperationPendingReview
}

// ResolutionActor names who is asking for a transition.
type ResolutionActor string

const (
	// ActorCaller is the request path that ran (or refused to run) the
	// mutation, calling OperationLedger.Finalize.
	ActorCaller ResolutionActor = "caller"
	// ActorReconciler is OperationLedger.ReconcilePending acting on a
	// crash-orphaned row whose lease expired.
	ActorReconciler ResolutionActor = "reconciler"
	// ActorOperator is a human resolving a pending_review row through
	// OperationLedger.ResolvePendingReview. No automatic path holds this actor.
	ActorOperator ResolutionActor = "operator"
)

// EvidenceKind names what a transition's proof consists of.
type EvidenceKind string

const (
	// EvidenceCommitted: Ref is the canonical identifier (for example the
	// brain's Git commit) that contains the operation's internal ID. Legal
	// only for a transition to committed.
	EvidenceCommitted EvidenceKind = "canonical_committed"
	// EvidenceAbsent: Ref names the canonical head that was examined in full,
	// including uncommitted working-tree state, and found to lack the
	// operation's internal ID. Legal only for a release by the reconciler,
	// which produced it while holding the brain's exclusive BrainFence.
	EvidenceAbsent EvidenceKind = "canonical_absent"
	// EvidenceNoCanonicalAttempt: Ref is empty. The caller certifies that the
	// canonical writer was never entered (validation, scope or limit
	// rejection before EnterCanonical). Legal only for a release by the
	// caller, and the ledger refuses it once EnterCanonical has recorded
	// entry: after that a caller may only report success (committed), an
	// unknown outcome (pending_review), or leave the row for the reconciler.
	EvidenceNoCanonicalAttempt EvidenceKind = "no_canonical_attempt"
	// EvidenceUnknown: Ref is a sanitized reason class (never a raw error
	// body) for why the outcome could not be proven. Legal only for a
	// transition to pending_review.
	EvidenceUnknown EvidenceKind = "outcome_unknown"
	// EvidenceOperatorReview: Ref is the review reference the operator
	// inspected. Legal only for an operator transition out of pending_review.
	EvidenceOperatorReview EvidenceKind = "operator_review"
)

// Evidence is the durable proof stored with the transition that produced a
// phase. It replaces the earlier free-form canonicalRef string, which could
// not distinguish "proved landed" from "proved absent" from "unknown".
type Evidence struct {
	Kind EvidenceKind
	Ref  string
}

var (
	// ErrOperationTransition: the (from, to, actor) combination is not in the
	// state machine above.
	ErrOperationTransition = errors.New("hosted/contracts: illegal operation phase transition")
	// ErrOperationEvidence: the evidence kind or reference does not fit the
	// transition.
	ErrOperationEvidence = errors.New("hosted/contracts: operation evidence does not justify this transition")
	// ErrOperationConflict: the row already reached a different final phase
	// (or the same phase with different evidence); a repeated identical
	// Finalize is idempotent and never returns this.
	ErrOperationConflict = errors.New("hosted/contracts: operation already resolved differently")
	// ErrOperationNotFound: no row has this internal ID.
	ErrOperationNotFound = errors.New("hosted/contracts: operation not found")
	// ErrOperationInProgress: a live reserved row already holds this
	// (account, brain, client key). The caller must retry later, not charge.
	ErrOperationInProgress = errors.New("hosted/contracts: operation is in progress")
	// ErrOperationPendingReview: the row holding this client key is in
	// pending_review. A retry is refused until an operator resolves it, so a
	// retried key can neither double-charge nor discard a mutation that may
	// have landed.
	ErrOperationPendingReview = errors.New("hosted/contracts: operation awaits operator review")
	// ErrOperationKeyReuse: the client key is already bound to a different
	// request fingerprint or source, whatever phase its row is in.
	ErrOperationKeyReuse = errors.New("hosted/contracts: client key reused for a different operation")
	// ErrOperationNotReserved: EnterCanonical found the row is no longer
	// reserved (released, pending review or committed by another actor). The
	// writer must not write canonical state; it reports a retryable failure
	// and makes no ledger call.
	ErrOperationNotReserved = errors.New("hosted/contracts: operation is no longer reserved")
	// ErrOperationLimitExceeded: at least one delta would push its metric
	// past its limit in the operation's quota period. No delta is reserved.
	ErrOperationLimitExceeded = errors.New("hosted/contracts: operation would exceed an allowance")
	// ErrOperationInvalid: the request or its deltas are malformed.
	ErrOperationInvalid = errors.New("hosted/contracts: invalid operation request")
)

// ValidateTransition is the exhaustive, table-driven statement of the state
// machine. It returns nil only for a legal (from, to, actor, evidence)
// combination; it never inspects a ledger.
//
//	reserved → committed        caller|reconciler   EvidenceCommitted, Ref≠""
//	reserved → released         caller              EvidenceNoCanonicalAttempt, Ref=""
//	reserved → released         reconciler          EvidenceAbsent, Ref≠""
//	reserved → pending_review   caller|reconciler   EvidenceUnknown, Ref≠""
//	pending_review → committed  operator            EvidenceOperatorReview, Ref≠""
//	pending_review → released   operator            EvidenceOperatorReview, Ref≠""
//
// Anything else, including a same-phase "transition" (idempotency is a
// ledger concern, decided by comparing stored evidence) and any move out of
// a final phase, is ErrOperationTransition.
func ValidateTransition(from, to OperationPhase, actor ResolutionActor, ev Evidence) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("%w: unknown phase %q → %q", ErrOperationTransition, from, to)
	}
	needRef := func(kind EvidenceKind, wantRef bool) error {
		if ev.Kind != kind {
			return fmt.Errorf("%w: %s → %s by %s needs %s evidence, got %q", ErrOperationEvidence, from, to, actor, kind, ev.Kind)
		}
		if wantRef && ev.Ref == "" {
			return fmt.Errorf("%w: %s evidence needs a reference", ErrOperationEvidence, kind)
		}
		if !wantRef && ev.Ref != "" {
			return fmt.Errorf("%w: %s evidence must have an empty reference", ErrOperationEvidence, kind)
		}
		return nil
	}
	bad := func() error {
		return fmt.Errorf("%w: %s → %s by %s", ErrOperationTransition, from, to, actor)
	}
	switch {
	case from == OperationReserved && to == OperationCommitted:
		if actor != ActorCaller && actor != ActorReconciler {
			return bad()
		}
		return needRef(EvidenceCommitted, true)
	case from == OperationReserved && to == OperationReleased:
		switch actor {
		case ActorCaller:
			// A caller can certify only that it never entered the canonical
			// writer. It cannot certify absence: that needs the brain fence,
			// which only ReconcilePending holds.
			return needRef(EvidenceNoCanonicalAttempt, false)
		case ActorReconciler:
			return needRef(EvidenceAbsent, true)
		}
		return bad()
	case from == OperationReserved && to == OperationPendingReview:
		if actor != ActorCaller && actor != ActorReconciler {
			return bad()
		}
		return needRef(EvidenceUnknown, true)
	case from == OperationPendingReview && (to == OperationCommitted || to == OperationReleased):
		if actor != ActorOperator {
			return bad()
		}
		return needRef(EvidenceOperatorReview, true)
	}
	return bad()
}

// OperationDelta is one named counter an operation moves, as persisted.
type OperationDelta struct {
	Metric string `json:"metric"`
	Units  int64  `json:"units"`
}

// OperationRecord is one durable journal row. ClientKey is the optional
// caller-supplied retry key, scoped to (AccountID, BrainID). A record that is
// already Committed under the same ClientKey is returned by Reserve as a
// replay; it never charges twice.
type OperationRecord struct {
	ID          string
	AccountID   string
	BrainID     string
	ClientKey   string // "" when the caller supplied no retry key
	Deltas      []OperationDelta
	QuotaPeriod string // the period the operation was reserved in; Finalize charges this period even after rollover
	Phase       OperationPhase
	Source      string // canonical operation source, e.g. "gateway.remember"
	// Fingerprint binds ClientKey to canonical payload identity; see
	// RequestFingerprint. Empty only for unkeyed operations that supplied none.
	Fingerprint string
	// CanonicalEnteredAt is when the writer recorded, through EnterCanonical,
	// that it was about to write canonical state. Zero means the canonical
	// writer was never entered, which is the only state in which a caller may
	// release with EvidenceNoCanonicalAttempt.
	CanonicalEnteredAt time.Time
	// Evidence is the proof stored by the transition that produced Phase. It
	// is the zero value while Phase is reserved.
	Evidence       Evidence
	CreatedAt      time.Time
	LeaseExpiresAt time.Time // ReconcilePending may act on a reserved row only after this instant
	FinalizedAt    time.Time // set when Phase becomes committed or released; zero otherwise
}

// ReserveDelta is one counter a Reserve call asks to move, with the ceiling
// it is admitted against. Limit is supplied per call (as meter.Reserve's
// limit is today) and is not persisted.
type ReserveDelta struct {
	Metric string
	Units  int64
	Limit  int64
}

type ReserveRequest struct {
	AccountID, BrainID, ClientKey, QuotaPeriod, Source string
	// Fingerprint is RequestFingerprint over every semantically relevant
	// mutation parameter (for remember: the exact fact bytes and the target
	// brain). Required whenever ClientKey is set, so a retry that reuses a key
	// with different content fails instead of replaying the first result:
	// equal token counts and operation kinds can hide a different fact.
	Fingerprint string
	Deltas      []ReserveDelta
	// LeaseFor is how long the reservation may stay reserved before
	// ReconcilePending may act on it. It must be positive.
	LeaseFor time.Duration
}

// Validate reports ErrOperationInvalid for a malformed request: missing
// identifiers, no deltas, an empty or duplicated metric, or a negative unit
// count or limit. A metric may appear once per request, so a single row can
// never move one counter twice.
func (r ReserveRequest) Validate() error {
	if r.AccountID == "" || r.BrainID == "" || r.QuotaPeriod == "" || r.Source == "" {
		return fmt.Errorf("%w: account, brain, quota period and source are required", ErrOperationInvalid)
	}
	if r.LeaseFor <= 0 {
		return fmt.Errorf("%w: lease must be positive", ErrOperationInvalid)
	}
	if r.ClientKey != "" && r.Fingerprint == "" {
		return fmt.Errorf("%w: a client key requires a request fingerprint", ErrOperationInvalid)
	}
	if len(r.Deltas) == 0 {
		return fmt.Errorf("%w: at least one delta is required", ErrOperationInvalid)
	}
	seen := map[string]bool{}
	for _, d := range r.Deltas {
		if d.Metric == "" || d.Units < 0 || d.Limit < 0 {
			return fmt.Errorf("%w: delta %q has a negative amount or empty metric", ErrOperationInvalid, d.Metric)
		}
		if seen[d.Metric] {
			return fmt.Errorf("%w: metric %q appears twice", ErrOperationInvalid, d.Metric)
		}
		seen[d.Metric] = true
	}
	return nil
}

// CanonicalVerdict is what a CanonicalChecker concluded about one operation.
type CanonicalVerdict struct {
	Outcome CanonicalOutcome
	// Ref is the canonical identifier for CanonicalLanded, the examined head
	// for CanonicalAbsent, and a sanitized reason class for CanonicalUnknown.
	Ref string
}

type CanonicalOutcome int

const (
	// CanonicalUnknown: the check ran but could not prove either outcome (for
	// example the canonical history was unreadable or the operation's marker
	// was not verifiable).
	CanonicalUnknown CanonicalOutcome = iota
	// CanonicalLanded: the operation's internal ID is present in canonical
	// state.
	CanonicalLanded
	// CanonicalAbsent: canonical state was read in full and lacks the ID,
	// both in committed history and in any uncommitted working-tree or
	// touched-queue state. writer.Flush re-marks a failed commit's paths as
	// touched so a later successful Flush commits them; a fact that is absent
	// from Git history but still pending in the working tree can therefore
	// still land, and is CanonicalUnknown, never CanonicalAbsent.
	CanonicalAbsent
)

// CanonicalChecker inspects a brain's canonical state for one operation. Task44
// implements it per operation kind (for remember: whether the operation's
// internal ID marker is in the brain's canonical Git history). A returned
// error means "could not check right now" and is retryable; a nil error with
// CanonicalUnknown means "checked, cannot prove".
type CanonicalChecker interface {
	Check(ctx context.Context, rec OperationRecord) (CanonicalVerdict, error)
}

// OperationLedger is the durable journal task44 implements.
//
// Reserve refuses with ErrOperationLimitExceeded (no delta reserved) when
// committed usage plus every capacity-holding row (reserved and
// pending_review) plus the request would pass any delta's limit. It must NOT
// release, expire or otherwise change any other row: today's
// meter.Meter.Reserve opportunistically releases open rows whose lease
// expired; the ledger must not, because releasing an operation whose
// canonical outcome is unknown is exactly the failure this seam exists to
// prevent. Only EnterCanonical (records entry), Finalize, ReconcilePending and
// ResolvePendingReview change a row.
//
// A non-empty ClientKey with an existing non-released row is resolved in this
// order: a different Fingerprint or Source is ErrOperationKeyReuse in every
// phase; otherwise committed returns the stored row (a replay, never a second
// charge), reserved is ErrOperationInProgress, pending_review is
// ErrOperationPendingReview. A released row frees its key. The stored row's
// deltas are authoritative for a replay; a differing recomputation of the
// deltas (for example after a tokenizer change) never invalidates a retry.
//
// EnterCanonical is called by the writer inside its BrainFence commit section
// immediately before the first canonical byte. It returns the row after
// stamping CanonicalEnteredAt (idempotent), or ErrOperationNotReserved if the
// row is no longer reserved, in which case the writer must stop.
//
// Finalize is idempotent: repeating a transition already recorded with the
// same outcome and evidence returns the stored row (the deltas are never
// applied twice); a different outcome or different evidence on an
// already-resolved row is ErrOperationConflict.
type OperationLedger interface {
	Reserve(ctx context.Context, req ReserveRequest) (OperationRecord, error)
	EnterCanonical(ctx context.Context, operationID string) (OperationRecord, error)
	Finalize(ctx context.Context, operationID string, outcome OperationPhase, ev Evidence) (OperationRecord, error)
	// ReconcilePending acts on every reserved row whose lease expired. For
	// each row it acquires the brain's exclusive fence, and holding it: runs
	// checker, then performs the ledger transition, then releases the fence.
	// landed → committed, absent → released, unknown → pending_review. If the
	// fence cannot be acquired before ctx ends (a writer is inside its commit
	// section) or checker returns an error, the row stays reserved and counts
	// as Deferred, so a transient failure or a slow live writer is retried,
	// never converted into a guess. Rows whose lease is live, and rows in any
	// other phase, are untouched. Expiry is a liveness filter only; the fence
	// is what makes release safe.
	ReconcilePending(ctx context.Context, checker CanonicalChecker, fence BrainFence) (ReconcileReport, error)
	// ResolvePendingReview is the only exit from pending_review: an operator
	// records committed or released with EvidenceOperatorReview.
	ResolvePendingReview(ctx context.Context, operationID string, outcome OperationPhase, ev Evidence) (OperationRecord, error)
}

// ReconcileReport separates PendingReview and Deferred from both final
// outcomes so an operator can see unresolved work without reading rows.
type ReconcileReport struct {
	Committed     int
	Released      int
	PendingReview int
	Deferred      int // checker returned an error; row stays reserved for the next pass
}

// FingerprintField is one named, semantically relevant mutation parameter.
type FingerprintField struct {
	Name  string
	Value []byte
}

// RequestFingerprint returns a stable identity for a mutation request: a
// hex SHA-256 over a domain-separated, length-prefixed encoding of the kind
// and every field, sorted by name so argument order never changes the result.
// Include every parameter that changes what the canonical writer stores (for
// remember: the exact fact bytes and the brain), never a derived value such
// as a token count. Field names must be non-empty and unique.
func RequestFingerprint(kind string, fields []FingerprintField) (string, error) {
	if kind == "" {
		return "", fmt.Errorf("%w: fingerprint kind is required", ErrOperationInvalid)
	}
	sorted := append([]FingerprintField(nil), fields...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	put := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	put([]byte("serenity-operation-fingerprint-v1"))
	put([]byte(kind))
	for i, f := range sorted {
		if f.Name == "" || (i > 0 && sorted[i-1].Name == f.Name) {
			return "", fmt.Errorf("%w: fingerprint field names must be non-empty and unique", ErrOperationInvalid)
		}
		put([]byte(f.Name))
		put(f.Value)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
