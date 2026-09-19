package contracts

import (
	"context"
	"errors"
	"fmt"
)

// Storage admission (interfaces.md "Storage admission", owner task44, design
// review owner task41).
//
// STATUS: PROPOSED and BLOCKED on architecture approval. interfaces.md says
// "if neither a mathematical growth bound nor a separately approved
// staged-write design is established, task44 stays blocked; schema owner
// cannot invent a hidden overage allowance." This file specifies the staged
// design well enough to approve or reject; it does not lift the block. The
// executable specification is contractstest.RunStagingSuite.
//
// Two defects of the previous draft are corrected here.
//
//  1. Hard staging bound. The previous draft measured a stage after it had
//     been written and only then admitted it, and serialized admission per
//     account. Per-account serialization does not bound the aggregate: stages
//     for different accounts run concurrently, so their bytes can exhaust
//     the device before any admission decision runs, and a measured value
//     cannot retroactively stop a write already in progress. The bound here
//     is enforced up front and during the write: ReserveStage takes a fixed
//     ceiling from a global staging budget before any stage byte exists, the
//     stager writes through a StageMeter that aborts the stage the moment its
//     bytes pass that ceiling, and the ceiling is a configured constant
//     (MaxMutationStageBytes) that task44 must prove against the largest
//     mutation the product accepts. Measured growth is then admitted against
//     the account and the operator headroom, but it can never exceed the
//     ceiling by construction.
//
//  2. Feasibility. Canonical brain state is a live Git working tree committed
//     by writer.Flush (internal/writer/commit.go); there is no existing
//     temp-and-rename publish step, so "apply the mutation in an isolated
//     stage" is not something task44 can do without a core-writer change. That
//     change (for example writing objects into a Git quarantine directory and
//     publishing them by moving objects and updating the ref) is a shared-seam
//     request under T23.44 step 4, and it is a prerequisite of this design,
//     not an implementation detail. If the reviewer rejects that seam, the
//     fallback is a proven per-mutation growth ceiling with a corner fixture
//     (see interfaces.md §1); a statistical growth multiplier remains
//     withdrawn.

// StageRequest asks for room to stage one mutation.
type StageRequest struct {
	AccountID   string
	BrainID     string
	OperationID string // the operation ledger row this stage belongs to
}

// StageTicket is a granted staging reservation. CeilingBytes is the most the
// stager may write; it is fixed by the gate, never chosen by the caller.
type StageTicket struct {
	ID           string
	AccountID    string
	BrainID      string
	OperationID  string
	CeilingBytes int64
}

// StageOutcome says what became of a stage when its ticket is released.
type StageOutcome int

const (
	// StageDiscarded: the stage was thrown away; nothing reached canonical
	// storage. Its ceiling and any admitted growth return to their pools.
	StageDiscarded StageOutcome = iota
	// StagePublished: the stage became canonical state. Its measured growth
	// now belongs to the account's physical usage.
	StagePublished
)

// StagingGate is the storage admission gate task44 implements once approved.
//
// ReserveStage grants a ticket only if the global staging budget and the
// operator headroom can absorb one more ceiling: the sum of outstanding
// ceilings plus this one must stay within the budget, and the observed free
// bytes minus every outstanding ceiling must stay at or above the headroom.
// It fails without touching disk. ErrStagingBusy is retryable.
//
// AdmitMeasured records the stage's real physical growth. It fails with
// ErrStageCeilingExceeded if measured passes the ticket ceiling, with
// ErrStorageQuotaExceeded if the account's physical bytes plus growth
// already admitted but not yet published plus this growth would pass the
// account quota, and otherwise holds that growth for the account. Two
// admissions for the same account can therefore never both pass against the
// same remaining bytes, with or without a lock.
//
// Release is idempotent. It returns the ceiling to the global budget and, for
// StageDiscarded, the admitted growth to the account.
type StagingGate interface {
	ReserveStage(ctx context.Context, req StageRequest) (StageTicket, error)
	AdmitMeasured(ctx context.Context, ticket StageTicket, measuredBytes int64) error
	Release(ctx context.Context, ticket StageTicket, outcome StageOutcome) error
}

var (
	// ErrStagingBusy: the global staging budget or operator headroom cannot
	// absorb another stage right now. Retryable; no bytes were written.
	ErrStagingBusy = errors.New("hosted/contracts: staging capacity unavailable")
	// ErrStageCeilingExceeded: a stage passed its reserved ceiling. The stager
	// must abort and discard; the mutation is not acknowledged.
	ErrStageCeilingExceeded = errors.New("hosted/contracts: stage exceeded its reserved ceiling")
	// ErrStorageQuotaExceeded: admitting the measured growth would pass the
	// account's physical-byte quota. The mutation is refused before any
	// canonical byte changes and no acknowledged fact is ever removed.
	ErrStorageQuotaExceeded = errors.New("hosted/contracts: account storage quota would be exceeded")
	// ErrStageTicketUnknown: the ticket was not issued by this gate or was
	// already released.
	ErrStageTicketUnknown = errors.New("hosted/contracts: unknown stage ticket")
)

// StageMeter enforces a StageTicket's ceiling on the writing side. The stager
// calls Add for every byte it is about to write to the stage (file data, Git
// objects, index and vector growth) before writing them; once the running
// total would pass the ceiling Add refuses, so the stage never holds more than
// the ceiling. It has no I/O and is not safe for concurrent use: one stage,
// one goroutine, one meter.
type StageMeter struct {
	ceiling int64
	used    int64
}

// NewStageMeter returns a meter bound to the ticket's ceiling.
func NewStageMeter(t StageTicket) *StageMeter { return &StageMeter{ceiling: t.CeilingBytes} }

// Add records n more bytes. It returns ErrStageCeilingExceeded, and records
// nothing, if the total would pass the ceiling.
func (m *StageMeter) Add(n int64) error {
	if n < 0 {
		return fmt.Errorf("hosted/contracts: negative stage bytes %d", n)
	}
	if n > m.ceiling-m.used {
		return ErrStageCeilingExceeded
	}
	m.used += n
	return nil
}

// Used is the bytes accepted so far; pass it to StagingGate.AdmitMeasured.
func (m *StageMeter) Used() int64 { return m.used }
