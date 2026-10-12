# Governed memory hosted activation receipt

The integration coordinator activated the existing hosted service on 2026-10-12 under the user's autonomous completion request. This was a bounded upgrade using the existing KMS signer, root verifier, S3 staging and SSM transaction; no new stack, signing key, DNS change, billing activation or broader principal permissions were introduced.

| Binding | Value |
| --- | --- |
| Installed source | `972d3692dc1b8fb9284fb9c68c850b14ae28da65` |
| Release | `v0.1.14-governed-memory`, sequence 3 |
| Manifest SHA256 | `cbd7c61ef6529873bec31eb7af151a52dc561c378ee8dc4c3415256945b8fa3a` |
| Archive SHA256 | `3eb6ded8aaafefd1cd56996d8a4d4eaec6825721bad9ffb86483c3305977b097` |
| Running executable SHA256 | `f1b6551028a7a89e6012e5e1576e4af028205c76ee6b2d49de821c552a15f402` |

The Zati adapter retains qualified source pin `7a5f647a37214c619300b8873974872a5ee30860`. Installed source972d adds only test fixtures and documentation relative to that production implementation; production Go behavior is equivalent. The later four-minute initial-token fixture is test-only and is not present in the production binary.

## Independently read activation evidence

The activation command completed successfully at 12:17 UTC. The root verifier checked the exact archive, signature, policy, approval and prior-state pin, then prepared and committed sequence3 under the shared release lock. Private readiness and the actual process executable digest were checked before commit. Previous binary, broker configuration and approval remain preserved.

Read-only postflight at 12:19 UTC confirmed installed/highest sequence3, the exact manifest and executable above, matching broker source/build metadata, no pending release file, and active hosted service, broker and backup timer. The most recent backup at that checkpoint reported success at 12:00 UTC. A later read-only check confirmed the first scheduled post-activation backup succeeded at 13:01:21 UTC (exit status0), with the same running executable and service processes, active broker and timer, installed/highest sequence3 and no pending transaction. This is backup completion evidence, not restore qualification.

The authority update retained previous builds and immutable versions, added the exact new executable/source, and selected version5. The base invocation policy remained unchanged. The exact new-version policy was checked while reserved concurrency was0, then the original concurrency1 was restored. A direct Lambda invocation was explicitly denied.

## Public capability and acceptance boundaries

At 12:18 UTC, unauthenticated public checks returned200 for health, readiness and OAuth discovery, and401 for both governed memory POST and MCP. Before activation the governed route returned404; the route is now exposed and requires authentication.

These checks do not establish a real user's OAuth consent, customer-memory write/recall, native browser flow or end-to-end Zati acceptance against production. The real-controller acceptance with synthetic account/browser/lifecycle and shortened initial token lifetime remains separately recorded in the [integration receipt](GOVERNED_MEMORY_INTEGRATION_RECEIPT.md).

## Future incremental upgrades

Use the existing reviewed release process: reread current state, preserve previous artifacts and fresh backups, construct the exact next-sequence manifest and approval, verify before quiescing, update only approved build/configuration bindings, and hold the root release lock through prepare, readiness, process measurement and commit. Cryptographic approval is an operational exact-artifact gate; this repository does not impose an additional conversational approval ceremony when activation is already within the user's authorization.

## Failure and rollback boundary

Before release preparation, leave the installed candidate unchanged on failure; reconcile any authority routing/concurrency edits against the reviewed prior configuration before restarting. After preparation, retain the pending release marker and artifacts, stop uncommitted runtime/broker activation, and use only exact root-verifier recovery for the pinned manifest. Never erase pending state, lower sequence state or improvise an older binary restart.

Preserving an older binary/configuration is not proof of compatible rollback. A rollback requires an explicitly approved compatible binary, authority/configuration and data-state assessment, followed by the verifier's permitted monotonic recovery procedure. This candidate's tests do not establish a full production rollback campaign.

## Installed-service lifetime

The manifest and approval expiration at 15:55 UTC bounds release verification and installation. Only the separate release-authenticator command reads those windows. Hosted and broker startup use their managed configuration, and authority leases renew against the approved executable/source mapping. The installed service does not stop or lose restart eligibility when that release admission window expires.
