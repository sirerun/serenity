# T23.44 HEAD checker and hosted mutation fencing design

Date: 2026-10-01  
Baseline: hosted remember routing `5e6413aef237138068c5f7c5f3397c055e13a` plus reviewed core queue seam.  
Status: read-only design recommendation. No checker, forget routing, fence adapter, startup reconciliation, ticker, or acceptance change is implemented here.


## Coordinator amendment — pinned absence contract and lock order

The earlier recommendations below are preserved as design history. The current candidate supersedes two points: ordinary Flush acquires the context-aware run lock, attempts a nonblocking shared guard, and releases the run lock before waiting if an exclusive checker is active/queued. It never holds the shared guard while waiting for a provider job's run lock. Canonical submissions retain shared guard then run lock; the checker never takes run lock or Runtime.Mutations.

The pinned CanonicalAbsent/EvidenceAbsent contract requires examining uncommitted source and touched-queue state as well as committed state. HEAD plus a zero CanonicalEnteredAt is insufficient for this contract. The unwired checker therefore returns Unknown for both entered-missing and no-entry-missing outcomes until a complete bounded absence proof is implemented and reviewed. Positive matching committed facts can still prove Landed. No contract wording is relaxed, no retained marker is added, and startup/ticker/operator activation remains off.

## Current hosted surface and gaps

`internal/hosted/pool/pool.go:open` builds memory handlers and then allowlists only `remember`, `recall`, `forget`, and `read_memory_fact`. Thus current hosted canonical source mutations are remember and forget. `cancel_memory_operation` exists in `internal/server/memory/cancel.go` as an extension tool but the hosted pool filters it out; it is not a hosted route today. Do not treat it as one or expose it as part of checker activation without a separate ownership/key-mapping decision.

Remember now uses the trusted `AfterFlush` route from `internal/writer/memoryfact.go`: `SubmitAndFlush` protects the fact write and Git commit, then `internal/server/memory/remember.go` submits optional index/embedding work. That index job uses the same queue’s ordinary `Submit`, but it mutates the index database rather than canonical Git source. It must remain outside the commit guard. The gateway’s `Runtime.Mutations` lock currently spans the handler and optional provider work; it serializes user mutations but is not an exclusive canonical fence and must not be used as one.

Forget remains unguarded by the new queue seam. `internal/writer/memoryfact.go:Forget` uses ordinary `Queue.Submit`; its render writes an expiry, writes an operation-key cancellation fence, purges index rows, removes the fact, and calls `rewriteForgottenPath`. `internal/hosted/gateway/gateway.go` flushes only after the handler returns. A checker could therefore race a worktree/history mutation before that flush unless forget becomes an inline-flush path. `rewriteForgottenPath` removes the fact path from every history ref and prunes old objects, so a previously entered operation whose fact was forgotten has no positive evidence after that rewrite. Under the selected conservative policy it stays Unknown; this design adds no retained applied marker.

Pool lifecycle paths also publish canonical queue state: `Runtime.Flush`, `Runtime.close`, `Pool.FlushAll`, gateway export/flush, `Pool.Drop` during brain deletion, and service backup/close flows. In `pool.go`, Runtime currently keeps the queue only inside closures. Every such flush must use the same queue guard as hosted remember/forget before a checker can run. Runtime/index initialization also calls `RecoverMemorySearch`; it updates the derived index and calls the embedder, not canonical Git source, and stays outside the commit fence.

The current cancellation record is not a positive proof. `MemoryExpiryPayload` format 2 stores an `OperationKey` and timestamp; `CancelRemoteOperation` creates it before a missing operation can land, while `eraseFact` creates or reuses the same shape after a fact is forgotten. `MemoryProjection.OperationCancellation` does not distinguish those cases. A future checker must ignore this marker as evidence that an operation landed.

## Conservative checker outcomes

Construct the checker per hosted brain from that brain’s private Runtime and resolve only `OperationRecord.ID` from the control ledger. Do not read a client request key from source payloads as the lookup authority. Read committed Git `HEAD` objects only. Do not call `SourceStore.All/Read` for checker proof because those APIs inspect the working tree.

- If HEAD contains exactly one valid memory-fact payload whose `CanonicalOperationID` and hosted `OperationKey` both match the ledger ID, return Landed with the existing replay-compatible `fact:<source SHA>` reference. An expired but still committed fact was applied and remains Landed.
- If HEAD contains multiple conflicting matches, a malformed matching record, an unreadable/unborn HEAD, or an inconsistent ledger/source identity, return Unknown/error.
- If no matching fact exists and `CanonicalEnteredAt` is set, return Unknown. This includes a pre-write cancellation, a fact removed by forget/history purge, a commit failure with only working-tree bytes, and any ambiguous history rewrite. Never infer Absent from the current cancellation marker or from the fact’s prior SHA.
- If `CanonicalEnteredAt` is empty, the ledger’s no-entry state can remain the absent proof after the same brain’s canonical writers are fully fenced. If HEAD nevertheless contains a fact with that operation ID, treat the contradiction as Unknown rather than releasing.
- Any cancellation marker, dirty/touched failed queue job, or uncommitted-only source can never produce Landed. No working-tree bytes count as positive evidence.

This intentionally leaves an entered operation Unknown after its fact was erased. Its reservation remains held for operator review under the existing contract. It avoids adding an ID-retention exception to ADR019. The separate content-free applied-marker design in `t23-44-content-free-applied-evidence-design-2026-10-01.md` remains unapproved and out of this path.

## Smallest queue/fence adapter

The existing `Queue.WithCommitFence(ctx, check)` is enough for an isolated callback, but not for `contracts.BrainFence.Fence`: `OperationLedger.ReconcilePending` must hold the exclusive fence across both checker inspection and the ledger transition. Add a context-aware acquire/release API over the queue’s existing writer-preferring gate, for example `Queue.AcquireCommitFence(ctx) (release func(), err error)`. Make release idempotent. A shared `Queue.EnterCommit(ctx) (release func(), err error)` is the matching adapter seam if non-queue callers need to bracket canonical writes; `SubmitAndFlush` already acquires the shared section internally and must not be wrapped in a second/manual acquisition.

Store the queue and brain ID on `pool.Runtime` and implement the `contracts.BrainFence` adapter from that exact queue. It must validate the requested brain ID and return the queue’s context-aware release closure. The adapter may expose `Fence`; do not make it use `Runtime.Mutations`. Reconciliation holds the exclusive release handle, reads HEAD using only read-only Git operations, finalizes the ledger row, then releases. Its callback must not call `Submit`, `SubmitAndFlush`, `Flush`, provider/index work, or any operation that reacquires the same queue guard.

The writer lock order remains shared commit guard → queue `runMu` → canonical work/`writer.Flush` → release both → trusted AfterFlush callback. The checker takes only the exclusive guard, then HEAD reads and a short ledger transition; it never takes `runMu` or `Runtime.Mutations`. This permits a slow provider-only queue job to overlap the checker without extending the exclusive section. `FlushContext(ctx, root)` (or the equivalent guarded Runtime wrapper) must acquire the shared guard before `runMu`; legacy `Flush` must not bypass fencing for hosted paths. Keep ordinary provider-only `Submit` unfenced. A checker finding `CanonicalEnteredAt` but no HEAD fact returns Unknown regardless of any pending working-tree/touched-path state; failed queues cannot become false Absent.

## Minimal later routing ownership

| File area | Later change needed before checker activation |
| --- | --- |
| `internal/writer/queue.go`, `commit_guard.go`, `commit.go` | Expose context-aware exclusive acquire/release; add guarded context flush with shared-guard-before-`runMu` order. Preserve `SubmitAndFlush` behavior and keep provider-only `Submit` unguarded. |
| `internal/writer/memoryfact.go`, `internal/server/memory/forget.go` | Add a hosted/context-aware forget entry point that uses one `SubmitAndFlush` span for expiry/cancellation, purge, removal, and history rewrite. Preserve existing local `Forget` behavior if needed. Keep `CancelRemoteOperation` out of hosted allowlist; if later exposed, route through the same guard and authoritative ledger ID mapping. |
| `internal/hosted/pool/pool.go` | Retain the one per-brain queue on Runtime; adapt `Runtime.Flush`, close, `FlushAll`, and drop lifecycle flushes to the guarded context path; implement the `BrainFence` adapter for that queue. |
| `internal/hosted/gateway/gateway.go`, `lifecycle.go`, `internal/hosted/service/service.go` | Preserve remember’s AfterFlush outcome; route forget and export/lifecycle flushes through guarded Runtime methods. A future reconcile caller must hold a Pool runtime lease but must not take `Runtime.Mutations`. No startup/ticker registration in this step. |
| `internal/hosted/contracts/operations.go`, `commitfence.go`, `internal/hosted/operation/ledger.go` | Align the no-entry/Unknown semantics and ensure `ReconcilePending` holds the exact Runtime queue fence across checker plus transition. Unknown leaves capacity held. |
| `internal/gitrun/gitrun.go` plus checker code | Provide an isolated read-only HEAD reader (or a reviewed read-only mode) using fixed `rev-parse`, `ls-tree`, and raw `cat-file`/`show` calls; reject replacement/object redirection and arbitrary path input. No raw `exec.Command("git", ...)`, checkout, fetch, status-based positive evidence, or provider calls. |

The current hosted pool allowlist bounds this audit to memory tools, but generic local writers (`tombstone`, `publish`, `import`, `dirtytree`, directions, entities) and any future tool additions need a fresh reachability audit before sharing a checker-enabled queue. Do not declare the queue guard complete repository-wide from this hosted-only routing plan.

## Bounded proof tests before any activation

1. Checker over a real temporary Git repository: matching fact in HEAD is Landed; missing/erased fact after `CanonicalEnteredAt` is Unknown; no-entry plus no matching HEAD fact is Absent; no-entry plus matching source, multiple/mismatched records, invalid payload/version, cancellation-only record, and unreadable HEAD are Unknown.
2. Working-tree-only fact and a failed Git commit never report Landed or Absent for an entered row. The checker test proves it read the captured HEAD tree, not `SourceStore` working-tree state.
3. A held queue exclusive fence blocks remember, hosted forget, and guarded flush through context cancellation. Conversely, a slow provider-only `Submit` does not hold the shared commit guard and does not block the checker.
4. Forget test confirms source removal/history rewrite and its final Git flush occur inside the shared guard. Checker cannot observe an intermediate rewritten-but-unflushed state. After completion it returns Unknown for the entered operation; no new retained marker is asserted.
5. Runtime/lifecycle tests cover explicit `Flush`, `FlushAll`, close/drop, backup, and export using the same queue gate. No runtime maintenance path can commit touched canonical paths outside it.
6. Ledger contract tests prove the exclusive release remains held until the checker verdict and transition are both complete, releases on every error/panic path, preserves Unknown rows, and does not acquire provider, account, or Runtime.Mutations locks.
7. Hostile Git configuration/replacement refs and malformed source paths cannot redirect the reader or execute hooks. Tests use the isolated Git helper and fail closed on unreadable canonical objects.

Do not enable startup or periodic reconciliation until every hosted mutation and commit route above is fenced and the exact checker/adapter paths receive independent review. This plan does not make absence-after-purge inferable, does not introduce new retained operation IDs, and does not mark T23.44 accepted.
