# T23.44 hosted remember after-flush routing

Date: 2026-10-01  
Base: coordinator `hosted/canonical-assembly-20261001` at `ea7a7d7a0f4aeb10f92bdac00f768a75f04a7d3f`.  
Scope: trusted hosted remember source durability before optional index/provider work. This lane does not implement or activate a canonical checker, reconciliation startup/ticker, forget/cancel markers, or hosted acceptance.

## Change

Trusted `CanonicalOperation` metadata now has an `AfterFlush` notification carrying the ledger operation ID and durable fact SHA. `MemoryFact.RememberContext` takes the inline `Queue.SubmitAndFlush` path only when this callback is present and the trusted ID, `BeforeCommit`, and request operation key match. Existing local and legacy callers without `AfterFlush` continue to use ordinary `Queue.Submit`.

The writer wraps `BeforeCommit` to record successful canonical entry, then calls `AfterFlush` only when the inline flush returned without error, that entry occurred, and the resulting fact record carries the same canonical operation ID. The callback runs after `SubmitAndFlush` releases its queue guards. Exact retries, semantic duplicates, preflight refusals, canceled jobs, and ambiguous Git failures cannot create a new durable notification.

The hosted gateway captures the fact SHA from that callback. Its deferred ledger finalizer commits from the trusted durable outcome even if later response rendering, optional index/provider work, or the gateway’s subsequent flush fails. An entered operation without a successful callback remains `PendingReview/Unknown`; an operation that never entered remains `Released/NoCanonicalAttempt`. The later `Runtime.Flush` remains for separately touched index state and is no longer used as remember’s source-durability signal.

## Verification

The writer race suite confirms the callback observes the matching source bytes in Git `HEAD` and can reacquire the exclusive commit fence, proving both queue guards have been released. Writer tests also cover the existing legacy callback path and ensure duplicate/refused attempts do not synthesize the after-flush result. Gateway tests cover committed, pending-review, and released finalizer outcomes.

An assembled hosted-service regression injects an embedding failure. At provider entry it resolves the memory source and verifies the exact fact SHA and canonical operation ID are already present in `HEAD`. The call returns degraded search state, the ledger is committed, and retrying the same raw operation key replays the same fact ID without a second provider call, source row, or committed write.

Passing focused checks:

- `go test -race ./internal/writer -count=1`
- `go test -race ./internal/hosted/gateway -count=1`
- `go test -race ./internal/hosted/service -run '^TestHostedRememberCommitsBeforeProviderFailureAndReplays$' -count=1`
- Existing hosted service regression: `go test -race ./internal/hosted/service -run '^TestHostedWriterRetryKeyIsScopedToQuotaPeriod$' -count=1`
- `git diff --check`

All Go caches and temporary files for these checks were kept under the external build volume. No multi-package build was run. The canonical checker is still absent, and other mutation/flush paths—including remember’s separate index job, forget/cancel, and generic queue writers—remain outside this change’s fence. T23.44 and hosted acceptance therefore remain open.
