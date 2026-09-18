# Shared interfaces — task41 freeze gate

**State: specification to freeze, not implemented APIs.** Task41 owns this file and every schema migration/shared assembly edit. Existing main implementation remains authoritative until a reviewed commit changes it. Sonnet workers43/60 may prepare their independent harnesses now; dependent feature tasks do not start until41 supplies exact compile-tested Go types/SQL and the technical review receipt.

## Required invariants and bounded design work

| Seam | Implementation owner | Required contract / preferred mechanism | Freeze evidence |
|---|---|---|---|
| Operation identity/accounting | 44, schema41 | Durable operation journal, explicit internal ID for every mutation, optional client retry key scoped to account+brain, original quota period, reserved write/input units, canonical source/outcome and terminal accounting. Both counters finalize in one transaction. Canonical committed-but-unfinalized operations reconcile before lease release. Unkeyed independent writes remain distinct. | SQL constraints/indexes, phase state machine, recovery query and named canonical evidence seam; crash cases through every phase |
| Storage admission | 44, review41 | Preserve published physical-byte meaning. Before acknowledging mutation, reserve a proven conservative growth envelope for canonical/Git/index/vector/WAL changes plus operator headroom; serialize account admission. Define which transient/operational bytes are outside customer quota and still bounded globally. Never erase acknowledged data or silently relabel bytes as tokens. | Mathematical/fixture growth bound or a separately approved staged-write design; one-byte headroom and concurrent-brain proof. If neither is established, task44 stays blocked; schema owner cannot invent a hidden overage allowance. |
| Billing truth | 47 | `ReconcileCustomer(ctx, accountID)` fetches the server-owned provider customer, validates prices/ownership, replaces stale subscription/checkout state atomically and returns eligibility without activating frozen accounts. No provider lookup based on a client-supplied customer ID. | Exact result/error types, transaction boundaries, current-window vs historic usage and authoritative grace timestamp rules |
| Billing closure | 47, deletion48 | `CloseBillingAccount(ctx, accountID)` is resumable for deleting accounts, prevents new checkout, expires pending sessions, lists/cancels all owned provider subscriptions including unobserved ones, certifies closure only after reconciliation. Provider ambiguity leaves deletion pending. | Checkout/deletion race ordering, idempotency keys, provider timeout and late-webhook behavior |
| Deletion journal | 48, infrastructure54 | Independent durable append/read adapter, records opaque account/brain IDs and deletion intent/outcome only; no memory/email. Account and brain deletions both covered. Append durability precedes acknowledged delete. Versioned object storage is the preferred existing AWS substrate; avoid a new service/queue. | Exact object/record format, IAM prefixes, ordering and integrity/completeness proof, journal watermark included in snapshots, cost model, retention bound |
| Recovery activation barrier | 50, review41 | Old service must be fenced before activation (stop/revoke old instance access under the approved runbook or prove a durable generation fence). Verify journal completeness through the activation barrier, not only visible highest sequence. Apply later deletions and provider truth, then atomically unfreeze eligible survivors; all old credentials remain revoked. | Concurrent delete/missing-tail/missing-middle/returning-old-node tests; explicit failure if authority cannot be proved |
| Backup manifestv2 | 49 | Version, source/build/schema, created UTC, artifact relative names/length/SHA256, sorted unique brain inventory, canonical bundle heads, journal watermark. COMPLETE binds manifest hash. Hashes detect corruption; they do not authorize untrusted uploaded snapshots. | JSON/schema, legacyv1 refusal/support policy, full inventory reconciliation and staging/atomic-publish behavior |
| Recovery CLI | 50, registration41/57 | `serenity hosted recovery plan` and `... apply`: private paths/config inputs, immutable plan hash, exact source snapshot/journal/provider state, resumable per-account phases. No `activate-all`. New secret values never appear in flags/output. | Exact flags/exit codes and source of recovery authority; supported schema/version pair |
| Telemetry | 53, hooks41/57 | Fixed-cardinality structured event/metric interface with typed operation/outcome/duration/count; opaque tenant identifiers and request content forbidden. Recursive redaction and upstream error sanitization. | Producer/metric/alarm mapping, units, missing-data behavior and bounded sink queue |
| Provider pin | 42 | Provider-neutral key, baseURL/model/version/dimensions and privacy-qualified routing; numeric embeddings handled with cosine-compatible similarity. No silent model/provider fallback into a different embedding space. | Live response/usage encoding verified before final pin; reject corrupt or mismatched vectors; model-change rebuild is an explicit maintenance operation |
| Accounting units | 44 with42 | Product input allowance and provider billed tokens are separately named. Existing cl100k_base can only remain as an explicitly documented product unit; actual provider usage drives cost. Every call including readiness/recovery is accounted in operator costs. | Plans/docs/UI naming consistency, provider usage fixtures and cost reconciliation |
| Registration mode | 46, config41 | `public` or `invite_only`; default policy during qualification restricts new registration to controlled test identities; private allowlist stays outside Git/logs. Paid controls gated independently. | Direct login/account-creation enforcement; config validation; no UI-only access control |
| Fault barriers | Feature44/46/47/48/49/50, harness58 | Named deterministic phase hooks compiled only with `hostedtest`; activated via private inherited control channel in test subprocess. Normal build has no active hook or externally triggerable pause/crash path. | Exact phase names/transport, build-tag test and actual tagged-vs-untagged binary proof |

## File-change handshake

1. Feature task submits `docs/launch/evidence/T23.N/integration-request.md`: desired signature/schema/config, failure semantics, exact shared-file diff and tests. This is a proposal, not worker authority to edit shared files.
2. Integrator41 applies/reviews the needed shared commit (separate scoped commit/PR when necessary) and records its SHA in this file’s frozen interface table. Only integrator allocates migration numbers. Add migrations; do not rewrite applied versions1–3.
3. Feature worker rebases on that commit and proves behavior through its public seam. Integrator wires the feature into the real service promptly, then reruns assembled tests. Task57 closes final assembly; it does not defer all wiring to the end.
4. A changed interface reopens affected dependent receipts. Coordinator regenerates contracts when scope changes, rather than verbally granting a second writer.

## Lock ordering and cancellation

Task41 must document one lock order spanning maintenance, account admission, runtime ownership, mutation and DB transactions. Do not hold the global gateway/pool map lock across disk/provider work. No waiting before the request deadline without cancellation. Export/delete/recovery receive bounded capacity independent of ordinary write allowance. Fault barriers must not conceal a production deadlock by serializing every test.

## Threshold review

Task41’s reviewer freezes the task43 Hit@5 corpus/scoring and task60 workload/latency/throughput targets before live results. The proposed targets are deliberately explicit so reviewers can adjust them once with rationale before execution. A failed live result never authorizes a worker to edit the threshold.

## Freeze receipt (to be filled by task41)

- Source/PR/reviewer: **not yet executed**.
- Exact Go interface and SQL revision: **not yet executed**.
- Storage envelope design approved: **not yet executed**.
- Journal completeness/old-writer fence approved: **not yet executed**.
- Provider/model/accounting-unit contract approved: **not yet executed**.
- Quality/load thresholds frozen: **not yet executed**.

These are real dependencies. This planning PR does not mark task41 accepted.
