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
    ControlDBLength int64 `json:"control_db_length"`
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

`SnapshotInventorySHA256` is the SHA-256 of a separate domain-separated canonical inventory payload, version 1, whose fields in order are `version`, `manifest_sha256`, `source_build_token`, `source_schema_version`, `accounts`, `control_db_length`, `brain_count`, `brain_inventory_sha256`, `verified_artifact_count`, `declared_artifact_bytes`. `accounts` is the complete sorted sequence of `{account_id,status}` rows. The brain digest is computed only from the input `[]contracts.BrainArtifact` using the exact brain projection above. Exact artifact bytes and all manifest metadata are already committed by raw `M`; this extra digest binds the inspection result to the envelope and catches projection mismatch. It does not imply a brain restore verification beyond what the backup owner establishes. `SnapshotInventoryV1` therefore requires explicit `ControlDBLength`; do not attempt to reconstruct either `ControlDBLength` or per-brain lengths from `ManifestSHA256` or `BrainInventorySHA256`. This pure helper's source projection is a caller-supplied scalar/list input only. A future authority-bearing factory must source it from the owner's verified manifest/inspection handle, since current `SnapshotInspection` reports aggregate count/bytes but not the control DB's individual length.

The three journal values are never merged: `manifest_journal_watermark` is exact M_w from the snapshot; `snapshot_cut` is C from the journal authority; `last_sealed` is H as a separate historical seal. Match source caps exactly: `C.ActiveGeneration` is 1..10,000; any nonzero sequence is 1..9,999,999,999. `SnapshotCut` may be historical relative to the currently observed old writer generation; require only `1 <= C.ActiveGeneration <= OldWriter.Generation <= 10,000`, not equality between cut and writer. For `ELIGIBLE`, require `ContractPlan.Generation == OldWriter.Generation` (the artifact fence generation describes the old writer, not C). A nonzero H is a valid historical watermark with generation 1..OldWriter.Generation and sequence 1..9,999,999,999; zero H must have all three zero fields. Do not equate H with C's predecessor seal: the owner verifier authenticates the exact relationship between C, H, M_w, M, and store identity. Generation one C requires a valid nonempty genesis issuance ID, empty successor ID, and zero predecessor seal. Later C generations require a valid successor allocation ID, empty genesis ID, and a valid predecessor seal at exactly C generation minus one. `LastObjectInGeneration` may be the exact zero watermark (all three fields zero) for an empty positive C generation; otherwise it must be a valid watermark in C's generation. A legacy watermark is either completely zero or has generation 1..10,000, sequence 1..9,999,999,999, and a lowercase 64-hex entry hash. M_w must not be later than C: zero precedes genesis; otherwise its generation is lower than C's, or it is in C's generation with sequence no greater than C's nonzero head. Bind `JournalStoreID == OldWriter.JournalStoreID`. For `ELIGIBLE`, require `ContractPlan.JournalWatermark == M_w`. Eligible IDs must be a nonempty, exact match for contract-plan accounts, each a member of the exact sorted `ActivationAllowlist`, which may contain additional approved IDs. `FROZEN_ONLY` binds C and old writer to the same store but carries no contract plan. These structural checks match `deletion.validateJournalPosition` plus the source's generation/sequence caps, but the pure component does not authenticate the fields. In particular, it may not use the legacy zero watermark to represent a positive empty cut.

The journal-store's later successor allocation/adoption is a separate effect/authority boundary. A source generation at the 10,000 cap may be valid for this inert plan envelope while no `G+1` successor is allocatable. The pure codec does not assert that recovery can adopt `G+1`; an owner-controlled backend must reject before effects when its successor-generation guard cannot be met. The backend's generation+1 allocation guard, locking, adoption, and resulting service generation remain effect/authority obligations and are not encoded as a promise in this v1 payload.

The existing contract plan's `JournalWatermark` can represent M_w but cannot retain the additional C issuance/allocation identity. Its legacy field stays M_w and must not be repurposed to store C or H. A future owner-controlled ancestry result must carry C in full alongside M_w and M, so the envelope's cut is not inferred from the contract plan watermark.

## Validation, size bounds, and deterministic encoding

Bounds are checked before allocating slices, decoding nested payloads, or hashing. Reject rather than truncate, normalize, sort on behalf of caller, or silently fill defaults. Proposed hard limits for v1 are: at most 100 eligible IDs; at most 10,000 snapshot accounts and 10,000 brains; at most 10,000 dispositions; at most 1 MiB combined encoded evidence-reference fields and approval proof bindings represented here; at most 4 MiB canonical envelope; and at most 1 MiB canonical contract-plan projection, preserving the existing RCP helper bound. The source manifest reader retains its existing 32 MiB bound. Checked arithmetic is required for aggregate sizes and byte counts. `DeclaredArtifactBytes` must be positive and at most the pure codec's fixed 1 TiB ceiling; the backup owner may enforce a stricter independently configured inspection cap, which is not an input/API to this pure package. Counts must equal actual slice lengths. A decoded envelope must have `VerifiedArtifactCount >= 1`, `ControlDBLength > 0`, and `DeclaredArtifactBytes >= ControlDBLength`; because the wire does not carry all brain artifact lengths, this is only a necessary bound. Exact count/byte equality is established only by `SnapshotInventoryDigestV1` over its full brain projection and then by an owner verifier reopening the source.

Account and brain IDs are validated against the relevant existing backup manifest validators, not a new weaker approximation; snapshot account statuses are restricted to the source enum `active`, `deleted`, `deleting`, `restore_pending`. Account rows, brain rows/heads, allowlist, plan accounts, eligible IDs, and dispositions must be strictly sorted and unique by their specified key. Every eligible ID and every disposition ID must be in the complete snapshot account inventory. Only the `FROZEN_ONLY` arm carries dispositions, and those dispositions cover each snapshot account exactly once; the `ELIGIBLE` arm has no frozen dispositions. `ELIGIBLE` requires a nonempty plan account set, exact equality between eligible IDs and contract-plan accounts, with eligible IDs a subset of the authorized activation allowlist, and no frozen arm. `FROZEN_ONLY` requires an empty eligible scope and exactly the frozen arm; it has no contract plan. Empty inventories are allowed only if the source snapshot validator permits them; do not invent an eligible ID to satisfy `contracts.RecoveryPlan.Validate`.

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

## Normative v1 details (supersedes any earlier deferred wording)

This section closes the pure codec contract. It does not close the separately identified authority-owner evidence gaps. Every domain prefix below is the exact ASCII byte sequence shown, including the final NUL byte (`00`); it is concatenated directly with canonical JSON bytes. JSON strings use UTF-8 and Go `encoding/json` string escaping (including HTML escaping of `<`, `>`, and `&`); integers use base-10 with no leading zeros except `0`; no insignificant whitespace or terminal newline is included in hashed bytes.

### Exact nested canonical payloads and independently reproducible vectors

Brain projection is canonical JSON for this ordered schema:

```go
type brainInventoryPayloadV1 struct {
    Version int `json:"version"` // 1
    Brains []brainV1Wire `json:"brains"`
}
type brainV1Wire struct {
    ID string `json:"id"`
    Empty bool `json:"empty"`
    Artifact brainArtifactV1Wire `json:"artifact"`
    Heads []brainHeadV1Wire `json:"heads"`
}
type brainArtifactV1Wire struct {
    RelativePath string `json:"relative_path"`
    Length int64 `json:"length"`
    SHA256 string `json:"sha256"`
}
type brainHeadV1Wire struct {
    Ref string `json:"ref"`
    ObjectID string `json:"object_id"`
}
```

For `Empty == true`, `artifact` is exactly `{ "relative_path":"", "length":0, "sha256":"" }` and `heads` is an empty array; an empty brain may not carry artifact bytes or heads. For nonempty brains all artifact fields are required and validated against the inspected manifest; heads are sorted by `ref`, unique, nonempty, and bounded. `Brains` are sorted by `ID`, unique, with no nil/absent slice distinction. Compute `BrainInventorySHA256 = SHA256("serenity.recovery-brain-inventory.v1\x00" || JSON(brainInventoryPayloadV1))`.

The inventory payload is this exact ordered schema; `accounts` contains all snapshot accounts sorted by ID, while `brains` binds the count and previously specified brain digest:

```go
type snapshotInventoryPayloadV1 struct {
    Version int `json:"version"` // 1
    ManifestSHA256 string `json:"manifest_sha256"`
    SourceBuildToken string `json:"source_build_token"`
    SourceSchemaVersion int `json:"source_schema_version"`
    Accounts []accountInventoryV1Wire `json:"accounts"`
    ControlDBLength int64 `json:"control_db_length"`
    BrainCount int `json:"brain_count"`
    BrainInventorySHA256 string `json:"brain_inventory_sha256"`
    VerifiedArtifactCount int `json:"verified_artifact_count"`
    DeclaredArtifactBytes int64 `json:"declared_artifact_bytes"`
}
```

Compute `SnapshotInventorySHA256 = SHA256("serenity.recovery-snapshot-inventory.v1\x00" || JSON(snapshotInventoryPayloadV1))`. Thus the outer field `SnapshotBrainInventory{Count, CanonicalDigest}` contains the same count and `BrainInventorySHA256`; the account list in the outer envelope is exactly the full account projection whose digest appears in this inventory hash. `ControlDBLength` is the manifest-verified control database length. `VerifiedArtifactCount` is exactly one control database plus one for each nonempty brain; `DeclaredArtifactBytes` is exactly the checked sum of `ControlDBLength` and each nonempty brain's `ArtifactRef.LengthBytes`. Empty brains contribute neither count nor bytes. This codec computes hashes from caller-provided typed projection rows only. A digest-only value cannot recreate brain rows, manifest bytes, or `backup.VerifiedSnapshotInspection`; no decoder claims to recover those source objects.

Synthetic golden vector, independently assembled as literal JSON (all hashes below are SHA-256 lowercase hex):

```text
brain JSON: {"version":1,"brains":[{"id":"brain-01","empty":true,"artifact":{"relative_path":"","length":0,"sha256":""},"heads":[]}]}
brain prefix hex: 736572656e6974792e7265636f766572792d627261696e2d696e76656e746f72792e763100
brain SHA256: f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e

inventory JSON: {"version":1,"manifest_sha256":"0000000000000000000000000000000000000000000000000000000000000000","source_build_token":"fixture-build","source_schema_version":2,"accounts":[{"account_id":"acct0000000000001","status":"active"}],"control_db_length":17,"brain_count":1,"brain_inventory_sha256":"f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e","verified_artifact_count":1,"declared_artifact_bytes":17}
inventory prefix hex: 736572656e6974792e7265636f766572792d736e617073686f742d696e76656e746f72792e763100
inventory SHA256: 04a8e01ab69e24a230882daabf374d67eb9bb2bb004c8b81bb2e22dab6f7bc70
```

The vectors were independently assembled and hashed from literal bytes using Python's standard SHA-256 implementation, not by the proposed Go encoder. They are synthetic and make no claim about live source evidence.

### Exact pure typed API and ownership behavior

The source freeze must implement only the following pure package-local surface in `internal/hosted/recovery`; all wire structs remain unexported. Names/signatures are normative unless a reviewer approves an additive amendment:

```go
type RecoveryEnvelopeKind string
const (
    RecoveryEnvelopeEligible RecoveryEnvelopeKind = "ELIGIBLE"
    RecoveryEnvelopeFrozenOnly RecoveryEnvelopeKind = "FROZEN_ONLY"
)
type RecoveryEnvelopeHash string
var (
    ErrRecoveryEnvelopeInvalid = errors.New("recovery: invalid recovery envelope")
    ErrRecoveryEnvelopeTooLarge = errors.New("recovery: recovery envelope exceeds limit")
    ErrRecoveryEnvelopeNonCanonical = errors.New("recovery: noncanonical recovery envelope")
    ErrRecoveryEnvelopeContext = errors.New("recovery: recovery envelope context canceled")
)
type RecoveryEnvelopeV1 struct {
    FormatVersion int
    Kind RecoveryEnvelopeKind
    PlanRef, SnapshotPinID string
    ReservationVersion int64
    ManifestSHA256 string
    ManifestJournalWatermark WatermarkV1
    SourceBuildToken string
    SourceSchemaVersion int
    SnapshotAccountInventory []SnapshotAccountV1
    SnapshotBrainInventory BrainInventoryV1
    ControlDBLength int64
    VerifiedArtifactCount int
    DeclaredArtifactBytes int64
    SnapshotInventorySHA256 string
    PlanApprovalRef EvidenceRefV1
    PlanApprovalDigest, PlanApprovalNonce string
    PlanApprovalExpiresAt time.Time
    ActivationAllowlist []string
    OperationID string
    OldWriter WriterV1
    JournalStoreID string
    SnapshotCut JournalPositionV1
    LastSealed WatermarkV1
    PrefixEvidenceRef EvidenceRefV1
    Eligible *EligibleArmV1
    Frozen *FrozenArmV1
}
type WatermarkV1 struct { Generation, SequenceID int64; EntryHash string }
type JournalPositionV1 struct {
    ActiveGeneration int64
    LastObjectInGeneration, PredecessorSeal WatermarkV1
    GenesisIssuanceID, SuccessorAllocationID string
}
type SnapshotAccountV1 struct { ID, Status string }
type BrainInventoryV1 struct { Count int; CanonicalDigest string }
type EvidenceRefV1 struct { Authority, RecordID, Version string }
type WriterV1 struct {
    Provider, ScopeRef, WriterRef, BootRef, JournalStoreID string
    Generation int64
}
type ContractPlanV1 struct {
    SourceSnapshot string
    JournalWatermark WatermarkV1
    Generation int64
    ProviderTruthAt time.Time
    Accounts []string
}
type EligibleArmV1 struct {
    LegacyPlanArtifactHash string
    RecoveryContractPlanHash string
    ContractPlan ContractPlanV1
    EligibleAccountIDs []string
}
type FrozenArmV1 struct { Dispositions []FrozenDispositionV1 }
type FrozenDispositionV1 struct { AccountID, Disposition, ReasonCode string }
const (
    FrozenDispositionWithheld = "WITHHELD_FROZEN"
    FrozenDispositionDeleted = "DELETED"
    FrozenDispositionDeleting = "DELETING"
)
type SnapshotInventoryV1 struct {
    ManifestSHA256 string
    SourceBuildToken string
    SourceSchemaVersion int
    Accounts []SnapshotAccountV1
    Brains []contracts.BrainArtifact
    ControlDBLength int64
    VerifiedArtifactCount int
    DeclaredArtifactBytes int64
}
type SnapshotInventorySummaryV1 struct {
    ManifestSHA256 string
    SourceBuildToken string
    SourceSchemaVersion int
    Accounts []SnapshotAccountV1
    ControlDBLength int64
    BrainCount int
    BrainInventorySHA256 string
    VerifiedArtifactCount int
    DeclaredArtifactBytes int64
}
func CloneRecoveryEnvelopeV1(in RecoveryEnvelopeV1) (RecoveryEnvelopeV1, error)
func ValidateRecoveryEnvelopeV1(ctx context.Context, in RecoveryEnvelopeV1) error
func EncodeRecoveryEnvelopeV1(ctx context.Context, in RecoveryEnvelopeV1) ([]byte, RecoveryEnvelopeHash, error)
func DecodeRecoveryEnvelopeV1(ctx context.Context, encoded []byte) (RecoveryEnvelopeV1, RecoveryEnvelopeHash, error)
func RecoveryEnvelopeHashV1(ctx context.Context, in RecoveryEnvelopeV1) (RecoveryEnvelopeHash, error)
func BrainInventoryDigestV1(ctx context.Context, brains []contracts.BrainArtifact) (string, error)
func SnapshotInventorySummaryDigestV1(ctx context.Context, in SnapshotInventorySummaryV1) (string, error)
func SnapshotInventoryDigestV1(ctx context.Context, in SnapshotInventoryV1) (string, error)
```

The exact pure Go value types and fields are shown above; their ordered wire types and tags are specified in the prior section, and the public typed values are never marshaled directly. These values are not authority tokens and contain no `Verified*` fields or booleans. `Clone...` validates bounds before allocating and returns an independent value. It clones every nested account slice, contract-plan `Accounts`, eligible IDs, dispositions, and all `contracts.BrainArtifact` values including each `Heads` slice. `RecoveryEnvelopeV1` includes `ControlDBLength int64` between `SnapshotBrainInventory` and `VerifiedArtifactCount`, matching the wire order. Encode/Validate/Hash never mutate caller input. Decode copies input and returns independently owned slices. Both timestamps must have `Location() == time.UTC` and exact format/parse round-trip; a fixed-offset zero time is rejected. Errors wrap stable sentinels `ErrRecoveryEnvelopeInvalid`, `ErrRecoveryEnvelopeTooLarge`, `ErrRecoveryEnvelopeNonCanonical`, and `ErrRecoveryEnvelopeContext`; error strings may add field context but callers branch on sentinels. All functions check `ctx.Err()` before work, between bounded phases, and immediately before returning; cancellation returns the zero result and context sentinel joined with `ctx.Err()`.

`SnapshotInventoryV1`, `SnapshotInventorySummaryV1`, and `SnapshotAccountV1` have exactly the fields/types shown. `SnapshotInventoryDigestV1` requires `VerifiedArtifactCount == 1 + number of nonempty brains` and `DeclaredArtifactBytes == ControlDBLength + sum(ArtifactRef.LengthBytes for nonempty brains)` with checked arithmetic; `ControlDBLength > 0`, every nonempty artifact length > 0, and the aggregate is positive and at most the pure codec's fixed 1 TiB ceiling. The backup owner may use a stricter inspection cap independent of this helper; no options argument/configured cap is exposed by the pure API. It rejects a zero-count or zero-byte source inventory. These equalities are checked only when this helper receives the full account/brain rows plus control database length and aggregate fields. It derives `BrainCount` and `BrainInventorySHA256` from those rows, then calls `SnapshotInventorySummaryDigestV1`. The summary helper validates the summary fields and computes the canonical inventory hash from the payload fields available in the envelope. `ValidateRecoveryEnvelopeV1` and `DecodeRecoveryEnvelopeV1` must recompute this inventory hash from the envelope's account rows, brain count/digest, source fields, control DB length, artifact count, and byte total and reject any mismatch. Because the envelope does not carry every brain row, neither it nor the summary helper can independently recompute the brain digest or prove exact artifact count/total bytes; only the full-row helper checks those equalities, and an owner-controlled verifier must later reopen the source and recompute them. Neither helper nor envelope codec claims its caller inputs came from the backup owner.

`Encode` validates, builds canonical bytes, and returns their outer hash; `Hash` follows the exact same validation and canonical payload path without returning bytes. `Decode` performs bounded lexical validation before any proportional allocations: reject invalid UTF-8, duplicate keys at every depth, unknown/missing keys, non-integer numbers, overlong strings/arrays, excess nesting, trailing values, and input over 4 MiB. It then decodes, validates, re-encodes, and requires byte-for-byte equality with the original. It returns the outer hash only after exact equality. No API accepts a caller-supplied expected hash or treats a hash as proof.

### Closed field validation and numeric limits

All string lengths below are measured in UTF-8 bytes, after requiring valid UTF-8. ASCII-only fields reject every byte outside their stated alphabet; text fields reject NUL, C0 controls, DEL, and Unicode control-category runes, and are never trimmed, case-folded, or normalized. Empty values are permitted only where stated.

| Field | Normative validation |
| --- | --- |
| SHA-256 values (`ManifestSHA256`, inventory/brain digests, `EntryHash`, plan hashes, approval digest) | Exactly 64 lowercase ASCII hex bytes. Empty only for a zero watermark's `EntryHash`. |
| `SourceBuildToken` | 1–256 bytes; valid UTF-8; no Unicode whitespace/control characters. Opaque; no hex requirement. |
| Plan ref, snapshot pin ID, operation ID, journal store ID, authority, record ID, version, nonce, writer provider/scope/writer/boot refs, genesis issuance ID, successor allocation ID | 1–256 bytes; ASCII `[A-Za-z0-9._:/-]`; must begin/end alphanumeric; no empty values. |
| Account ID | 16–64 ASCII `[A-Za-z0-9]`, matching backup `safeID`. |
| Brain ID | 1–255 bytes; valid UTF-8; not `.` or `..`; no slash, backslash, Unicode whitespace, or control rune. This matches source `validName`. |
| Relative artifact path | 1–255 bytes; exactly one safe path element, valid UTF-8, not `.` or `..`, no slash, backslash, Unicode whitespace, or control rune. Empty only for `Empty` brain. |
| Head ref | 1–1024 bytes; valid UTF-8; exact `HEAD` or prefix `refs/` followed by a nonempty suffix; no Unicode whitespace or control rune, matching source `validRef`. |
| Head object ID | Exactly 40 or 64 lowercase ASCII hex bytes, matching source `validObjectID`. |
| Evidence ref strings | Each follows the 1–256 ASCII reference rule above; combined UTF-8 byte length across all refs in an envelope ≤1 MiB. No refs are accepted as authority. |
| Frozen reason code | 1–64 ASCII `[A-Z0-9_]+`; disposition-specific values below only. |
| Timestamps | 1–35 ASCII bytes, exact UTC RFC3339Nano parse/format round-trip; require terminal `Z`. |
| `Kind`, status, disposition | Exact enums below; case-sensitive. |

Explicit ceilings: envelope encoded/input bytes 4 MiB; contract-plan canonical projection 1 MiB (preserves RCP's existing limit); eligible/allowlist/account/disposition count 100 for eligible plan scopes and 10,000 for complete snapshot accounts; brains 10,000; total heads 100,000; heads per brain 1,000; artifact count 100,000; declared artifact bytes at most 1 TiB; decoded nesting depth at most 12; any individual JSON string at most 1024 bytes except source build token 256, timestamp 35, digest 64, and the narrower per-field maxima in the table. All integer fields are nonnegative unless generation/version requires positive; reject values above signed 64-bit max before conversion. `ReservationVersion` is 1..`math.MaxInt64`; schema version 1..`math.MaxInt32`; format version exactly 1; `ActiveGeneration` 1..10,000; journal sequence IDs 0..9,999,999,999; artifact lengths and aggregate bytes 0..1 TiB. `contracts.ArtifactRef.LengthBytes` for nonempty brains and the control database is positive, matching source validation; empty-brain length is zero. Checked addition is mandatory for head, artifact, encoded, and evidence byte totals; overflow is invalid. Limits are checked before slice allocation or hashing. Canonical representation contains no maps. The outer 4 MiB bound does not relax the RCP contract-plan's 1 MiB bound.

Snapshot account status enum is exactly `active`, `deleted`, `deleting`, `restore_pending`. Frozen disposition enum is exactly `WITHHELD_FROZEN`, `DELETED`, `DELETING`. `WITHHELD_FROZEN` requires a reason from `{APPROVAL_SCOPE, PROVIDER_INELIGIBLE, EVIDENCE_UNAVAILABLE, AMBIGUOUS, CURRENT_CHECK_FAILED, OTHER}`; `DELETED` requires reason `JOURNAL_DELETED`; `DELETING` requires reason `JOURNAL_DELETING`. A snapshot row whose status is `deleted` pairs only with `DELETED`; status `deleting` pairs only with `DELETING`; `active` and `restore_pending` pair only with `WITHHELD_FROZEN`. Deletion state wins over a candidate eligibility claim. No caller-supplied reason can turn a deleted/deleting account eligible. Only `FROZEN_ONLY` represents every complete account exactly once in its frozen dispositions; `ELIGIBLE` has the common complete inventory and eligible IDs, but no disposition list or Frozen arm. For `ELIGIBLE`, eligible IDs are a nonempty subset of snapshot accounts with status `active` or `restore_pending`, exact-match contract-plan accounts, and are a subset of `ActivationAllowlist`. The allowlist may contain additional approved IDs. A `FROZEN_ONLY` envelope has zero eligible IDs and dispositions for the whole snapshot. After the later authorized restore, the owner-controlled publisher must persist every account frozen/deleted/deleting before service admission; that state is outside this envelope and cannot be inferred from its integrity hash. These values are data classifications, not authenticated current status.

### Hash domains and compatibility note

The required domain prefixes are exactly:

```text
brain inventory:   serenity.recovery-brain-inventory.v1\x00
snapshot inventory: serenity.recovery-snapshot-inventory.v1\x00
outer envelope:    serenity.recovery-envelope.v1\x00
```

The landed RCP v1 `CanonicalContractRecoveryPlanHash` has no byte-prefix: it is SHA-256 of its canonical payload containing `domain_version:1`, with a 1 MiB limit. Preserve that exact source behavior and do not silently add a prefix. Legacy artifact bytes and hashing remain untouched. The two inventory golden vectors above are synthetic, fully specified, and independently calculated; reviewers should reproduce them before accepting a later implementation.
