# Independent review of corrected SEC-H04 candidate

## Reviewed source and scope

- Dedicated review clone: `/Volumes/BuildOffload/worktrees/serenity-gitconnector-contained-independent-review-20261003`
- Reviewed branch/head: `review/gitconnector-root-pin-20261003` / `3ad35797983186133d688e23b449bcdb531977d1`
- Compared with held candidate `ce5ce63afe00e5dafdd6b02baafd8a7421b1d440` and baseline `4b2fd8e656e448d712b2c7d9fa2bc28b9d78c469`.
- Source-only review. No provider/live/credential/deployment actions or claim changes.

## Disposition

**HOLD pending root-close error propagation.** The corrected candidate addresses both findings from the first review. The only remaining issue found is that `Poll` explicitly discards the `os.Root.Close` error at `internal/connector/gitrepo/gitrepo.go:149-152`. The previous per-read root helper propagated close errors, and the assignment specifically asked to verify close-error handling. A read-only directory handle has no pending writes, but suppressing a close error can hide a resource/descriptor failure. Coordinator is preparing a change to propagate this cleanup error without masking an earlier Poll error; final review remains pending that exact head.

## Corrected behavior verified

- Poll opens a non-symlink directory root and verifies the opened root against `Lstat` with `os.SameFile` (`gitrepo.go:285-302`). It retains one root across HEAD/listing and all reads. Each read checks the current path identity against that root before returning bytes (`gitrepo.go:305-316`).
- The opened file descriptor is checked for regular-file mode before reading (`gitrepo.go:352-359`).
- Linux/Darwin use `O_NONBLOCK|O_NOFOLLOW`; other platforms use read-only flags. FIFO/no-follow qualification is therefore limited to Linux/Darwin. No cross-platform runtime qualification is claimed.
- Static symlink/nonregular skip counting remains. Raced leaf symlink and Root escape errors map to existing skip sentinels; `containedEscapeError` derives Root's unexported escape identity using a guaranteed refused parent traversal and `errors.Is`, without matching error strings (`gitrepo.go:336-344, 560-564`). Other read errors still abort Poll.
- Go `os.Root` does not prohibit mount points or hard links; those limits remain explicit.

## Independent controls

- `go test -race ./internal/connector/gitrepo -run '^(TestContainedReadNeverReadsSwappedExternalSymlink|TestPollRejectsRepositoryRootReplacementDuringListing)$' -count=10`: passed.
- Mutated only `gitrepo.go` back to held `ce5ce63` while retaining corrected tests, then ran the same two tests once under race detection. Both failed as expected: the leaf race produced uncounted `openat ... path escapes from parent`; the Poll root-replacement fixture returned the outside sentinel as a source item. Restored the corrected source byte-for-byte; SHA256 is `77ac25c0f851bc85119c6711059ac9b2b5d01f986f91fcbe56096591757ef8a1`.
- After restoration, `go test -race ./internal/connector/gitrepo`, `go vet ./internal/connector/gitrepo`, and `golangci-lint run ./internal/connector/gitrepo` passed. `git diff --check 4b2fd8e..HEAD` passed; review clone is clean.
- Go commands used `GOFLAGS=-p=2`, external `GOCACHE=/Volumes/BuildOffload/go-build`, external `GOPATH=/Volumes/BuildOffload/go`, and unique external `GOTMPDIR=/Volumes/BuildOffload/tmp/gitconnector-review-20261003-followup`.

## Evidence hashes

- `corrected-controls-repeat10-review2.log`: `bc2666bf8f471aa1d03d907a930e32eda7cd1b408a40bd94a7cd806b1c50fac8`
- `held-ce5-controls-mutant-review2.log`: `64ac3fe12bf5efec707c8d2f763c32d89270e357be5b5b4afc97335453e67f0b`
- `corrected-package-race-review2.log`: `846a797d166c6ac790f6cca1f6da80110f09d64080281d78e3b2b216b3bd3aad`
- `corrected-package-vet-review2.log`: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty output; exit 0)
- `corrected-package-lint-review2.log`: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`

## Coordination channels

Read the original project feed `/Users/dndungu/Code/sirerun/serenity/ajent.social` and board `/Users/dndungu/.claude/bus/serenity/coordination.md`. Both record the original SEC-H04 hold and no lift for this correction. No Ajent MCP tool was available; coordinator was informed so the required unavailability note can be placed in the authorized PR/issue channel.
