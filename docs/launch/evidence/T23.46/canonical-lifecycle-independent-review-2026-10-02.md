# Serenity canonical/lifecycle independent source review

Verdict: **PASS for the exact source SHA below; no blocking finding in the assigned files.** This is a bounded source review and does not claim integration or whole-task acceptance.

- Exact reviewed source: `f6733a15998b13cf58b46ba79229a025a00e6bf5`
- Parent: `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`
- Review clone: `/Volumes/BuildOffload/worktrees/serenity-canonical-lifecycle-independent-review-20261002`
- Review branch: `review/canonical-lifecycle-20261002-independent`
- Final tracked status: clean; exact source files restored byte-for-byte to the source SHA.
- `canonical.go` SHA-256: `89a42e128e8672a779039f0b2549809063447ab1a5808f97a859fd5f6c2d77b4`
- `lifecycle.go` SHA-256: `664196715660db4e9a4922568edb6f9602b2d67c179164b2621f8e041f456cd5`

## Review findings

The canonical path uses the current `gitrun.Brain` writer and `CanonicalReadOnly` proof API. Committed repositories are checked through `rev-parse` and `show`; interrupted initialization accepts only a bounded descriptor read of the exact regular `.git/HEAD` symbolic target, rechecks same-file identity and size/mtime, then checks main-ref absence and captures command diagnostics. The refusal path precedes identity/config writes, `.gitignore`, index staging, and commit. Tests cover malformed and dangling loose/packed main refs, detached/corrupt/non-main HEAD, symlink HEAD and non-directory `.git`, with allocating-state and filesystem preservation assertions. The valid retry preserves unrelated staged content, excludes it from the baseline commit, fixes all four author/committer identity values while preserving local writer identity, and leaves the commit stable on retry. Cancellation leaves the brain allocating and retryable.

Lifecycle export remains within account ownership checks, maintenance/account/runtime locks, fencing and pool acquisition, runtime flush, caller context, stderr-bearing errors, and temporary cleanup. Bundle creation remains `--all`; tests exercise hostile inherited `GIT_DIR`, denied account access, child failure and cancellation cleanup. ZIP/history redesign is outside this reviewed change.

No provider, credential, deployment, spend, or user-data operation was used. No shared multi-package build lease was needed. Ajent MCP tooling was unavailable in this environment; the repository-local `ajent.social` feed was read at session start and its holds were preserved.

## Independent validation

All Go commands were single-package commands, with fresh uptime checks below the limit and SSD cache/temp paths. Full-module race was not run.

| Check | Result | Log SHA-256 |
| --- | --- | --- |
| Focused provision regressions on reviewed source | PASS | `a5787e1c954c2cb696a7b1d1b436e872c51d8ee022bad9e9b4d0a333b85b2ea9` |
| Provision race | PASS | `de9530628315db889a551a9eedd1b38bd78db89bfce284a9fa5fcbed602abc8c` |
| Gateway race | PASS | `9745bce431b17ba4e5360182e96d44ddfa2b54df025ba675e31ec43b2fc1ea98` |
| Provision vet | PASS | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty output) |
| Gateway vet | PASS | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty output) |
| Provision lint | 0 issues | `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47` |
| Gateway lint | 0 issues | `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47` |
| `git diff --check` | PASS | — |

## Genuine RED controls

| Mutation | Observed failure | Log SHA-256 |
| --- | --- | --- |
| Provision production source reset to base `e6dc3f66` while retaining new tests | New tests failed for hostile inherited Git environment, non-main/detached/corrupt refs repaired or config changed | `ca70174ef157a34ee5ed8f56018839bff2cc616691dfe802a7ffd01a48e85cd9` |
| Lifecycle production source reset to base `e6dc3f66` while retaining new tests | `TestExportBundleUsesOwnedGitEnvironmentAndAllRefs` failed on inherited `GIT_DIR`: “Need a repository to create a bundle” | `135476722026df2bd2e14c9f5e7a4781abb0a979cde03bc0b5d1d001d2429063` |
| Temporarily removed only `for-each-ref` main-ref absence proof | `TestProvisionRejectsInvalidUnbornHeadWithoutRepair` failed because malformed loose and packed refs created `.gitignore` | `ec22fbcfe6d5349c1aeb90e8783e75a4b58cd9fe81ee2b8b273ff99271f0a14d` |

The original production bytes were restored and verified after each mutation. Logs are stored under `/Volumes/BuildOffload/serenity-canonical-review-20261002/logs/`.
