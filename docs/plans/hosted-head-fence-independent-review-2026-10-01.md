# Independent review: hosted HEAD fence candidate

Reviewed source: `263d5bb0ba463a70365004dc0f66e350141e8847` (author receipt subsequently banked at `8ee5bd8e124e8ba36f7d6ef27ef4c2055f4b952f`). Scope was the unwired hosted source fence, canonical HEAD checker, and forget/cancellation path. The checker is not activated by service startup or a periodic worker; this review does not qualify live hosted operation.

## Findings

I found no blocking defect in the reviewed scope. The commit gate is writer-preferring and context-aware. Inline `SubmitAndFlush` takes the shared gate before the writer run lock and holds both through render and flush; hooks run after release. `FlushContext` uses a nonblocking shared acquisition while holding the run lock and releases the run lock before waiting when an exclusive fence is pending. Plain provider-only queue jobs do not take the shared gate. The exclusive callback contract forbids queue/run-lock calls, avoiding lock inversion.

Hosted forget is marked at the gateway boundary and uses the contextual forget path. That path carries the request context through index purge and history rewrite. Unix history Git subprocesses run in their own process group, and cancellation kills the group; unsupported platforms fail closed. Cleanup uses a bounded context. The writer retains dirty paths after flush failure so the operation can be retried. Index purge may have partially completed if cancellation arrives during it; retry is idempotent.

The canonical checker binds the runtime to a brain ID and rejects mismatched records. It inspects the captured HEAD commit, accepts only the exact regular source blob paths and mode, verifies source bytes against the path hash, and requires both the canonical operation ID and operation key to match the exact ledger operation. It returns only an opaque source reference, not fact contents. Invalid repositories, malformed evidence, and unverified absence produce `Unknown`. Both entered-but-missing and no-entry/missing cases remain `Unknown`; the implementation does not establish proof of absence from the working tree.

The repository checks reject common object redirection and unsafe metadata: linked/special refs or object paths, alternates, grafts, promisor packs, non-regular HEAD/config/packed-refs, and unapproved Git config entries. The config parser rejects unknown settings and invalid boolean spellings. The helper also disables global/system configuration, hooks, lazy fetch, and replacement objects. These checks assume the canonical repository is otherwise controlled by the application; they are not a defense against an unrelated actor concurrently rewriting the repository outside the runtime gate.

## Independent validation

On the isolated review checkout, the following focused race checks passed:

- `go test -race -count=1 ./internal/hosted/pool -run '^TestCanonical'`
- `go test -race -count=1 ./internal/writer -run 'Test(FlushContext|HistoryProcessGroupCancellation|SubmitAndFlush|CommitPathsContext|WaitingExclusiveFence|PlainQueueJob|WithCommitFence)'`
- `go test -race -count=1 ./internal/server/memory -run '^TestHostedForgetPublishesErasureBeforeHandlerReturns$'`

Two mutation checks produced the expected runtime failures and passed again after restoring the source: removing object-tree symlink rejection caused the symlinked-object-directory regression test to accept the unsafe repository; forcing an entered-but-missing operation to `CanonicalAbsent` caused the post-forget test to reject the false absence verdict. The review checkout was clean after restoring both mutations.

## Limits

This is a local code and focused-test review. It does not establish hosted checker activation, startup wiring, durable ledger completeness, qualification against an authoritative absence proof, or live-provider behavior. The filesystem/HEAD scan is not atomic against writers that bypass the runtime's commit gate. The process-group cancellation regression is Unix-specific; other platforms deliberately return an unsupported error rather than claiming cancellation safety.
