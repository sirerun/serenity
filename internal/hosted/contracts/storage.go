package contracts

import "context"

// Storage admission (interfaces.md "Storage admission", owner task44, design
// review owner task41). PROPOSED and explicitly the least settled of the
// four bounded decisions: interfaces.md itself says "If neither [a
// mathematical growth bound nor a separately approved staged-write design]
// is established, task44 stays blocked; schema owner cannot invent a hidden
// overage allowance." This file freezes only the call shape a growth-bound
// check will have once task41's proposed decision (see interfaces.md
// proposed-decisions section) is reviewed; it does not assert the envelope
// formula is correct or approved.
//
// GrowthEnvelope reserves a conservative upper bound on physical bytes a
// mutation may add — covering canonical/Git, derived index, vector and WAL
// growth, not just the logical payload size — before the mutation is
// acknowledged. HeadroomBytes is the fixed operator reserve that must remain
// free regardless of any single account's own quota math.
type GrowthEnvelope struct {
	AccountID     string
	BrainID       string
	LogicalBytes  int64 // the mutation's own payload size
	ReservedBytes int64 // AdmissionChecker's conservative upper bound
	HeadroomBytes int64
}

// AdmissionChecker is the storage admission gate task44 implements.
// ReserveGrowth must serialize per-account admission (interfaces.md) and
// must never acknowledge a mutation whose conservative reservation would
// cross either the account's byte quota or the global operator headroom.
type AdmissionChecker interface {
	ReserveGrowth(ctx context.Context, req GrowthEnvelope) error
}
