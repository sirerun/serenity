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
a named checkpoint (`hook_hostedtest.go`). Proven by seven real subprocess
tests in `testhooks_test.go`.

**Concurrency defect found and fixed (this pass):** an independent review
found the first draft of `hook_hostedtest.go` had every paused `At()` call
read a single shared `*bufio.Reader` directly, with no lock — a genuine
data race (`bufio.Reader` is not safe for concurrent use) that also caused
a real lost-wakeup hang: a `release <phaseB>` line could be consumed and
discarded by the goroutine paused at `<phaseA>`, permanently hanging the
goroutine actually waiting on `<phaseB>`. Fixed: exactly one goroutine ever
reads the arm pipe (the dispatch loop started inside `ensureOpen`); every
`At()` call only waits on its own phase-keyed channel, closed by the
dispatch loop when the matching `release` line arrives. EOF/error on the
arm pipe now unblocks every current and future paused checkpoint rather
than hanging. Proven by a new subprocess regression
(`TestConcurrentPhasesReverseOrderReleaseAndUnrelatedCheckpoint`, built
with `-race`) that releases two concurrently paused phases in reverse
program-text order and confirms a third, unarmed phase is never blocked
behind them — spot-checked red: reverting to the prior shared-reader design
makes this new test hang and fail deterministically; restored
byte-identical and reconfirmed green. A second new test
(`TestTaggedBinaryPauseUnblocksOnArmPipeEOF`) proves the EOF behavior.

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

**Revision note (this pass):** an independent code review
(`docs/launch/runs.../T23.41-review.md`, read and addressed by this task)
found concrete defects in the first draft of decisions 2, 3 and 4 below —
not just missing approval, but the proposed mechanisms themselves not
actually satisfying the invariant each was supposed to close. Each section
below states what was wrong in the prior draft and what changed. None of
this makes any decision approved; it only makes the proposal actually worth
reviewing.

### 1. Physical storage reservation/headroom (storage admission, task44)

Least settled of the four, and now the only one left explicitly **BLOCKED**
with no adopted mechanism, not merely unapproved. The first draft proposed
`ReservedBytes = ceil(LogicalBytes * growth_factor) + fixed_overhead`, with
`growth_factor` derived from an observed p95 physical-growth ratio times a
1.5 safety margin. **That is not a hard ceiling** — it is a statistical
estimate, and interfaces.md's own required invariant is a *proven*
conservative bound; a single outlier mutation exceeding p95*1.5 would
silently breach the advertised quota, which is exactly the "never erase
acknowledged data or silently relabel bytes as tokens" failure this seam
exists to prevent. Withdrawn.

Proposal instead (interfaces.md's own named alternative, "a separately
approved staged-write design"): (1) apply the mutation in an isolated stage
that never touches canonical account data; (2) measure its real physical
growth directly — the same `filepath.WalkDir` technique `gateway.Inventory`
already uses, not a prediction; (3) admit it via `AdmissionChecker
.ReserveGrowth` using that *measured* value against the account's remaining
quota and a separate, fixed, global `OperatorHeadroomBytes` pool (the
"transient/operational bytes... outside customer quota and still bounded
globally" interfaces.md's seam table requires); only on success (4)
atomically publish the stage into canonical storage — itself crash-
recoverable if the process dies between (3) and (4), by tracking the staged
mutation's own phase through decision 2's operation ledger. Per-account
admission stays serialized through the existing `accountLocks` sharding
(`gateway.go`), so two concurrent staged mutations for one account can never
both measure against the same remaining headroom. **Explicitly BLOCKED**:
task44 may not implement any interim estimate-based approximation of this —
per interfaces.md's own words, "task44 stays blocked" until either this
staged-write mechanism or a proven mathematical bound is chief-architect
approved.

### 2. Crash-safe canonical-operation accounting (task44)

**Defect found and fixed:** the first draft's `OperationRecord` carried a
single `Metric`/`Units` pair. The pre-existing invariant this seam must
satisfy is plural — "Both counters finalize in one transaction" — because
the current implementation issues **two independent** `meter.Reserve`/
`Finish` cycles per `remember` call: one for `"writes"`
(`gateway.go:296-311`) and one for `"input_tokens"` (`gateway.go:333-345`),
each its own transaction. A single-metric `OperationRecord` would not close
that split; it would just reproduce it one layer down as two sibling
records with no field tying them together and no defined rule for
reconciling them to a consistent joint outcome if a crash lands between the
two. Fixed: `OperationRecord`/`ReserveRequest` now carry `Deltas
[]OperationDelta` — every counter one logical mutation touches, reserved
and finalized together as a single row in a single transaction. Task44's
future implementation calls `Reserve`/`Finalize` exactly **once per logical
mutation** (e.g. one call carrying both a `"writes"` delta and an
`"input_tokens"` delta for one `remember`), never once per metric.

**Also fixed:** `OperationRecord` had no field carrying the "durable
canonical operation identity/evidence" the seam table requires in prose —
added `CanonicalRef string`, populated by `Finalize`'s new `canonicalRef`
parameter (e.g. the brain's Git commit SHA a `remember` landed in).
`ReconcilePending`'s description queried `lease_expires_at`, a column with
no matching Go field — added `LeaseExpiresAt time.Time` to
`OperationRecord` so the field and the query it backs live in the same
place. `OperationPhase` gained a fourth state, `OperationPendingReview`:
`ReconcilePending` must never resolve a row it cannot prove either way to
`Committed` (risks crediting a mutation that never happened) or to
`Released` (risks silently discarding one that did, and letting a retried
`ClientKey` double it) — "never release unknown canonical outcomes" is now
a named, non-optional rule, and `ReconcileReport` reports
`PendingReview` as its own counted field so an operator can see it without
reading individual rows.

Proposed table (new schema migration, not yet applied): `operations(id,
account_id, brain_id, client_key, deltas_json, quota_period, canonical_ref,
lease_expires_at, phase CHECK(phase IN
('reserved','committed','released','pending_review')), source, created_at,
finalized_at)`, `deltas_json` a JSON-encoded array of `{metric,units}` pairs
so every delta commits atomically with the row's own `phase` transition —
one `UPDATE` statement, one transaction, all counters — with a partial
unique index over `(account_id, brain_id, client_key) WHERE phase NOT IN
('released')`, mirroring the existing `reservation_operation` index pattern
in `store/migrations.go`. `ReconcilePending` scans `phase='reserved' AND
lease_expires_at <= now()`; task44 defines the exact per-operation-kind
canonical check (e.g. for `remember`, whether the fact is present in the
brain's canonical Git HEAD) and the operator path for resolving a
`pending_review` row by hand, in its own freeze evidence. **Not approved**:
no chief-architect sign-off on the table shape, the multi-delta grouping or
the pending-review rule yet.

### 3. Independently durable deletion journal (task48, infra54)

**Defect found and fixed:** the first draft keyed journal objects
`deletion-journal/<subject_type>/<subject_id>/<recorded_at>-<outcome>.json`
and described the `ReadThrough` watermark as an S3 `ListObjectsV2`
continuation token, with completeness "proven" by a non-truncated listing.
This does not compose: `ListObjectsV2` returns keys in **lexicographic
order**, which that key format groups by `subject_type` then `subject_id` —
an opaque, effectively random ID — not by append time. A
`StartAfter`/continuation-token watermark only ever returns keys
lexicographically *greater* than the marker, so a later-appended entry whose
`subject_id` happens to sort before an already-consumed key becomes
permanently invisible to every future `ReadThrough` call — not a
truncation, so `ErrDeletionJournalIncomplete`'s only check (a truncated
listing) never catches it. "Strong list-after-write consistency" guarantees
a single from-scratch listing is complete; it says nothing about an
incrementally advanced marker across writes made after the marker moved.
Withdrawn.

Proposal instead: entries are addressed by a durable, monotonic
`SequenceID`, assigned once by the single currently-fenced writer — via a
counter row in the existing hosted control DB (the same single-writer
SQLite instance every other durable sequence in this service already
trusts; no new service), paired with the writer `Generation` from decision 4
— never by the object store. Keys become
`deletion-journal/<generation>/<sequence, fixed-width zero-padded>.json`, so
lexicographic key order and append order are the same thing by
construction. `ReadThrough(from)` proves completeness by reading the
control DB's own current high-water `SequenceID` for `from.Generation` as
the target (never an S3 listing), then confirming an object exists for
*every* `SequenceID` in `(from.SequenceID, target]` — rejecting with
`ErrDeletionJournalIncomplete` on the first missing one, whether the gap is
at the tail or in the middle, rather than trusting listing order at all. A
watermark naming a non-current generation is refused
(`ErrDeletionJournalStaleGeneration`) until decision 4's activation barrier
certifies that generation closed.

A dedicated IAM policy still scopes the service role to
`PutObject`/`GetObject`/`ListBucket` under the `deletion-journal/` prefix
only, with `DeleteObject` withheld entirely. **Object Lock / compliance-mode
retention is withdrawn as a proposed mechanism, not merely unapproved**: it
carries real operational and cost consequences (objects become undeletable,
including by the account owner, for the lock's duration) that are SPEND-gate
territory, not a detail this task can wave through. Retention is instead an
open question for whoever owns the SPEND gate alongside task48/task61; this
proposal only commits to bucket versioning (reversible, no cost commitment)
as the durability floor. **Not approved**: sequence/generation design, IAM
scoping and retention are all proposals, not a reviewed decision.

### 4. Restore eligibility / activation barrier (task50)

**Defect found and fixed:** the first draft offered two "equally valid"
fencing options, (a) checking whether `writer.AcquireBrain`'s lock file
"shows no live PID," or (b) an explicit fence command. Reading
`internal/writer/ownership_unix.go` in full: `AcquireBrain` is an advisory
POSIX `flock()` on a local file: **no PID is ever written to it** — kernel
`flock` ownership is tracked against the holding process's open file
descriptor, never persisted into the file's bytes, and the file's own doc
comment already states "this fences cooperating CLI processes on one
filesystem, not arbitrary local writers." Worse: restore's own runbook
scenario is downloading a snapshot onto an **isolated, different host**
(`docs/launch/hosted-runbook.md:47`) — a `flock()` is kernel-local, so a
restored copy of `.serenity/writer.lock` on a new host carries no memory of
any lock held on the original host's copy. `AcquireBrain` against the
restored copy trivially succeeds every time, regardless of whether the
original instance is still alive and serving writes elsewhere (e.g. a
network partition, restore's actual worst case). Option (a) was a
false-safe no-op for the scenario that matters most. **Dropped entirely for
the cross-host restore path.**

Proposal now requires, before any account is unfrozen: (a) **independently
verified old-instance stop** — confirmed via the infrastructure provider
(AWS `DescribeInstances` showing the old EC2 instance `stopped`/
`terminated`), never a local file/PID check, which cannot observe a
different host at all — **and** (b) **explicit revocation of every
credential/session the old instance could still use**, spelled out rather
than left as an undefined "credentials": rotate `STRIPE_SECRET_KEY`,
`EMBEDDINGS_API_KEY` and any DB-adjacent secret the old instance held, *and*
explicitly deny or rotate the old instance's already-established AWS IAM
role/session so it cannot still write to S3 (the deletion journal's own
substrate) even if the instance itself is merely unreachable rather than
verifiably dead — stopping the instance alone does not invalidate a still-
live temporary credential session. A same-host process-restart recovery (no
host change) may still use `AcquireBrain` as a narrower, legitimate
same-filesystem check — that scenario is what the lock's own doc comment
actually describes. (a)+(b) is a fully specified alternative to a
distributed generation-barrier mechanism (e.g. a conditional-write fencing
token); either remains open for chief-architect to choose, but a bare local
lock check is no longer offered as one of the options.

After fencing: (c) call `DeletionJournal.ReadThrough` from the manifest's
own `JournalWatermark` through the current head and require a nil
`ErrDeletionJournalIncomplete`; (d) call `BillingReconciler
.ReconcileCustomer` per account so no stale entitlement resurrects. Only
after (a)+(b), (c) and (d) all succeed does `RecoveryApplyResult.Unfrozen`
become true, applied atomically per account — never a manual
`status='active'` SQL statement (evidence.md already forbids this
explicitly). **Not approved**: the AWS-verified-stop-plus-revocation
mechanism (vs. a fully specified distributed generation barrier) is a
proposal; chief-architect must pick one, and task50 must not start against
either without that ruling. A regression proving `AcquireBrain` against a
copied lock file falsely succeeds across two directories (simulating two
hosts) is required before task50 begins, per the review that found this
defect.

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

- Source/PR/reviewer: drafted in worktree branch `hosted/t23.41-20260918` on
  base `810349ba7c1ed2c05fe34e3892de764a26a4633c`; **not yet reviewed by
  chief-architect, not yet merged**. Headless Sonnet worker session had no
  synchronous reviewer available — see
  `docs/launch/evidence/T23.41/architecture-review-request.md`. An
  independent code review found and this task fixed four concrete defects
  in the first draft (a concurrency bug in the fault-barrier package, and
  design gaps in decisions 2/3/4 below) — see that request's revision note
  for the full list; none of the four decisions moved from "not approved"
  to "approved" as a result.
- Exact Go interface and SQL revision: `internal/hosted/contracts/**` (9
  non-gated seams frozen with compile-time conformance tests) and
  `internal/hosted/testhooks/**` (fault barrier, frozen, tested, and its
  concurrency defect fixed this pass) are real, compiled, `go vet`-clean
  code in this worktree. No SQL migration applied: the `operations` table
  proposed for decision 2 is design-only pending review; `store/
  migrations.go` is unchanged this pass.
- Storage envelope design approved: **not approved, and no interim
  mechanism is offered**. The original growth-factor-multiplier proposal
  was withdrawn (not a hard ceiling — see §1's revision note); the
  staged-write alternative in §1 above requires chief-architect approval
  before task44 may implement anything for this seam.
- Journal completeness/old-writer fence approved: **not approved**. Both
  underlying mechanisms were revised this pass after review found the first
  drafts unsound (§3: S3-continuation-token watermark could permanently
  miss entries — replaced with a control-DB-assigned sequence/generation
  design; §4: local-lock-file fencing cannot observe a different host and
  writes no PID — replaced with AWS-verified-stop-plus-credential-
  revocation, offered alongside a distributed generation barrier as the
  still-open alternative).
- Provider/model/accounting-unit contract approved: `AccountingUnit`,
  `RegistrationMode` and the `ProviderPin` *shape* are frozen (not gated by
  the four decisions); the actual provider/model/version value remains
  task42's to verify and pin. Not evaluated by this task.
- Quality/load thresholds frozen: out of this task's scope this pass; task43
  (Hit@5 corpus) and task60 (workload/latency targets) own the actual
  numbers and were not touched.

These are real dependencies. This worktree does not mark task41 accepted.
