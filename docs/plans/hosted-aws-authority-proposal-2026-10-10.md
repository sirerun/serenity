# AWS hosted startup and release authority proposal

Status: proposed, not an accepted trust policy or production implementation. Founder accepted accountable ownership on 2026-10-10 and requested the AWS design. This document prepares the concrete architecture decision; no resource, key, runtime policy or deployment is activated by publishing it.

## Problem and verified baseline

Release baseline `6ba1fb184db249a347d3b80f8e3995fcbec9eacf` includes PR368 CIMD, PR367 OAuth binding and PR363 explorer. `internal/cli/hosted.go` rejects serve, backup and restore. Service assembly accepts recovery admission, not authenticated ordinary restart. A writer ID and configured generation do not establish production authority.

Read-only AWS inspection found the existing running Graviton EC2 host, an SSM-managed instance profile and versioned backup bucket. The inspected runtime policy grants backup object read/write and encryption-key operations. No Serenity journal authority table or dedicated gateway appeared in the inspected regional resource inventory. This is scoped inventory, not a complete proof of all account principals, object prefixes or mutation rights. No customer objects or secret values were read. Existing AWS account administrators and delegated principals still require inventory before qualification.

GitHub Actions for PR368 failed before job execution due to account billing. The repository reported zero self-hosted runners. The newest published candidate is older source `e98c382` and lacks the Sigstore bundle required by `deploy/hosted/verify-release.sh`. Local tests cannot create GitHub workflow provenance.

## Recommended architecture decision

Keep the existing EC2 application and S3 backup storage. Add a small, separately privileged journal authority service: an IAM-authenticated AWS endpoint, a version-pinned Lambda gateway, and one single-region DynamoDB table. The gateway is the only application principal allowed to mutate that table. The hosted process receives no direct table or journal-bucket write credentials. S3 remains backup/archive storage, not the active transaction authority.

This is a deliberate amendment of the proposed S3-based live journal contract, not a shortcut satisfying that contract unchanged. AWS documents eventual propagation of IAM changes; reading back a new policy cannot by itself prove every old request was denied. The proposed replacement checks the writer epoch in the same transaction as each journal mutation, removing that revocation race. [IAM consistency](https://docs.aws.amazon.com/IAM/latest/UserGuide/troubleshoot.html).

The accountable owner approves the threat boundary: AWS account administrators, deployment approvers, the pinned gateway code and the root-owned runtime broker are trusted. The design protects against stale service processes, replay, crash, network ambiguity and unauthorized ordinary workload credentials. It does not promise protection from an authorized administrator deliberately rewriting the authority table or replacing the gateway. Administrator access, delegation and emergency access must be inventoried, separated from runtime credentials and audited; an account compromise requires quarantine and re-enrollment. This explicitly narrows the earlier packet's administrator-fencing expectation and needs independent security review and owner acceptance.

### Concrete resource and principal separation

| Component | Allowed capability | Explicitly excluded |
| --- | --- | --- |
| Existing hosted process | Memory handlers; bounded journal RPC through local broker | IMDS, table writes, signing, infrastructure changes |
| Root-owned runtime broker | Dedicated scoped IAM endpoint invocation; inspect the exact managed process and binary | Direct table writes, release signing, key/policy administration |
| Pinned authority gateway version | Conditional authority and journal transactions; bounded consistent reads | Deployment approval, role/key administration, arbitrary table deletion |
| Deployment approver | Enroll a specific verified release, instance and namespace; initial genesis authorization | Routine service journal writes |
| Release signer | Sign reviewed release manifests with dedicated asymmetric KMS key | Runtime admission, table mutation, trust-root installation |
| Founder/key administrator | Approve enrollment, policy changes, trust roots and break-glass actions | No claim that this administrator is cryptographically fenced from their own account |

The exact resource IDs and principal inventory belong in a private operator manifest. The public proposal contains no infrastructure identifiers or secrets. IAM endpoint policy must be qualified against real requests; caller-supplied identity fields are never accepted as authentication. Published gateway versions and approval records bind code digest; runtime credentials cannot update function code, aliases, endpoint integration, policies or the table's security settings.

### Runtime identity and startup state machine

A deployment approval record binds deployment ID, table identity, namespace, allowed EC2 identity, exact archive/binary digest and source SHA. The root-owned broker verifies the executable and managed service process before making a scoped AWS-authenticated request. The gateway verifies AWS-supplied identity and the enrolled binding, issues an expiring unpredictable challenge, and atomically consumes the exact attempt and nonce. Namespace, instance and build are checked against enrollment, never trusted merely because a request contains them. No CLI flag, environment variable, unsigned local file or generic SDK default credential chain acts as identity.

This is a trusted-host measurement design, not hardware attestation. EC2 credentials alone do not prove a process or build identity. Before implementation freeze, the broker contract must define process lifetime binding, executable replacement handling, privilege boundary, which IAM-authenticated endpoint exposes the required verified claims, and how those claims bind to the enrolled instance. If the chosen platform cannot prove these bindings, startup remains unavailable; do not fill the gap with a caller assertion.

1. Genesis requires an owner-approved enrollment and single-use first-issuance operation recorded durably. Empty storage never authorizes genesis. Existing application data is not an empty genesis: adopt it only after a separately reviewed baseline and deletion-history migration.
2. Restart atomically changes the current authority state to PREPARING with an incremented epoch, exact operation ID and attempt binding. Every writer mutation conditions on ACTIVE and its exact epoch. An in-flight old transaction either commits before this change and is included in the captured head, or fails afterward.
3. Observe a bounded immutable journal through the captured head using strongly consistent reads. Check complete ancestry, canonical hashes, all issued writer IDs, positive generation, predecessor seal, entry count and bytes. No new writes are allowed in PREPARING.
4. Reserve the exact observed head and activate one new epoch with an atomic conditional update. Attach the issued lease only to the admitted process. Replay from the same frozen observation; do not publish handlers until proof and replay complete. Observation or replay failure leaves startup unavailable and does not silently re-enable the old epoch.
5. Shutdown stops admission, joins workers, then revokes the exact epoch. Ambiguous revoke or append resolves through durable operation records; timeout is not success. Lease timeout/TTL is not a correctness fence. A crashed PREPARING attempt requires an authenticated, idempotent resolver, not a background expiry that reopens writes.

### Atomic journal mutation contract

For each append or seal, use one transaction containing a conditional update of the authority/head item, a create-only canonical journal object, and a create-only durable operation/result record. The authority update condition covers ACTIVE state, exact epoch, exact prior head, namespace and generation. The journal object is bounded to 64 KiB. A replay with the same operation ID and payload resolves to the same result; changed payload is rejected. Journal and operation records are immutable to the application code. Trusted gateway implementation and its role are part of this guarantee; an IAM permission alone cannot enforce the transaction expression.

DynamoDB transactions are atomic within one account/region. They cannot both condition-check and update the same item as separate actions: put the condition on the update. SDK transaction tokens expire after ten minutes, so durable idempotency must use stored operation records. Avoid global tables, TTL, DAX, streams or secondary-index reads as authority. [Transaction contract](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/transaction-apis.html).

An ordinary S3 conditional PUT cannot atomically validate a DynamoDB epoch. Do not retain a direct S3 journal writer behind a separate lease check. Adapter interfaces must expose epoch-bound operations; an arbitrary ObjectStore plus a boolean admission receipt is insufficient.

### Migration, backup and disaster recovery

This design changes the canonical journal, so migration and recovery are separate acceptance requirements. Inventory existing journal prefixes and deletion history without exposing customer contents. Freeze old mutation paths using a reviewed migration procedure; prove stable final head and import exact canonical history plus writer provenance. If the older service has no authoritative journal, a reconciled application/deletion baseline needs an explicit adoption decision. Never invent missing deletion history or call populated storage genesis.

Bind the enrollment to the exact authority table identity. Restored tables receive new identities and remain denied until a recovery authority authenticates complete history and approved successor allocation. A stale table snapshot must not reset writer epochs or resurrect deleted accounts. Retain the existing off-host recovery admission semantics; do not make a snapshot or COMMITTED recovery record into ordinary restart authority.

S3 archives, DynamoDB backups/PITR and application snapshots are separate artifacts. DynamoDB warns backups/exports can capture only part of a recently propagated transaction; restore must reconcile authority, immutable entries and durable results against an independently recorded complete cut. That cross-artifact cut and disaster-recovery witness is an unresolved detailed contract, not supplied by enabling PITR. [Backup caveat](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/transaction-apis.html).

### File-level implementation sequence

| Sequence | Candidate ownership | Required result |
| --- | --- | --- |
| A | New startup authority contract and ADR | Owner-approved threat model, migration and identity/backup-cut contracts; independent freeze |
| B | New authority gateway, transactional adapter and isolated infrastructure template | Real conditional epochs and idempotency; bounded access; no broad service role |
| C | New trusted runtime broker and startup package | Authenticated enrollment/attempt binding; exact admitted writer lease |
| D | Service lifecycle and CLI | Mutually exclusive startup versus recovery paths; join then revoke; operators implemented without widening recovery |
| E | Backup/recovery adapters | Exact authority cut and qualified migration/restore; stale authority restoration refused |
| F | Deploy/release verifier and manifest tooling | Explicit approved signing policy and verified exact artifact before extraction |
| G | Existing-host rollout | Exact release, synthetic canary, real login/explorer, rollback and evidence |

A-D cannot be called production-ready while E is unresolved. Shared source interfaces stay frozen until the detailed contracts pass review. The independent S3 transport-bound patch is valid under the existing observer limit and does not implement or activate this design.

## Billing-independent release signing proposal

Retain GitHub Sigstore verification as the default. Add a separately and explicitly selected owner-KMS-manifest-v1 policy only after owner approval. Never automatically fall back when Sigstore verification fails or a bundle is missing. Use one dedicated regional ECC_NIST_P256 SIGN_VERIFY KMS key with ECDSA_SHA_256, separate from existing encryption keys and all runtime authority. No secret signing key is exported to the DGX or production host.

The owner-approved bootstrap pins the public-key SPKI SHA-256 fingerprint, algorithm, policy version and exact key identity through an independent trusted operator channel. The artifact must not supply its own trust root. Restrict signing to a dedicated operator principal; production has no Sign permission. Public-key rotation and revocation require another recorded review and trust-root update; overlap must be bounded and cannot silently re-trust old artifacts.

Sign exact canonical bytes of a strict release manifest: format/version, repository, tag and immutable commit SHA, source tree, archive basename/size/SHA-256, OS/architecture, Go toolchain and build-recipe digest, reviewed-head and test-receipt digests, policy/key IDs, release sequence, issuance and expiry. Reject duplicate/unknown fields, unsupported algorithms, altered bytes, mismatched archive/source/tag and replay outside an explicit deployment approval. Use a bounded manifest and signature input, offline cryptographic verification with the pinned public key, then checksum/size validation before extraction. Pin the expected manifest digest in the deployment approval rather than accepting any valid signature.

Signing proves the authorized key endorsed those bytes; it does not independently prove the claimed tests ran, the build was hermetic, or GitHub hosted it. Retain exact DGX source, toolchain, build logs and independent review receipts and label this operator-built provenance. The release workflow must never report a hosted success that did not occur.

Rollback uses a separately approved exact prior manifest and compatible data/journal state. An old unsigned installed binary is not automatically admitted under the new policy. Rehearse the actual version pair and data boundary; a symlink switch is not a database rollback. Key compromise stops new releases; signing-policy failure never enables unsigned promotion.

AWS supports offline verification with an exported asymmetric public key. One KMS key has a USD1/month storage floor, plus requests and related charges; this is not a total budget. [Sign API](https://docs.aws.amazon.com/kms/latest/APIReference/API_Sign.html), [KMS pricing](https://aws.amazon.com/kms/pricing/).

## Cost and activation decision

No new spend has been authorized or incurred. Proposed approval is initially for architecture and implementation only; provider activation needs an exact regional resource/principal manifest and cost worksheet. One signing key adds the USD1/month floor. DynamoDB transactional reads/writes, storage/PITR, endpoint requests, Lambda, audit/log retention and archives add usage charges. Existing hosting costs remain separate. Do not use the UpCloud credit as AWS budget authority.

Recommend a USD10/month incremental operating budget if the measured workload and regional worksheet fit it, with bounded qualification calls, gateway request/byte quotas, bounded log retention, consumed-capacity accounting and a stop threshold below the remaining budget. This is a proposed budget, not an AWS-enforced hard cap; billing and alerts can lag. If qualification or the measured production workload cannot fit, stop and present a revised budget before activation. Do not promise unlimited availability within that amount.

## Acceptance gates and falsification tests

- Two simultaneous startups yield one active epoch; retries after ambiguous responses cannot issue two writers.
- A paused old writer resumes after an epoch change and cannot append or seal; a just-committed prior write appears in the captured head.
- Wrong instance/build/namespace, expired challenge, nonce replay, altered enrollment and unsigned caller identity all fail before mutable credentials or handlers exist.
- Mid-transaction, restart, cancel, observation-limit and replay failures preserve durable state and leave publication closed.
- Old table restores, partial backup transaction cuts, missing provenance and expired approvals cannot admit account resurrection or writer reuse.
- S3/remote responses cannot allocate beyond configured per-object and aggregate limits; reads never silently truncate. A 64-KiB S3 transport patch has its own regression and compatibility record.
- All real-crypto negative signing cases fail before extraction, including valid signature with wrong approved manifest, key, tag, commit, archive or expiry.
- Source-pinned Go race/vet/lint/build, public and authenticated browser checks, CIMD/DCR OAuth regression and the inspector 10K/high-water performance gate pass.
- A separate synthetic AWS qualification environment exercises real IAM denies, epoch races, crash/readback and backup/restore. Existing customer data is not a qualification fixture.
- Independent exact-head code/security/evidence review clears the candidate; main matches the reviewed tree; production acceptance records exact binary, readiness, login and owner explorer. Existing holds clear only with their required evidence.

## Decision requested after independent review

Approve or revise the DynamoDB gateway approach, trusted-host/admin boundary, and explicit KMS release policy for implementation. Accept the founder as accountable owner (already assigned). No infrastructure activation follows until the detailed identity and backup-cut contracts, private principal inventory and bounded cost worksheet are reviewed and the founder approves the concrete activation manifest. This proposal is an actionable architecture direction; unresolved contracts are named implementation gates, not claims that production authority already exists.
