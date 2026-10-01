# T23.44 canonical routing corrected-source review supplement

Supplement to the pinned-source RED review in [the original receipt](t23-44-submit-canonical-independent-review-2026-10-01.md); that receipt remains unchanged. Reviewed corrected source `08b9004e0c9c689ae94b5f9709c9d3af7abee5ab` and author receipt `b5e72cc56373f1c6e3fda886661e7b31c78b4241` in a separate SSD worktree.

All three reported route blockers are corrected:

- `cancelOperation` calls `CancelRemoteOperationContext(ctx, ...)`.
- Non-hosted forget calls `ForgetQueuedContext(ctx, ...)`, which uses `SubmitCanonical` and keeps the prior queue-and-later-flush behavior. The `Forget` compatibility wrapper delegates with `context.Background()`. Hosted forget continues through `ForgetContext` and inline flush.
- `TombstoneCascade` passes its request context to `TombstoneContext`.

The author's committed regression tests exercise cancellation behind the exclusive commit fence and verify no marker or source deletion occurs after releasing the fence. Independent focused race-enabled route checks passed:

- `go test -race -p 1 ./internal/server/memory -run 'TestCancelOperationHonorsContextWhileWaitingForCommitFence|TestLocalForgetHonorsRequestCancellationWhileWaitingForFence|TestHostedForgetPublishesErasureBeforeHandlerReturns' -count=1`
- `go test -race -p 1 ./internal/supersede -run '^TestTombstoneCascadeHonorsContextWhileWaitingForCommitFence$' -count=1`

Both used external `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, and `TMPDIR`. The corrected routing changes present no remaining blocker in this review scope. The queue lock ordering and AfterGuard/hook order are unchanged; canonical source remains touched for a later flush, and hosted forget returns only after canonical flush plus index purge succeeds.

This is not full T23.44 acceptance: the committed-head checker remains unwired, missing facts must remain Unknown, and hosted cancellation is not allowlisted. No full repository suite, lint, or integration gate was run by this reviewer.
