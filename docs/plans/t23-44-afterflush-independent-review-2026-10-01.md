# T23.44 inline-flush outcome review

Reviewed candidate `64d007f829d55ebf512e2f9bb448c31c652b660c` (parent `ea7a7d7a0f4aeb10f92bdac00f768a75f04a7d3f`) in a separate review worktree. The source checkout was `/Volumes/BuildOffload/worktrees/serenity-remember-afterflush-20261001`; this review branch contains only this receipt. No production files were changed.

## Verdict

No blocking correctness issue found in the narrow hosted `remember` inline-flush outcome path. The change closes the source-write-to-Git-publication gap before the memory handler proceeds to search indexing, and the gateway can finalize from the resulting durable fact identity even if later response or index work fails. This is a local writer/gateway seam only; it does not complete broader canonical-operation recovery or fencing.

## Trust and ordering checks

- The operation context key is unexported, and its helper says request JSON must not populate the trusted metadata (`internal/writer/canonical_operation.go:5-25`). The gateway reserves the ledger record, replaces the handler's `operation_key` with the reserved internal operation ID, and attaches callbacks only for hosted `remember` with the operation ledger (`internal/hosted/gateway/gateway.go:456-509`). The stored source payload receives that ID through the writer path (`internal/writer/memoryfact.go:137-208`). The client-supplied `canonical_operation_id` is not used to populate this context.
- In `MemoryFact.RememberContext`, inline mode requires a valid trusted ID and `BeforeCommit`; its wrapper sets `entered` only after the original callback succeeds. It calls `SubmitAndFlush` against `w.Sources.Root`, propagates flush errors without notification, and calls `AfterFlush` only when entry occurred, the result payload carries the same canonical ID, and the fact SHA is nonempty (`internal/writer/memoryfact.go:90-131`). The callback is made after `SubmitAndFlush` returns. The new writer test verifies it can read the blob from Git `HEAD` and reacquire the queue commit fence from inside `AfterFlush` (`internal/writer/memory_afterflush_test.go:24-78`), supporting that the notification runs after publication and after the queue guards are released.
- Gateway `AfterFlush` records only a nonempty fact ID paired with the reserved ledger ID. Deferred finalization uses that durable ID as committed evidence before falling back to pending review for an entered operation or release for no entry (`internal/hosted/gateway/gateway.go:456-463`, `504-508`; `rememberFinalizeOutcome` in `gateway.go`). It does not depend on the eventual tool result being a success response.
- The memory handler returns from `RememberContext` before it queues search refresh and embedding work (`internal/server/memory/remember.go:65-123`). Thus the canonical source and Git commit precede optional indexing/provider work. The gateway's later runtime flush can still fail after the inline flush; the already-captured fact ID remains available to deferred finalization.
- Calls without `AfterFlush` continue to use the previous `Queue.Submit` path (`internal/writer/memoryfact.go:119-124`), preserving local legacy `Remember` behavior.

## Runtime evidence

I ran an overlay mutation that forced `inlineFlush := false`, exercising the old queued route without changing the worktree. The focused new writer tests went red at runtime: the callback identity was empty for the fact, and the retry case observed `before=1 after=0`. Restoring the candidate made both focused tests pass.

Commands used, with build cache and temporary files on the external SSD:

- Mutated route: `go test -overlay=.../overlay.json -count=1 ./internal/writer -run '^TestHostedRememberAfterFlush'` — expected runtime failure. One initial invocation was rejected by Go because my temporary overlay JSON was malformed; it ran no tests. The corrected overlay produced the red result recorded above.
- Restored candidate: `go test -count=1 ./internal/writer -run '^TestHostedRememberAfterFlush'` — passed (0.659s).
- Restored candidate race check: `go test -race -count=1 ./internal/writer -run '^TestHostedRememberAfterFlush'` — passed (1.763s).

The one-minute load samples immediately before those valid test launches were 8.04, 6.69, and 4.94, respectively, within the required limit. I did not run the full writer suite or the service package in this independent lane. The existing service integration test `internal/hosted/service/canonical_operation_test.go:39-78` covers validation refusal followed by a successful gateway-to-writer operation and verifies the committed ledger ID matches the stored source marker; I inspected it but did not rerun it here.

## Remaining scope and coverage

This seam applies only when the hosted gateway handles `remember` with `g.Operations != nil` and supplies the trusted callback. The path without an operation ledger and local writer callers without `AfterFlush` retain legacy flush timing. Hosted `forget`, other source mutations, and other routes do not gain this callback or inline-flush guarantee from this change.

The new focused tests cover durable publication, queue-guard release, duplicate suppression, and refusal suppression. They do not directly inject an indexing/handler failure after publication and assert the gateway's database row remains committed; that outcome is supported by the gateway's deferred finalization logic but would benefit from a service-level failure regression. Cancellation and Git flush failure also have no new direct regression: current behavior is conservative because `RememberContext` returns before `AfterFlush` on flush error, leaving an entered operation pending review.

There is no checker or recovery path here for a process death after Git commit but before the callback/finalization, no Forget integration, marker adoption, startup activation gate, or cross-route fence. A crash in that interval can remain `pending_review`; resolving it requires the separately owned checker/activation work. The receipt therefore supports this candidate as a narrow writer/gateway fix, not as completion of the larger hosted-operation task.

The repository instructions' Ajent feed was unavailable as a callable tool in this review environment; I read the canonical `ajent.social` project file at lane start.
