# T23.41 architecture review request

**Requested reviewer:** chief-architect (`docs/launch/hosted-plan.md`, task41 required steps: "With chief-architect, resolve and publish the four bounded design decisions").

**Requester:** headless Claude Code worker, branch `hosted/t23.41-20260918`, base `810349ba7c1ed2c05fe34e3892de764a26a4633c`. The exact revision to review is the commit named in `result.json` `source_sha`.

**This is a request, not a receipt.** No reviewer was available synchronously. Nothing below is approved, and no file in this change marks any decision approved. Until a reply names a reviewer and a revision, task44, 47, 48 and 50 treat all four decisions as blocked, per `docs/launch/hosted-plan.md`: "Unresolved decision means dependent task is blocked, not an agent guess."

## How to read the packet

`docs/launch/hosted-completion/interfaces.md` has the full text under "Proposed decisions". Each decision there lists what an earlier draft got wrong, the proposed rules, and the questions below with a recommendation. Every proposed rule has an executable scenario in `internal/hosted/contracts/contractstest`, and each reference model was mutation-tested (a deliberate defect fails the scenario meant to catch it). The reference models are test-support code. They show the rules are self-consistent and testable. They are not evidence about production code, which does not exist.

## Rulings requested

Reply per line: **approve**, **approve with amendment**, or **reject with required alternative**.

### 1. Storage admission - currently BLOCKED

| Question | Recommendation | Ruling |
|---|---|---|
| Adopt the staged-write design (global staging budget reserved before any stage byte exists, a stager that aborts at its ceiling, admission of measured growth including unpublished growth)? | Approve the design, conditional on the OS-enforced staging limit and allocation accounting below; neither exists in production yet. | |
| Accept the prerequisite core-writer seam (Git quarantine objects, or equivalent) as a shared-seam request under T23.44 step 4? | Approve. Canonical state is a live Git worktree with no temp-and-rename step, so a stage cannot exist without it. | |
| If the seam is rejected: adopt in-place admission with a proven per-mutation ceiling and a write-freezing tripwire instead? | Fallback only. It costs customers the last `MaxMutationStageBytes` of quota and is only as strong as the corner-case proof. | |
| Claim a bound as "hard" only with an OS-enforced size limit (dedicated size-limited filesystem or quota volume under the staging area) plus allocation-based accounting that includes external Git and index writes | Approve. `StageMeter` is a cooperative counter of the bytes its caller reports. No fixture counter proves a hard bound, and until this exists the packet claims an accounting ceiling only. | |
| Statistical growth multiplier | Stays withdrawn. | |

### 2. Operation accounting, quiescence and retry-key binding

| Question | Recommendation | Ruling |
|---|---|---|
| State machine: `pending_review` is a holding phase that holds capacity and exits only through an operator | Approve. | |
| Absence-based release only through a reconciler that holds the brain's exclusive fence across the canonical check and the ledger transition; writers call `EnterCanonical` inside a commit section and stop if the row is no longer reserved | Approve. Lease expiry alone cannot show a writer stopped. | |
| Scope of the fence: in-process, plus host-exclusive brain ownership; other hosts only through decision 4 | Approve. | |
| Commit section wraps the whole `remember` handler including the provider call | Accept. A slow embedding only defers that brain's reconciliation. | |
| Retry key bound to a payload fingerprint; reuse with different content fails in every phase | Approve. | |
| Working-tree state pending after a failed flush is Unknown, never Absent; task44 proposes a writer change so a failed flush rolls back | Approve. | |
| Migration version 4 reserved for the `operations` table, applied only after approval | Approve. | |

### 3. Deletion journal

| Question | Recommendation | Ruling |
|---|---|---|
| Journal completeness comes from the journal alone (hash chain, gapless sequence, generation seal), never the local control database | Approve. Restore replaces the control database with a snapshot copy that cannot know later entries. | |
| Substrate: the existing versioned AWS object bucket with conditional create; no new service | Approve, conditional on task48 qualifying conditional create against our bucket, policy and CLI build on an authorized disposable resource. AWS documents the feature (`s3-conditional-write-source-check.md`); no live request was made in this task. | |
| Adapter obligations from the documented semantics: current versions and delete markers protected (no `DeleteObject` or `DeleteObjectVersion`, conditional header required by bucket policy); no lifecycle rule on `deletion-journal/` and no snapshot-purge access to it; `created=false` only for a definitive 412, other failures are errors; an ambiguous put is accepted only if the exact bytes read back | Approve. A conditional header alone does not make a key immutable on a versioned bucket. | |
| IAM: service role gets put, get and list under `deletion-journal/` only, no delete or delete-version | Approve. | |
| Retention beyond bucket versioning (Object Lock or compliance mode) | Not decided here. SPEND-gate territory. | |
| Watermark read before the backup copies data; recovery replays after it; purge idempotent | Approve. | |

### 4. Restore fencing

| Question | Recommendation | Ruling |
|---|---|---|
| Unfreeze requires all three facts: journal generation sealed, provider-verified old-instance stop, revocation of every old credential and session | Approve. Each defeats a different failure and none implies another. | |
| Versus a distributed generation barrier | Reject the alternative. It adds a coordination service this deployment does not otherwise need. | |
| Recovery operator role gets read-only instance-state permission so the stop is verified through the provider | Approve. | |
| Local writer lock as a fencing option for cross-host restore | Dropped. `TestLocalWriterLockCannotFenceAnotherHost` shows it cannot work. | |

## Already concrete and not part of this review

Billing truth and closure, backup manifest v2 (except its journal watermark), the recovery plan/apply shape (except the journal and fence fields), telemetry, provider pin, accounting units, registration mode and the fault-barrier package are frozen, compiled and covered by tests. They can be reviewed as ordinary code.

Backup manifest v2 changed in this pass: it now carries the control database artifact (relative path, length, SHA256), each non-empty brain's bundle heads, and a `Validate` method, because task49 step 1 and its roundtrip acceptance need them (independent audit D2). Nothing imports the type and task49 has not started. It reserves no incremental-backup fields. `Validate` proves structure and internal consistency only; it does not prove that files exist, that checksums match, that the inventory equals the control database's, or that a manifest is authentic.

The journal write method is `DeletionJournal.AppendDeletion`, not `Append`, so the file-first gate (`internal/gate`) stays unchanged and passes. Task41 step 3 and task48 step 2 contract wording now follows the rename.

## Not verified in this task

- S3 conditional create against the real service, our bucket policy and the CLI/SDK build (decision 3). Only the AWS documentation was checked, by the coordinator. Live qualification needs an authorized disposable resource and was not attempted.
- The worst-case physical growth of one mutation on the real runtime (decision 1, `MaxMutationStageBytes`). No measurement was taken; the packet requires task44 to produce one in allocated bytes.
- Any OS-enforced size limit for the staging area (decision 1). The reference staging gate and `StageMeter` are accounting only and prove no hard bound.
- The actual `remember` handler's commit-section timing under provider load (decision 2).
- Any production code path. No migration is applied and nothing under `internal/hosted/service` or `internal/cli` imports the contracts.
