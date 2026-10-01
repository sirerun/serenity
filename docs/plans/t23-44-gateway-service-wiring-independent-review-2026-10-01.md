# T23.44 Gateway/Service operation wiring review

Reviewed source: `c5c4e29f349f3c8993a8cd3a6da73e6edd7ddd52` in an isolated clone at `/Volumes/BuildOffload/worktrees/serenity-operation-wiring-review-20261001`.

## Verdict

**Blocked on the Gateway adapter's account-lock behavior.** The startup/ticker/Unknown handling is conservative by inspection, but `gateway.OperationReconciler` violates the frozen `BrainFence` lock contract in `internal/hosted/contracts/commitfence.go`.

`OperationReconciler.acquire` takes `Gateway.Maintenance.TryRLock`, then `accountLocks[shard].TryLock`, and retains both through pool acquisition and the canonical check. The interface contract explicitly says reconciliation must never take an account lock or `Runtime.Mutations`; it relies on the ledger's short SQL transactions to serialize quota transitions and avoids contending with locks held around slow request/provider work. Although `TryLock` avoids waiting and therefore avoids the immediate lock cycle, it still blocks the same-account request path while Git/canonical inspection runs, and it may repeatedly defer expired reservations under sustained same-account traffic. The current gateway test `TestOperationReconciliationHoldsLifecycleLocks` asserts that the account lock is held, so the tests codify this contract mismatch rather than detect it.

The adapter also implements the writer half of `BrainFence` through the same account-locking `acquire(..., true)` path. `EnterCommit` therefore returns `ErrBrainNotQuiescent` immediately when invoked from a request that already holds the account shard, instead of entering the per-brain shared commit section with context-aware waiting as the interface requires. Current canonical writes use the queue guard directly, so I found no active call site for this adapter method; it remains an invalid implementation of the exposed interface. Keep writer commit entry on the queue/runtime fence, and keep reconciliation's exclusive fence independent of the Gateway account shard.

## Verified conservative behavior

- `Service.AssembleWithDependencies` performs the operation sweep after deletion replay and provisioner recovery and before handler construction/publication. It checks startup context again before publishing the handler, then starts the operation worker.
- `Service.Close` cancels and joins the operation worker before closing Gateway, Pool, and Store. The periodic worker uses a one-minute ticker and each sweep has a five-second context.
- The service and Gateway share one `operation.Ledger` instance. Gateway fencing rechecks brain/account binding and requires `path_key == brainID`, a valid path, `ready` brain state, and active account status before opening the pool runtime; the checker also matches brain ID, account ID, and path.
- Pool reconciliation retains one exact Runtime lease across its exclusive queue fence and canonical check, and releases queue/runtime leases on error and release paths.
- The canonical checker returns `Landed` only for an exact matching committed fact. It returns `Unknown` for incomplete evidence and never synthesizes `Absent`. `ReconcilePending` scans only expired `reserved` rows; Unknown becomes `pending_review`, which retains capacity and has no automatic operator resolution path.

## Validation limits

This was a static review of the pinned source and existing focused tests. No Go test was run: the observed host load was 17.02 at review time, above the repository's load limit of 10. No provider, cloud, or service actions were performed. This receipt does not qualify a full build or the broader T23.44 work.

## Correction review

Follow-up source reviewed: `54c68582aa80b96485f155ce34dfc440b9c97e64` (production changes through `12db5d8b7da9be9c8d783c7b41627e6d33239d3f`). The two contract blockers above are corrected by this pin; I found no additional static blocker in the Gateway/Service wiring.

`OperationReconciler.Fence` now retains only the maintenance read fence and makes its active/ready account+brain query, owned-tree preflight, and runtime fence without acquiring an account lock. Its `EnterCommit` delegates directly to the pool adapter's exact runtime queue section. The service uses a warm-only `NewExistingReconciler` for periodic work; `AcquireExisting` neither opens nor evicts runtimes, so a concurrent `Drop` either sees the lease or wins first and causes a safe deferred result. Startup explicitly uses `NewStartupOperationReconciler` while the pool and service are still private, before handler publication or worker startup, allowing it to open cold runtimes for the initial conservative sweep. The added ticker regression covers cold deferral without creating a `.git` directory. Account binding and ready/active checks remain in the Gateway adapter and checker.

The earlier findings about exact Runtime retention, Landed-only positive proof, Unknown staying capacity-holding in `pending_review`, and reserved-only automatic reconciliation still apply. The separate positive Landed startup-counter test was not part of this source pin, so that case remains unverified here.

No Go checks were run: coordinator reported load 16.22, above the repository threshold of 10. This follow-up is a static review only and does not qualify full integration, production activation, or live behavior.
