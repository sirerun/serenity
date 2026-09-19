package contracts

import "context"

// Storage admission (interfaces.md "Storage admission", owner task44, design
// review owner task41). PROPOSED and explicitly the least settled of the
// four bounded decisions: interfaces.md itself says "If neither [a
// mathematical growth bound nor a separately approved staged-write design]
// is established, task44 stays blocked; schema owner cannot invent a hidden
// overage allowance."
//
// Revision note: an earlier draft of this file's doc comment (and
// interfaces.md's decision text) proposed computing ReservedBytes as a
// predicted upper bound — observed p95 physical-growth ratio times a 1.5
// safety margin — before a mutation runs. T23.41's own review, echoing
// interfaces.md's own requirement, found a statistical multiplier is not a
// hard ceiling: a single outlier mutation (an unusually large Git repack,
// pathological vector-index growth) can exceed p95*1.5, silently breaching
// the advertised quota — exactly the "never erase acknowledged data or
// silently relabel bytes as tokens" failure interfaces.md forbids. Fixed
// below: the shape now names the staged-write mechanism interfaces.md
// offers as the alternative, using measured (not predicted) growth.
// task44 remains explicitly BLOCKED from implementing this seam until
// chief-architect approves the mechanism below or a proven mathematical
// bound — see interfaces.md's "Proposed decisions" §1.
//
// Proposed flow: (1) apply the mutation in an isolated stage that never
// touches canonical account data; (2) measure its real physical growth
// directly, the same filepath-walk technique gateway.Inventory already
// uses, not a prediction; (3) call ReserveGrowth with that measured value;
// only on success (4) atomically publish the stage into canonical storage —
// a step that must itself be crash-recoverable if the process dies between
// (3) and (4), tying into decision 2's operation ledger Reserved phase.
// Per-account admission stays serialized through gateway.go's existing
// accountLocks sharding, so two concurrent staged mutations for one account
// can never both measure against the same remaining headroom.
type GrowthEnvelope struct {
	AccountID     string
	BrainID       string
	LogicalBytes  int64 // the mutation's own payload size, for reference only
	MeasuredBytes int64 // actual physical growth of the already-staged mutation, measured post-hoc
	// OperatorHeadroomBytes is the fixed, global floor that must remain free
	// regardless of any single account's own quota math — the "transient/
	// operational bytes... outside customer quota and still bounded
	// globally" interfaces.md's seam table requires.
	OperatorHeadroomBytes int64
}

// AdmissionChecker is the storage admission gate task44 implements once
// approved. ReserveGrowth must never acknowledge a staged mutation whose
// MeasuredBytes would cross either the account's remaining byte quota or
// OperatorHeadroomBytes.
type AdmissionChecker interface {
	ReserveGrowth(ctx context.Context, req GrowthEnvelope) error
}
