# T23.44 bounded absence-proof design

This is a read-only design receipt for a possible `CanonicalAbsent` result. It does not modify the checker, relax the pinned contract, activate reconciliation, or claim T23.44 acceptance. The current checker correctly returns `Unknown` for missing HEAD evidence.

## Contract and current limit

The pinned `CanonicalVerdict` contract in `internal/hosted/contracts/operations.go` permits `CanonicalAbsent` only when canonical history, uncommitted working-tree state, and touched-queue state have all been read and lack the operation ID. `docs/launch/hosted-completion/interfaces.md` adds the key concurrency rule: the brain fence covers the check and ledger transition; it excludes writers in this process, and the writer records `EnterCanonical` before its first canonical byte. A nonzero `CanonicalEnteredAt` with no surviving source is therefore always `Unknown`; forget/history rewrite cannot turn it into proof of non-application.

`internal/hosted/pool/canonical.go` currently reads only committed `HEAD`. Its source-tree listing and per-source collection have no explicit entry, byte, or elapsed-time ceilings. It cannot yet return `Absent`, and it must not do so from HEAD alone.

## Required routing gate

The queue’s exclusive `commitGate` only excludes work that joins `EnterCommit` or `SubmitAndFlush`. `Queue.drain` in `internal/writer/queue.go` runs ordinary `Submit` jobs under `runMu` without the shared gate. Source-writing compatibility paths still use ordinary submit: `MemoryFact.Remember`’s legacy branch, `MemoryFact.Forget`, `MemoryFact.CancelRemoteOperation`, and `SourceTombstone.Tombstone` in `internal/writer/memoryfact.go` and `internal/writer/tombstone.go`. These paths can write before `MarkTouched`; `runMu` alone does not exclude them from the canonical fence. The hosted remembered-source path uses trusted `SubmitAndFlush`, and hosted `ForgetContext` uses it, but that does not close the other routes.

Before `Absent` is enabled, audit the runtime wiring and move every possible `brain/sources/**` mutation onto the shared commit guard before its first source write, holding it through touched-path recording and publication. Provider/index-only jobs must remain outside that guard. Keep plain queue submission available for non-source work, but classify source-mutating APIs explicitly so an unmarked closure cannot silently write a canonical source. Any caller that cannot be shown to follow this rule leaves the checker permanently `Unknown` for that runtime. Do not wait for `runMu` or invoke `Flush`, `Submit`, providers, or `Runtime.Mutations` inside the exclusive callback.

## Bounded proof candidate

Once the routing gate is independently verified, the checker can use this conservative sequence while `ReconcilePending` holds the exclusive brain fence and before it finalizes the ledger row:

1. Validate the record and capture one canonical `HEAD` object ID using the existing isolated `gitrun.CanonicalReadOnly` runner. Keep the current strict rejection of replacement refs, alternates, symlinked Git metadata/object paths, unsupported config, malformed trees, duplicate records, and mismatched content-addressed source hashes.
2. Scan the committed `brain/sources/**` namespace. A unique matching `memory_fact` with the trusted internal operation ID and a recorded canonical entry is `Landed`. Conflicting, duplicate, malformed, incomplete, or unreadable records are `Unknown`. A matching expiry/cancellation `OperationKey` is a marker, not absence proof, so it is `Unknown` when no fact is present.
3. Only consider absence when `CanonicalEnteredAt` is zero. Obtain a **non-destructive** queue touched-path snapshot with a generation/version. Do not call `takeTouched`, because it clears the retry state. Reject any source-namespace touched path, invalid path, malformed snapshot, or generation change. The queue snapshot must not acquire/wait for `runMu` or block provider work.
4. Read the staged index and working tree under `brain/sources/**`, including tracked changes, staged additions/deletions, untracked files, and ignored files. Use read-only Git plumbing and a no-follow filesystem walk. Reject conflicts, symlinks, special files, invalid source layout, undecodable records, unreadable paths, duplicate paths, and any operation-ID or cancellation marker. For an exact source candidate, validate both metadata and bytes and apply the same record decoder used for committed blobs. A missing path that is still staged or touched is not clean evidence.
5. Re-read `HEAD` and the touched-path generation before returning. A changed head, changed snapshot, cancellation, read error, or exhausted bound returns `Unknown`. Return `CanonicalAbsent` with the captured HEAD as `Ref` only if all three views—HEAD, staged/worktree source namespace, and touched snapshot—were completely read and contain no marker, and no path could still be published.

The queue snapshot API would be a sorted defensive copy plus a monotonic generation under `touchedMu`; it would never remove paths. The checker may ignore touched paths outside `brain/sources/**`, but any path that cannot be safely normalized is `Unknown`. This snapshot is not enough by itself: it is authoritative only after all source-writing jobs participate in the shared gate. The current legacy ordinary-submit source writers fail that prerequisite.

The worktree/index walk should compare and decode current source files, not treat every unrelated edit in the brain as evidence for this operation. Any source candidate that cannot be parsed strictly is `Unknown`. If a later implementation chooses the simpler policy of returning `Unknown` whenever anything under `brain/sources/**` is dirty, that is safe but may defer unrelated operations; it must not turn ignored or malformed data into a clean result.

## Bounds and privacy

Do not use unbounded `Output("ls-tree", ...)` or build a map containing every payload. Stream the listing through a hard byte limit; retain only paths and object IDs; load and discard one bounded blob at a time. A reasonable first implementation budget to validate against the T23.60 workload is at most 32 MiB of tree/index listing, 50,000 source records, 2 MiB per blob, 128 MiB total decoded source bytes, and a five-second context budget. These are proposed fail-closed ceilings, not measured production capacity or a change to product quotas. If any bound is exceeded, return `Unknown`; never truncate and infer absence. The current 1 MiB gateway request-body cap is a useful fact-size input, but payload encoding adds overhead and direct store callers exist, so it does not alone justify the per-blob cap.

Keep output and errors content-free: expose only a captured HEAD for proven absence/landed references and fixed reason classes for `Unknown`. Do not log operation IDs, source paths, fact text, metadata, or Git output. The scan must honor context cancellation; temporary memory must be proportional to the fixed caps, not total repository size.

## Required regressions before wiring

- Matching committed fact plus entered row is `Landed`; the same fact without ledger entry, duplicate facts, mismatched identity/key, or matching expiry marker is `Unknown`.
- No-entry clean HEAD/index/worktree/touched snapshot can prove `Absent`; entered-but-purged, canceled/erased, malformed, unreadable, or unsupported evidence stays `Unknown`.
- Dirty tracked, staged, deleted, untracked, ignored, symlinked, and touched source candidates containing the ID are `Unknown`; unrelated valid source records with no matching marker do not create a false positive.
- A matching plain `Submit` source mutation cannot overlap the exclusive checker after routing; provider/index-only work stays outside the commit guard. Context cancellation, head/snapshot changes, and each size/count/time limit produce `Unknown` without clearing touched paths.
- A runtime-size fixture validates that the selected hard caps fit the intended deployment envelope. If not, adjust only with measured workload evidence; do not silently raise bounds or return partial proof.

This proof remains unavailable until source-mutation routing and the bounded reader are implemented and reviewed. Until then, missing HEAD evidence—including no-entry reservations—must remain `Unknown` and hold capacity for operator review. Entered operations remain `Unknown` after forget/history purge even after this proposal is implemented; no new retained operation IDs or markers are introduced by this design.
