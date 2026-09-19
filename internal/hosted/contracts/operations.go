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
// OperationPhase is a strict forward state machine. Every operation starts
// Reserved, and terminates exactly once at either Committed or Released —
// never both, never a downgrade from a terminal phase back to Reserved.
type OperationPhase string

const (
	OperationReserved  OperationPhase = "reserved"
	OperationCommitted OperationPhase = "committed"
	OperationReleased  OperationPhase = "released"
)

// OperationRecord is one durable journal row. ClientKey is the optional
// caller-supplied retry key, scoped to (AccountID, BrainID) so two brains
// under the same account never collide on the same key. Two calls sharing a
// non-empty ClientKey and already-Committed Outcome must return the original
// committed record rather than double-charge (mirrors meter.Reservation's
// existing Replay behavior, generalized to a durable, crash-surviving row).
type OperationRecord struct {
	ID          string
	AccountID   string
	BrainID     string
	ClientKey   string // "" when the caller supplied no retry key
	Metric      string
	Units       int64
	QuotaPeriod string // matches meter.Entitlement.Window's existing period key shape
	Phase       OperationPhase
	Source      string // canonical operation source, e.g. "gateway.remember"
	CreatedAt   time.Time
	FinalizedAt time.Time // zero while Phase == OperationReserved
}

// OperationLedger is the durable journal task44 implements. Finalize commits
// both the operation row and the usage counter it backs in one transaction
// (interfaces.md: "Both counters finalize in one transaction"); it is the
// only way a Reserved row leaves that phase.
type OperationLedger interface {
	Reserve(ctx context.Context, req ReserveRequest) (OperationRecord, error)
	Finalize(ctx context.Context, operationID string, outcome OperationPhase) (OperationRecord, error)
	// ReconcilePending finds every operation still Reserved whose lease has
	// expired (a crash left it uncommitted) and resolves it deterministically
	// — task44 supplies the exact resolution rule in its own freeze evidence.
	// This is the "canonical committed-but-unfinalized operations reconcile
	// before lease release" invariant's recovery query.
	ReconcilePending(ctx context.Context) (ReconcileReport, error)
}

type ReserveRequest struct {
	AccountID, BrainID, ClientKey, Metric, QuotaPeriod string
	Units                                              int64
	Source                                             string
}

type ReconcileReport struct {
	Resolved int
	Errors   int
}
