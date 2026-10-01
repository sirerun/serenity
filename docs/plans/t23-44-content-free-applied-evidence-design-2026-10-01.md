# T23.44 content-free applied evidence design

Date: 2026-10-01  
Baseline: hosted remember after-flush routing at `5e6413aef237138068c5f7c5f3397c055e13c675`.  
Status: design recommendation only. No implementation, checker activation, forget routing, schema change, or architecture ratification is included.

## Finding

The memory source already provides positive evidence while the fact exists: `MemoryFactPayload.CanonicalOperationID` is written with the trusted ledger ID, and T23.44 now commits that fact before provider/index work. The current operation-only `MemoryExpiryPayload` is not proof that the fact was ever applied. `CancelRemoteOperation` uses it to prevent a future remember, and `eraseFact` writes the same shape after forgetting an existing fact. Both are format-version-2 expiry records with `OperationKey`, no `TargetSHA256`, and an `ExpiredAt`. The projection exposes both cases through `OperationCancellation`; its shape does not distinguish pre-write cancellation from post-application erasure.

`rewriteForgottenPath` removes the target fact source directory from every Git ref, expires reflogs, and prunes unreachable objects. A checker that later finds neither the fact nor a separate retained applied proof cannot infer whether the operation never landed or landed and was erased. It must report Unknown. A cancellation marker alone must never be interpreted as committed or absent for an entered operation.

## Minimal tracker proposal

Add one separate, versioned canonical record per successfully applied hosted operation. Its payload should contain only:

```json
{"format_version":1,"record_type":"memory_operation_evidence","operation_id":"<trusted opaque ledger ID>","proof":"applied"}
```

The writer creates it only after the cancellation check passes and it has successfully rendered the matching fact whose `CanonicalOperationID` equals the trusted operation ID. Commit the fact and this proof record in the same `SubmitAndFlush` Git commit. The existing post-flush callback remains the signal to the gateway; the tracker is persisted proof for later reconciliation. Do not include the client key, fact/provenance/entity, fact SHA, commit SHA, expiry reason, customer/account data, or timestamp. A source-object digest used internally by the existing content-addressed store is not part of the payload and must never be copied into evidence or logs.

Keep the tracker outside the individual fact source directory. `rewriteForgottenPath` filters the exact fact path, so a separate path survives the selective history rewrite while the fact blobs are removed. The marker must be read from the current committed Git tree by a future checker; `SourceStore.All()` reads the working tree and cannot establish this proof. A fact present in HEAD with the matching internal ID remains sufficient positive evidence for legacy records that predate the tracker. After a forget rewrite, only a well-formed applied tracker can prove a legacy operation landed; missing or unreadable evidence is Unknown.

`CancelRemoteOperation` remains a separate cancellation signal. With the current serialized writer, cancel-before-remember yields only the cancellation record; the later remember sees it and writes neither fact nor applied tracker. Remember-before-cancel yields the committed fact and applied tracker first; a later cancellation/forget does not remove the tracker. If the writer sees an applied tracker but no corresponding ledger operation ID, or conflicting/malformed records for one ID, it returns Unknown rather than choosing a state. Cancellation alone never creates an applied tracker.

The initial tracker should be append-only and have one accepted positive proof state (`applied`). Do not add a `forgotten` state: fact erasure changes payload visibility, not whether the operation was applied. This avoids overwriting positive proof with an ambiguous cancellation state. If a future cancellation protocol needs positive proof of “never applied” for an entered operation, it requires a separate, fenced negative-proof transition; the current cancellation record is insufficient.

## Parsing, checker, and backward compatibility

Reserve a new source record kind such as `memory_operation_evidence` with its own format version. Do not overload `MemoryExpiryPayload` format 2: that version already means cancellation, and treating old records as applied would create false commits. Preserve existing fact format 1 and expiry formats 1/2 unchanged. The new decoder must validate exact record type/version, the bounded opaque ID, and the single allowed proof value; reject unknown fields, duplicate keys, trailing values, malformed JSON, and unsupported versions. Treat a corrupt or unsupported tracker as checker error/Unknown, never as absence. If multiple records map to one operation ID, accept only identical canonical applied records; conflict is Unknown.

Use the `OperationRecord.ID` from the control ledger as the lookup key; never trust a client-supplied key. The checker reads Git `HEAD` to establish landed evidence and returns a sanitized marker/HEAD reference. It must not treat working-tree files as positive evidence. ADR017 currently requires an absent verdict to account for uncommitted files and touched queue state. Preserve that safety rule by returning Unknown for dirty/touched state, using the guarded queue’s pending-path state or another separately reviewed mechanism; do not call a working-tree source a landed operation. Enable reconciliation only after all canonical writers/flushes for the brain are protected by the fence/guard and that pending-state check is authoritative.

The current gateway’s `EvidenceCommitted.Ref` and replay path use `fact:<SHA>`. That reference is outside the proposed tracker but remains a content identifier in the control ledger after forget. Before adopting the tracker, decide whether the ledger should keep the fact SHA for stable response replay or move to a content-free operation/HEAD reference and define what a committed same-key retry returns after forget. Do not silently claim the overall evidence chain is hash-free while that existing behavior remains.

## Privacy, deletion, and physical growth

ADR019 accepts an expiry event as an audit record and requires fact bytes, fact history, and indexes to be purged; hosted brain/account deletion removes the active brain directory, while backup copies follow the approved expiry/deletion lifecycle. ADR017 approves operation reconciliation using an internal-ID marker in canonical history and the narrow commit section, but it does not explicitly decide whether an applied marker may persist after a fact is forgotten, what identity-linked retention is acceptable, or how long markers live. Because the operation ID can be mapped to an account through the ledger, it is pseudonymous and linkable even without fact content.

This needs an explicit architecture ruling or ADR amendment before implementation: approve or reject retaining the opaque applied-operation marker through selective fact-history purge; define whether it is operational metadata outside the forgotten payload; state when it may be garbage-collected; and specify that brain/account deletion and the existing backup/export lifecycle remove it. No such approval is inferred here.

If retained for each operation, the tracker adds O(successful hosted remembers) records and Git objects. Count its allocated bytes in brain storage/quota and the task44 stage budget, including temporary/history-rewrite growth; do not exempt it because the payload is small. A bounded retention policy could prune only after the ledger row can no longer be reconciled and the required backup window has ended, but that requires cross-store lifecycle coordination and must not delete proof for `reserved` or `pending_review` rows. ADR017’s physical-bound admission remains blocked until its allocation accounting and OS-enforced staging limit are qualified.

## Bounded acceptance tests for a later implementation

1. New hosted remember writes the fact and applied tracker in the same HEAD commit, then signals AfterFlush. A failed/canceled commit emits no applied callback and leaves the ledger unresolved.
2. Cancel-before-remember writes cancellation only, blocks the remember before `BeforeCommit`/fact creation, and produces no applied tracker. Cancel-after-success preserves the applied tracker.
3. Forget removes fact bytes and every fact-history blob but preserves the tracker in rewritten HEAD/history; no fact text, provenance, entity, source SHA, user key, account data, or reason appears in the tracker.
4. Brain/account deletion removes tracker files from the active brain. Export and backup tests verify their current-tree/history and expiry rules include the tracker consistently.
5. Checker uses committed HEAD and the ledger’s internal operation ID. A present fact with a matching ID or an applied tracker is Landed; cancellation-only, missing legacy evidence after purge, dirty/touched matching state, unknown format, malformed/conflicting markers, unreadable HEAD, and mismatched ledger mapping are Unknown. Absence is releasable only with separately approved positive proof that the writer never entered or a future explicit negative-proof protocol.
6. Test strict parser rejection for unknown fields/versions, duplicate keys, duplicate IDs with conflict, invalid IDs/states, and trailing data. Malformed reserved records must fail closed.
7. Measure tracker storage in bytes and Git object/pack growth under task44’s staging/admission limits; verify pruning cannot remove proof while a pending/reserved ledger row depends on it.

This proposal is not an implementation plan approval, T23.44 acceptance, or permission to enable reconciliation. The privacy-retention decision and any change to ledger replay references remain open.
