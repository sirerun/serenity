# Shared interfaces — task41 freeze gate

**State: specification to freeze, not implemented APIs.** Task41 owns this file and every schema migration/shared assembly edit. Existing main implementation remains authoritative until a reviewed commit changes it. Sonnet workers43/60 may prepare their independent harnesses now; dependent feature tasks do not start until41 supplies exact compile-tested Go types/SQL and the technical review receipt.

## Required invariants and bounded design work

| Seam | Implementation owner | Required contract / preferred mechanism | Freeze evidence |
|---|---|---|---|
| Operation identity/accounting | 44, schema41 | Durable operation journal, explicit internal ID for every mutation, optional client retry key scoped to account+brain, original quota period, reserved write/input units, canonical source/outcome and terminal accounting. Both counters finalize in one transaction. Canonical committed-but-unfinalized operations reconcile before lease release. Unkeyed independent writes remain distinct. | SQL constraints/indexes, phase state machine, recovery query and named canonical evidence seam; crash cases through every phase |
| Storage admission | 44, review41 | Preserve published physical-byte meaning. Before acknowledging mutation, reserve a proven conservative growth envelope for canonical/Git/index/vector/WAL changes plus operator headroom; serialize account admission. Define which transient/operational bytes are outside customer quota and still bounded globally. Never erase acknowledged data or silently relabel bytes as tokens. | Mathematical/fixture growth bound or a separately approved staged-write design; one-byte headroom and concurrent-brain proof. If neither is established, task44 stays blocked; schema owner cannot invent a hidden overage allowance. |
| Billing truth | 47 | `ReconcileCustomer(ctx, accountID)` fetches the server-owned provider customer, validates prices/ownership, replaces stale subscription/checkout state atomically and returns eligibility without activating frozen accounts. No provider lookup based on a client-supplied customer ID. | Exact result/error types, transaction boundaries, current-window vs historic usage and authoritative grace timestamp rules |
| Billing closure | 47, deletion48 | `CloseBillingAccount(ctx, accountID)` is resumable for deleting accounts, prevents new checkout, expires pending sessions, lists/cancels all owned provider subscriptions including unobserved ones, certifies closure only after reconciliation. Provider ambiguity leaves deletion pending. | Checkout/deletion race ordering, idempotency keys, provider timeout and late-webhook behavior |
| Deletion journal | 48, infrastructure54 | Independent durable append/read adapter, records opaque account/brain IDs and deletion intent/outcome only; no memory/email. Account and brain deletions both covered. `AppendDeletion` durability precedes acknowledged delete. Versioned object storage is the preferred existing AWS substrate; avoid a new service/queue. | Exact object/record format, IAM prefixes, ordering and integrity/completeness proof, journal watermark included in snapshots, cost model, retention bound |
| Recovery activation barrier | 50, review41 | Old service must be fenced before activation (stop/revoke old instance access under the approved runbook or prove a durable generation fence). Verify journal completeness through the activation barrier, not only visible highest sequence. Apply later deletions and provider truth, then atomically unfreeze eligible survivors; all old credentials remain revoked. | Concurrent delete/missing-tail/missing-middle/returning-old-node tests; explicit failure if authority cannot be proved |
| Backup manifestv2 | 49 | Version, source/build/schema, created UTC, artifact relative names/length/SHA256, sorted unique brain inventory, canonical bundle heads, journal watermark. COMPLETE binds manifest hash. Hashes detect corruption; they do not authorize untrusted uploaded snapshots. | JSON/schema, legacyv1 refusal/support policy, full inventory reconciliation and staging/atomic-publish behavior |
| Recovery CLI | 50, registration41/57 | `serenity hosted recovery plan` and `... apply`: private paths/config inputs, immutable plan hash, exact source snapshot/journal/provider state, resumable per-account phases. No `activate-all`. New secret values never appear in flags/output. | Exact flags/exit codes and source of recovery authority; supported schema/version pair |
| Telemetry | 53, hooks41/57 | Fixed-cardinality structured event/metric interface with typed operation/outcome/duration/count; opaque tenant identifiers and request content forbidden. Recursive redaction and upstream error sanitization. | Producer/metric/alarm mapping, units, missing-data behavior and bounded sink queue |
| Provider pin | 42 | Provider-neutral key, baseURL/model/version/dimensions and privacy-qualified routing; numeric embeddings handled with cosine-compatible similarity. No silent model/provider fallback into a different embedding space. | Live response/usage encoding verified before final pin; reject corrupt or mismatched vectors; model-change rebuild is an explicit maintenance operation |
| Accounting units | 44 with42 | Product input allowance and provider billed tokens are separately named. Existing cl100k_base can only remain as an explicitly documented product unit; actual provider usage drives cost. Every call including readiness/recovery is accounted in operator costs. | Plans/docs/UI naming consistency, provider usage fixtures and cost reconciliation |
| Registration mode | 46, config41 | `public` or `invite_only`; default policy during qualification restricts new registration to controlled test identities; private allowlist stays outside Git/logs. Paid controls gated independently. | Direct login/account-creation enforcement; config validation; no UI-only access control |
| Fault barriers | Feature44/46/47/48/49/50, harness58 | Named deterministic phase hooks compiled only with `hostedtest`; activated via private inherited control channel in test subprocess. Normal build has no active hook or externally triggerable pause/crash path. | Exact phase names/transport, build-tag test and actual tagged-vs-untagged binary proof |

## Frozen Go interfaces (task41)

`internal/hosted/contracts/**` carries compiled, tested Go signatures for the seams in the table above. `internal/hosted/testhooks/**` freezes the fault-barrier transport: named phase constants and a single `testhooks.At(phase)` call site. A binary built without `-tags hostedtest` links a true no-op (`hook_prod.go`). Only a `hostedtest`-tagged binary started with a harness-inherited control pipe (never an HTTP endpoint, never a bare environment switch) can pause or crash at a named checkpoint (`hook_hostedtest.go`). One goroutine reads the arm pipe and routes each `release` line to its own phase channel, so concurrent pauses cannot lose a wakeup, and EOF releases every paused phase. Phase names added by this pass: `PhaseOperationCanonicalEntered`, `PhaseOperationCanonicalWritten`, `PhaseReconcileFenced`, `PhaseStageMeasured`, `PhaseStagePublished`, `PhaseJournalSealed`.

## Status of every seam

Chief-architect reviewed the four design decisions at PR236 revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`; see the [ruling comment](https://github.com/sirerun/serenity/pull/236#issuecomment-5746461189) and the per-line receipt in `docs/launch/evidence/T23.41/architecture-review-request.md`. **FROZEN** means the Go signature is compiled and tested and does not depend on an open decision; **PROPOSED** means a concrete mechanism with an executable specification exists but still needs implementation and ordinary review; **BLOCKED** means dependent implementation must not start.

| Seam | Status | Artifact | Executable specification |
|---|---|---|---|
| Billing truth and closure | FROZEN | `contracts/billing.go` | compile-time conformance in `contracts_test.go` |
| Backup manifest v2 | FROZEN shape, revised this pass (control DB artifact, per-brain bundle heads, `Validate`); `JournalWatermark` PROPOSED (decision 3) | `contracts/backup.go` | `TestManifestV2Validate*`, `TestManifestV2VersionErrorIsDistinct`, `TestManifestV2JSONRoundTrip` |
| Recovery plan/apply shape | FROZEN except `JournalWatermark`, `Generation`, `Fence` (PROPOSED, decisions 3-4) | `contracts/backup.go` | `TestRecoveryResultConsistency` |
| Telemetry, provider pin, accounting units, registration mode | FROZEN | `contracts/{telemetry,provider}.go` | compile-time conformance |
| Fault-barrier transport and phase names | FROZEN | `testhooks/**` | seven subprocess tests plus `TestPhaseNamesAreDistinctAndWireSafe` |
| Decision 1 - storage admission | PROPOSED, **BLOCKED** | `contracts/storage.go` | `contractstest.RunStagingSuite` |
| Decision 2 - operation accounting, quiescence, retry-key binding | PROPOSED | `contracts/operations.go`, `contracts/commitfence.go` | `contractstest.RunLedgerSuite`, `TestValidateTransitionIsExhaustive` |
| Decision 3 - deletion journal | PROPOSED | `contracts/deletion.go` | `contractstest.RunJournalSuite` |
| Decision 4 - restore fence | PROPOSED | `contracts/fence.go` | `TestFenceReceiptRequiresEveryFact`, `TestLocalWriterLockCannotFenceAnotherHost` |

Compile-time conformance means a `var _ contracts.X = fakeX{}` assertion in `contracts_test.go`; a signature change that breaks an implementation fails `go vet`.

`internal/hosted/contracts/contractstest` holds reference models and deterministic scenarios for the PROPOSED rows. Each scenario is single-goroutine and sequenced by explicit calls, a fake clock and pre-cancelled contexts, so an interleaving is reproduced exactly. A reference model passing its own suite shows the proposed rules are self-consistent and testable. It is not evidence about production code, which does not exist, and it is not approval. Task44, 48 and 50 run the same `Run...Suite` functions against their real implementations. Nothing under `internal/hosted/service` or `internal/cli` imports it, and no schema migration is applied.

This pass changed three shapes that an earlier draft of this document listed as frozen (`ManifestV2.JournalWatermark`, `RecoveryPlan.JournalWatermark` and `.Generation`, `RecoveryApplyResult.Fence`) because they carry the journal position and fence proof, which are open decisions. It also revised `ManifestV2` because an independent audit (finding D2, `launch-audit-pass2-handoff.md`) showed it could not hold what task49 step 1 and its roundtrip acceptance require. `ManifestV2` now has `ControlDB ArtifactRef` (relative path, length, SHA256), each non-empty brain's `Heads []BundleHead` (ref and object ID, strictly ascending by ref, none for an empty brain), and `Validate`. Every artifact path is a single safe path element, unique across the control database and bundles compared case-insensitively, and never `manifest.json`. `Validate` checks structure and internal consistency only. It does not prove the files exist or match, that the inventory equals the control database's, or that the manifest is authentic: task49 checks those. No field is reserved for an incremental-backup scheme, and the audit's suggestion to reserve `Kind` and `Base` was not taken. No package outside `contracts` imports any of these types, so no dependent receipt reopens. Task49 has not started and rebases on this shape.

## Architecture-ratified design decisions

Each decision states what to adopt, the rules that make it checkable, what the previous draft got wrong, and the questions the reviewer must answer, with a recommendation. Names in code font are in `internal/hosted/contracts`. See `docs/launch/evidence/T23.41/architecture-review-request.md` for the per-decision approve/amend/reject form.

### 1. Physical storage reservation and headroom (task44) - BLOCKED

**Status: PROPOSED and BLOCKED.** The seam table requires either a proven growth bound or a separately approved staged-write design. This section specifies the staged design far enough to approve or reject. It does not lift the block.

**What the previous draft got wrong.**

1. It measured a stage after writing it and only then admitted it, and it serialized admission per account. Per-account serialization does not bound the aggregate: stages for different accounts run concurrently, so their bytes can exhaust the device before any admission decision runs, and a measurement cannot stop a write already in progress.
2. It assumed "apply the mutation in an isolated stage" was implementable. Canonical brain state is a live Git working tree committed by `writer.Flush` (`internal/writer/commit.go`), and there is no temp-and-rename publish step. A stage needs a core-writer change.
3. The current check-then-act `inventory.StorageBytes >= Plan.StorageBytes` (`gateway.go:322`) reserves nothing, so one mutation's whole physical growth can overshoot the ceiling.

**Proposed rules.**

1. `StagingGate.ReserveStage` grants a `StageTicket` only if the sum of outstanding ceilings plus `MaxMutationStageBytes` stays within `StagingBudgetBytes` **and** observed free bytes minus every outstanding ceiling minus `MaxMutationStageBytes` stays at or above `OperatorHeadroomBytes`. No stage byte may be written before a ticket exists. Unwritten ceilings count against free space, which is what makes the bound hold before the write instead of after it.
2. The stager reports every write to `contracts.StageMeter`, bound to the ticket, in allocated bytes (rule 8). `Add` refuses the report that would pass the ceiling. The stage stays within its ceiling only as far as the stager reports faithfully: the meter is a cooperative counter, not an enforced limit (rule 8).
3. `AdmitMeasured` compares the stage's measured growth with the account's physical bytes plus growth already admitted but not yet published. A refusal discards the stage. No canonical byte changes and no acknowledged fact is removed. Two admissions for one account cannot both pass against the same remaining bytes, with or without an account lock.
4. `Release(StagePublished)` moves admitted growth into the account's physical usage. `Release(StageDiscarded)` returns it. Release is idempotent.
5. A crash between publish and finalize is resolved by decision 2: the operation row is still reserved, the canonical checker finds the operation marker, and the reconciler commits it. Checkpoints `PhaseStageMeasured` and `PhaseStagePublished` cover both edges.
6. Transient and operational bytes (repack, WAL checkpoint, backup staging, exports) stay outside customer quota and are bounded only by `OperatorHeadroomBytes`. When free space falls below headroom, writes fail with `ErrStagingBusy` while export, delete and idempotent replay stay available.
7. `MaxMutationStageBytes` is a configured constant that task44 must prove against the largest accepted mutation: a 4096-byte fact (`gateway.go:266`), the plan's maximum memory count, the fixed embedding dimension, the index and WAL growth for one insertion and the Git tree rewrite for that many entries. The proof is a measured corner fixture, not a percentile. The measurement is of physical allocation, as rule 8 defines it. The configured value must be at least the measured worst case, and the fixture fails the build if a runtime change raises the worst case above it. The independent audit's synthetic proxy (`launch-audit-pass2-handoff.md` D3, not reproduced in this task) reported that one 519-byte page grew the loose-object store by 10.6 KB at 400 entries per directory and by 297 KB at 12,000, which is why a payload-based figure is unsound and why the measurement must include Git tree rewrites or a repack policy.
8. **Accounted bytes are not enforced bytes.** Every byte figure in this decision (ticket ceiling, meter total, `measuredBytes`, staging budget, headroom) is physical allocation: blocks rounded up per file, directory and inode metadata, Git loose objects, packs and the index, SQLite WAL and journal files, and vector-store growth. `StageMeter` counts only what the stager passes to `Add`, so a serialized-length count undercounts, and Git and index writes that the stager does not serialize itself reach the meter only if the seam reports them. Task44 must convert each write to allocated bytes and account for external Git and index writes before a ceiling is a physical ceiling. Even then the meter is cooperative: a stager defect, a child process or an unrouted write lands on disk unopposed. No fixture `Add` counter, reference model or passing suite proves an OS-enforced hard bound. A production claim of "hard" requires an enforcement layer the writing process cannot exceed, for example the staging area on a dedicated size-limited filesystem or a quota-limited volume sized to `StagingBudgetBytes`, so an overrun fails inside the stage and cannot consume the shared device. Until task44 shows that layer, this decision claims an accounting ceiling, not a physical one. `TestStageMeterIsCooperativeNotEnforcing` pins the limitation.

**Prerequisites.** (a) A stage requires a core-writer seam, for example writing Git objects into a quarantine object directory and publishing them by moving the objects and updating the ref. T23.44 step 4 makes that a shared-seam request. It is a cost the reviewer must accept, not an implementation detail. (b) A hard bound requires an OS-enforced size limit under the staging area, plus allocation-based accounting including external Git and index writes (rule 8). Both are open work for task44 and the infrastructure owner. The storage envelope stays BLOCKED until both exist, whichever route the reviewer picks.

**Questions for the reviewer.**

- *Route.* **Recommended: staged write (this section)**, because it is hard by construction. Fallback if the writer seam is rejected: in-place admission with a proven ceiling. Admit iff `physical + MaxMutationStageBytes <= quota`, run the mutation in place, measure afterwards, and if measured growth passes `MaxMutationStageBytes`, latch all writes frozen (never delete acknowledged data) and page the operator. The fallback costs customers the last `MaxMutationStageBytes` of quota and is only as strong as the corner proof.
- *Statistical multiplier.* Stays withdrawn under either route.

**Executable specification.** `RunStagingSuite`: one byte over quota refused with pending growth counted; aggregate staging bounded across accounts (sequential and 50-goroutine); unwritten ceilings subtracted from free space; a stage past its ceiling refused and its budget returned; idempotent release. Mutation-tested: dropping the pending-growth term, the free-space term or the budget check each fails the intended scenario. The suite exercises accounting only. It contains nothing that limits real disk use.

### 2. Crash-safe operation accounting, quiescence and retry-key binding (task44)

**Status: PROPOSED.** Files: `contracts/operations.go`, `contracts/commitfence.go`. Migration version 4 is reserved for this feature and stays unwritten until approval; `store/migrations.go` is unchanged.

**What the previous drafts got wrong.**

1. *Terminal state that was not terminal.* `pending_review` was called terminal while an operator could still move it. It is now a holding phase, and `ValidateTransition` is the single exhaustive table of legal `(from, to, actor, evidence)` combinations.
2. *Lease expiry treated as proof the writer stopped.* A writer paused inside the canonical write can resume after a checker observed the operation absent and released its reservation, leaving a landed mutation with no charge. Expiry is now only a liveness filter. See the quiescence rules below.
3. *No limit in `ReserveRequest`.* The ledger could not admit anything. Each `ReserveDelta` now carries its `Limit`.
4. *Retry key bound to source and deltas only.* Equal token counts and operation kinds can hide a different fact. The key now binds to a request fingerprint.
5. *Working-tree state ignored.* `writer.Flush` re-marks a failed commit's paths as touched, so a later successful flush commits them. A fact absent from Git history but pending in the working tree can still land. The checker now returns Unknown for it, never Absent.
6. *Caller could release on its own absence check.* Absence-based release is now reachable only through the fenced reconcile path.

**State machine.**

| From | To | Actor | Evidence |
|---|---|---|---|
| reserved | committed | caller, reconciler | `canonical_committed`, non-empty ref |
| reserved | released | caller | `no_canonical_attempt`, empty ref; refused once `EnterCanonical` recorded entry |
| reserved | released | reconciler | `canonical_absent`, non-empty ref, produced under the brain fence |
| reserved | pending_review | caller, reconciler | `outcome_unknown`, non-empty sanitized reason class |
| pending_review | committed or released | operator | `operator_review`, non-empty review reference |

`committed` and `released` are final. A same-phase repeat with identical evidence is idempotent and returns the stored row; different evidence or a different phase is `ErrOperationConflict`. `reserved` and `pending_review` hold capacity: committed usage plus held rows plus the request must fit every delta's limit, so an unresolved operation cannot let a customer pass an allowance. `Reserve` never releases or expires another row. `meter.Meter.Reserve` does release expired open rows today, and the mutating-operation path must stop using that behavior. Read metrics keep `meter.Meter`, because they have no canonical effect to lose.

**Exactly-once across counters.** One logical mutation is one row. `Reserve` moves every counter's hold, and `Finalize(committed)` applies every delta in one SQL transaction and the row's phase in the same statement group. The counters cannot disagree, and a repeated `Finalize` never applies a delta twice. The row is charged to the `QuotaPeriod` it was reserved in, even after rollover.

**Quiescence rules.**

1. A writer brackets each canonical write in `BrainFence.EnterCommit` ... `leave`, and calls `OperationLedger.EnterCanonical` inside that section before its first canonical byte. If the row is no longer reserved, the writer receives `ErrOperationNotReserved` and writes nothing.
2. `ReconcilePending` acts only on reserved rows whose lease expired. For each it acquires the brain's exclusive `Fence`, then runs the checker, then performs the ledger transition, then releases the fence. Absence inspection and the transition are one fenced span.
3. If the fence cannot be acquired before the context ends (a writer is inside its section) or the checker returns an error, the row stays reserved and is counted `Deferred`. A slow writer costs liveness, never correctness.
4. Landed becomes committed, absent becomes released, unknown becomes pending review. `Unknown` includes any uncommitted working-tree or touched-queue state that carries the operation marker.
5. Scope of the proof: the fence excludes writers in this process. Other processes on the same host are excluded by `writer.AcquireBrain`, a kernel flock that dies with its holder. A startup reconcile before serving is therefore trivially quiescent. Writers on another host are excluded only by decision 4, never by this fence.

**Lock order.** See "Lock ordering and cancellation" below. In short: the reconciler takes the brain fence and a ledger transaction and never an account lock or `Runtime.Mutations`, so it cannot deadlock with a writer that holds those and waits for the fence.

**Retry-key binding.** `RequestFingerprint(kind, fields)` is a domain-separated, length-prefixed SHA-256 over every parameter that changes what the canonical writer stores, sorted by name. For `remember`: the exact fact bytes and the brain. Never a derived value such as a token count. A key without a fingerprint is `ErrOperationInvalid`. With an existing non-released row, a different fingerprint or source is `ErrOperationKeyReuse` in every phase, including after commit. Same fingerprint then behaves by phase: committed returns the stored row (a replay, no second charge), reserved is `ErrOperationInProgress`, pending review is `ErrOperationPendingReview`. The stored row's deltas are authoritative for a replay, so a tokenizer change between deploys cannot invalidate a retry. A released row frees its key.

**Proposed table.** `operations(id, account_id, brain_id, client_key, fingerprint, deltas_json, quota_period, phase CHECK(phase IN ('reserved','committed','released','pending_review')), source, evidence_kind, evidence_ref, canonical_entered_at, lease_expires_at, created_at, finalized_at)`, with a partial unique index over `(account_id, brain_id, client_key) WHERE client_key <> '' AND phase <> 'released'`. `deltas_json` is an array of `{metric, units}`.

**Questions for the reviewer.**

- *Commit-section extent — chief-architect ruling.* The section begins at the first canonical byte and ends when `writer.Flush` returns. Embedding and other provider calls happen before it; the fence must not be held across provider I/O, which could defer reconciliation for up to 60 seconds. Task44 must enter the section immediately around the canonical write.
- *Failed flush.* **Recommended:** task44 proposes a core-writer change so a failed `Flush` rolls back the working tree. Until then the checker's Unknown rule sends those rows to pending review.
- *Operator path for pending review.* **Recommended:** a `serenity hosted operations review` command owned by task44 with CLI registration by the integrator, printing only opaque IDs and the stored evidence.
- *Fingerprint fields for other mutations.* `forget`: the fact identifier. Owned by task44 in its freeze evidence.

**Executable specification.** `RunLedgerSuite` and `TestValidateTransitionIsExhaustive`. The quiescence group reproduces, deterministically, a paused writer, an expired reservation, an absence check and a late commit attempt:

| Scenario | Expected |
|---|---|
| Writer paused before `EnterCanonical`; reconcile releases | Late writer receives `ErrOperationNotReserved` and writes nothing; retry of the same key charges once. |
| Writer paused inside its commit section; reconcile runs | Row is deferred; the writer commits and finalizes; one charge. |
| Same interleaving with a fence that excludes nothing (negative control) | A landed write with no charge. The assertion documents that this defect is real. |
| Crash after canonical commit, before finalize | Reconciles to committed once; the resumed writer's identical finalize is idempotent. |
| Working-tree write pending, flush failed | Pending review, never released; the retry of the key is refused. |

Mutation-tested: removing the reconciler's fence, dropping the fingerprint comparison, and resolving Unknown to released each fail the intended scenario.

### 3. Independently durable deletion journal (task48, infrastructure54)

**Status: PROPOSED.** Files: `contracts/deletion.go`. No SQL: the journal needs no migration.

**What the previous drafts got wrong.**

1. *Subject-first keys with a listing marker* could permanently miss a later-appended entry whose subject sorted earlier. Withdrawn earlier.
2. *The control database was the completeness authority.* The replacement counted entries in the local control database. Restore replaces that database with the snapshot's copy, which cannot know about entries appended after the snapshot, and those are exactly the entries recovery must find. Completeness now comes from the journal alone. `TestLocalHighWaterCannotProveTailCompleteness` reproduces the failure.

**Proposed rules.**

1. The journal is a hash-chained, generation-scoped, gapless sequence of immutable objects at `deletion-journal/<generation, 10 digits>/<sequence, 10 digits>.json`. Key order is append order. `JournalObject` fixes the stored form.
2. The object store assigns positions. A writer claims sequence N+1 with a conditional create (`JournalObjectStore.PutIfAbsent`). If another object holds the position, the writer is fenced: `ErrDeletionJournalFenced`, or `ErrDeletionJournalSealed` when the occupant is the seal. The service must then stop acknowledging deletions. A retry after a lost response is recognized by byte-identical content and succeeds.
3. Each object names its predecessor's hash. `ReadThrough` verifies that keys are contiguous, that each object's coordinates match its key, that the chain holds, and that the object at the resume watermark is the one the watermark's `EntryHash` names. A missing middle entry, a replaced entry or a rewritten tail fails with `ErrDeletionJournalIncomplete` or `ErrDeletionJournalHistoryMismatch`.
4. `Seal(generation)` conditionally creates a seal object at the first free position and then verifies nothing exists beyond it. It is idempotent. A cooperating stale writer that resumes finds the seal and receives `ErrDeletionJournalSealed`. An object beyond the seal is `ErrDeletionJournalFenceViolated`. Only recovery may claim tail completeness, and only from a read whose `Sealed` field is true.
5. A new generation can begin only after its predecessor's seal exists; its first object chains to the seal's hash. `ReadThrough` traverses sealed generations into their successors and rejects a generation that began without its predecessor sealed.
6. `ManifestV2.JournalWatermark` is read from the journal before the backup copies data. Recovery replays every entry after it. Replaying an entry the snapshot already reflects is safe because purge is idempotent, and a deletion that runs during the copy is replayed either way. Reading the watermark after the copy could skip a deletion the snapshot missed. Safety rests on this ordering and on idempotent purge, not on `Gateway.Maintenance`. `Service.Backup` write-locks it (`service.go:283`), and `callBound` (every tool call) and `DeleteBrain` (`lifecycle.go:81`) read-lock it, so the backup lock does block those. `DeleteAccount` and `RecoverDeletions` reach it only through `DeleteBrain`, one brain at a time, so the lock can fall between two brains of one account deletion. `Export` and `RecoverDeletions`' own `os.RemoveAll` take no lock. The lock therefore cannot be the guarantee.
7. An entry with outcome `requested` is authoritative deletion intent. Recovery removes the subject even if no `purged` entry followed.
8. Entries hold opaque account or brain IDs and outcomes only. `DeletionEntry.Validate` rejects an email-shaped subject.

**Limits stated so no reviewer assumes more.** The protocol proves completeness relative to what the object store holds at seal time. An adversary with delete rights who removes the newest entries before the seal is written is bounded by the IAM policy (`DeleteObject` withheld) and bucket versioning (delete markers stay visible), not by the protocol. A writer that ignores its generation's seal is stopped by credential revocation (decision 4), not by the protocol.

**Substrate assumption and what is verified.** `PutIfAbsent` maps to S3 conditional `PutObject` (`If-None-Match: *`). A coordinator check of the AWS documentation (`s3-conditional-write-source-check.md`) confirms the feature exists: conditional writes, and policy enforcement through the `s3:if-none-match` and `s3:if-match` condition keys. That establishes availability only. No request was made against our bucket, our bucket policy or the CLI/SDK build task48 will use, so live qualification is missing. Task48 must qualify all three on an authorized disposable resource before the adapter is accepted. If it cannot, the substrate returns to this review, because a conditional-write table would be a new service and needs its own approval.

The documented semantics add adapter obligations, and `contracts.JournalObjectStore` records them:

- *Versioning.* On a versioned bucket the existence test is against the current version, and a current delete marker permits a new write. A sequence key is immutable only while current versions and delete markers cannot be removed. The service role gets no `DeleteObject` or `DeleteObjectVersion` on `deletion-journal/`, and a bucket policy requires the conditional header on that prefix so a writer cannot omit it. The header alone is not enough.
- *Lifecycle.* No lifecycle rule applies to `deletion-journal/`, and snapshot purge and retention jobs never list, expire or delete it. Journal retention is a separate decision.
- *Outcomes.* `created=false` with a nil error means only a definitive "already exists" (412). A concurrent-operation conflict (409), a 404, a 5xx, a timeout or a cancelled context is an error, never `created=false`. The caller treats it as unknown and retries the same bytes.
- *Ambiguity.* A success response is not proof of durability or of tail completeness. After an ambiguous put the position is accepted only if reading the key back returns exactly the bytes the writer tried to write. `MemObjectStore.LoseNextPutResponse` reproduces the lost-response case in the reference model.

**Questions for the reviewer.**

- *Substrate.* **Recommended: the existing versioned AWS object bucket with conditional create**, no new service, conditional on the live qualification above.
- *Retention.* Unchanged from the earlier draft: Object Lock or compliance retention stays withdrawn as SPEND-gate territory. This proposal commits only to bucket versioning.
- *IAM.* **Recommended:** the service role gets `PutObject`, `GetObject` and `ListBucket` under `deletion-journal/` only, with `DeleteObject` and `DeleteObjectVersion` withheld and the conditional header required by bucket policy. Recovery needs `ListBucket` and `GetObject`.

**Executable specification.** `RunJournalSuite`: round trip and resume; an earlier-sorting subject after a consumed one; missing middle; entry removed behind a seal; rewritten tail detected by a snapshot watermark; replaced middle entry; foreign append fences the writer; lost put response retried idempotently; seal stops a stale writer and is idempotent; object beyond the seal; writer skipping ahead of the seal; new generation requires a sealed predecessor; a seal that loses a position to a live writer retries after it; and a bounded concurrent seal that loses no acknowledged append. Two negative controls reproduce the withdrawn designs. Mutation-tested: dropping the hash chain, the watermark hash check or the beyond-seal check each fails the intended scenario.

### 4. Restore eligibility and external fencing (task50)

**Status: PROPOSED.** Files: `contracts/fence.go`.

**What the previous draft got wrong.** It offered a local lock-file check as a fencing option. `writer.AcquireBrain` is a kernel-local `flock`, records no holder, and cannot observe another host. `TestLocalWriterLockCannotFenceAnotherHost` shows a restored copy of the lock file is acquired successfully while the original holder is alive.

**Proposed rules.** No account is unfrozen until a `FenceReceipt` is `Sufficient`: all three facts are required because each defeats a different old-writer failure and none implies another.

1. `JournalSeal`: `DeletionJournal.Seal` closed the old generation. Stops a cooperating stale writer.
2. `OldInstanceStopped`: the infrastructure provider reports the old instance stopped or terminated. Never a lock file, PID or local check.
3. `OldCredentialsRevoked`: every credential and session the old instance can still use is revoked or denied, including its live cloud role session. Stopping an instance does not invalidate a session token already issued to it.

**Order.** Stop the old instance, revoke its credentials, seal the journal generation, read the journal through the seal, run `ReconcileCustomer` for each account, then unfreeze eligible accounts atomically per account. The new writer starts generation + 1. Sealing after the stop avoids racing a live writer. `RecoveryApplyResult.Consistent` enforces that `Unfrozen` requires a sufficient receipt. There is no manual `status='active'` SQL.

A same-host process restart, where the host does not change, may satisfy the instance-stopped fact with the local writer lock. That is what the lock actually fences, and task50 specifies the narrower case separately.

**Questions for the reviewer.**

- *Mechanism.* **Recommended: this three-fact rule.** The alternative is a distributed generation barrier. It adds a coordination service, which this single-VM deployment does not otherwise need. The journal seal is already the generation barrier for deletion authority.
- *Provider verification.* **Recommended:** the recovery operator role gets read-only instance-state permission so `OldInstanceStopped` is verified through the provider API, not attested by hand.

**Executable specification.** `TestFenceReceiptRequiresEveryFact` (each missing fact refused, all reported), `TestRecoveryResultConsistency`, `TestLocalWriterLockCannotFenceAnotherHost`, and the journal seal scenarios above.

### Proposed configuration fields

All are PROPOSED and unapplied. Config stays JSON per the crosswalk.

| Field | Owner | Meaning |
|---|---|---|
| `operations.lease_seconds` | task44 | Reservation lease; must exceed the longest request deadline plus margin. |
| `operations.reconcile_interval_seconds`, `operations.fence_wait_seconds` | task44 | Reconciler cadence and how long it waits for a brain's commit sections. |
| `storage.staging_budget_bytes`, `storage.max_mutation_stage_bytes`, `storage.operator_headroom_bytes` | task44 | Global staging budget, per-stage ceiling, free-space floor. |
| `deletion_journal.bucket`, `deletion_journal.generation` | task48, task50 | Journal location; generation is set only by recovery apply. |

## Integration requests (for the coordinator; not applied)

Feature workers and this task may not edit shared files. These follow from the proposals above and need a ruling or an owner.

| # | Request | Owner | Why |
|---|---|---|---|
| 1 | **File-first gate versus the journal write method: resolved by renaming, gate untouched.** `internal/gate/filefirst_test.go` flags any non-test call whose selector is named `Append` outside its allowlist. The journal's write method was named `Append` in the task41 and task48 contract text, and the gate matches by name only. An earlier commit on this branch (`1c44bc2`) avoided the false positive by calling through a method value, which evades the gate's matching and was rejected. This pass removes that helper and renames the method `DeletionJournal.AppendDeletion` everywhere (`contracts`, the reference journal, the suite, the fakes and this file). The gate, its `writeCalls` set and its allowlist are unchanged, and `go test ./internal/gate` passes on the current tree. The task41 step 3 and task48 step 2 wording ("Append/ReadThrough") should read `AppendDeletion`/`ReadThrough`/`Seal`. Task48's production adapter must call `AppendDeletion` and must not call a bare `Append` outside the allowlist. | Coordinator, to update the task48 contract wording | The gate file and `tasks.json` are outside task41's write scope. Nothing here weakens the gate. |
| 2 | Core-writer seam for staged writes (Git quarantine objects, or equivalent). | Runtime44 with the writer owner | Decision 1 cannot be implemented without it. |
| 3 | Stop using `meter.Meter.Reserve`'s expired-row release for mutating operations; keep it for read metrics. | Runtime44 (`R-hosted-meter`) | Releasing an expired row discards an unknown canonical outcome. |
| 4 | `DeleteBrain` and `DeleteAccount` fence the brain and resolve its reserved ledger rows as part of purge, in journal order. | Durability48 with runtime44 | Otherwise the reconciler defers those rows forever. |
| 5 | A failed `writer.Flush` rolls back the working tree instead of re-marking paths as touched. | Writer owner | Removes the pending-working-tree case that sends rows to pending review. |
| 6 | Operator command for pending-review rows, registered in `internal/cli/hosted.go`. | Runtime44 with integrator | Decision 2 needs a human exit path. |

## Lock ordering and cancellation

Documented from the current implementation plus the proposed additions. Not a chief-architect-signed invariant.

Writer path:

1. `Gateway.Maintenance` (RWMutex; write-locked only by `Service.Backup`, read-locked by every tool call in `callBound` and by `DeleteBrain`; `DeleteAccount` and `RecoverDeletions` take it only through `DeleteBrain`, and `Export` does not take it).
2. `Gateway.accountLocks[hash(account)]` (per-account mutex; also used by `Export`, `DeleteBrain`, `DeleteAccount`).
3. `Runtime.Mutations` (per-brain mutex; write and forget calls only).
4. Brain commit section, shared side of `BrainFence` (PROPOSED). Covers exactly the canonical write, including `writer.Flush`.
5. `store.Store.Transaction` for `EnterCanonical`, `Finalize` and the other ledger calls. Short, never held across canonical or provider I/O, never nested inside another lock's wait.
6. Canonical disk and provider I/O, outside any database transaction.

Reconciler path (PROPOSED): `Gateway.Maintenance` (read), then the brain's exclusive `BrainFence`, then the canonical checker, then a ledger transaction. It never acquires an account lock or `Runtime.Mutations`. Both transitions it makes are single SQL transactions, so a concurrent same-account `Reserve` is safe: a commit converts held capacity to used, a release only frees capacity. Never acquire an account lock while holding a brain fence.

Deadlock argument: a writer holds the account lock and `Runtime.Mutations` while it may wait for the fence's shared side. The reconciler holds the fence's exclusive side while it waits for the canonical read and a ledger transaction. Neither of those waits on a lock the writer holds, so there is no cycle. A refused or timed-out `Fence` holds nothing and leaves no waiter behind.

Interactions to settle in task44 and task48: `DeleteBrain` must fence the brain and resolve that brain's reserved rows as part of purge, in the order the journal entry requires, or the reconciler defers them forever.

**Known violation, not this task's to fix.** `Gateway.ServeHTTP` (`gateway.go`) holds the connection-bookkeeping map lock `Gateway.mu` across `Pool.Acquire`, which can hit a cold-open disk path. That breaks the rule not to hold the global gateway or pool map lock across disk or provider work. Flagged for task45 (`R-hosted-runtime`); not edited.

Export, delete and recovery calls bypass ordinary metering (`Export`, `DeleteBrain` and `DeleteAccount` take no `meter.Reservation`), which is the existing "bounded capacity independent of ordinary write allowance" behavior. Every `context.Context` passed into a metered path carries the caller's deadline. `Meter.Finish`'s deferred calls use `context.WithoutCancel` with their own bounded timeout so a canceled request still finalizes its reservation. Fence and journal calls take the caller's context and never wait past it.

## File-change handshake

1. Feature task submits `docs/launch/evidence/T23.N/integration-request.md`: desired signature/schema/config, failure semantics, exact shared-file diff and tests. This is a proposal, not worker authority to edit shared files.
2. Integrator41 applies/reviews the needed shared commit (separate scoped commit/PR when necessary) and records its SHA in this file’s frozen interface table. Only integrator allocates migration numbers. Add migrations; do not rewrite applied versions1–3.
3. Feature worker rebases on that commit and proves behavior through its public seam. Integrator wires the feature into the real service promptly, then reruns assembled tests. Task57 closes final assembly; it does not defer all wiring to the end.
4. A changed interface reopens affected dependent receipts. Coordinator regenerates contracts when scope changes, rather than verbally granting a second writer.

## Threshold review

Task41’s reviewer freezes the task43 Hit@5 corpus/scoring and task60 workload/latency/throughput targets before live results. The proposed targets are deliberately explicit so reviewers can adjust them once with rationale before execution. A failed live result never authorizes a worker to edit the threshold.

## Freeze receipt (task41)

This receipt records the design rulings only; it does not mark task41 accepted, approve implementation, or authorize merge.

- **Source and review.** Drafted in worktree branch `hosted/t23.41-20260918` on base `810349ba7c1ed2c05fe34e3892de764a26a4633c`. Chief-architect reviewed revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`; its rulings are recorded in the architecture-review request. This follow-up reconciles the rejected wide commit-section recommendation. Not merged. An independent code review and a coordinator review found defects in earlier drafts; those were fixed and are listed under their decision above.
- **Exact Go interface and SQL revision.** `internal/hosted/contracts/**`, `internal/hosted/contracts/contractstest/**` and `internal/hosted/testhooks/**` in this branch. No SQL applied: the `operations` table is design-only pending review and `store/migrations.go` is unchanged.
- **Storage admission design:** approved conditionally by chief-architect. It remains **BLOCKED** until task44 supplies allocation-based accounting, an OS-enforced staging limit, and the `MaxMutationStageBytes` measurement; none is claimed here.
- **Operation accounting, quiescence and retry-key binding:** approved by chief-architect at PR236 revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`, with the narrow commit-section extent stated above. Proposed schema/migration remains unapplied until implementation and ordinary review.
- **Journal completeness:** approved by chief-architect at PR236 revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`; the existing versioned-bucket substrate remains conditional on task48's live S3 qualification against our bucket, policy and CLI build. **Restore fencing:** the three-fact receipt is approved. Implementation and ordinary review remain outstanding.
- **Provider, model and accounting-unit contract.** `AccountingUnit`, `RegistrationMode` and the `ProviderPin` shape are frozen and independent of the four decisions. The actual provider, model and version remain task42's to verify and pin.
- **Quality and load thresholds.** Not evaluated here. Task43 owns the Hit@5 corpus and task60 owns the workload targets.

These are real dependencies. This worktree does not mark task41 accepted.
