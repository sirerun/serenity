# Hosted journal authority

This package implements the source protocol for the owner-approved DynamoDB
journal authority. It does not provision AWS resources or establish provider
qualification. Runtime uses `Client` through the privileged local broker. AWS
request authentication belongs to the exact REST AWS_IAM Gateway integration;
body fields never select a principal, namespace, table, or enrollment.

`DynamoStore` requires explicit clients and an exact table and partition.
Each logical transition atomically CASes the whole-state revision (covering
phase, epoch, lease, expiry and journal head), conditionally creates canonical
objects and a durable operation receipt, and stores one pending witness. No
subsequent transition passes a pending witness. `S3Witness` creates immutable
content-addressed witnesses, verifies the bytes, then CASes the independent
TIP. DynamoDB acknowledges completion only after that CAS. Unknown provider
outcomes are read back or returned as errors; they are never converted to
successful absence or successful append.

The TIP binds the exact namespace lineage and active table ARN. `Current`
compares that independent tip even if DynamoDB has no pending operation, so an
older restored table cannot regain authority by presenting an old active lease.
Lease expiry is checked using the gateway clock before dispatch and again
before acknowledgment. Expiry is not a provider-side clock or proof that every
in-flight request stopped: durable revision/epoch CAS is the synchronous fence.
A commit delayed beyond expiry may be witnessed for recovery but cannot return
runtime mutation authority. Retrying a durable receipt rechecks the exact live
capability before returning success.

Runtime requests are challenge, prepare, activate, current, renew, close,
append, bounded get/list, cut and finalize_cut. Renew is witnessed, preserves
lease identity/epoch, requires the broker-measured executable and process
lifetime to equal the admitted session, strictly increases expiry within the
configured lifetime, and cannot resurrect an expired or revoked lease. Runtime
append cannot create generation seals. Prepared readers are fixed at the
admission position; the same writer may advance its live head during replay.

Owner requests use a distinct authenticated endpoint and principal. `Enroll`
is the only initial bootstrap; empty journal storage is never permission.
`Seal` closes the exact lease while advancing to the next generation. Historical
`ReadFinalizedCut` and `CutReader` require owner authentication and current
lineage, and intentionally do not depend on the original runtime lease still
being alive. Finalized cuts bind exact manifest SHA256 to the stored authority
cut; a local sidecar is only a reference to that record.

`RestoreAuthority` is disaster recovery into a new empty table. It independently
reads the effective TIP, follows and verifies every ancestor, imports immutable
items and completed receipts with create-only/equal retry, validates canonical
journal history, then CAS-rebinds TIP to the new table and leaves it CLOSED.
Normal measured prepare/activate is still required. The original source must
be owner-drained/closed or unavailable. If the source advances during recovery,
the exact TIP CAS fails and the destination stays unavailable: this is not a
zero-downtime migration or an automatic PITR acceptance path. Preserve a failed
candidate table for owner disposition; do not overwrite or delete it to retry.
An immutable witness written before failed TIP CAS is an unattached candidate,
not acknowledged history; it is preserved and excluded from ancestry.

Transport bodies are bounded before decode; duplicate/unknown JSON fields and
irrelevant action fields are rejected. Journal objects are at most 64KiB, pages
at most 256 keys, and the existing journal observation limits still apply. The
witness history additionally has a 100000-transition/64MiB hard ceiling; all
control operations and renewals count. The authority refuses to acknowledge
normal transitions above 99000 transitions or 63MiB, retaining headroom for an owner restore transition within the supported restore bound. Capacity monitoring and any
future compaction protocol are separate operational requirements.

Tests cover unknown commits, witness failure/retry, stale-table detection,
genesis and nonce replay, stale receipt fencing, renewal expiry/process binding,
historical backup cuts, table restore, strict context rejection, conditional
S3 CAS and DynamoDB transaction shape. Fakes establish source semantics only.
Disposable AWS qualification must still test actual transactions, API Gateway
identity values, IAM exclusions, immutable bucket policies, exact Lambda version
invocation permissions, concurrent stale clients, interrupted restore and
capacity before production activation.
