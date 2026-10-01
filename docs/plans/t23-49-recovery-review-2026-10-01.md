# T23.49 recovery review against current main — 2026-10-01

## Recommendation

Do not rebase or cherry-pick PR #273 wholesale. Keep it draft. Treat it as three independent bodies of work: T23.49 backup/restore (`internal/hosted/backup/**`), T23.48 deletion-journal (`internal/hosted/deletion/**`), and shared integration (`internal/hosted/service/**`, `internal/hosted/gateway/**`, `internal/cli/hosted.go`, config/deployment). Build a new recovery branch from current main only after owners and dependency gates are resolved. Land and receipt the backup-owned package independently where possible; route shared wiring through the integrator after rebasing it against the current assembly.

## Revisions and task state

SSH fetch in this worktree resolved current main to `42f6dfd8615b193fffb797175daf4b5f5d02dc94`, PR #273 to `f41b88214356d13cc6914738afeaa95c17fef8ea`, and their merge base to `d0502ff1cf9adf45349a1998ca066d5e9eee9814`. The PR is open/draft, with no submitted reviews, review comments, or issue comments returned by GitHub. Main has advanced from the PR base through partner API, deployment, hosted lifecycle, and other changes; the PR is not a current-main candidate.

The current rendered registry still calls T23.47, T23.48, and T23.49 `planned`. T23.49 depends on T23.41; T23.48 depends on both T23.41 and T23.47. The coordinator acceptance record for T23.41 explicitly accepts downstream dispatch at `c61ab91`, while retaining the physical storage condition and requiring separate T23.48 live S3 qualification. Do not treat `ready` or the old dispatch acceptance as implementation acceptance.

## Evidence and blockers: keep these distinct

- **T23.49 source receipt:** PR evidence `docs/launch/evidence/T23.49/result.json` is `PARTIAL`, source-pinned to `95cdfb01…`, local-process evidence, and records 28/28 planned cases passing. Its checks cover manifest corruption, inventories, paths/symlinks, exact content and Git-head round trip, credentials, and private staging. It explicitly says service/CLI integration and production deletion journal remain incomplete and does not qualify power-loss or assembled concurrent-write behavior. The coordinator verification note confirms the backup race/lint checks at that historical source; it is not a receipt for `42f6dfd`.
- **T23.48 source receipt:** PR evidence is `PARTIAL`, verified at `131148da…`. It records deletion/gateway/service race tests, `go test ./...`, scoped lint, and a disposable S3 qualification as passed. Its remaining blockers are unambiguous: T23.47 billing closure is not qualified; T23.54 must provision a dedicated retained journal bucket, least-privilege IAM/policies, and service config; full crash-phase and snapshot/delete/restore sibling-preservation qualification remains. The S3 artifact proves only the isolated test bucket exercise, not production configuration.
- **Remote checks:** GitHub reports 12 check runs for PR head `f41b882`; test and lint jobs show failure conclusions, but their annotations confirm they never started because the account was locked; two crossbuilds were cancelled. These are billing/account execution blocks, not source test failures. Founder-approved local validation on 2026-10-01 is the authorized validation route, so do not make restored paid CI a recovery prerequisite. The historical branch-local passes remain pinned to their old source SHAs; record current local validation against the recovered source before review.
- **T23.47 billing gate:** the merged billing PR is not enough to lift this gate. The current registry leaves T23.47 planned, and the T23.48 receipt says live billing closure remains unqualified. Keep the billing blocker separate from PR #273 CI/account-lock status.

## Current-main integration hazards

1. The PR’s merge base predates the partner API. Current main contains migration 8 for `partners`, `partner_links`, `link_requests`, and `partner_entitlements`, plus partner-bound credentials and account ownership fields. PR #273 does not modify `internal/hosted/store/migrations.go`; preserve current migration 8 exactly. Its old `service.go`/gateway/config assembly must be manually forward-ported and tested against current partner routes and ownership behavior. Do not resolve a conflict by taking the PR version of shared files wholesale.
2. The frozen T23.41 interface assigns the deletion-journal adapter/seam to T23.48 and infra qualification to T23.54. The PR adds `internal/hosted/deletion/**` and changes service/gateway/CLI callers. Keep those shared caller changes out of a T23.49-only cherry-pick. The journal is an independent object-store/hash-chain schema; it is not migration 8 and should not silently consume partner schema ownership.
3. Production `Service.New` refuses non-development assembly without `deletion_journal_bucket` and `deletion_journal_region`; the cited hosted stack and example config do not supply them. Do not route production to the backup bucket: the pinned T23.48 receipt records a broad 30-day current/noncurrent lifecycle there, incompatible with retained journal history. Required IAM must deny delete and delete-version and scope list/read/write to the journal prefix, as the frozen contract requires.
4. PR #273 also changes offline CLI and service backup behavior. T23.49's exclusive implementation scope is `internal/hosted/backup/**` plus its evidence. Treat service/CLI callers as integration requests until an integrator coordinates their ownership and current-main assembly.

## Safe recovery sequence and ownership

1. Keep PR #273 draft and preserve its branch/evidence. Establish which exact tasks the current recovery owners will claim; do not edit claims or registry during code recovery.
2. Start a fresh implementation branch at current main. For T23.49, port only the backup-package commits, in their original dependency order (`a9ce01f`, `3f9a9fb`, `9cd0a2d`, `c680510`, `9815c7a`, `f8fb606`, `d6212f8`, `a6cc775`, `95cdfb0`), reviewing each against current contracts and refreshing all tests/evidence. These commits are source candidates, not guaranteed clean cherry-picks. Do not bring along the deletion-journal commit `2c6238d` or edits to service/gateway/CLI as T23.49 scope.
3. Have the integrator separately port T23.48's journal package and caller wiring after T23.47 billing behavior and current partner-aware service lifecycle have been reviewed. Preserve migration 8; add no database schema change for the S3 journal unless a reviewed interface explicitly requires one.
4. Resolve T23.54's dedicated bucket, retention, conditional-write and delete-denial policy, scoped IAM, and production configuration before claiming operational journal readiness. Then run T23.48's crash and sibling-preservation qualification. Keep the T23.47 live billing-closure receipt as its own dependency artifact.
5. Once the T23.49 port is complete, run the package race command from its task contract (`go test -race -count=1 ./internal/hosted/backup`) and scoped lint; test changed shared caller packages separately under their owner. Then regenerate a source-pinned T23.49 receipt against final current-main-derived code using the founder-approved local validation route; preserve the draft review hold until the concrete code, integration, and infrastructure blockers are resolved.
6. Coordinator reviews and updates the task registry only after dependency revisions, code ownership, CI, deployment gates, and source-pinned evidence are reconciled. No merge or acceptance follows from this recovery report.

## Touched files by lane

- T23.49 source lane: `internal/hosted/backup/**`, `docs/launch/evidence/T23.49/**`; preserve exclusive scope.
- T23.48 source lane: `internal/hosted/deletion/**`, `docs/launch/evidence/T23.48/**`.
- Integrator-only port: `internal/hosted/service/**`, `internal/hosted/gateway/**`, `internal/cli/hosted.go`, hosted runtime config/deployment. These files need explicit coordination against current main and migration 8.
