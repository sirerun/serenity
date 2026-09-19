// Package testhooks defines named, deterministic fault-barrier checkpoints
// for hosted feature crash/recovery tests (interfaces.md "Fault barriers").
//
// A feature file calls At(phase) at an exact named checkpoint. In an ordinary
// build (no hostedtest tag), At is an unconditional no-op — see hook_prod.go.
// Only a binary built with `-tags hostedtest` links hook_hostedtest.go, and
// even that build only activates a hook when the process was started with an
// actual inherited control pipe (never an HTTP endpoint, never a bare
// environment-variable switch reachable in production). See hook_hostedtest.go
// for the exact transport and wire protocol.
package testhooks

// Phase names a feature file may call At() with. New phases are appended here
// by their owning task; feature code never invents an ad hoc phase string.
const (
	// PhaseOperationReserved fires after task44's operation journal records a
	// reservation durably but before the metered call executes.
	PhaseOperationReserved = "operation_reserved"
	// PhaseOperationCommitted fires after task44 finalizes both the operation
	// journal row and the usage counter in one transaction, before either the
	// lease is released or the caller's response is written.
	PhaseOperationCommitted = "operation_committed"
	// PhaseDeletionJournaled fires after task48 durably appends a deletion
	// intent to the independent journal, before the corresponding purge.
	PhaseDeletionJournaled = "deletion_journaled"
	// PhaseDeletionPurged fires after task48 purges canonical brain/account
	// bytes, before the journal outcome record is appended.
	PhaseDeletionPurged = "deletion_purged"
	// PhaseOperationCanonicalEntered fires after task44's
	// OperationLedger.EnterCanonical recorded entry inside the brain's commit
	// section, before the first canonical byte is written. A crash here leaves
	// a reserved row that has entered the canonical writer; only the fenced
	// reconciler may resolve it.
	PhaseOperationCanonicalEntered = "operation_canonical_entered"
	// PhaseOperationCanonicalWritten fires after the canonical write (including
	// writer.Flush) completed and the commit section was left, before
	// Finalize. A crash here must reconcile to committed exactly once.
	PhaseOperationCanonicalWritten = "operation_canonical_written"
	// PhaseReconcileFenced fires after ReconcilePending acquired a brain's
	// exclusive fence, before it runs the canonical checker. Pausing here
	// proves a live writer is stopped or waited for rather than raced.
	PhaseReconcileFenced = "reconcile_fenced"
	// PhaseStageMeasured fires after task44 measured a staged mutation's
	// physical growth, before StagingGate.AdmitMeasured.
	PhaseStageMeasured = "stage_measured"
	// PhaseStagePublished fires after a staged mutation was published into
	// canonical storage, before the operation is finalized.
	PhaseStagePublished = "stage_published"
	// PhaseJournalSealed fires after DeletionJournal.Seal created and verified
	// the seal object, before task50 assembles the fence receipt.
	PhaseJournalSealed = "journal_sealed"
	// PhaseBackupManifestWritten fires after task49 stages a manifest v2
	// artifact, before it is atomically published as the current backup.
	PhaseBackupManifestWritten = "backup_manifest_written"
	// PhaseRestoreFenced fires after task50 proves the old writer is fenced,
	// before it applies later deletions/provider truth to the restored set.
	PhaseRestoreFenced = "restore_fenced"
	// PhaseRestoreUnfrozen fires after task50 applies later deletions and
	// provider truth, before it atomically unfreezes eligible survivors.
	PhaseRestoreUnfrozen = "restore_unfrozen"
)

// At is the single call site a feature file uses at a named checkpoint. It is
// safe to call from any build: production binaries never link an active hook.
func At(phase string) { at(phase) }
