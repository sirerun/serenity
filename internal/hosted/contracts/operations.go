package contracts

import (
	"context"
	"time"
)

// Operation identity/accounting (interfaces.md "Operation identity/
// accounting", owner task44, design review owner task41). PROPOSED: the
// mechanism (a durable operation journal keyed by internal ID, phase state
// machine below) is task41's proposed answer to the "crash-safe canonical-
// operation accounting" bounded decision in
// docs/launch/hosted-completion/interfaces.md's proposed-decisions section.
// It is not yet chief-architect approved; task44 must not begin production
// implementation against it until the freeze receipt records approval.
//
// Revision note: an earlier draft of this file gave OperationRecord a single
// Metric/Units pair. T23.41's own review found that shape does not
// implement the pre-existing "both counters finalize in one transaction"
// invariant this seam is required to satisfy — the current implementation
// issues two independent meter.Reserve/Finish cycles per "remember" call
// (writes, then input_tokens; internal/hosted/gateway/gateway.go:296-345),
// and a single-metric OperationRecord would just reproduce that same split
// one layer down as two sibling records with no way to reconcile them to a
// consistent joint outcome. Fixed below: one OperationRecord now carries
// every counter delta one logical mutation touches, reserved and finalized
// together as a single row/transaction — task44's future implementation
// must call Reserve/Finalize exactly once per logical mutation, not once
// per metric.
//
// OperationPhase is a strict forward state machine. Every operation starts
// Reserved, and terminates exactly once at Committed, Released or
// PendingReview — never a downgrade from a terminal phase back to Reserved,
// and never a change from one terminal phase to another.
type OperationPhase string

const (
	OperationReserved OperationPhase = "reserved"
	// OperationCommitted: canonical evidence proved the mutation landed;
	// every delta in OperationRecord.Deltas is applied to its usage counter.
	OperationCommitted OperationPhase = "committed"
	// OperationReleased: canonical evidence proved the mutation never
	// landed; no delta is applied.
	OperationReleased OperationPhase = "released"
	// OperationPendingReview is ReconcilePending's terminal state when the
	// canonical-evidence check could not prove either outcome (e.g. the
	// brain's canonical Git history was itself unreadable at reconcile
	// time). ReconcilePending must never resolve an unprovable row to
	// Committed (would risk crediting a mutation that never happened) or to
	// Released (would risk silently discarding one that did and letting a
	// retried ClientKey double it) — "never release unknown canonical
	// outcomes" is a hard requirement, not a tunable default. Only a human
	// operator, inspecting CanonicalRef by hand, may move a
	// PendingReview row to a true terminal phase; task44's freeze evidence
	// must specify that operator path, not auto-resolve it.
	OperationPendingReview OperationPhase = "pending_review"
)

// OperationDelta is one named counter this operation moves. One
// OperationRecord carries every delta a single logical mutation touches
// (e.g. a "remember" call reserves both a "writes" delta and an
// "input_tokens" delta) so they finalize together, never independently.
type OperationDelta struct {
	Metric string
	Units  int64
}

// OperationRecord is one durable journal row representing exactly one
// logical mutation, however many counters it touches. ClientKey is the
// optional caller-supplied retry key, scoped to (AccountID, BrainID) so two
// brains under the same account never collide on the same key. Two calls
// sharing a non-empty ClientKey and an already-Committed record must return
// the original committed record rather than double-charge (mirrors
// meter.Reservation's existing Replay behavior, generalized to a durable,
// crash-surviving row that finalizes every delta atomically instead of one
// metric at a time).
type OperationRecord struct {
	ID          string
	AccountID   string
	BrainID     string
	ClientKey   string // "" when the caller supplied no retry key
	Deltas      []OperationDelta
	QuotaPeriod string // matches meter.Entitlement.Window's existing period key shape
	Phase       OperationPhase
	Source      string // canonical operation source, e.g. "gateway.remember"
	// CanonicalRef is the durable, opaque reference to the canonical
	// evidence Finalize (or ReconcilePending) checked to decide Phase — e.g.
	// a brain's Git commit SHA proving a "remember" landed. Empty while
	// Phase == OperationReserved; always non-empty on every terminal phase,
	// including PendingReview (where it names what NIL result the check
	// against). This is the "durable canonical operation identity/evidence"
	// interfaces.md's seam table requires and the prior draft only
	// described in prose without a field to carry it.
	CanonicalRef string
	CreatedAt    time.Time
	// LeaseExpiresAt is when ReconcilePending may treat a still-Reserved row
	// as crash-orphaned. Mirrors meter.Reservation's existing
	// lease_expires_at column (internal/hosted/store/migrations.go's
	// reservations table) and must appear in the same-named SQL column
	// task44's migration adds — interfaces.md previously described
	// ReconcilePending querying lease_expires_at without this field
	// existing on the Go type; fixed here so the field and the query it
	// backs stay in the same place.
	LeaseExpiresAt time.Time
	FinalizedAt    time.Time // zero while Phase == OperationReserved
}

// OperationLedger is the durable journal task44 implements. Finalize commits
// the operation row and every one of its Deltas' usage counters in one
// transaction (interfaces.md: "Both counters finalize in one transaction");
// it is the only way a Reserved row leaves that phase, other than
// ReconcilePending resolving an orphaned one.
type OperationLedger interface {
	Reserve(ctx context.Context, req ReserveRequest) (OperationRecord, error)
	Finalize(ctx context.Context, operationID string, outcome OperationPhase, canonicalRef string) (OperationRecord, error)
	// ReconcilePending finds every operation still Reserved whose
	// LeaseExpiresAt has passed (a crash left it uncommitted) and resolves
	// each deterministically against its own canonical evidence: Committed
	// if evidence proves the mutation landed, Released if evidence proves
	// it didn't, PendingReview if the check itself could not be completed.
	// Never resolves an unprovable row to Committed or Released — see
	// OperationPendingReview. Task44 supplies the exact per-operation-kind
	// canonical check in its own freeze evidence.
	ReconcilePending(ctx context.Context) (ReconcileReport, error)
}

type ReserveRequest struct {
	AccountID, BrainID, ClientKey, QuotaPeriod string
	Deltas                                     []OperationDelta
	Source                                     string
}

// ReconcileReport separates PendingReview from both terminal outcomes so a
// caller can never mistake "resolved" for "safely resolved" — an operator
// must be able to see PendingReview > 0 without reading individual rows.
type ReconcileReport struct {
	Committed     int
	Released      int
	PendingReview int
	Errors        int
}
