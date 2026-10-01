# T23.44 core writer seam independent review — 2026-10-01

## Verdict

The corrected queue seam is locally coherent for an explicitly routed canonical write and an inspection/ledger-only exclusive checker. The correction addresses the observed queue-wait cancellation gap and documents the callback restriction needed to avoid lock inversion. I found no blocker in the reviewed queue/guard behavior. This is an unused library seam: there is no production caller or checker activation in this source, so it does not close T23.44 or qualify hosted reconciliation.

## Reviewed source

- Seam implementation: `c78c939` (`feat(writer): add atomic submit and flush seam`).
- Initial validation correction: `a917ea7bdcafa50a2bccf53626793e119e2b6f10`.
- Reviewed lock-order/cancellation correction: `8f30a8a3291f737b0d12eca7a2123db579de51a0` (`fix(writer): order commit gate before queue lock`).
- Review checkout: `/Volumes/BuildOffload/worktrees/serenity-core-writer-queue-corrected-review-20261001`, branch `review/core-writer-queue-corrected-20261001`, based on `8f30a8a`.

## Findings

The commit gate is a writer-preferring shared/exclusive lock. Readers cannot enter while an exclusive waiter is queued; cancellation removes a waiting reader or writer from the counts and wakes contenders. Acquired release functions are idempotent. `SubmitAndFlush` acquires the shared commit section before `runMu`; the queue then holds `runMu` continuously across render, touched-path accounting, and selective Git flush. It releases `runMu` and the commit section before calling the hook. The shared section is not recursively reacquired during inline flush.

The corrected submission path uses an ordered-send token without holding the queue mutex across a blocking channel send. Context cancellation while waiting to enqueue returns promptly. If an accepted `SubmitAndFlush` job has not started, it is marked canceled and the drain skips rendering it. Once rendering has started, the caller waits for the settled response so the commit result is not hidden; the caller-provided render closure must itself observe its captured context if it can block. Git operations receive the context. The focused queued-cancellation regression test asserts that a preceding blocked ordinary queue job does not prevent the canceled inline request from returning and that the skipped render never runs.

Ordinary `Submit` intentionally bypasses the commit gate, as verified by `TestPlainQueueJobDoesNotBlockExclusiveCommitFence`. It is therefore only safe under an exclusive canonical check when the plain job is provider-only and does not mutate canonical state. Ordinary `Flush` also does not acquire the gate; canonical source changes must use the gated `SubmitAndFlush` path for fence correctness.

`WithCommitFence` holds exclusivity while invoking its callback. The corrected API comment explicitly forbids callback use of `Submit` or `Flush` and waiting on `runMu`; this is necessary because those operations can need the shared gate/run lock while the callback owns exclusivity. Under the intended callback contract—inspect canonical state and update the ledger only—the gate has no such lock cycle. No production checker currently invokes this callback.

The queue hooks run after both the shared gate and `runMu` are released. The shared `flushTouchedLocked` helper preserves selective staging/commit behavior; on commit failure it restores the touched paths for retry. Tests cover preservation of unrelated staged and unstaged files, failed-commit requeue, hook visibility after commit and lock release, writer preference, and plain provider-job progress under an exclusive fence.

## Validation and limits

Ran the focused race tests in the isolated corrected checkout:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/writer -run 'TestSubmitAndFlush|TestWaitingExclusiveFence|TestPlainQueueJobDoesNotBlockExclusiveCommitFence|TestWithCommitFence'`

Result: PASS (`2.868s`). I attempted the full package race suite against the pre-correction baseline but interrupted my own run after it remained at zero CPU for over two minutes while other builds were active. I did not repeat the full suite; the author reports a separate full writer race pass on the corrected source. No production code was changed by this review.

The seam remains unconnected to hosted remember/reconciliation. Plain `Submit` and ordinary `Flush` remain outside the fence, so production integration must route every canonical source write and publication through the gated path and keep checker callbacks free of queue operations. This review does not claim T23.44 acceptance, checker activation, or hosted qualification.

## Coordinator regression correction

An omitted-inline-flush mutation revealed that the publication hook test called the fixture Git helper, which invokes testing.Fatal inside the drain goroutine on a missing committed path and prevents the job reply. The test also ignored the checker error. The coordinator changes only this test to return Git errors and join checker/acquisition errors, preserving all assertions and production code. A clean negative-control rerun and restored focused check are required before the integration gate.

## Follow-up review of hook and deletion-route fixtures

I independently reviewed test-only correction `b032610da7d6835007966c1c10fafd0a023e0e79`. In `TestSubmitAndFlushPublishesBeforeHookAndLeavesGuards`, the hook now runs `git show` through `runGit(ctx, ...)` and returns command errors from the checker. It joins checker and exclusive-acquisition errors before storing the result under the hook mutex. This prevents a `t.Fatalf`/`runtime.Goexit` from terminating the queue drain goroutine and stranding the submitting test while still exposing either failure to the test goroutine. The focused race test passed:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/writer -run '^TestSubmitAndFlushPublishesBeforeHookAndLeavesGuards$'`

Result: PASS (`1.710s`). No production source changed in this correction.

I also reviewed test-only commit `246b871613f1545c0297471b3f547668f710ccd2`. The service-level dashboard fixture now exercises the assembled `/account/delete` route: pending closure returns 503 while the account remains `deleting` and brain bytes remain; retrying with the same session cookie and CSRF token succeeds after the closer reports `Closed`, then verifies the brain path is removed. This tests the actual `DeletionSession` retry fallback for a restricted account. The old direct dashboard fixture was removed from `TestLegacyCancelAccountRejectsPendingClosure`, leaving that test scoped to the legacy billing adapter's active account/plan behavior. The focused service race test passed:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/hosted/service -run '^TestDashboardPendingDeletionFreezesAndRetriesRetainingMemory$'`

Result: PASS (`1.856s`). The closer is a local fake; no live provider calls occurred. The full integration race/vet/lint gate was reported green by the coordinator, not independently rerun for this append. These remain test-fixture improvements and do not change the earlier limitation that the production canonical checker is not activated.
