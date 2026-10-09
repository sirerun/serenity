# Hosted OAuth binding recovery

Port the recovered token-bound account/project contract to current main. Reuse current MCP authentication, retain rate limiting, and disclose no token. The unfinished capabilities route is excluded: no handler or qualified capability contract existed. No deployment or complete Zatiti memory qualification is included.

## E1 - Recover and land binding contract
fidelity: executable
Acceptance: Authenticated binding metadata and rejection tests pass on independently reviewed and landed source.

- [x] T1.0 Reconcile recovered source and current ownership Owner: binding-coordinator kind: agent stage: preflight acc: [current main and isolated checkout are recorded and scope excludes unsupported capabilities]
- [x] T1.1 Implement shared verification and binding contract Owner: binding-coordinator kind: agent stage: implement blocked-by: [T1.0] acc: [GET binding returns only authenticated metadata and rejects invalid authority]
- [x] T1.2 Verify endpoint and regression checks Owner: binding-coordinator kind: agent stage: verify blocked-by: [T1.1] acc: [API positive and negative tests, contained-read race regression, full race tests, vet and lint pass]
- [ ] T1.3 Independently review candidate Owner: binding-reviewer kind: agent stage: review blocked-by: [T1.2] acc: [independent exact base and head review has no unresolved blocking findings]
- [ ] T1.4 Rebase merge reviewed candidate Owner: binding-coordinator kind: agent stage: merge blocked-by: [T1.3] acc: [reviewed head merges without overriding protection or holds]
- [ ] T1.5 Verify landed source Owner: binding-coordinator kind: agent stage: verify-landed blocked-by: [T1.4] acc: [landed tree and endpoint acceptance match approved candidate]

## Batch and evidence

| Batch | Members | PR boundary | Dependencies |
| --- | --- | --- | --- |
| B1 | T1.0 through T1.5 | Binding contract, implementation and tests | Current main |

Base: 8262cbac122deecefa1141cee9ec87130455ed59. Original recovered draft: b4febdf7bbc0d3c33f9939c79099dc64cce89e84. A passing historical test preceded an uncompilable capabilities edit. Recovery originals are preserved; capabilities remain separate. Binding is an authorization snapshot, not a lease, release attestation or memory operation status.

Candidate and landed evidence will be attached to the PR and coordination handoff; unchecked delivery stages remain pending until those receipts exist.

Linux preflight found a pre-existing unused syscall import in admintransport/peer_linux.go on the base revision. Removing that import is included to restore Linux compilation and enable verification; peer credential behavior is unchanged.

Full verification also exposed an existing contained-read race: EvalSymlinks can return readlink EINVAL if a symlink becomes a regular file. Classify only that raced readlink as a safe skip; retain os.Root containment and all other errors. The existing adversarial test provides the regression check. Git test fixtures require the installed system templates rather than the missing user template directory.

The existing recovery lock-replacement fixture removed an inode before recreation, allowing immediate Linux inode reuse. Retain the original inode and assert the replacement differs, so the test exercises its documented device/inode identity contract deterministically. Production recovery behavior is unchanged; this does not claim detection of same-identity recreation or close production qualification.

2026-10-09 candidate checks: full Go build, full race suite, vet and golangci-lint pass on Linux ARM64 with Go 1.26.5. Binding-route omission and original contained-read overlay fail their regression tests; corrected contained-read passes three repetitions. Wazi reader/schema/semantic validation passes contract 0.0.1. Review, merge and landing remain pending below this source record and are finalized in the PR handoff.
