package contracts

import (
	"context"
	"errors"
	"fmt"
)

// Storage admission (interfaces.md "Storage admission", owner task44, design
// review owner task41).
//
// STATUS: design approved conditionally by chief-architect on PR236 revision
// 218d7234d9abea5964f9d4640d1bfffe5c9f8087. Task44 remains blocked on
// allocation-based accounting, a measured MaxMutationStageBytes and an
// OS-enforced staging limit; schema owner cannot invent a hidden overage
// allowance. The executable specification is contractstest.RunStagingSuite.
//
// Three points are settled here: two defects of the previous draft, and what
// the word "bound" does and does not mean.
//
//  1. Hard staging bound. The previous draft measured a stage after it had
//     been written and only then admitted it, and serialized admission per
//     account. Per-account serialization does not bound the aggregate: stages
//     for different accounts run concurrently, so their bytes can exhaust
//     the device before any admission decision runs, and a measured value
//     cannot retroactively stop a write already in progress. The bound here
//     is enforced up front and during the write: ReserveStage takes a fixed
//     ceiling from a global staging budget before any stage byte exists, the
//     stager reports every write to a StageMeter that aborts the stage the
//     moment the reported bytes pass that ceiling, and the ceiling is a configured constant
//     (MaxMutationStageBytes) that task44 must prove against the largest
//     mutation the product accepts. Measured growth is then admitted against
//     the account and the operator headroom, and cannot exceed the ceiling as
//     accounted. Point 3 says what "as accounted" leaves open.
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
//
//  3. Accounted bytes are not enforced bytes. Every byte figure in this file
//     (StageTicket.CeilingBytes, StageMeter.Used, AdmitMeasured's
//     measuredBytes, the staging budget, the headroom) means one unit:
//     physical allocation. That is what the filesystem consumes: blocks
//     rounded up per file, directory and inode metadata, Git loose objects,
//     packs and the index, SQLite WAL and journal files, and vector-store
//     growth. StageMeter does not measure any of it. It counts the values its
//     caller passes to Add, so a stager that adds the serialized length of a
//     write undercounts, because a small page still allocates a whole block
//     and rewrites Git tree objects that the payload length does not show.
//     Git and the index also write files the stager never serializes, so
//     their allocation reaches the meter only if the seam reports it. Task44
//     must convert each write to allocated bytes and account for the external
//     Git and index writes before a CeilingBytes figure is a physical ceiling,
//     and must measure MaxMutationStageBytes that way on the real runtime.
//     No such measurement exists yet.
//
//     Even a correct accounting is cooperative. The meter refuses only what a
//     stager asks it about; a stager defect, a child process, or a write the
//     seam does not route through Add lands on disk with nothing to stop it.
//     Neither this package, a reference model, nor a passing meter test can
//     prove a hard bound. A production claim of "hard" needs an enforcement
//     layer the writing process cannot exceed: the staging area on a dedicated
//     size-limited filesystem or a quota-limited volume, sized to the global
//     staging budget, so an overrun fails inside the stage (ENOSPC or a quota
//     error) and cannot consume the shared device. The meter then gives an
//     early, precise abort and the operating system gives the bound. Until
//     task44 demonstrates that layer, the proposal claims an accounting
//     ceiling, not a physical one.

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
// AdmitMeasured records the stage's physical growth (allocated bytes, point 3
// above). It fails with ErrStageCeilingExceeded if measured passes the ticket
// ceiling, with
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

// StageMeter enforces a StageTicket's ceiling on the writing side, as a
// cooperative counter. The stager calls Add with the allocated bytes of every
// write it is about to make to the stage (file blocks, Git objects, index and
// vector growth; see point 3 above) before making it; once the running total
// would pass the ceiling Add refuses. The stage holds no more than the
// ceiling only if the stager reports every write in allocated bytes: Add
// records whatever it is told, does no I/O, and cannot see a write it is not
// told about, so it is an accounting tool and not an OS-enforced limit. It is
// not safe for concurrent use: one stage, one goroutine, one meter.
type StageMeter struct {
	ceiling int64
	used    int64
}

// NewStageMeter returns a meter bound to the ticket's ceiling.
func NewStageMeter(t StageTicket) *StageMeter { return &StageMeter{ceiling: t.CeilingBytes} }

// Add records n more allocated bytes. It returns ErrStageCeilingExceeded, and
// records nothing, if the total would pass the ceiling.
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

// Used is the bytes accepted so far. It is the sum of what the caller reported,
// which is a physical figure only if the caller reported physical bytes; pass it
// to StagingGate.AdmitMeasured.
func (m *StageMeter) Used() int64 { return m.used }
