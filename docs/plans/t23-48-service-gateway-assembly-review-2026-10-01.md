# T23.48 service and gateway assembly review

Reviewed source: `8a03db12b16d8f4ab91dcfd546d22f16c9f78a3c` in an isolated external-SSD clone. This is a scoped independent review, not T23.48 acceptance or deployment approval. No production source was changed in this review.

## Findings

The recovery assembly in `internal/hosted/service/service.go` follows the frozen history boundary: it validates the injected journal and admission dependency, obtains the admitted watermark, reads through that cut, reads the complete history from the zero cursor, and requires both results to be sealed at the same watermark. It replays the complete history, then passes that same journal object to the gateway and backup path. `internal/hosted/service/deletion.go` keeps `restore_pending` privileged to the startup replay callback; ordinary deletion preflight does not admit that state. Provider closure is rechecked for replayed terminal subjects before local cleanup. Startup errors close the pool and the constructor closes the control store.

Three isolated mutation controls established that the service regressions detect their intended failures:

- Replacing `fullRead.Entries` with `cutRead.Entries` made `TestRecoveryReplaysIntentBeforeSnapshotWatermark` fail because the pre-cut intent left restored files.
- Removing the sealed-history predicates made `TestRecoveryRejectsUnsealedOrMismatchedHistory/unsealed` admit unsafe history.
- Allowing ordinary deletion preflight to pass `admittedReplay=true` made `TestRestorePendingPreflightRequiresAdmittedReplay` fail.

The current pin still has readiness blockers:

- `internal/cli/hosted.go` exposes legacy `hosted backup` and `hosted restore` commands outside the admitted service assembly. `go test ./internal/cli` fails to compile because `backup.Request` now requires build identity and a deletion journal while the CLI supplies only three arguments. More seriously, the direct `backup.Restore` call bypasses `RecoveryAdmission` and journal replay. Keep both commands unavailable or fail-closed until an admission-aware production path exists.
- `AssembleWithDependencies` calls `provisioner.Recover(context.Background())` at `internal/hosted/service/service.go:372`, although `Recover` accepts a context. Startup cancellation therefore does not bound this local recovery step. A separate reviewer owns that context-propagation correction.
- No production `RecoveryAdmission` implementation is present in `internal/hosted`; the injected implementations are test fixtures. The legacy `serve` constructor correctly fails closed, but this source is not an operational production startup path. The filesystem journal is a test adapter, and no cloud journal was exercised.
- The host-clone gate `go test -race ./internal/hosted/service` failed in the stale `TestStartupDeletionClosesBillingBeforePurging/{unavailable,closed}` fixture with “startup did not certify provider closure.” The focused recovery and restore-state tests passed; the fixture failure prevents a package-green claim at this pin.
- `go test -race ./internal/hosted/gateway` failed in the new `TestValidateBrainTreeRejectsAnyExternalCoreWorktreeValue`. Its fixture creates `.git/config` without initializing a valid repository, so `git config --local` exits 1 with no values and the validator treats the key as absent. A manually initialized repository with both external and internal `core.worktree` values emits both NUL-separated values as expected. Correct the fixture to `git init` before relying on this control.

The gateway source at this pin uses request-context `WalkDir` checks and a two-second `gitrun.Brain` query with `--local --no-includes --null --get-all core.worktree`; the duplicate-value fixture needs the correction above. Account deletion still journals intent before preflight/mutation and terminal completion after cleanup. This review found no additional replay-order defect in that path.

## Validation and limits

On the pinned source, the focused command
`go test -race ./internal/hosted/service -run '^(TestRecoveryReplaysIntentBeforeSnapshotWatermark|TestRecoveryRejectsUnsealedOrMismatchedHistory|TestRestorePendingPreflightRequiresAdmittedReplay)$' -count=1`
passed. The full service and gateway package results are recorded above. The CLI compile check failed with the concrete signature mismatch above. Mutation tests were run only in this isolated clone and all changes were restored; `git diff --quiet` confirmed a clean source tree afterward.

The clone and Go caches live on the external SSD. Tests used no AWS, Stripe, S3, cloud journal, or deployment path. This report does not certify production journal admission, live provider closure, physical backup bounds, full repository race/vet/lint, or T23.48/T23.49 acceptance.
