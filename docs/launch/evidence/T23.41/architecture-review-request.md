# T23.41 architecture review request

**Requested reviewer:** chief-architect (`docs/launch/hosted-plan.md`, task41 required steps: "With chief-architect, resolve and publish the four bounded design decisions").

**Requester:** headless Claude Code worker, branch `hosted/t23.41-20260918`, base `810349ba7c1ed2c05fe34e3892de764a26a4633c`. The reviewed revision was `218d7234d9abea5964f9d4640d1bfffe5c9f8087`; this follow-up records the rulings and reconciles the corrected fence scope.

**Review receipt:** chief-architect reviewed revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`; the ruling was posted by chief to [PR236](https://github.com/sirerun/serenity/pull/236#issuecomment-5746461189) on 2026-09-20 and recorded in `hq/receipts/2026-09-19.md`. The decision tables below record those rulings. This revision reconciles the one rejected recommendation (decision 2's commit-section extent). The ruling does not approve implementation or merge, and does not lift decision 1's explicit storage BLOCKED state.

## How to read the packet

`docs/launch/hosted-completion/interfaces.md` has the full text under "Proposed decisions". Each decision there lists what an earlier draft got wrong, the proposed rules, and the questions below with a recommendation. Every proposed rule has an executable scenario in `internal/hosted/contracts/contractstest`, and each reference model was mutation-tested (a deliberate defect fails the scenario meant to catch it). The reference models are test-support code. They show the rules are self-consistent and testable. They are not evidence about production code, which does not exist.

## Rulings requested

Reply per line: **approve**, **approve with amendment**, or **reject with required alternative**.

### 1. Storage admission - currently BLOCKED

| Question | Recommendation | Ruling |
|---|---|---|
| Adopt the staged-write design (global staging budget reserved before any stage byte exists, a stager that aborts at its ceiling, admission of measured growth including unpublished growth)? | Approve the design, conditional on the OS-enforced staging limit and allocation accounting below; neither exists in production yet. | Approved conditionally by chief-architect; storage admission remains BLOCKED until task44 supplies both prerequisites and `MaxMutationStageBytes`. |
| Accept the prerequisite core-writer seam (Git quarantine objects, or equivalent) as a shared-seam request under T23.44 step 4? | Approve. Canonical state is a live Git worktree with no temp-and-rename step, so a stage cannot exist without it. | Approved by chief-architect. |
| If the seam is rejected: adopt in-place admission with a proven per-mutation ceiling and a write-freezing tripwire instead? | Fallback only. It costs customers the last `MaxMutationStageBytes` of quota and is only as strong as the corner-case proof. | Approved as fallback only by chief-architect. |
| Claim a bound as "hard" only with an OS-enforced size limit (dedicated size-limited filesystem or quota volume under the staging area) plus allocation-based accounting that includes external Git and index writes | Approve. `StageMeter` is a cooperative counter of the bytes its caller reports. No fixture counter proves a hard bound, and until this exists the packet claims an accounting ceiling only. | Approved by chief-architect; the hard-bound conditions remain unmet. |
| Statistical growth multiplier | Stays withdrawn. | Confirmed withdrawn by chief-architect. |

### 2. Operation accounting, quiescence and retry-key binding

| Question | Recommendation | Ruling |
|---|---|---|
| State machine: `pending_review` is a holding phase that holds capacity and exits only through an operator | Approve. | Approved by chief-architect. |
| Absence-based release only through a reconciler that holds the brain's exclusive fence across the canonical check and the ledger transition; writers call `EnterCanonical` inside a commit section and stop if the row is no longer reserved | Approve. Lease expiry alone cannot show a writer stopped. | Approved by chief-architect. |
| Scope of the fence: in-process, plus host-exclusive brain ownership; other hosts only through decision 4 | Approve. | Approved by chief-architect. |
| Commit section covers the canonical write only, from its first canonical byte through `writer.Flush` returning; embedding/provider calls happen before the section | Approve this narrow scope. Reject wrapping the whole `remember` handler across provider I/O; a slow provider must not defer reconciliation. | Approved by chief-architect; this replaces the prior wide-scope recommendation. |
| Retry key bound to a payload fingerprint; reuse with different content fails in every phase | Approve. | Approved by chief-architect. |
| Working-tree state pending after a failed flush is Unknown, never Absent; task44 proposes a writer change so a failed flush rolls back | Approve. | Approved by chief-architect. |
| Migration version 4 reserved for the `operations` table, applied only after approval | Approve. | Approved by chief-architect. |

### 3. Deletion journal

| Question | Recommendation | Ruling |
|---|---|---|
| Journal completeness comes from the journal alone (hash chain, gapless sequence, generation seal), never the local control database | Approve. Restore replaces the control database with a snapshot copy that cannot know later entries. | Approved by chief-architect. |
| Substrate: the existing versioned AWS object bucket with conditional create; no new service | Approve, conditional on task48 qualifying conditional create against our bucket, policy and CLI build on an authorized disposable resource. AWS documents the feature (`s3-conditional-write-source-check.md`); no live request was made in this task. | Approved conditionally by chief-architect; task48 live S3 qualification remains required. |
| Adapter obligations from the documented semantics: current versions and delete markers protected (no `DeleteObject` or `DeleteObjectVersion`, conditional header required by bucket policy); no lifecycle rule on `deletion-journal/` and no snapshot-purge access to it; `created=false` only for a definitive 412, other failures are errors; an ambiguous put is accepted only if the exact bytes read back | Approve. A conditional header alone does not make a key immutable on a versioned bucket. | Approved by chief-architect. |
| IAM: service role gets put, get and list under `deletion-journal/` only, no delete or delete-version | Approve. | Approved by chief-architect. |
| Retention beyond bucket versioning (Object Lock or compliance mode) | Not decided here. SPEND-gate territory. | |
| Watermark read before the backup copies data; recovery replays after it; purge idempotent | Approve. | Approved by chief-architect. |

### 4. Restore fencing

| Question | Recommendation | Ruling |
|---|---|---|
| Unfreeze requires all three facts: journal generation sealed, provider-verified old-instance stop, revocation of every old credential and session | Approve. Each defeats a different failure and none implies another. | Approved by chief-architect. |
| Versus a distributed generation barrier | Reject the alternative. It adds a coordination service this deployment does not otherwise need. | Approved by chief-architect. |
| Recovery operator role gets read-only instance-state permission so the stop is verified through the provider | Approve. | Approved by chief-architect. |
| Local writer lock as a fencing option for cross-host restore | Dropped. `TestLocalWriterLockCannotFenceAnotherHost` shows it cannot work. | Confirmed dropped by chief-architect. |

## Already concrete and not part of this review

Billing truth and closure, backup manifest v2, recovery plan/apply, telemetry, provider pin, accounting units, registration mode and the fault-barrier package have frozen, compiled signatures and contract tests. Architect approval of the journal and restore-fence designs covers their fields; implementation and ordinary review remain open.

Backup manifest v2 changed in this pass: it now carries the control database artifact (relative path, length, SHA256), each non-empty brain's bundle heads, and a `Validate` method, because task49 step 1 and its roundtrip acceptance need them (independent audit D2). Nothing imports the type and task49 has not started. It reserves no incremental-backup fields. `Validate` proves structure and internal consistency only; it does not prove that files exist, that checksums match, that the inventory equals the control database's, or that a manifest is authentic.

The journal write method is `DeletionJournal.AppendDeletion`, not `Append`, so the file-first gate (`internal/gate`) stays unchanged and passes. Task41 step 3 and task48 step 2 contract wording now follows the rename.

## Not verified in this task

- S3 conditional create against the real service, our bucket policy and the CLI/SDK build (decision 3). Only the AWS documentation was checked, by the coordinator. Live qualification needs an authorized disposable resource and was not attempted.
- The worst-case physical growth of one mutation on the real runtime (decision 1, `MaxMutationStageBytes`). No measurement was taken; the packet requires task44 to produce one in allocated bytes.
- Any OS-enforced size limit for the staging area (decision 1). The reference staging gate and `StageMeter` are accounting only and prove no hard bound.
- The actual `remember` handler's commit-section timing under provider load (decision 2).
- Migration v4 and the operation-table constraints are implemented on this branch, with local tests. The migration does not implement the production operation ledger. Nothing under `internal/hosted/service` or `internal/cli` imports the operation contract yet.
- The latest multi-package race suite passed for store, service, contracts, contractstest, testhooks and the file-first gate. Ordinary code review and PR CI remain pending.
