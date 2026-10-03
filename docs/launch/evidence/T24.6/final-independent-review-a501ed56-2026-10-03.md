# Final independent review: SEC-H04 contained Git connector reads

## Disposition

**CLEAR for the reviewed source scope** at `a501ed56f397b2056762d5b1cdfa5684c7aa2e84` (`review/gitconnector-close-check-20261003`). The root pin and read-skip corrections pass independent controls, and Poll now propagates a root-close error when no earlier error exists. This is source-only clearance for the contained-read change; it is not full security, provider, deployment, or launch acceptance.

## Reviewed changes

Compared with the prior corrected head `3ad35797983186133d688e23b449bcdb531977d1`, the only production change is Poll's named return values and root-close handling. If `confined.Close()` fails after an otherwise successful Poll, Poll returns no items, the original input cursor, and a wrapped close error. If a primary Poll error already exists, it remains primary. Existing tests and source behavior otherwise remain unchanged in this final diff.

The corrected root pin opens one non-symlink directory, verifies `os.SameFile` between the path and opened root, keeps that `os.Root` through HEAD/listing/reads, and checks path identity before each read. The descriptor used for `io.ReadAll` is checked for regular-file mode. Linux and Darwin use `O_NONBLOCK|O_NOFOLLOW`; other platforms use `O_RDONLY`, so FIFO/no-follow qualification is limited to Linux/Darwin. Static symlink/nonregular entries retain skip counting. Raced leaf symlinks and Root confinement errors map to the existing skip sentinels using typed error checks, not error-string matching. Mount points and hard links remain outside the `os.Root` guarantee and are not claimed as prevented.

## Independent evidence

On exact final head `a501ed56`:

- `go test -race ./internal/connector/gitrepo`: passed.
- Focused root-replacement and raced-leaf controls with `-race -count=5`: passed.
- `go vet ./internal/connector/gitrepo`: passed.
- `golangci-lint run ./internal/connector/gitrepo`: zero issues.
- `git diff --check 4b2fd8e..HEAD`: passed; review clone is clean.

Preserved prior independent negative-control evidence: mutating only production `gitrepo.go` back to held source `ce5ce63` while retaining the corrected tests made both controls fail. The leaf race reported an uncounted Root escape error, and the fake-Git Poll test returned the outside sentinel as a source item. The candidate source was restored byte-for-byte before final checks. This was a single-run regression proof on each control; coordinator's separate original unchanged-source 50-repeat RED evidence remains preserved.

Go commands used `GOFLAGS=-p=2`, `GOCACHE=/Volumes/BuildOffload/go-build`, `GOPATH=/Volumes/BuildOffload/go`, and task-scoped SSD `GOTMPDIR=/Volumes/BuildOffload/tmp/gitconnector-review-20261003-final`. Only the assigned package was run.

## Evidence artifacts

- `final-package-race-review3.log`: `62f86035f240035d67b6f8c1941eb2b8fe4c3c63789b1fbf6ed9f662bc0a8bd8`
- `final-focused-controls-review3.log`: `481489053dbef813634024a5188cb2c82eed80a5427293e2feaa5ed754aff5e7`
- `final-package-vet-review3.log`: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty output; exit 0)
- `final-package-lint-review3.log`: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`
- Held-`ce5ce63` mutation log from the previous review: `held-ce5-controls-mutant-review2.log`, SHA256 `64ac3fe12bf5efec707c8d2f763c32d89270e357be5b5b4afc97335453e67f0b`.

The project Ajent feed and coordination board were both reread for this final review; they still record the earlier hold and show no lift. No Ajent MCP was available; coordinator has been told to disclose that in the authorized PR/issue channel.
