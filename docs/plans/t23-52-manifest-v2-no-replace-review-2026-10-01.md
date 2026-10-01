# T23.52 no-replace publication review — pinned supplement

Reviewed exact coordinator source `8a03db12b16d8f4ab91dcfd546d22f16c9f78a3c` in isolated full clone `/Volumes/BuildOffload/worktrees/serenity-manifest-v2-review-8a03db-20261001` (external SSD, `--no-hardlinks`; repacked reachable objects and removed inherited alternates). The source was clean before review controls.

## Result

The concurrent-destination publication hold from the prior receipt is resolved in this source. Both Create and Restore now call `renameNoReplace`: Darwin uses `renameatx_np(RENAME_EXCL)`, Linux uses `renameat2(RENAME_NOREPLACE)`, and unsupported platforms fail closed with `ErrAtomicPublicationUnavailable`. There is no ordinary rename fallback.

The publication regression verifies that a competing empty destination keeps its inode and contents and the staged directory remains intact, and that a complete stage publishes successfully. The Darwin flag-removal control made the competing-destination test fail because ordinary `renameatx_np(..., 0)` replaced the destination, demonstrating that the test detects loss of the atomic exclusion flag.

The exact JSON-schema parser correction is included in this source and its earlier alias, unknown-field, duplicate-key, trailing-document, size, and nesting tests passed in the independent package race run. This is bounded backup-library clearance for this pinned source; it is not full T23.52 or production acceptance.

## Validation

- `go test -race -p 1 ./internal/hosted/backup` — PASS, 20.191s, including publication, manifest, checksum, restore, and no-overwrite tests. Immediately beforehand load averages were 6.94/8.75/7.86; external Go caches and temp paths were used.
- Darwin atomic-flag removal control — RED as intended: changing `RENAME_EXCL` to `0` caused `TestAtomicPublicationPreservesCompetingEmptyDestination` to fail because the competing destination was replaced. The original flag was restored.
- `GOOS=linux GOARCH=amd64 go test -c -p 1 -o /Volumes/BuildOffload/serenity-e24-validation-20261001/tmp/backup-linux-amd64.test ./internal/hosted/backup` — PASS (compile only; Linux syscall was not executed here). Pre-command load averages were 8.13/8.91/7.97.
- `gofmt -d` on the backup production/test files returned no differences; `git diff --check` passed. Temporary mutation was restored and tree was clean before writing this receipt.
- No cloud/provider calls were made.

## Scope limits

No live Linux syscall qualification was performed. No production adapters, IAM/S3, scheduler/uploader integration, production restore acceptance, or complete T23.52 qualification is claimed. The coordinator owns the full assembly gate.
