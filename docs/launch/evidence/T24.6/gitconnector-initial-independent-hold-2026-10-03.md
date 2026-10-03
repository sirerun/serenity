# Independent review: SEC-H04 confined Git connector reads

Review clone: `/Volumes/BuildOffload/worktrees/serenity-gitconnector-contained-independent-review-20261003`
Branch/head: `review/gitconnector-contained-20261003` / `ce5ce63afe00e5dafdd6b02baafd8a7421b1d440`
Baseline: `4b2fd8e656e448d712b2c7d9fa2bc28b9d78c469`

## Decision

**HOLD** for this source change. The leaf-swap exfiltration fix is effective in the tested fixture, but two correctness gaps remain: root identity is not pinned across Poll, and raced-in leaf symlinks become generic read errors that abort Poll instead of being counted and skipped. This is source-only review; it does not qualify the connector's broader security posture or acceptance.

## Findings

1. **Root identity race remains.** `readContained` opens a new root by pathname for each file (`internal/connector/gitrepo/gitrepo.go:260-270`), after `Poll` has already enumerated paths. The root itself is not held from repository discovery or compared against a previously captured identity. If an attacker replaces/renames the repository path before a per-file `os.OpenRoot`, that call can anchor to a different directory (including a symlink target); subsequent `EvalSymlinks` checks compare against the currently resolved path and can accept that replacement as the root. The candidate closes the leaf/pathname race inside the selected root, but it does not establish that this is the original repository root. Pin the root once before enumeration and retain it through Poll, or fail closed on a verifiable root identity change; add a root rename/replacement regression.

2. **Raced leaf symlink aborts Poll.** Linux/Darwin `O_NOFOLLOW` makes `OpenFile` fail when a leaf changes to a symlink after `Lstat`/`EvalSymlinks` (`contained_flags_unix.go:10-12`). That error is returned as `gitrepo: confined read ...` without `errEscapes`/`errNonRegular` (`gitrepo.go:290-293`). `Poll` only counts those two sentinels and returns all other errors (`gitrepo.go:181-190`). Thus the hostile entry is safely not read, but it can terminate the crawl and hide later entries. Preserve skip/count/continue for the no-follow race error while keeping unrelated I/O errors fatal; add a Poll-level fixture where a later valid doc is still returned and the raced path is counted.

## Source checks

- Actual bytes are read from the descriptor returned by `os.Root.OpenFile`; the opened descriptor is checked for regular-file mode before `io.ReadAll` (`gitrepo.go:290-307`).
- Existing static symlink/nonregular path checks and sentinel-based skip accounting remain in place.
- Linux and Darwin use `O_NONBLOCK|O_NOFOLLOW`; other platforms use `O_RDONLY` only. FIFO/no-follow qualification is consequently limited to Darwin/Linux; no cross-platform runtime claim is made here.
- File and root close errors are returned instead of silently succeeding. Deferred close order is file then root. No leak was found by source inspection.
- `os.Root` confinement does not itself prohibit mount points or hard links; no such broader claim is made.

## Reproduced evidence

- Candidate package: `go test ./internal/connector/gitrepo` passed.
- Candidate race loop: `go test -race ./internal/connector/gitrepo -run '^TestContainedReadNeverReadsSwappedExternalSymlink$' -count=50` passed.
- Baseline mutation: saved the candidate `gitrepo.go`, replaced only that file with baseline `4b2fd8e` while keeping candidate tests, and ran `go test ./internal/connector/gitrepo -run '^TestContainedReadNeverReadsSwappedExternalSymlink$' -count=50`. It failed with outside fixture bytes at multiple iterations, including early reads. The test log is `baseline-mutant-repeat50.log`; the candidate file was restored byte-for-byte (SHA256 `b47004adb036fd5a502082a4156c3dc2c6b69bd5dd9ce81f43c2fd4181aa7e59`).
- After restoration: `go test -race ./internal/connector/gitrepo`, `go vet ./internal/connector/gitrepo`, and `golangci-lint run ./internal/connector/gitrepo` passed.
- `git diff --check 4b2fd8e..HEAD` passed; review clone worktree is clean.

Go commands used `GOFLAGS=-p=2`, `GOCACHE=/Volumes/BuildOffload/go-build`, `GOPATH=/Volumes/BuildOffload/go`, and a task-unique `GOTMPDIR` on BuildOffload. Only the assigned package was built/tested. No claim mutation was made.

## Artifact hashes

- `baseline-mutant-repeat50.log`: `39b81af4e56bc7c14509c3acb511c48b79f62816ab5c17f356d42cffa8ca0207`
- `candidate-package-race.log`: `40cb8009a5aaa3be2124a97d153ab1185719d4d38b8c5643d6d62949452aa681`
- `candidate-package-vet.log`: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty output; exit 0)
- `candidate-package-lint.log`: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`
