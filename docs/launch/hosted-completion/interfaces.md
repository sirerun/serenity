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

## Frozen Go interfaces (task41, this pass)

`internal/hosted/contracts/**` now carries compileable, tested Go signatures
for every seam in the table above whose shape does not depend on the four
proposed decisions below: `BillingReconciler`/`ReconcileResult` (billing
truth), `BillingCloser`/`CloseResult` (billing closure), `ManifestV2`/
`BrainArtifact`/`SourceRef` (backup manifest v2), `RecoveryPlan`/
`RecoveryPlanRequest`/`RecoveryApplyRequest`/`RecoveryApplyResult` (recovery
CLI shape — the eligibility rule Apply enforces is still PROPOSED),
`Telemetry`/`TelemetryEvent` (telemetry), `ProviderPin` (provider pin shape;
task42 still verifies and pins the actual value), `AccountingUnit`
(`UnitProductInputToken` vs `UnitProviderBilledToken`) and `RegistrationMode`
(`RegistrationPublic`/`RegistrationInviteOnly`). Each interface has a
compile-time conformance fixture in `contracts_test.go` (a `var _
contracts.X = fakeX{}` assertion per interface) so a future signature change
that breaks a dependent implementation fails `go vet`/`go build`, not just a
prose diff — verified by a real spot-check mutation in this task's evidence.

`internal/hosted/testhooks/**` freezes the fault-barrier transport: named
phase constants (`PhaseOperationReserved`, `PhaseOperationCommitted`,
`PhaseDeletionJournaled`, `PhaseDeletionPurged`,
`PhaseBackupManifestWritten`, `PhaseRestoreFenced`, `PhaseRestoreUnfrozen`)
and a single `testhooks.At(phase)` call site. A binary built without
`-tags hostedtest` links a true no-op (`hook_prod.go`); only a
`hostedtest`-tagged binary started with a harness-inherited control pipe
(never an HTTP endpoint, never a bare env-var switch) can pause or crash at
a named checkpoint (`hook_hostedtest.go`). Proven by five real subprocess
tests in `testhooks_test.go`, including one spot-checked red→green.

`OperationLedger`/`OperationRecord`, `AdmissionChecker`/`GrowthEnvelope` and
`DeletionJournal`/`DeletionEntry` are also drafted in `contracts/` so
dependent code has something to compile against, but they are marked
PROPOSED in source and in the freeze receipt below — see "Proposed
decisions" next. Freezing their *signature* here is not authority to treat
the underlying mechanism as approved.

## Proposed decisions — pending chief-architect review

This task could not obtain live chief-architect review (headless worker
session, no reviewer available synchronously). The four bounded decisions
below are concrete, reviewed-ready proposals, not approvals. No dependent
task may start production implementation against them until the freeze
receipt below records an actual reviewer and revision. See
`docs/launch/evidence/T23.41/architecture-review-request.md` for the
standalone review request.

### 1. Physical storage reservation/headroom (storage admission, task44)

Least settled of the four. Proposal: `AdmissionChecker.ReserveGrowth`
(`internal/hosted/contracts/storage.go`) reserves a conservative
`ReservedBytes = ceil(LogicalBytes * growth_factor) + fixed_overhead` before
acknowledging a mutation, covering Git object overhead, derived index/vector
writes and WAL growth — not just the logical payload. `growth_factor` and
`fixed_overhead` must come from an empirical fixture: task44 runs a corpus of
synthetic writes across realistic size buckets, measures actual on-disk
growth via the same `filepath.WalkDir` technique `gateway.Inventory` already
uses, and derives a constant with an explicit safety margin (e.g. observed
p95 growth ratio times 1.5), checked into a fixture test that fails if any
corpus write's real growth ever exceeds its reserved envelope. Per-account
admission stays serialized through the existing `accountLocks` sharding
(`gateway.go`); a new global `HeadroomBytes` config field reserves a fixed
operator floor that blocks *all* accounts' admission once crossed, regardless
of individual quota headroom — the exact number is an operator input (ties to
the AWS instance's actual disk size, SPEND gate) and is explicitly not set
here. **Not approved**: no mathematical bound or fixture evidence exists yet;
per interfaces.md's own words, task44 stays blocked until one does.

### 2. Crash-safe canonical-operation accounting (task44)

Proposal: a durable `operations` table (new schema migration, not yet
applied — see below) matching `contracts.OperationRecord`: `id`,
`account_id`, `brain_id`, `client_key` (nullable, scoped by a partial unique
index over `(account_id, brain_id, client_key) WHERE phase != 'released'`,
mirroring the existing `reservation_operation` index pattern in
`store/migrations.go`), `metric`, `units`, `quota_period`, `phase`
(`reserved`|`committed`|`released`, forward-only), `source`, `created_at`,
`finalized_at`. `OperationLedger.Finalize` commits both this row and the
`usage_windows` counter in one transaction, generalizing `meter.Meter.Finish`
(`internal/hosted/meter/meter.go`) from an in-memory-lease-scoped reservation
to a row that survives a crash. `ReconcilePending` is the required recovery
query: it scans `phase='reserved' AND lease_expires_at <= now()` and resolves
each by consulting the operation's own canonical evidence seam — task44
defines the exact per-operation-kind check (e.g. for `remember`, whether the
fact is present in the brain's canonical Git HEAD) in its own freeze
evidence; this task only fixes the table shape and the phase state machine.
**Not approved**: no chief-architect sign-off on the table shape or the
"one transaction" mechanism yet.

### 3. Independently durable deletion journal (task48, infra54)

Proposal: versioned S3 objects (existing AWS substrate per ADR014, no new
service), one object per entry at key
`deletion-journal/<subject_type>/<subject_id>/<recorded_at-rfc3339nano>-<outcome>.json`,
bucket versioning plus Object Lock in compliance mode for a bounded retention
(task48 picks the exact window against the promised deletion SLA) so the
journal is durable against deletion even by the hosted service's own
credentials — the literal meaning of "independently durable." A dedicated
IAM policy scopes the service role to `PutObject`/`ListBucket` under the
`deletion-journal/` prefix only, with `DeleteObject` withheld entirely.
`ReadThrough`'s watermark is an S3 `ListObjectsV2` continuation token;
completeness is proven by requiring a non-truncated listing (S3's strong
list-after-write consistency) before advancing the watermark, returning
`ErrDeletionJournalIncomplete` otherwise — never a bare highest-sequence
check. Cost model and exact retention window are owned by task61/task48's own
evidence. **Not approved**: object-lock retention window and IAM prefix
design are proposals, not a reviewed decision.

### 4. Restore eligibility / activation barrier (task50)

Proposal: a durable generation fence extending the existing
`writer.AcquireBrain` exclusive-lock pattern (`internal/cli/hosted.go`'s
`serve` command already acquires one). Before any account is unfrozen,
`RecoveryApplyRequest` handling must, in order: (a) prove the old service's
writer lock has been released and no live holder exists — either the
existing lock file shows no live PID, or an explicit
`serenity hosted recovery fence` operator step (task50) revokes the old
instance's credentials under the approved runbook; (b) call
`DeletionJournal.ReadThrough` from the manifest's own `JournalWatermark`
through "now" and require a nil `ErrDeletionJournalIncomplete`; (c) call
`BillingReconciler.ReconcileCustomer` per account so no stale entitlement
resurrects. Only after all three succeed does `RecoveryApplyResult.Unfrozen`
become true, applied atomically per account — never a manual `status='active'`
SQL statement (evidence.md already forbids this explicitly). **Not
approved**: the fence mechanism (lock-file liveness vs. explicit fence
command) is a proposal; chief-architect must pick one before task50 starts.

## Lock ordering and cancellation (task41, drafted)

Documented from the current implementation, not yet a chief-architect-signed
invariant:

1. `Gateway.Maintenance` (RWMutex; write-locked only by `Service.Backup`,
   read-locked by every mutating call in `callBound`).
2. `Gateway.accountLocks[hash(account)]` (per-account mutex; also used by
   `Export`/`DeleteBrain`/`DeleteAccount`).
3. `Runtime.Mutations` (per-brain mutex; write/forget calls only).
4. `store.Store.Transaction` (SQL transaction; `meter.Reserve`/`Finish`,
   credential/brain writes).
5. Canonical disk/provider I/O (`runtime.Flush`, embedding calls) — outside
   any DB transaction, after step 4 has committed or been explicitly deferred
   past it.

**Known violation, not this task's to fix**: `Gateway.ServeHTTP`
(`gateway.go`) holds the connection-bookkeeping map lock `Gateway.mu` across
`Pool.Acquire`, which can hit a cold-open disk path — exactly the "do not
hold the global gateway/pool map lock across disk/provider work" invariant
this section is required to state. Flagged here for task45 (admission); not
edited, since `gateway.go` is task45's write scope (`R-hosted-runtime`), not
task41's.

Export/delete/recovery calls bypass ordinary metering (`Export`,
`DeleteBrain`, `DeleteAccount` take no `meter.Reservation`) — already true in
the current implementation; this is the existing "bounded capacity
independent of ordinary write allowance" behavior, not a new requirement.
Every `context.Context` passed into a metered path already carries the
caller's deadline; `Meter.Finish`'s deferred calls deliberately use
`context.WithoutCancel` with their own bounded timeout so a canceled request
still finalizes its reservation instead of leaking it open.

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

- Source/PR/reviewer: drafted in worktree `hosted/T23.41` on base
  `810349ba7c1ed2c05fe34e3892de764a26a4633c`; **not yet reviewed by
  chief-architect, not yet merged**. Headless Sonnet worker session had no
  synchronous reviewer available — see
  `docs/launch/evidence/T23.41/architecture-review-request.md`.
- Exact Go interface and SQL revision: `internal/hosted/contracts/**` (9
  non-gated seams frozen with compile-time conformance tests) and
  `internal/hosted/testhooks/**` (fault barrier, frozen and tested) are
  real, compiled, `go vet`-clean code in this worktree. No SQL migration
  applied: the `operations` table proposed for decision 2 is design-only
  pending review; `store/migrations.go` is unchanged this pass.
- Storage envelope design approved: **not approved**. Proposal in "Proposed
  decisions" §1 above; blocked on empirical growth-bound fixture evidence
  task44 has not yet produced.
- Journal completeness/old-writer fence approved: **not approved**.
  Proposals in §3/§4 above (S3 versioned+object-locked substrate; generation
  fence via `writer.AcquireBrain` or an explicit fence command).
- Provider/model/accounting-unit contract approved: `AccountingUnit`,
  `RegistrationMode` and the `ProviderPin` *shape* are frozen (not gated by
  the four decisions); the actual provider/model/version value remains
  task42's to verify and pin. Not evaluated by this task.
- Quality/load thresholds frozen: out of this task's scope this pass; task43
  (Hit@5 corpus) and task60 (workload/latency targets) own the actual
  numbers and were not touched.

These are real dependencies. This worktree does not mark task41 accepted.
