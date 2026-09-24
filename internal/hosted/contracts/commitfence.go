package contracts

import (
	"context"
	"errors"
)

// Per-brain commit fence (interfaces.md "Operation identity/accounting",
// owner task44, review owner task41).
//
// STATUS: design approved by chief-architect on PR236 revision
// 218d7234d9abea5964f9d4640d1bfffe5c9f8087. The reference implementation and
// its deterministic scenarios are contractstest.MemBrainFence and
// contractstest.RunLedgerSuite; task44 owns the production implementation.
//
// Purpose: make "the writer has stopped" a checkable fact instead of an
// inference from a timestamp. A writer brackets every canonical write in a
// commit section (EnterCommit ... leave). The reconciler takes the exclusive
// side (Fence) and holds it across the canonical absence inspection and the
// ledger transition. While the fence is held no writer is inside a commit
// section and none can enter one, so:
//
//   - a writer that finished its section before the fence is visible to the
//     checker (landed, or a partial write reported as unknown);
//   - a writer that enters after the fence is released calls
//     OperationLedger.EnterCanonical and finds its row already released, so it
//     stops before writing (stale-writer stop);
//   - a writer paused inside its section makes Fence wait; if ctx ends first,
//     Fence returns ErrBrainNotQuiescent and the row stays reserved. A slow
//     writer costs liveness, never correctness.
//
// Scope of the proof, so no reviewer assumes more: the fence excludes
// writers in this process. Other processes on the same host are excluded by
// the brain's exclusive ownership (writer.AcquireBrain, a kernel flock that
// dies with its holder), which the service holds for the brain's lifetime and
// which also makes a startup reconcile, before serving, trivially quiescent.
// Writers on a different host are excluded only by the restore fence
// (FenceReceipt), never by this type.
//
// Lock order (extends interfaces.md "Lock ordering and cancellation"):
//
//	writer:     Maintenance(read) → account lock → Runtime.Mutations →
//	            commit section (shared) → ledger transaction (EnterCanonical,
//	            Finalize) — the SQL transactions are short, never held across
//	            canonical I/O, and never nested inside another lock's wait.
//	reconciler: Maintenance(read) → brain fence (exclusive) → ledger
//	            transaction. The reconciler NEVER takes an account lock or
//	            Runtime.Mutations: it only converts held capacity to used
//	            (committed) or frees it (released), both safe against a
//	            concurrent same-account Reserve because each is one SQL
//	            transaction. Because it never waits on a lock a writer holds
//	            while that writer waits for the fence, the two cannot deadlock.
//	            Never acquire the account lock while holding the fence.
//
// The commit section must cover exactly the canonical write, from the first
// canonical byte through writer.Flush returning, and not the embedding or
// other provider calls before it: holding it across provider I/O would let a
// slow provider defer reconciliation for the brain.
type BrainFence interface {
	// EnterCommit opens a shared commit section on brainID. It blocks while a
	// Fence is held or waiting, and returns ctx.Err() if ctx ends first. The
	// returned leave must be called exactly once when the canonical write,
	// including Flush, has finished or failed.
	EnterCommit(ctx context.Context, brainID string) (leave func(), err error)
	// Fence opens the exclusive side on brainID once every commit section has
	// left, and returns release. If ctx ends first it returns
	// ErrBrainNotQuiescent and holds nothing.
	Fence(ctx context.Context, brainID string) (release func(), err error)
}

// ErrBrainNotQuiescent means a writer is still inside a commit section, so no
// claim about the brain's canonical state can be made yet. Retry later.
var ErrBrainNotQuiescent = errors.New("hosted/contracts: brain has a writer in its commit section")
