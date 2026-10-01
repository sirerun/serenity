# T23.44 core writer seam implementation receipt — 2026-10-01

Implemented on an isolated branch from `afba4e0`, limited to `internal/writer` queue/commit support and focused tests. This is a core seam only; it does not route `MemoryFact`, hosted gateway, checker, service, or contracts through the new API and does not satisfy T23.44 acceptance.

The API preserves `Result` and ordinary `Queue.Submit` behavior. `Queue.SubmitAndFlush(ctx, root, job)` returns `SubmitAndFlushResult{Result, Committed}`: `Result.Err == nil` means Render and the requested flush completed, including a successful no-op; `Committed` is true only when Git created a new commit. The drain goroutine acquires the per-queue shared commit section before `runMu`, then holds both across Render and a shared locked flush helper. The existing hook runs after both guards release. Git subprocesses on this path receive the caller context; all paths taken before an error or cancellation are restored to the touched set.

`Queue.EnterCommit` and `Queue.WithCommitFence` provide a separate, context-aware writer-preferring shared/exclusive guard. The checker error and fence-acquisition error are returned separately. Exclusive fence callbacks must be canonical-state/ledger-only; they must not call `Submit` or `Flush` or wait for `runMu`, which would invert the documented commit-section → `runMu` order. A plain `Submit` and ordinary `Flush` do not acquire the new shared guard by design; production canonical mutations and flush paths must be explicitly routed before activating a checker fence.

Cancellation while waiting for the queue's ordered-send slot or before the drain starts the job returns promptly without rendering it. After the drain starts, cancellation reaches the Render closure and Git, and the caller waits for a settled result because source mutation may already be in flight; a closure must honor its context. This preserves unknown outcomes instead of returning while a mutation continues invisibly.

Focused tests cover inline publication/hook timing, preservation of unrelated staged and unstaged paths, failed-commit requeue and retry, Git/context cancellation, cancellation behind a stalled plain queue job, exclusive-fence fairness/cancellation, checker error separation, and a provider-style plain queue job that holds `runMu` without blocking the exclusive commit fence. Validation passed after releasing `R-core-writer`:

```text
GOCACHE=/Volumes/BuildOffload/.cache/core-writer/gocache \
GOTMPDIR=/Volumes/BuildOffload/.cache/core-writer/tmp \
go test -race ./internal/writer -count=1
ok   github.com/sirerun/serenity/internal/writer  63.188s
```
