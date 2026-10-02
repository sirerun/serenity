# Supplemental review evidence and correction

This supplement adds the original hosted Git migration regression proof and supersedes the earlier report's mistaken statement that `/Users/dndungu/Code/sirerun/serenity/ajent.social` was absent. The original report and logs remain unchanged and immutable.

## Original migration baseline RED

In this isolated review clone, I replaced only `internal/hosted/pool/pool.go` with its exact contents from original main baseline `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`. I retained current regression tests and ran these four controls together:

`go test -run '^(TestPoolRejectsMissingOrUnsafeGitBeforeSideEffects|TestPoolRefusesPreCanceledOpenBeforeMutation|TestPoolIgnoresInheritedGitDir|TestPoolIgnoresInheritedGitConfig)$' -count=1 ./internal/hosted/pool`

The command exited 1 at fresh one-minute load 2.10. All four controls failed for the intended reasons: missing `.git` was accepted and changed the tree; symlink and regular-file `.git` controls detected tree side effects; pre-canceled cold open mutated the tree; inherited `GIT_DIR` redirected Git; and injected `core.hooksPath` ran the hook. Immutable combined output is `git-migration-e6-baseline-regression-red.log`, SHA-256 `c5d8f7853c0b05e72b9d4ba7c7eed33fd4f0a48a342a9110c9563dc25df3818e`.

After the probe, `pool.go` was restored byte-for-byte to reviewed HEAD, SHA-256 `9076feac5f2ef387a85d768654342b71c69374893e5876a75697d0c3ea616b93`; both `pool.go` and `pool_test.go` have no tracked diff from HEAD.

## Coordination feed correction

Ajent MCP tooling remains unavailable. The project-root feed exists and was read read-only at `/Users/dndungu/Code/sirerun/serenity/ajent.social`; the latest pool update records the banked warm-cancellation correction and the prior review-isolation correction. It contains no pool-specific hold/lift-hold claim to act on. I cross-checked the local Serenity coordination board's latest entry, which independently records the exact correction and the pending dedicated final review. Other unrelated holds in the feed were not treated as applying to this source.

## Verdict

The exact source verdict remains CLEAR for local review of commit `2296a9582623b749841626358d0308c118e8b56d`. This is a source-slice review only. It does not accept full T23.45/T23.46/T23.48 or qualify live/provider behavior, integration, deployment, or activation.
