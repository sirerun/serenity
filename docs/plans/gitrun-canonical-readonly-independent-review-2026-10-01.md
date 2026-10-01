# Independent review: canonical read-only Git helper

## Verdict

The reviewed helper enforces the intended object interpretation settings in its constructed Git command, and behavior-level tests now cover replacement-ref and lazy-promisor-fetch isolation. The focused `internal/gitrun` race suite passes. This is evidence for the helper only: it does not qualify the hosted source checker, its callers, or a complete repository-safety boundary.

## Reviewed commits

- `401e673500e8bdb5eff6c8f294e736d10c74577b`: add `CanonicalReadOnly`.
- `840ca93bb766ba455670b078ce6dd04a38ac5670`: test replacement-ref isolation.
- `faf676590a4f81e48c8a74fe1d79249c2a402d96`: test lazy promisor-fetch isolation.
- `1508564babe604bae000f45f1c23641670a7ea01`: isolate hostile fixtures from user global Git configuration.

Review clone: `/Volumes/BuildOffload/worktrees/serenity-gitrun-canonical-independent-review-20261001`, at `1508564babe604bae000f45f1c23641670a7ea01`.

## Source and fixture findings

`CanonicalReadOnly` sets both `GIT_NO_REPLACE_OBJECTS=1` and `GIT_NO_LAZY_FETCH=1`. It also inherits `Foreign` behavior: a read-only subcommand allowlist, ignored global and system Git configuration, disabled hooks, disabled optional index locks, and the shared hardening prefix. Inherited `GIT_*` overrides are scrubbed before these two settings are appended. The added tests use Git operations rather than only inspecting the environment.

The replacement-ref fixture creates a second commit with changed file contents and installs it as the replacement for `HEAD`. It checks that `Foreign` reads the replacement contents, proving the fixture is active, while `CanonicalReadOnly` reads the original committed bytes.

The promisor fixture removes a committed blob from the loose object store, marks the repository as a partial clone with a promisor remote, and points that remote at `httptest.NewServer` on loopback. Ordinary Git is the live control: it fails to retrieve the missing blob after making one or more requests to that local endpoint. The canonical reader also fails to retrieve the absent blob, and the test asserts its call causes no further requests. The fixture makes no external network request.

Both new hostile tests now call `isolateGlobalConfig(t)` before repository setup and control execution. This closes the test-only risk that a real user global `url.*.insteadOf`, protocol policy, commit signing, or hook configuration could change the fixture or redirect its control away from loopback.

## Validation

Ran in the isolated clone with Go cache and temporary files on the external SSD:

`GOCACHE=/Volumes/BuildOffload/tmp/serenity-gitrun-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/serenity-gitrun-review-gotmp go test -race -count=1 ./internal/gitrun`

Result: PASS (`5.344s`). The race run was performed after fetching the global-config isolation correction. No production source was changed for this review.

## Scope limits

This review does not audit the hosted canonical checker’s parsing, repository validation, or caller behavior; those require a separate review of the full checker candidate. It also does not establish that all Git versions deployed by the product honor `GIT_NO_LAZY_FETCH`; the behavior-level fixture establishes this property on the Git version used in this local run. No live provider, hosted production, or external-network qualification was performed.
