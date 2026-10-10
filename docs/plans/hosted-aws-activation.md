# AWS activation dossier - source candidate, not activated

## Exact intended resources and boundaries

Deploy the versioned template in deploy/hosted/authority-stack.json in the existing hosted region. It creates one single-region on-demand DynamoDB table with point-in-time recovery, deletion protection and retained lifecycle; one dedicated versioned S3 witness bucket; one ARM64 256MiB Lambda with 25-second timeout; two IAM-authenticated REST routes; a 14-day log group; and one ECC_NIST_P256 SIGN_VERIFY KMS key. Existing EC2, backup bucket and encryption key are retained.

The instance role receives only the exact runtime API invoke permission and explicit denials for direct journal-table, witness, Lambda and release-signing access. The gateway role alone can mutate the table and witness. Owner invocation uses a separate exact principal and caller ID. Artifact bucket/key/version, code hash, approved executable-to-source mapping, owner IDs, runtime role/session IDs, namespace and lineage are explicit inputs. No credential values belong in a deployment parameter file.

Lambda starts with reserved concurrency ZERO. The generated API allow statements are not an exclusive invocation perimeter: same-account identity permissions could otherwise invoke Lambda directly with a forged event. Generate the complete deny policy with render_invoke_policy.py, preserve/read existing policy, and install the reviewed full policy before enabling execution. It denies invocation when SourceArn is absent or differs from the two exact routes, and separately denies a missing/wrong SourceAccount. Only the exact published version is allowed. Account administrators who can replace the policy/code remain trusted.

The current installed AWS CLI lacks put-resource-policy. A qualified CLI supporting that documented API is a required activation prerequisite; never substitute allow-only add-permission. No resources were created to test this boundary. The official API workflow is described at https://docs.aws.amazon.com/lambda/latest/dg/permissions-function-services.html. Full policies replace old policies, so preserve and compare the complete document rather than merging blindly.

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

No new spend is authorized by this document. The KMS key has a published one-dollar monthly key charge plus requests; API Gateway REST has a published first-tier $3.50 per million calls; Lambda also charges requests and duration; DynamoDB charges transactional request units, storage and PITR; S3 charges witness/backup storage and requests; CloudWatch charges ingestion/storage. Existing EC2/EBS cost continues.

Use actual region-specific AWS Pricing API rates and measured call counts for the approval estimate. Do not multiply logical deletions by one request: transactions contain several item writes, witness publication uses multiple S3 operations, and bounded history traversal reads many objects. History has explicit cumulative limits and cannot be silently truncated; future compaction requires its own reviewed protocol. A low-traffic forecast is not a spend cap. Initial provider qualification must have a call-count/deadline limit and an owner-approved dollar reservation before activation.

Pricing references: https://aws.amazon.com/kms/pricing/ ; https://aws.amazon.com/apigateway/pricing/ ; https://aws.amazon.com/lambda/pricing/ ; https://aws.amazon.com/dynamodb/pricing/ ; https://aws.amazon.com/s3/pricing/
