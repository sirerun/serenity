# Serenity OAuth production upgrade

Owner explicitly requested Deploy PR376. Exact source da5c346b586b82b8a25f2d8f7a2211e6bdbaf573, reviewed tree and landed tests verified. Existing deployment sequence1 and source ed8f0b9 preserved. No schema or authority code change. Existing stack, pinned KMS policy and root verifier retained.

## E1 - Signed incremental upgrade
fidelity: executable
Acceptance: Signed sequence2 installed with exact runtime identity, live Claude metadata discriminator corrected, service and backup healthy, no claim of real client acceptance without observation.

- [x] T1.1 Inspect ownership and build exact reviewed ARM artifact Owner: release-coordinator kind: agent stage: preflight acc: [exclusive deployment claim, prior sequence/digest and clean source verified; artifact and receipts hashed]
- [x] T1.2 Review upgrade transaction Owner: release-reviewer kind: agent stage: review blocked-by: [T1.1] acc: [stage.sh activate.sh manifest approval and CloudFormation changes independently clear]
- [x] T1.3 Stage and verify signed candidate Owner: release-coordinator kind: agent stage: implement blocked-by: [T1.2] acc: [S3 artifacts immutable task prefix; root verifier checks existing pinned policy and independent exact approval before downtime]
- [x] T1.4 Qualify authority and activate Owner: release-coordinator kind: agent stage: implement blocked-by: [T1.3] acc: [fresh backup then stop backup timer and runtime; authority concurrency0 during update; retain old version and builds; new published-version exclusive invoke policy exact readback; restore concurrency1; root lock prepare install readiness executable check commit sequence2]
- [x] T1.5 Verify production and record Owner: release-coordinator kind: agent stage: verify-landed blocked-by: [T1.4] acc: [public readiness and Claude/Gumloop safe authorization probes; installed state and executable pins; backup timer active and fresh backup; receipt and cleanup owned claim]

## Execution and recovery

One coordinator performs provider mutations. AWS CLI only, existing profile. No credentials transferred. Independent reviewer inherits session selection under shared dispatch. DGX build, CPU2, isolated source/artifacts. Operator transfer files on external SSD.

Stage signed archive/manifest/signature and independently installed exact approval in a new root-owned directory, keeping existing root trust and deployment state. Verify archive and manifest before extracting its sole regular serenity member; extracted executable must match independently computed digest. Review/test digests are operator attestations, not claims of hosted CI.

Authority change set adds new allowed executable/source while retaining old, publishes an immutable configuration version and API deployment, retaining old versions. No new authority code, tables, keys, principal permissions, or data namespaces. Pause invocation concurrency at0 while switching and installing the exact existing exclusive-deny policy on the new qualified version; verify full policy readback before restoring1. Existing base deny policy is checked unchanged. Inspect cloud resource changes before execution.

Run fresh backup before stopping existing service. Stop timer and runtime during authority update; preserve current binary, broker config and approval. Under existing root release-state lock reverify, prepare, install only new binary and exact broker build/hash, start broker/runtime, check private readiness and actual process executable, then commit. Existing data and journal are never reset or restored. After prepare, failure leaves services stopped and pending candidate intact for exact root-verifier recovery; do not erase pending state or blindly roll back. Old binary/config remain available for separately approved compatible recovery, not an asserted tested rollback campaign. Before preparation any provider failure leaves the old installed source; restore prior compatible API target/concurrency if needed and verify before restarting. Report outage/failure explicitly rather than bypassing trust.

No full Claude/Gumloop grant is needed for the safe discriminator: intentionally unsupported response_type must produce302 unsupported_response_type after successful public metadata resolution rather than500. Browser acceptance is a distinct later check.
