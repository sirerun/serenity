# Independent operation reconciliation Pool and ledger review

Review pin: `39bfe213d95090aa955fa22afb92364ce8aebf90` in an isolated full clone. The `T23.47` filename is the receipt's local tracking label for the T23.44 conservative operation-reconciliation slice; it does not indicate full T23.47 acceptance.

Scope is limited to `internal/hosted/pool` and `internal/hosted/operation` Pool/Ledger behavior. This is not a full T23.47 acceptance, production qualification, or review of Gateway/Service integration.

## Findings

The Pool-backed `BrainFence` retains the exact runtime lease through `Check` and until its once-only release callback. `Check` requires a matching held fence and returns Unknown when the fence is absent, mismatched, or released. `Pool.Close` marks the pool closed before waiting for the lease wait group, preventing new acquisitions while existing leases drain; it closes runtimes only after the wait. Existing-only reconciliation uses `AcquireExisting`: it does not open a cold runtime or evict/open one after `Drop`, while startup reconciliation retains the normal cold-acquire path.

Ledger reconciliation snapshots candidate IDs, acquires the per-brain fence, rereads each candidate before checking, and rereads it again inside the transition transaction. Only still-reserved, still-expired rows are checked or transitioned. The candidate query only selects reserved rows, so `pending_review` is not automatically settled by this pass. The pool-backed checker does not claim Absent: non-landed evidence remains Unknown (`working_tree_absence_unverified`).

No remaining blocker was found in this bounded Pool/Ledger scope. This finding does not establish whole-feature acceptance or production safety outside the reviewed code and tests.

## Validation

- `go test -count=1 -p 1 ./internal/hosted/pool -run '^TestReviewPoolCloseWaitsForReconciliationLease$'` passed using a temporary reviewer-owned test that held a fence while `Pool.Close` ran, checked the held runtime during the wait, then verified Close completed after release. Removing the Close wait made this test fail because the runtime closed while leased. The temporary test and mutation were removed/restored.
- `go test -count=1 -p 1 ./internal/hosted/operation -run '^TestReconcileSkipsFinalizedCandidateAndContinuesBatch$'` passed on restored source. Disabling the post-fence reread guard made it fail because the checker received a candidate finalized while waiting for the fence. Disabling the in-transaction reread guard made it fail with `operation already resolved differently` after finalization during the checker call. Both mutations were restored.
- `go test -count=1 -p 1 ./internal/hosted/pool -run '^TestExistingReconcilerNeverOpensOrReopensColdRuntime$'` passed.
- `git diff --check` passed after restoring all source mutations; the clone had no source diff before this receipt was added.

All Go commands used external `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, and `TMPDIR`. Each command was preceded by `uptime`; the one-minute load was within the configured limit. No full race or multi-package build was run in this review lane.
