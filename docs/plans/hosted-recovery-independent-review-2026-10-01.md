# Independent review: quarantine Git helper and retry-period migration

**Reviewed revisions:** `a820ed4e3a1480b9d71e7c943e2164c0768070e2` (initial Quarantine helper), `58dd112231fec7ec7b49217fbd4c7f2712782596` (period-key lookup and migration9), and coordinator follow-up `91f444d258a31c44e6080ef493e2d2b32622a4b4` (CloneBundle isolation). Review branch is isolated at `91f444d`.

## Verdict

The migration9 change matches the T23.44 period-scoped retry requirement and is a narrowly transactional index change. The first Quarantine implementation did not enforce its built-in network denial when a repository-local `protocol.http.allow=always` was present; I reproduced the request against a listener bound only to `127.0.0.1`. Follow-up `91f444d` closes that concrete path by replacing generic quarantine cloning with `CloneBundle`, creating a private fresh working directory, disabling ancestor repository discovery, and explicitly denying built-in network protocols. Its new fixture targets the original override. This is helper-level local evidence, not hosted restore integration or T23.44 acceptance.

The generic `Quarantine` runner still accepts trusted caller-supplied Git commands and reads local repository configuration. It must not be described as an absolute protocol sandbox for arbitrary/custom transports. The safe restore boundary is the narrower `CloneBundle` function; future callers should use that rather than issuing their own clone through `Quarantine`.

## Git configuration and hook review

At `a820ed4`, Quarantine set `protocol.allow=never` and `protocol.file.allow=always`, but Git resolves protocol-specific policy before the generic fallback. In an owned temporary repository with only local config `protocol.http.allow=always`, the same helper arguments reached `127.0.0.1` at `/repo.git/info/refs?service=git-upload-pack`. Global and system config were disabled. No external request was made. The original test used a non-repository working directory and `example.invalid`, so it missed the repository-local override.

`91f444d` adds per-protocol denies for `http`, `https`, `git`, `ssh`, `ftp`, `ftps`, and `rsync`, keeps `ext` denied, sets `GIT_NO_LAZY_FETCH=1`, and makes generic Quarantine cloning fail unless the private `CloneBundle` path enabled it. `CloneBundle` resolves and Lstat-checks a regular local bundle, requires a nonexistent target, uses fixed `git clone --quiet --` arguments, creates a private temporary working directory, and sets `GIT_CEILING_DIRECTORIES` to prevent local config discovery from an ancestor repository. The follow-up test puts `TMPDIR` inside a repository carrying both a local network policy and an `insteadOf` rewrite, then verifies local bundle cloning still succeeds. An independent symlink-ceiling probe also stopped Git repository discovery at the lexical ceiling (`fatal: not a git repository`), so I did not find a bypass there.

The scope caveat remains: arbitrary commands through `Quarantine(dir)` can still consult that directory's local config, and generic fallback denial alone cannot forbid a custom protocol with a local `protocol.<name>.allow=always`. The API now documents that local config is read and callers must supply trusted arguments; `CloneBundle` avoids that config source. Treat only `CloneBundle` as the network-isolated restore API.

`Brain` behavior remains separate: its flags are only `foreign=false, quarantine=false`, so the new hardening is not applied to owned brain hooks. Existing `TestBrainKeepsPostCommitHook` asserts the post-commit hook still runs; the Foreign runner's hook suppression remains separately covered. The new bundle test also uses a plain-Git control clone to establish that the hostile global hook would run, then checks the quarantined clone does not run it.

The utility is not yet wired into the hosted backup restore path: `internal/hosted/backup/backup.go:171` still directly invokes `exec.CommandContext` for `git clone`. The change therefore supplies a safer helper but does not establish that backup restoration uses it. Keep hosted backup restore integration and qualification as a separate open seam; do not claim it is fixed by this helper commit.

## Retry period and schema migration review

T23.44's amended step requires client-key lookup scope by quota period (`docs/launch/hosted-completion/tasks.json`, T23.44 step 6). The Oct 1 interface amendment at `docs/launch/hosted-completion/interfaces.md:298-300` reserves forward-only migration9 and defines the intended rule: keys are scoped to account, brain, and original quota period; a later period creates a distinct operation; existing migrations remain unchanged. It also explicitly leaves ordinary review and upgrade evidence required. This does not revise the other operation/fencing acceptance criteria or mark T23.44 accepted.

`internal/hosted/operation/ledger.go:79-80` scopes the replay query to `quota_period`. `internal/hosted/store/migrations.go:176-183` drops and recreates only the partial unique index, adding `quota_period` to the key while retaining the nonempty-key/non-released predicate. `store.Open` applies migration9 in the same transaction as the other version steps and rejects versions greater than `SchemaVersion` before schema initialization. The migration does not rewrite or delete operation rows.

`TestRetryKeyIsScopedToQuotaPeriod` (`internal/hosted/operation/ledger_test.go:67-103`) commits a key in September, reserves a distinct October operation with the same key, rejects a duplicate October reservation as in progress, and replays the September operation under the original period. `TestLegacySchemasUpgradeTwiceAndPreserveControlData` (`internal/hosted/store/migrations_test.go:17-46`) now upgrades fixtures for source schemas 1 through 8 twice to version9; assertions retain account, subscription, grace, login-token, usage, and version-8 partner/link data, and assert the operation schema/indexes. `TestFutureSchemaIsRejectedWithoutWriting` rejects schema10 and compares the database file hash.

One narrow fixture gap remains: the legacy upgrade test does not seed an operation row before migration9, so it asserts the operations table/index shape but not preservation of an existing reserved/committed/released operation record. The migration itself changes only an index, so this is not a demonstrated data-loss bug; adding an operation-row fixture would make the preservation claim direct.

The prior schema-v4 paragraph at `interfaces.md:143` still describes the old three-column index without saying that migration9 supersedes it. The new amendment explains the forward change, but labeling line143 as the v4 historical shape and pointing to migration9 would prevent a stale contract reading.

## Verification status and limits

Focused verification passed on the reviewed commit: `go test -race -count=1 ./internal/gitrun -run 'TestQuarantineLocalBundleRestoreIgnoresHooksAndRemoteTransport|TestBrainKeepsPostCommitHook'`, `go test -count=1 ./internal/hosted/operation -run '^TestRetryKeyIsScopedToQuotaPeriod$'`, and `go test -count=1 ./internal/hosted/store -run 'Test(LegacySchemasUpgradeTwiceAndPreserveControlData|FutureSchemaIsRejectedWithoutWriting)$'`.

Both requested mutations reproduced regressions in this isolated worktree and were restored before commit. Removing the per-protocol policy overrides from `gitrun.go` made `TestQuarantineLocalBundleRestoreIgnoresHooksAndRemoteTransport` fail: the hostile `insteadOf` rewrite reached `127.0.0.1:1` and returned connection refused instead of a policy denial. No external traffic was made. Removing `quota_period` from both the replay query and migration9 index made `TestRetryKeyIsScopedToQuotaPeriod` fail because an October retry returned the September operation. `git status` after restoration showed only this report untracked. The independent original override reproduction used a controlled loopback listener; the symlink ceiling probe also ran directly.

No live provider, backup publication, deployment, or hosted lifecycle qualification was performed. Neither these commits nor this report claim T23.44/T23.49 acceptance or overall hosted recovery completion.
