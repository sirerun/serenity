# T23.48 service, gateway, and CLI follow-up review

This follow-up preserves the initial hold receipt at `d98b4037c4531d96cfe8a8a315df6412f1b3b167` and records the corrected review state. Reviewed production source is pinned at `2f3a8205afddde74e79920581f5d0883c77c8f11`.

The previous service and gateway test blockers are resolved in this source. The service now propagates startup context through provisioning recovery, checks cancellation before publishing handlers, and refuses to replay a SQL account in `deleting` state unless complete verified journal history contains its requested intent. The query also reports `Rows.Close` errors. `hosted serve`, `hosted backup`, and `hosted restore` return typed `ErrStartupUnavailable` before reading configuration, opening a store, taking ownership, creating a socket, or reading/writing snapshots. The former restore route no longer bypasses journal admission.

Gateway tree validation now uses a bounded request-context `gitrun` invocation with local config and include processing disabled. Its config parser follows Git's `key\nvalue\0` output format and checks every `core.worktree` value. The duplicate-value fixture initializes a valid repository; malformed config is rejected.

Independent checks on the pinned source:

- `go test -race ./internal/hosted/service -run '^(TestAssembleCancellationAfterJournalAdmissionDoesNotRecoverAllocatingBrain|TestAssembleFailsClosedWhenDeletingAccountHasNoJournalIntent|TestRecoveryReplaysIntentBeforeSnapshotWatermark|TestRecoveryRejectsUnsealedOrMismatchedHistory|TestRestorePendingPreflightRequiresAdmittedReplay)$' -count=1` passed.
- `go test ./internal/cli -run '^TestHosted(SnapshotCommandsRequireAdmissionBeforeIO|ServeRequiresAdmissionBeforeConfigIO)$' -count=1` passed.
- `go test -race ./internal/hosted/gateway -run '^TestValidateBrainTreeRejects(AnyExternalCoreWorktreeValue|MalformedGitConfig)$' -count=1` passed.
- The full Gateway package race test passed on `9a8c7083db00865507b692faa7564aceabffd0dd`, whose Gateway implementation is unchanged at the final source pin. The service package race and CLI package tests passed on parent `8250d153e0317c49fd1feeec4655699b32caaec2` before the final `Rows.Close` handling and fail-closed `serve` boundary; the final-pinned targeted tests above cover those changes.
- The coordinator reported its final `go test -race ./...`, vet, lint, and Linux ARM64 build green, with the shared build lease released. This is coordinator evidence, not an independent rerun.

This review still does not establish a production `RecoveryAdmission` implementation. No cloud journal, AWS, S3, Stripe, deployed service, or live provider was exercised. Legacy CLI serving and snapshot operations remain intentionally unavailable until a reviewed dependency factory and production admission coordinator exist. The work is not production acceptance or deployment approval.
