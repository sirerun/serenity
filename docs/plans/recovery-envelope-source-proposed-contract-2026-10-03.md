# Recovery envelope v1: source-grounded pure component contract

**Status:** proposed source contract for independent review; not a frozen implementation assignment. Baseline is `66acb80f5a38fc00cd568f3d390b61167c587584`. Scope is only a pure, bounded, canonical encoder/decoder/validator/hash for an immutable `ELIGIBLE` or `FROZEN_ONLY` envelope. It does not implement planning, provider calls, approvals, pinning, persistence, admission, recovery effects, or CLI behavior. No source/API ownership is claimed by this document.

## Source preflight and corrections

| Proposal assumption | Landed source | Contract correction |
| --- | --- | --- |
| `SnapshotBuildSHA` is a binary SHA-256 and schema version is text | `contracts.SourceRef` has `BuildSHA string` and `SchemaVersion int`; manifest validation requires a nonempty whitespace-free build token and positive schema integer | Wire fields are `source_build_token` (opaque bounded UTF-8 token) and `source_schema_version` (positive integer). Never relabel or hash the build token as a binary digest. |
| Full inventory is the list of account IDs | `backup.SnapshotInspection` returns account `{ID, Status}` rows and `contracts.BrainArtifact` rows. The exact raw manifest digest covers the complete manifest, including brain artifact metadata and refs; inspection additionally verifies artifact bytes | Bind sorted account ID/status rows and an explicitly named verified brain inventory projection/digest. State which properties are bound by raw manifest SHA versus inspection-derived inventory. Do not call account IDs alone “full inventory.” |
| Snapshot cut is a deletion watermark | `contracts.DeletionWatermark` is a legacy resume point; zero means before the first generation-1 object. `contracts.JournalPosition` carries positive `ActiveGeneration`, `LastObjectInGeneration` (which can be zero for an empty active head), `PredecessorSeal`, and genesis/successor identity | Store raw manifest watermark `M_w`, authority-selected positive logical cut `C`, and prior last seal `H` in separate fields and separate wire values. Never turn zero `C` into generation zero or infer a seal. |
| Watermark-only ancestry APIs authenticate any cut | Existing `recovery.ReadAncestry` and `RecoveryEvidenceVerifier.VerifyJournalPrefix` are watermark-shaped; no source callable API proves a positive empty genesis/successor cut with issuance/allocation identity | Require an additive journal-owner proof binding the entire `C`, `M_w`, exact manifest digest `M`, stable journal-store identity, and authenticated genesis/successor root. Until then this envelope can be hashed as data, but cannot establish ancestry or readiness. |
| `EvidenceRef` is a shared source contract | No `contracts.EvidenceRef` exists on this baseline | This pure component may define a private recovery-local reference wire shape below. It is an opaque integrity binding only, not a source API, trusted evidence constructor, signature, or proof. Do not edit shared contracts in this scope. |
| Plan artifact hash equals contract plan hash | RCP source has separate `LegacyPlanArtifactHash` and `RecoveryContractPlanHash`; `VerifiedEligiblePlan` is private-fielded and produced only after artifact create/reload and contract hashing | Preserve all three domains: legacy artifact hash, contract-plan hash, and outer envelope hash. Never substitute one for another. An outer hash or reference cannot mint `VerifiedEligiblePlan`. |

Relevant existing shapes are `backup.SnapshotInspection{ManifestSHA256, Source, JournalWatermark, Accounts, Brains, VerifiedArtifactCount, DeclaredArtifactBytes}`, `backup.SnapshotAccount{ID, Status}`, `contracts.SourceRef{BuildSHA, SchemaVersion}`, `contracts.BrainArtifact{ID, ArtifactRef, Empty, Heads}`, and `contracts.JournalPosition` above. `ManifestSHA256` is computed over the exact raw `manifest.json` bytes verified by `InspectSnapshot`; never marshal a decoded manifest to reproduce `M`. `InspectSnapshot` is read-only and does not authenticate a writer or determine account eligibility.

## Authority boundary

All APIs in this component are pure data operations. Validation, canonical encoding, decoding, and SHA-256 computation can establish only that bytes have the declared canonical shape and that values are internally consistent. They never mint or deserialize authority: no `Verified*` token, eligible-plan token, plan approval, provider truth, writer identity, snapshot pin, journal ancestry proof, fence, or READY state. `ELIGIBLE` can contain an already owner-verified plan projection, but only an owner-controlled factory may construct that private token after reopening and verifying the artifact and source evidence. A serialized envelope cannot recreate the token.

Every source-derived field must later be projected from owner-controlled verified objects. Reject caller-supplied booleans such as `verified`, `approved`, `pinned`, `fenced`, or `ready`; reject arbitrary adapters whose only output is a reference or digest. Integrity hashes and references identify data, not authority. Global epoch approval, epoch records/receipts, effect evidence, and activation evidence are explicitly outside this envelope to keep the payload acyclic and avoid conflating plan-time facts with later authority.

## Version 1 canonical wire schema

The only canonical format is UTF-8 JSON encoded from private, ordered structs and sorted slices; no maps, interface-valued fields, implicit Go struct serialization, or `time.Time` JSON encoding. Field order below is normative, and each tag is exact. The outer hash is SHA-256 of the canonical payload bytes prefixed by the ASCII domain `serenity.recovery-envelope.v1\x00`; its own digest is not in the payload. The decoder accepts only these bytes: it rejects duplicate keys, unknown keys, trailing values/bytes, invalid UTF-8, noncanonical re-encoding, unsupported versions, and malformed union arms. Optional union arms are absent (not `null`) when inapplicable; all other fields are required, including empty arrays. Empty array and absent field are never interchangeable.

```go
type envelopeV1Wire struct {
    FormatVersion int `json:"format_version"` // exactly 1
    Kind string `json:"kind"`                 // exactly ELIGIBLE or FROZEN_ONLY
    PlanRef string `json:"plan_ref"`
    SnapshotPinID string `json:"snapshot_pin_id"`
    ReservationVersion uint64 `json:"reservation_version"`
    ManifestSHA256 string `json:"manifest_sha256"` // exact raw manifest digest M
    ManifestJournalWatermark watermarkV1Wire `json:"manifest_journal_watermark"` // M_w
    SourceBuildToken string `json:"source_build_token"`
    SourceSchemaVersion int `json:"source_schema_version"`
    SnapshotAccountInventory []accountInventoryV1Wire `json:"snapshot_account_inventory"`
    SnapshotBrainInventory brainInventoryV1Wire `json:"snapshot_brain_inventory"`
    VerifiedArtifactCount int `json:"verified_artifact_count"`
    DeclaredArtifactBytes int64 `json:"declared_artifact_bytes"`
    SnapshotInventorySHA256 string `json:"snapshot_inventory_sha256"`
    PlanApprovalRef evidenceRefV1Wire `json:"plan_approval_ref"`
    PlanApprovalDigest string `json:"plan_approval_digest"`
    PlanApprovalNonce string `json:"plan_approval_nonce"`
    PlanApprovalExpiresAt string `json:"plan_approval_expires_at"` // canonical UTC RFC3339Nano
    ActivationAllowlist []string `json:"activation_allowlist"` // sorted, unique, may be empty
    OperationID string `json:"operation_id"`
    OldWriter writerV1Wire `json:"old_writer"`
    JournalStoreID string `json:"journal_store_id"`
    SnapshotCut journalPositionV1Wire `json:"snapshot_cut"` // C; positive generation even at empty head
    LastSealed watermarkV1Wire `json:"last_sealed"` // H; zero only where source semantics allow
    PrefixEvidenceRef evidenceRefV1Wire `json:"prefix_evidence_ref"`
    Eligible *eligibleArmV1Wire `json:"eligible,omitempty"` // iff kind ELIGIBLE
    Frozen *frozenArmV1Wire `json:"frozen,omitempty"`       // iff kind FROZEN_ONLY
}
```

The private nested wire structs have these exact ordered fields and tags:

```go
type watermarkV1Wire struct {
    Generation int64 `json:"generation"`
    SequenceID int64 `json:"sequence_id"`
    EntryHash string `json:"entry_hash"`
}
type journalPositionV1Wire struct {
    ActiveGeneration int64 `json:"active_generation"`
    LastObjectInGeneration watermarkV1Wire `json:"last_object_in_generation"`
    PredecessorSeal watermarkV1Wire `json:"predecessor_seal"`
    GenesisIssuanceID string `json:"genesis_issuance_id"`
    SuccessorAllocationID string `json:"successor_allocation_id"`
}
type accountInventoryV1Wire struct {
    AccountID string `json:"account_id"`
    Status string `json:"status"`
}
type brainInventoryV1Wire struct {
    Count int `json:"count"`
    CanonicalDigest string `json:"canonical_digest"`
}
type evidenceRefV1Wire struct {
    Authority string `json:"authority"`
    RecordID string `json:"record_id"`
    Version string `json:"version"`
}
type writerV1Wire struct {
    Provider string `json:"provider"`
    ScopeRef string `json:"scope_ref"`
    WriterRef string `json:"writer_ref"`
    BootRef string `json:"boot_ref"`
    JournalStoreID string `json:"journal_store_id"`
    Generation int64 `json:"generation"`
}
type eligibleArmV1Wire struct {
    LegacyPlanArtifactHash string `json:"legacy_plan_artifact_hash"`
    RecoveryContractPlanHash string `json:"recovery_contract_plan_hash"`
    ContractPlan contractPlanV1Wire `json:"contract_plan"`
    EligibleAccountIDs []string `json:"eligible_account_ids"`
}
type contractPlanV1Wire struct {
    SourceSnapshot string `json:"source_snapshot"`
    JournalWatermark watermarkV1Wire `json:"journal_watermark"`
    Generation int64 `json:"generation"`
    ProviderTruthAt string `json:"provider_truth_at"` // canonical UTC RFC3339Nano
    Accounts []string `json:"accounts"`
}
type frozenArmV1Wire struct {
    Dispositions []frozenDispositionV1Wire `json:"dispositions"`
}
type frozenDispositionV1Wire struct {
    AccountID string `json:"account_id"`
    Disposition string `json:"disposition"` // exact closed enum; reason is opaque bounded code
    ReasonCode string `json:"reason_code"`
}
```

`ContractPlan` deliberately omits `contracts.RecoveryPlan.PlanHash`; `RecoveryContractPlanHash` is a separate field recomputed using the already specified RCP v1 canonical hash projection. The complete contract-plan identity remains distinct from `LegacyPlanArtifactHash`. The outer `RecoveryEnvelopeHash` binds both identities and all other fields, but does not validate the source artifact by itself. The frozen arm has neither hash nor contract plan and cannot be converted into an empty eligible plan.

`SnapshotInventorySHA256` is the SHA-256 of a separate domain-separated canonical inventory payload, version 1, whose fields in order are `manifest_sha256`, `source_build_token`, `source_schema_version`, `accounts`, `brains`, `verified_artifact_count`, `declared_artifact_bytes`. `accounts` is the complete sorted sequence of `{account_id,status}` rows. `brains` contains the inspected brain count and a digest over the canonical ordered projection of every `BrainArtifact`: ID, Empty, relative path, length, artifact SHA-256, and sorted heads `{ref,object_id}`. Exact artifact bytes and all manifest metadata are already committed by raw `M`; this extra digest binds the inspection result to the envelope and catches projection mismatch. It does not imply a brain restore verification beyond what the backup owner establishes.

The three journal values are never merged: `manifest_journal_watermark` is exact M_w from the snapshot; `snapshot_cut` is C from the journal authority; `last_sealed` is H as a separate prior seal. `C.ActiveGeneration` must be positive. Generation one requires a valid nonempty genesis issuance ID, empty successor ID, and zero predecessor seal. Later generations require a valid successor allocation ID, empty genesis ID, and a valid predecessor seal at generation minus one. `LastObjectInGeneration` may be the exact zero watermark for an empty positive generation; otherwise it must be a valid watermark in `ActiveGeneration`. These structural checks match `deletion.validateJournalPosition`, but the pure component does not authenticate the fields. In particular, it may not use the legacy zero watermark to represent a positive empty cut.

The existing contract plan's `JournalWatermark` can represent M_w but cannot retain the additional C issuance/allocation identity. Its legacy field stays M_w and must not be repurposed to store C or H. A future owner-controlled ancestry result must carry C in full alongside M_w and M, so the envelope's cut is not inferred from the contract plan watermark.

## Validation, size bounds, and deterministic encoding

Bounds are checked before allocating slices, decoding nested payloads, or hashing. Reject rather than truncate, normalize, sort on behalf of caller, or silently fill defaults. Proposed hard limits for v1 are: at most 100 eligible IDs; at most 10,000 snapshot accounts and 10,000 brains; at most 10,000 dispositions; at most 1 MiB combined encoded evidence-reference fields and approval proof bindings represented here; at most 4 MiB canonical envelope; and at most 4 MiB canonical contract-plan projection. The source manifest reader retains its existing 32 MiB bound. Checked arithmetic is required for aggregate sizes and byte counts. `DeclaredArtifactBytes` must be nonnegative and within the already configured inspection cap. Counts must equal actual slice lengths.

Account and brain IDs are validated against the relevant existing backup manifest validators, not a new weaker approximation; snapshot account statuses are restricted to the source enum `active`, `deleted`, `deleting`, `restore_pending`. Account rows, brain rows/heads, allowlist, plan accounts, eligible IDs, and dispositions must be strictly sorted and unique by their specified key. Every eligible/disposition ID must be in the complete snapshot account inventory; frozen dispositions cover each snapshot account exactly once. `ELIGIBLE` requires a nonempty plan account set, exact equality between eligible IDs and contract-plan accounts and the authorized subset, and no frozen arm. `FROZEN_ONLY` requires an empty eligible scope and exactly the frozen arm; it has no contract plan. Empty inventories are allowed only if the source snapshot validator permits them; do not invent an eligible ID to satisfy `contracts.RecoveryPlan.Validate`.

All hash fields are lowercase 64-character SHA-256 hex. Source build token is nonempty, valid UTF-8, contains no Unicode whitespace, and is at most 256 bytes; it is not hex-validated. `source_schema_version` and all generations are positive bounded integers. Plan reference, pin ID, operation ID, writer components, evidence reference components, nonce, digest, and reason code receive field-specific ASCII/UTF-8 length and control-character checks in the implementation contract; no values are trimmed or case-folded. Expiry and provider observation timestamps must parse and reformat byte-for-byte as UTC RFC3339Nano (`Z`, not a numeric offset). A zero journal watermark is encoded as `{ "generation":0, "sequence_id":0, "entry_hash":"" }`; any partially zero watermark is rejected.

The decoder must enforce strict JSON before constructing the typed value: UTF-8, duplicate-key rejection at every object depth, unknown-field rejection, one top-level object only, no trailing non-whitespace bytes, integer-only numeric fields without exponent/fraction, required-field presence, and exact canonical byte comparison after re-encoding. Since ordinary `encoding/json` silently accepts duplicate keys and unknown fields by default, a token-level duplicate/unknown scanner or equivalent strict parser is required. No general-purpose map is used for the canonical payload. Independent golden tests pin exact canonical bytes and digests, including both union arms, empty arrays, zero watermark, positive empty-head position, and each distinct journal identity field.

## Owner amendments required before authority-bearing integration

1. **Journal owner:** add a verified ancestry proof that binds `M`, M_w, the entire positive C (including genesis issuance or successor allocation), H, and stable store identity under the journal owner's lock and authenticated root. Existing watermark-only methods cannot establish the empty-head successor/genesis case. Proof lifetime, replay resistance, and lock identity must be explicit; no public boolean or caller-selected root.
2. **Backup owner:** expose a private verified snapshot/pin handle that allows reopening the exact pinned bytes and deriving `SnapshotInspection` without trusting caller fields. The envelope pure codec consumes projections only; only the backup owner establishes pin/inspection authority.
3. **Approval/evidence owner:** define actual immutable, bounded, verifiable plan-approval material or a typed verified token. The local evidence reference shape above merely binds opaque strings. A digest/ref alone is not approval.
4. **Writer/runtime owner:** supply authenticated writer/store identity and source-derived generation. Envelope fields and hashes do not prove old-writer identity or fencing.

Until these amendments are implemented and independently reviewed, the pure envelope may be validated and hashed as an inert canonical value only. It cannot be persisted as READY or used for admission or effects. This is a source-backed gap, not a request for this component to fabricate adapters.

## Independent verification plan for a later frozen source task

The later pure component task should have exact new-file ownership for its private wire types, canonical encoder, strict decoder, validation, hash domains, and tests only. Tests should use source-shaped values and independently assembled golden JSON/digests rather than calling the production encoder to produce expected values. Cover field order/tags; source token versus numeric schema version; raw manifest digest versus inventory digest; account statuses and brain projection; M, M_w, C and H separation; positive empty genesis/successor cuts; both union arms and no cross-arm fields; independent artifact/contract/envelope hash domains; strict duplicate/unknown/trailing/noncanonical rejection; UTF-8 and UTC canonicality; bounds and overflow; context cancellation where exposed; deep-copy/slice alias behavior at typed API boundaries; and private reflection/decoder safety. At least two behavioral mutants must compile and fail assertions (for example, substitute the legacy artifact hash for contract hash; erase C successor identity; omit account status from inventory digest; encode source schema as string). Compile failures do not count as RED. No test claims can establish authority, provider truth, durable pinning, crash safety, or readiness.

## Concrete preflight mismatches

The proposal's string `SnapshotSchemaVersion` is incompatible with source `contracts.SourceRef.SchemaVersion int`. Its `SnapshotBuildSHA` label overstates source `BuildSHA`, which is an opaque nonempty token. Its account-ID-only list omits status and obscures that verified brain artifacts are also inventory. Its watermark-shaped cut cannot represent `JournalPosition`'s positive empty genesis/successor identities. The current shared contracts have no `EvidenceRef`, and current ancestry interfaces are watermark-only. Finally, the legacy artifact hash and contract plan hash are separate source identities. This contract records those mismatches as required corrections and amendments; none is silently normalized into the proposed envelope.
