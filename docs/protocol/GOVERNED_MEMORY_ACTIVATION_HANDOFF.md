# Governed memory hosted activation handoff

Prepared 2026-10-12; no signing, deployment or production mutation performed.
Production implementation: `7773ed132d155638316925f72b79f7db39f0abed` (same production tree as `b1ef2fa195e2e25e268972e72c953aefc723e918`). Final qualified source and Zati adapter pin: `7a5f647a37214c619300b8873974872a5ee30860`, which adds documentation and the official-issuer test fixture without changing production code. The full race suite passed at that exact revision. This documentation-only follow-up does not require another adapter pin.

## Public capability observation

Read-only, unauthenticated standard `curl` probes on 2026-10-12 at approximately 10:20 UTC observed:

| Public route | Response |
| --- | --- |
| `/healthz` | 200 |
| `/.well-known/oauth-authorization-server` | 200; PKCE S256, registration and refresh advertised |
| `/oauth/binding` | 401 |
| `/memory/v1/commands/<synthetic UUID>?project_id=capability-probe&brain_id=<synthetic UUID>` | 404 |

The candidate registers the governed route and returns 401 for an unauthenticated request before interpreting command parameters (`ServeGovernedMemory`). Therefore the public deployment does not currently expose the governed route required by this Zati adapter. A 404 does not identify the running source revision or distinguish an older binary from missing proxy routing. OAuth discovery and binding availability alone do not qualify governed memory.

An initial Python urllib probe was refused by the edge with 403; that response is not evidence about the origin application. No token was submitted, grant created or memory changed by any probe. The last checked-in deployment receipt names source `da5c346b586b82b8a25f2d8f7a2211e6bdbaf573`, release sequence 2; this is historical evidence, not a fresh executable measurement.

## Minimal owner decision

Approve one reviewed incremental activation of this exact candidate on the existing hosted service using its already enrolled owner KMS/root-verifier process. The current implementation request does not grant indiscriminate release signing or production changes. Repository AGENTS instructions supply merge coordination rules, not a blanket deployment grant. Earlier activation plans record separate, task-specific owner authorization.

A new stack, a new signing key and a public GitHub release are not prerequisites. The previous approved upgrade used immutable S3 staging and a reviewed manual SSM transaction. See [that upgrade procedure](../plans/oauth-release-2026-10-11.md) and [its receipt](../plans/oauth-deployment-receipt-2026-10-11.md). Use the actual existing release policy and current state, not a guessed sequence or copied historical approval.

## Reviewable activation checklist

1. Freeze and independently review the integrated candidate and the corresponding Zati profile pin. Retain exact source/tree, build/test evidence, Linux ARM64 archive and executable digests. Existing local tests and synthetic managed-lifecycle fixtures do not qualify production activation.
2. With explicit activation authorization, inspect the existing AWS instance, root release policy/approval/state, authority enrollment and previous executable. Establish exclusive deployment ownership. Confirm current backup health and recoverability before downtime; preserve the existing executable, broker configuration, approval and prior authority versions.
3. Prepare a new exact manifest and independent approval binding the candidate, current previous-state digest and next allowed release sequence. Use only the existing approved KMS signer and root verifier. Stage immutable artifacts and reviewed activation scripts in the existing deployment system; verify archive, signature, manifest, approval and extracted executable before installation.
4. Review the narrow authority change set adding the candidate executable/source and immutable configuration version while retaining previous versions. Follow the existing invocation-concurrency pause and exact policy readback procedure. Do not reset data, journal positions, namespaces, trust roots or principal permissions.
5. Take a fresh backup. Under the existing root release-state lock, reverify, prepare, install the candidate and exact broker build binding, start broker/runtime, verify private readiness and measure the actual running executable, then commit the exact release transaction. Resume and verify the backup timer and a fresh backup.
6. Check the unauthenticated governed route now returns the candidate's 401 refusal, then perform explicitly authorized account-level acceptance: real OAuth setup, governed remember/recall, read-only reconciliation after restart, revocation, MCP continuity, and Zati's same-grant renewal with stable credential reference. Record startup readiness probes separately from governed command provider calls. No successful customer-memory acceptance is claimed until this is observed.

## Failure and rollback boundary

Before release preparation, leave the installed candidate unchanged on failure; reconcile any authority routing/concurrency edits against the reviewed prior configuration before restarting. After preparation, retain the pending release marker and artifacts, stop uncommitted runtime/broker activation, and use only exact root-verifier recovery for the pinned manifest. Never erase pending state, lower sequence state or improvise an older binary restart.

Preserving an older binary/configuration is not proof of compatible rollback. A rollback requires an explicitly approved compatible binary, authority/configuration and data-state assessment, followed by the verifier's permitted monotonic recovery procedure. This candidate's tests do not establish a full production rollback campaign.
