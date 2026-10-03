# Independent producer-source review — corrected f4de candidate

**Verdict: CLEAR for the backup-owned snapshot lease producer component**
**Exact reviewed head:** `f4de27e029d88fa1e7bdd61b5e140524ce7a6154`
**Detached review clone:** clean at exact head after behavioral-mutant restoration.
**Scope:** durable snapshot staging/pinning/resume/cancellation/reconciliation/release, candidate and restore borrowing/scratch, and private store identity/absence proof. No provider/startup factory, READY, deployment, or production acceptance.

## Finding resolved

The e67 blocker was that `verifyStagedBrain` left expanded `<brain>.repo` repositories in the scratch tree later retained as a lease, while `ArtifactBytes` and aggregate limits counted only raw verified artifacts. At f4de, each expanded verification clone is isolated under a uniquely named child created through the stage `os.Root`; the child identity is captured and `removeInspectionScratch` performs identity-checked, root-confined removal in a defer. Removal and root-close errors are joined into the inspection result, so Stage cannot publish a lease after cleanup failure. The new `TestSnapshotLeaseStageDoesNotRetainExpandedBrainRepository` rejects any unexpected or non-regular top-level entry in the durable lease.

The fixed byte limits still apply to the raw artifacts and lease-backed raw verifier copies. Transient expanded clone/destination growth has no hard physical quota here and remains open as a separate acceptance item; this review does not claim that the raw-byte limits bound it.

## Other reviewed producer seams

The required store identity is opaque and bound to the private root and stable lock inode; those identities are checked before and after lock acquisition. Pin ordering durably records `PIN_PENDING` before the owner commit and supports exact tuple resumption. `Close` and `Reconcile` preserve bytes when an owner has committed while local state remains staged, and malformed/conflicting owner attempt lists fail closed before cleanup. Exact cancellation uses a callback-scoped, private proof bound to tuple, root/store identity, and held locks; aliases share one synchronized consume state and expire after the callback. Permanent canceled-tuple handling does not advance or mutate later N+1 attempts. Borrow/close uses an in-process borrower count and cross-process lease lock. Candidate and RestoreVerified scratch track raw copied bytes and restore/reconcile owned scratch across crashes. No other critical source seam was found in this scope.

## Independent verification

- Focused regression: `go test ./internal/hosted/backup -run '^TestSnapshotLeaseStageDoesNotRetainExpandedBrainRepository$' -count=1` passed under shared lease `b8fc05f2c19878e87aefa5ab7796cd012b5b6142`.
- Behavioral RED: a compiled mutant that skipped the confined verification-directory cleanup failed the same regression because the expanded verification directory remained in the staged lease. It ran under lease `90f34dc20c9f8ecb7cd14a50290d5a045ec0de53`.
- After restoring the source file, its SHA-256 returned to `788b08e5dfb0903af9fecd81998bcaa406a0dbfa87d00322a316506f65e49e4e`; `git status` is clean and focused `go test ./internal/hosted/backup -run '^TestSnapshotLease' -count=1` passed under lease `42d59efd3b42f65a641e4c41240987b3ab99680c`. All three leases have verified release exit 0 and runner source fingerprints are recorded in their stage JSON.
- Coordinator full qualification, supplied separately from this review layer: exact f4de full race 3,047 test passes / 84 packages, nine test/subtest skips and four no-test packages; full vet, lint, and Linux build exit 0, with leases released.

All focused runner logs and stage records are preserved in this directory. No source changes, claims, pushes, or merges were made by this reviewer.
