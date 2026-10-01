# T23.44 core writer seam implementation receipt — 2026-10-01

Implemented on an isolated branch from `afba4e0`, limited to `internal/writer` queue/commit support and focused tests. This is a core seam only; it does not route `MemoryFact`, hosted gateway, checker, service, or contracts through the new API and does not satisfy T23.44 acceptance.

The API preserves `Result` and ordinary `Queue.Submit` behavior. `Queue.SubmitAndFlush(ctx, root, job)` returns `SubmitAndFlushResult{Result, Committed}`: `Result.Err == nil` means Render and the requested flush completed, including a successful no-op; `Committed` is true only when Git created a new commit. The queue drain holds one `runMu` interval across Render and a shared locked flush helper, acquires the per-queue shared commit section for that same interval, and invokes the existing hook only after both guards release. Git subprocesses on this path receive the caller context; all paths taken before an error or cancellation are restored to the touched set.

`Queue.EnterCommit` and `Queue.WithCommitFence` provide a separate, context-aware writer-preferring shared/exclusive guard. The checker error and fence-acquisition error are returned separately. A plain `Submit` does not acquire the guard by design; production canonical mutations must be explicitly routed through `SubmitAndFlush` before activating a checker fence.

Focused tests cover inline publication/hook timing, preservation of unrelated staged and unstaged paths, failed-commit requeue and retry, Git/context cancellation, exclusive-fence fairness/cancellation, checker error separation, and a provider-style plain queue job that holds `runMu` without blocking the exclusive commit fence. Validation passed after releasing `R-core-writer`:

```text
GOCACHE=/Volumes/BuildOffload/.cache/core-writer/gocache \
GOTMPDIR=/Volumes/BuildOffload/.cache/core-writer/tmp \
go test -race ./internal/writer -count=1
ok   github.com/sirerun/serenity/internal/writer  63.188s
```
