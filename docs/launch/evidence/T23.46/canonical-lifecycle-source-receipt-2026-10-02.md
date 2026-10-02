# Canonical provisioning and lifecycle gitrun source receipt

Status: source handoff for independent review; not merge or task acceptance.

Source base: `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`.
Branch: `source/canonical-lifecycle-20261002`.
Owned scope: `internal/hosted/provision/canonical.go`, its provision tests,
`internal/hosted/gateway/lifecycle.go`, and the new focused lifecycle test.

## Changes

Canonical provisioning now performs git writes through `gitrun.Brain` and
committed HEAD/show proofs through `gitrun.CanonicalReadOnly`. An interrupted
initialization is retried only when a bounded descriptor read proves the exact
regular-file `HEAD` contents `ref: refs/heads/main\n`, the git directory and
HEAD retain file identity and metadata across the read, and the allowed
`for-each-ref` proof reports no main ref or diagnostics. This refuses malformed
or dangling loose and packed main refs rather than mistaking failed HEAD
resolution for an unborn branch. The commit scopes its index change to
`.gitignore`, preserves other staged files and persistent local writer
identity, and supplies fixed author/committer values only to that commit.

Gateway bundle export now runs `bundle create ... --all` through the owned
brain's `gitrun.Brain` command, preserving the existing authorization, locks,
fencing, cancellation, cleanup, and bundle contents.

## Genuine pre-fix failures

- Provisioning hostile inherited `GIT_DIR`/config failed with `fatal: --local
  can only be used inside a git repository` (exit 1), isolated by
  `TestInterruptedCanonicalInitializationPreservesOtherFiles`.
- Lifecycle export under hostile inherited `GIT_DIR` failed with `fatal: Need
  a repository to create a bundle` (exit 1), isolated by
  `TestExportBundleUsesOwnedGitEnvironmentAndAllRefs` after warming the pool.
- Invalid unborn-HEAD states with malformed loose/packed main refs were
  repaired instead of refused (exit 1), isolated by
  `TestProvisionRejectsInvalidUnbornHeadWithoutRepair`.

Pre-fix logs are retained outside the repository under
`/Volumes/BuildOffload/serenity-canonical-lifecycle-20261002/logs/`:

| Log | SHA-256 |
| --- | --- |
| `pre-fix-provision-hostile-git-env.txt` | `19ffe6462539bc3ca12f6b195c5c7093c58333ccfd4c9ce499db6eb1473a3cce` |
| `pre-fix-lifecycle-hostile-git-env.txt` | `88e0a70926bfb797e772e5b43b2c831be4ccfb93a6241a73c205c861c2e54ab2` |
| `pre-fix-provision-invalid-unborn-head.txt` | `6416cbcfc522eea25ea019843b5d4c00e62c7120b0b5a0056809a186d4499e15` |

## Final qualification

All commands used the SSD Python validation runner, which checked fresh system
load (all observed one-minute loads were below 10) and set task-specific SSD Go
caches/temp with `GOFLAGS=-p=2`. Race checks were single-package only.

| Command | Result | Log SHA-256 |
| --- | --- | --- |
| `go test ./internal/hosted/provision -run 'TestProvisionValidCanonicalReadsIgnoreInheritedGitDir\|TestCanceledCanonicalCommitLeavesAllocatingBrainRetryable\|TestProvisionRejectsInvalidUnbornHeadWithoutRepair\|TestInterruptedCanonicalInitializationPreservesOtherFiles' -count=1` | pass | `073d8347e9475c1b24ea4db0f28b52ce7b71e5d6150b23e9603ec235cd3be717` |
| `go test ./internal/hosted/gateway -run '^TestExportBundle' -count=1` | pass | `500d0f6c3f592fa398d1b931710d7e04ced8b7078b5e60f6d9e03b3773963411` |
| `go test -race ./internal/hosted/provision -count=1` | pass | `22865a433eaf08ed956eabfa99a573d158ec0e103d42f921428d8d978bc01e75` |
| `go test -race ./internal/hosted/gateway -count=1` | pass | `1cbe16e8dcbbc4c6ccfcea7b7b24f811c08a4169a849f7ece4c1d4435ddc6993` |
| `go vet ./internal/hosted/provision` | pass | `34d1de9e6d26c784bbfd2a79947fe1130bb4cc6bf148c0dd3f8cdfbc946e166b` |
| `go vet ./internal/hosted/gateway` | pass | `05ea59c60001fe150f9baf59a550e70f7bf8719a1d6101c6a07ee58de17225c4` |
| `golangci-lint run ./internal/hosted/provision` | 0 issues | `e84972088be5f8a08477033c62ac5863d6d86b1cf9b606238ff0ac3e8668fbbb` |
| `golangci-lint run ./internal/hosted/gateway` | 0 issues | `d3e7f214eaeca4f9e8dac04f1103e9a1fe6770d784a814631b09bca0d28edd2f` |

`git diff --check` passed. No `os/exec` usage remains in either changed
production file. No shared build lease, provider, credential, deployment, or
user-data operation was used. This receipt does not claim whole-module
qualification, adversarial atomic filesystem guarantees, integration, merge,
or full task acceptance.
