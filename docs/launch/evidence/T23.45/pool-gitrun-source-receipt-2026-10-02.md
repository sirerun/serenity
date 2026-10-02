# T23.45 pool Git-runner source receipt — 2026-10-02

This is a partial source-slice receipt for the pool-owned migration only. It does not claim T23.45 acceptance, fair-admission qualification, or integration acceptance for T23.46/T23.48/T24.30.

## Source scope

Worker clone: `/Volumes/BuildOffload/worktrees/serenity-t23-45-pool-gitrun-20261002`, branch `codex/t23-45-pool-gitrun-20261002`, base `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`.

Changed source/test files:

- `internal/hosted/pool/pool.go`
- `internal/hosted/pool/pool_test.go`
- `internal/hosted/pool/reconciler_test.go` (coordinator authorized narrow canonical-Git test-fixture setup; no production reconciler edits)

The pool now checks cancellation and requires a real, non-symlink `.git` directory before writer acquisition or any pool/config mutation. It no longer initializes Git. Committed HEAD/config tracking proofs use `gitrun.CanonicalReadOnly`; owned add/commit uses `gitrun.Brain`. Baseline commits set only the four fixed author/committer environment entries through the returned command. Existing tracked-config handling, model-pin checks, `--only` staged-path behavior, retry flow, runtime fences and journal/admission implementation remain unchanged.

## Regression evidence

Before the production change, the focused tests failed against the original pool implementation (`GO_TEST_EXIT=1`):

- `TestPoolRejectsMissingOrUnsafeGitBeforeSideEffects/{missing,symlink,regular_file}` observed Acquire succeeding and creating Git/config/runtime files for missing metadata, and nested side effects for symlink/file metadata.
- `TestPoolRefusesPreCanceledOpenBeforeMutation` observed tree mutation on a pre-canceled open.
- `TestPoolIgnoresInheritedGitDir` observed raw Git being redirected to an unrelated external Git directory.
- `TestPoolIgnoresInheritedGitConfig` observed an injected `core.hooksPath` pre-commit hook running.

Raw log: `/Volumes/BuildOffload/validation/serenity-t23-45-pool-20261002/red-focused-r2.log`.

The first post-change full package run exposed six old fixtures that relied on pool auto-init. Those fixtures were explicitly prepared as canonical test repositories; the cold-runtime regression still removes `.git`, proves the fence leaves it absent, then prepares a repository only for its explicit warm Acquire. No production code outside the assigned pool file changed.

## Focused verification

All commands ran from the isolated SSD clone with `GOCACHE`, `GOTMPDIR`, `GOMODCACHE`, and `TMPDIR` under `/Volumes/BuildOffload`. A fresh uptime/load check was below 10 before each Go command.

- `go test -count=1 ./internal/hosted/pool` — PASS.
- `go test -race -count=1 ./internal/hosted/pool` — PASS.
- `go vet ./internal/hosted/pool` — PASS.
- `golangci-lint run ./internal/hosted/pool` — PASS, 0 issues.
- `git diff --check` — PASS.
- `pool.go` raw-process scan for `exec.Command`, `exec.CommandContext`, and literal `git` — no matches.

Logs: `/Volumes/BuildOffload/validation/serenity-t23-45-pool-20261002/focused.log`, `race.log`, `vet.log`, and `lint.log`.

## Limits / handoff

This source slice does not qualify T23.45 acceptance, module-wide scanner coverage, other hosted caller migrations, live/provider behavior, or a merged/integrated revision. The initial focused package run also documented dependent fixture migration; the assigned pool/reconciler tests are now green. Coordinator review and any broader integration remain required.

Task/resource claims held for final qualification: T23.45 `722246d4b56c64419175de009eb7289b33a9132f`; R-hosted-runtime `e70c00949f26ee1976b5a3e9251baf2056d4103a`.

## Coordinator evidence clarification

The first focused package run after the production change failed on six previously auto-initialized fixtures. That failure was observed in tool output, but its file was overwritten by the passing rerun; `focused.log` is the final GREEN log, not an immutable integration RED. The original pre-fix `red-focused-r2.log` remains preserved. This gap does not erase the recorded observation, but no extant author integration-RED file is claimed.

Initial claims were T23.45 `43a23afb63bb7d515971893107ba7c16e50e3716` and R-hosted-runtime `b9ca16ec686d97ecb61a29ef0ad38d75dbd3d178`. Later supported holder checks found no holder, and the worker reacquired T23.45 `722246d4b56c64419175de009eb7289b33a9132f` and R-hosted-runtime `e70c00949f26ee1976b5a3e9251baf2056d4103a`; both second claims were exactly released. The available evidence does not establish why the first pair was absent, so continuous ownership or renewal is not claimed. Unsupported status/show-ref attempts were not valid shared-remote evidence. Coordinator review applies to exact banked source b454ee59, not a claim of uninterrupted lease history.

The remote discrepancy was subsequently resolved: the second pair was acquired/released against the shared build-lease remote, not GitHub. The original authoritative GitHub pair remained held and was verified then exactly CAS-released by the author. The earlier unknown-absence account is superseded by this concrete wrong-remote finding. Coordinator acquired a new authoritative pair for the warm cancellation correction.
