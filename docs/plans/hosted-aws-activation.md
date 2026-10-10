# AWS activation dossier - source candidate, not activated

## Exact intended resources and boundaries

Deploy the versioned template in deploy/hosted/authority-stack.json in the existing hosted region. It creates one single-region on-demand DynamoDB table with point-in-time recovery, deletion protection and retained lifecycle; one dedicated versioned S3 witness bucket; one ARM64 256MiB Lambda with 25-second timeout; two IAM-authenticated REST routes; a 14-day log group; and one ECC_NIST_P256 SIGN_VERIFY KMS key. Existing EC2, backup bucket and encryption key are retained.

The instance role receives only the exact runtime API invoke permission and explicit denials for direct journal-table, witness, Lambda and release-signing access. The gateway role alone can mutate the table and witness. Owner invocation uses a separate exact principal and caller ID. Artifact bucket/key/version, code hash, approved executable-to-source mapping, owner IDs, runtime role/session IDs, namespace and lineage are explicit inputs. No credential values belong in a deployment parameter file.

Lambda starts with reserved concurrency ZERO. The generated API allow statements are not an exclusive invocation perimeter: same-account identity permissions could otherwise invoke Lambda directly with a forged event. Generate both policies with render_invoke_policy.py using required --output BASE_POLICY and --version-output VERSION_POLICY. Preserve/read each existing policy. Install BASE_POLICY with lambda put-resource-policy --resource-arn FUNCTION_ARN --policy file://BASE_POLICY; install VERSION_POLICY with --resource-arn FUNCTION_ARN:VERSION --policy file://VERSION_POLICY. The base policy contains denies covering the unqualified function and all qualifiers; the version policy repeats the denies and allows only that exact published version. Read back and compare both complete policies before enabling execution. It denies invocation when SourceArn is absent or differs from the two exact routes, and separately denies a missing/wrong SourceAccount. Only the exact published version is allowed. Account administrators who can replace the policy/code remain trusted.

An AWS CLI supporting put-resource-policy must be qualified before activation. A qualified CLI supporting that documented API is a required activation prerequisite; never substitute allow-only add-permission. The official API workflow is described at https://docs.aws.amazon.com/lambda/latest/dg/permissions-function-services.html. Full policies replace old policies, so preserve and compare the complete document rather than merging blindly.

## Activation sequence and stop conditions

1. Produce reviewed exact-source ARM64 binaries and deterministic Lambda archive. Render immutable version/deployment logical IDs from the actual archive hash. Verify artifact storage and deployment principal inputs.
2. Obtain owner approval for exact parameter/policy digests, charged resources, initial test budget and legacy-data disposition.
3. Provision with concurrency zero, install exclusive invocation policy, retrieve it and compare the exact normalized document. Do not activate if the CLI/API or required policy semantics are unavailable.
4. Enable a bounded qualification window. Prove same-account direct Lambda invocation is IAM-denied, alternate routes/stages/principals are denied, and only approved API identities reach protocol validation. Concurrency, throttles and DynamoDB request caps reduce exposure but are not hard billing caps.
5. Qualify fencing, lost-response recovery, restart, renewal, witness failure, old-table replay, backup restore and exact release rollback with bounded synthetic owner data. Stop and return concurrency to zero on any failure.
6. Install root-owned broker/operator/release policy files and separately bootstrapped verifier. Preserve previous binary/config and backups. Approve the exact signed manifest digest, sequence and rollback binding.
7. Only then deploy the verified application and perform authenticated owner-memory and cross-account-denial acceptance. Record source, local, provider and production evidence separately.

## Existing data and recovery

Fresh enrollment is not migration. RequireEmptyData persists until first successful activation; the runtime and privileged broker must independently refuse nonempty data for that admission. Failed preparation/close cannot erase this requirement. Existing legacy production data therefore needs a separately reviewed adoption/recovery decision; this implementation cannot invent missing deletion history.

Owner Inspect and exact Close reconcile lost reservation/lease responses. Disaster table restore imports the independently anchored witness history into a new empty table and compare-and-swaps the exact old TIP; concurrent old-table progress causes failure, never stale acceptance. Stop/drain the source first or establish it is unavailable. This is not a zero-downtime migration.

Snapshot restore verifies the authoritative finalized manifest and complete cut, then creates frozen pending-reactivation data. It does not itself approve account/billing reconciliation or satisfy the distinct recovery-admission contract.

## Backup credential isolation and transport

The uploader runs as trusted root with its environment in a separate root-owned directory. Snapshot requests and archive serialization run as the service UID in transient systemd units denied all IP networking and restricted to AF_UNIX. Privileged AWS never traverses application-controlled snapshot paths: it uploads only the root-owned archive produced by the unprivileged stream. Archives are bounded to 2GiB and serialization to 120 seconds. Failure writes no completion marker.

Remote layout is snapshots/<UTC>-<archive-hash-prefix>/snapshot.tar and COMPLETE.json. Old directory-layout backups remain untouched. COMPLETE is a transport receipt, not journal authority. Download into an operator-private directory, verify its declared archive hash/size, and run unpack_snapshot.py as an unprivileged operator. It refuses links, special files, traversal, duplicate paths, excessive inventory and replacement. Then the operator restore command validates the independent cut before producing frozen data.

## Cost worksheet and approval boundary

The original draft did not authorize spend; the approved activation envelope below now governs execution. The KMS key has a published one-dollar monthly key charge plus requests; API Gateway REST has a published first-tier $3.50 per million calls; Lambda also charges requests and duration; DynamoDB charges transactional request units, storage and PITR; S3 charges witness/backup storage and requests; CloudWatch charges ingestion/storage. Existing EC2/EBS cost continues.

Use actual region-specific AWS Pricing API rates and measured call counts for the approval estimate. Do not multiply logical deletions by one request: transactions contain several item writes, witness publication uses multiple S3 operations, and bounded history traversal reads many objects. History has explicit cumulative limits and cannot be silently truncated; future compaction requires its own reviewed protocol. A low-traffic forecast is not a spend cap. Initial provider qualification must have a call-count/deadline limit and an owner-approved dollar reservation before activation.

Pricing references: https://aws.amazon.com/kms/pricing/ ; https://aws.amazon.com/apigateway/pricing/ ; https://aws.amazon.com/lambda/pricing/ ; https://aws.amazon.com/dynamodb/pricing/ ; https://aws.amazon.com/s3/pricing/


### Region-specific price evidence and bounded forecast

Read-only AWS Pricing API queries on 2026-10-10 returned these first-tier Oregon
rates, excluding free-tier offsets: KMS key $1/month and P256-class asymmetric
requests $0.15/10,000; REST API $3.50/million calls; ARM Lambda $0.20/million
requests and $0.0000133334/GB-second; DynamoDB $0.625/million write request units
and $0.125/million read request units, $0.25/GB-month standard storage and
$0.20/GB-month PITR; S3 standard $0.023/GB-month, $0.005/1,000 PUT/LIST and
$0.004/10,000 GET. Transactional item size multiplies request units.

A planning scenario of 100,000 API/Lambda calls at 200ms and 256MiB, one million
each DynamoDB write/read units, 1GB table+PITR, 20,000 S3 writes, 500,000 S3 reads,
10GB S3 storage and 100 signatures totals approximately $3.17/month before logs,
transfer, taxes and the existing host. Reserve an additional allowance for those
items; this is a workload assumption, not measured demand or a hard budget.

Proposed approval envelope: up to $10 for a bounded initial qualification window,
then an explicitly approved ongoing budget after measured request/duration/storage
receipts. Do not activate merely because the estimate is small. Provider tests
must count backend calls and stop at their assigned deadline. No reservation has
yet been charged or assumed approved.

## Cross-generation recovery and existing data

Deployment failure stops both hosted service and broker under either signature
policy. The first broker-enabled upgrade must never restart an older binary
under newly installed units. Automatic binary-only rollback is unsupported;
qualification must demonstrate recovery of an approved compatible binary,
service units, broker configuration, authority state and data as one versioned
set. A release pending receipt alone cannot authorize runtime or data rollback.

Read-only production inventory found existing accounts and ready brains.
Fresh enrollment cannot adopt that directory. Before live activation, prepare
an owner-reviewed migration checkpoint: quiesce the old writer, preserve a
verified immutable full backup and inventory, establish the provenance and
completeness of existing deletion history, and bind the exact preserved data
to an explicit migration authority decision. If that history cannot be proven,
do not fabricate a clean journal or claim old privacy obligations are satisfied.
The owner must choose a reviewed migration policy or retain the existing host
while a separate fresh environment is qualified. This describes the original
unexecuted draft; the approved preserved migration below now authorizes its
necessary quiescence and explicit adoption. Data deletion and implicit adoption
remain forbidden.

## Approved activation envelope and legacy checkpoint

The founder approved deployment on 2026-10-10. Initial qualification is bounded
by the proposed $10 incremental allowance, a 24-hour execution window, at most
5,000 gateway requests and at most 200 direct provider qualification calls.
These are coordinator stop conditions, not a provider-enforced billing cap.
No paid runners or model fallback are authorized. Reserved concurrency remains
zero until the exclusive policy is installed and read back; then one bounded
qualification lane is enabled. On failed qualification, disable concurrency,
keep the old service active and report retained-resource carrying costs.
Production activation may retain the one stack for the existing service.

Existing data has no predecessor deletion journal. A stopped raw checkpoint
preserves all legacy bytes, prior binary, unit and configuration independently
of new journal snapshots. It is uploaded as a versioned private object with
verified digest before application writes. An isolated operational tree is
staged under the existing data mount. The verified-empty ext4 lost+found
maintenance directory alone is omitted from staging and retained in the root
checkpoint. UID, GID, mode, path, type, size and file digests bind the complete
staged inventory; symlinks and unexpected special/hardlinked files fail closed.
The immutable owner adoption receipt identifies the checkpoint, staged
inventory and prior build with origin owner-legacy-checkpoint-v1. It establishes
history only from that checkpoint. The measured root broker recomputes the
inventory and authority compares it before first activation. Failed prepare or
close cannot consume that requirement. Old snapshots are never relabeled as
new authorized cuts.

Current schema migration prevents a binary-only rollback. Before any new
acknowledged mutation, recovery can restore the coherent legacy checkpoint;
after current writes or deletions, stop and fix forward or use a qualified new
authority cut. A release receipt never authorizes application-data rollback.

Owner AWS credentials remain on the owner machine. Current-user owner CLI
requires private owned configuration and an explicit credential profile.
Restore operator remains root-only. Canonical release signing uses owner AWS
CLI KMS Sign and root-installed offline pinned verification; no credential
transfer or root-policy relaxation is needed.
