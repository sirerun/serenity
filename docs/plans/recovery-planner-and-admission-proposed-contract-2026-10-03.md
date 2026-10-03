# Recovery planner and production admission: proposed integration contract

**Status:** proposed resolution of the corrected review's remaining hash-domain hold; source baseline `8081632` after PR #349. This amendment preserves `a50c876`/`331df646` and all prior contract history. It is not an approved interface change or provider/deployment/activation authority. Re-review is required; frozen interfaces remain unchanged until owner review and additive amendments.

## Contract rulings

**Snapshot identity.** Set `contracts.RecoveryPlan.SourceSnapshot` to lowercase SHA-256 of the exact raw `manifest.json` byte sequence that `backup.InspectSnapshot` verified. Its 64 lowercase hex form satisfies the existing object-ID shape. Never hash re-encoded JSON. `ManifestV2.Source.BuildSHA` and `SchemaVersion` stay distinct metadata. Persist these with the envelope and include them in its canonical outer hash in the proposed plan-store format revision. `ExpectedSnapshotSHA256` is supplied with a reference to a verified operator case; the verifier returns the approved sorted account allowlist, nonce, and operation ID. Compare exact raw digest and scope before provider calls. Approval binds exact bytes and scope; it does not prove provider truth or old-writer fencing. All plan hashes are integrity keys, not authority.

**Three distinct, non-cyclic identities.** The source has two different existing plan types. `internal/hosted/recovery.Plan` is a canonical artifact with fields `PlanHash`, `FormatVersion`, `SnapshotSHA256`, `JournalWatermark`, `FenceGeneration`, canonical UTC `ProviderObserved` string, and sorted `Accounts`; `CreatePlan`/`LoadPlan` hash and verify its `planPayload` with `PlanHash` excluded. Separately, `contracts.RecoveryPlan` has fields `PlanHash`, `SourceSnapshot`, `JournalWatermark`, `Generation`, `ProviderTruthAt time.Time`, and `Accounts`; its existing `Validate` checks hash shape and plan relationships but does not recompute/authenticate that hash. Never copy `recovery.Plan.PlanHash` into `contracts.RecoveryPlan.PlanHash` or claim the artifact helper computed the contract hash.

Keep both existing serialized formats and `contracts.RecoveryPlan.Validate` unchanged. Eligible plan creation first calls existing `recovery.CreatePlan`, then `recovery.LoadPlan` to re-open and authenticate the exact artifact. Map those verified fields exactly: `SourceSnapshot = artifact.SnapshotSHA256`; `JournalWatermark = artifact.JournalWatermark`; `Generation = artifact.FenceGeneration`; `ProviderTruthAt = time.Parse(time.RFC3339Nano, artifact.ProviderObserved).UTC()` after requiring the parsed time to format back identically; `Accounts = sorted copy of artifact.Accounts`. `recovery.Plan.FormatVersion` has no `contracts.RecoveryPlan` field and remains committed by the artifact hash. Do not map either hash field.

Add a distinct additive `recovery.CanonicalContractRecoveryPlanHash(contracts.RecoveryPlan)` helper. It rejects unless `ProviderTruthAt.Location() == time.UTC` and the accounts are sorted and unique; it validates the remaining fields on a local copy with a SHA-shaped placeholder only to reuse unchanged `Validate`, and never persists or exposes that placeholder. It hashes `json.Marshal(recoveryContractPlanHashPayload{DomainVersion: 1, SourceSnapshot, JournalWatermark, Generation, ProviderTruthAt: ProviderTruthAt.Format(time.RFC3339Nano), Accounts})` with SHA-256 and returns lowercase hex as `RecoveryContractPlanHash`. The payload excludes `PlanHash`; its field order and JSON names are exactly those shown in the private type below. It includes exactly `SourceSnapshot`, `JournalWatermark`, `Generation`, canonical UTC RFC3339Nano `ProviderTruthAt`, and sorted unique `Accounts`. It changes neither the existing `Validate` method nor either legacy serializer. Assign `string(hash)` to `contracts.RecoveryPlan.PlanHash` for plans produced by the new coordinator, then call unchanged `contracts.RecoveryPlan.Validate`. Existing callers that supply a valid hash and existing persisted data retain existing behavior; only the new producer adopts this explicit helper. Call the two identities `LegacyPlanArtifactHash` (`recovery.Plan.PlanHash`) and `RecoveryContractPlanHash` (`contracts.RecoveryPlan.PlanHash`).

The third identity, `RecoveryEnvelopeHash`, is the SHA-256 of a new immutable tagged-union payload. `ELIGIBLE` embeds the verified `LegacyPlanArtifactHash`, the complete `contracts.RecoveryPlan` with its separately computed `RecoveryContractPlanHash`, and the exact allowlist/eligibility projection. `FROZEN_ONLY` has neither a legacy artifact nor any `contracts.RecoveryPlan`; it embeds the complete frozen payload and per-account terminal dispositions, with no synthetic/empty eligible plan. Common outer fields bind source, full inventory, planRef/pin, writer/journal cut, operation, and plan-creation authorization. The outer hash excludes itself, any global epoch approval, epoch records/receipts, fence/effect references, and admission evidence. Additive hashing must not change `recovery.planPayload`, `CreatePlan`/`LoadPlan` serialization, `contracts.RecoveryPlan` serialization, or `RecoveryPlan.Validate`.

The outer payload uses only structs and sorted slices (no unordered maps) and an exact tagged union. Eligible envelope load strictly decodes, reopens the artifact with `recovery.LoadPlan`, checks `LegacyPlanArtifactHash`, repeats the exact field mapping, recomputes `RecoveryContractPlanHash`, validates the contract plan, then recomputes and checks the outer hash. Frozen load verifies only its frozen payload and outer hash; it never constructs a legacy artifact or contract plan.



`VerifiedEligiblePlan` is constructed only after `CreatePlan` and `LoadPlan` authenticate the artifact, the exact mapping above succeeds, and `CanonicalContractRecoveryPlanHash` is computed. The outer canonicalizer accepts that private token for `ELIGIBLE`, requires its artifact hash and contract payload to equal the serialized arm, validates union/sort/coverage invariants, then hashes canonical JSON with SHA-256. The `FROZEN_ONLY` arm passes no token. This keeps both plan identities independently verifiable and the outer hash acyclic.

The initial `ApprovalBinding` signs known inputs only: expected raw snapshot digest, sorted allowed account scope (possibly empty), operation ID, nonce, and expiry. It cannot sign a future envelope hash. After `RecoveryEnvelopeHash` is finalized, a separate global epoch approval signs that exact outer hash plus exact full inventory, scope, writer/store, operation, action set, nonce, and expiry; `Begin` validates and consumes it before effects. Each hash-bearing API names its domain: planner/READY store, CLI `plan` result, `Begin`/`Continue`/`Commit`, recovery epoch reservation, admission, and one-account activation use `RecoveryEnvelopeHash`; `recovery.CreatePlan`/`LoadPlan` artifact-file calls use `LegacyPlanArtifactHash`; `RecoveryApplyRequest` and `FenceReceipt.SufficientFor(contracts.RecoveryPlan)` use the `RecoveryContractPlanHash` stored in `contracts.RecoveryPlan.PlanHash` for plans produced by this coordinator. New frozen/global fence and epoch records never call legacy `FenceReceipt.Validate`/`SufficientFor` with a fabricated plan.

Each new epoch record has its own `RecordDigest = SHA256(canonical record payload excluding RecordDigest)`. That payload includes `RecoveryEnvelopeHash`, the independently signed global-approval reference/digest/nonce, prior record digest, and phase/evidence references. The global approval signs only the already-final outer envelope hash and does not include an epoch-record digest, so there is no approval/record cycle. The committed epoch record digest is the digest of the final `EPOCH_COMMITTED` record and is available only after commit; account-activation approvals bind that committed record digest plus `RecoveryEnvelopeHash` and one account. Account records have their own `AccountRecordDigest` excluding itself and reference the exact epoch record used for that account transition; admission receipts reference the committed epoch record digest and outer envelope hash. No field hashes itself or a later record.

Hash routing is normative:

| Artifact or API | Required hash domain |
| --- | --- |
| `recovery.CreatePlan` / `LoadPlan` and their artifact files | `LegacyPlanArtifactHash` only |
| `contracts.RecoveryPlan.PlanHash`, `RecoveryApplyRequest.PlanHash`, and `FenceReceipt.SufficientFor(contracts.RecoveryPlan)` | `RecoveryContractPlanHash` for plans created by this coordinator; existing callers retain their supplied contract hash behavior |
| `Planner.PlanRecovery` result; immutable READY envelope; `PersistReadyPlan` / `LoadPlanByRef`; CLI plan output and begin/continue/commit/status keys; `EpochApprovalBinding`; `ReserveRecoveryEpoch`; `VerifyEpochReservation`; every `RecoveryEpochRecord`; recovered admission; one-account activation | `RecoveryEnvelopeHash` only |
| Append-only epoch record chain and committed-epoch reference consumed by activation/admission | `EpochRecordDigest`, computed from record payload excluding that digest |
| Continuation approval | Exact `RecoveryEnvelopeHash`, current `EpochRecordDigest`, one next phase, operation, nonce, expiry |
| Account activation approval | Both exact `RecoveryEnvelopeHash` and already committed `EpochRecordDigest`, plus one account |

The legacy APIs and `FenceReceipt` never receive a frozen plan or outer hash. Every external fence/effect method instead receives an unforgeable owner-created `RecoveryEpochPhaseGuard` bound to the exact `RecoveryEnvelopeHash`, durable current `EpochRecordDigest`, operation ID, and one authorized next phase. Thus even provider stop, revocation, journal seal/adoption, restore, publication, epoch commit, and their evidence verifiers consume the outer-hash-bound epoch record domain; none interprets it as either a legacy artifact hash or a contract-plan hash.

**Zero eligible accounts.** Preserve `RecoveryPlan.Validate`'s nonempty account invariant and never manufacture an eligible account to satisfy it. When source, inventory, journal ancestry, and global plan approval are valid but the approved provider candidate set is empty (including all candidates ineligible or all accounts outside activation scope), persist a distinct executable `FrozenRecoveryPlan`. It binds the same exact raw snapshot, complete inventory, journal cut/ancestry, old writer, plan approval, and planRef/pin as an ordinary plan, has zero activation-eligible IDs, and carries only opaque per-account frozen/deleted disposition reasons. It can enter only the existing `Begin` path, which dispatches by the persisted `PlanKind` and requires a separate exact global epoch approval and can never produce `RecoveryApplyRequest` or account activation authority. Missing/ambiguous source, identity, ancestry, approval, or inventory evidence remains an error. Never infer Free or eligibility from missing customer linkage or failed lookup.

**Plan-time journal facts versus future fence.** Plan is read-only. It records the observed active old writer generation `G`, authenticated writer identity and journal-store identity, snapshot cut, and ancestry/completeness through the snapshot cut using already durable journal history. It does not require a future seal, call `Seal`, or claim the active generation is sealed. The post-stop/post-revocation seal is obtained only in the later plan-wide recovery epoch. Re-observe active writer identity/generation before fencing; if changed from `G`, abort and require a newly approved plan.

**Plan-wide recovery versus account decisions.** One serialized recovery epoch owns stop, revocation, post-watermark seal, whole-snapshot restore/reconciliation, publication, and successor adoption. It covers the complete restored account inventory. Each snapshot account receives a durable terminal disposition: `WITHHELD_FROZEN` (with reason), or a journal-preserved `DELETED`/`DELETING` disposition; a successfully activated account has `ACTIVE_ELIGIBLE`. Missing/bad approval, provider ineligibility, ambiguity, stale evidence, or a current deletion intent means `WITHHELD_FROZEN` or the deletion disposition, never eligibility success. No account-local condition can block service for unrelated accounts after the global frozen restore. One-account activation cannot advance another account or repeat global restore/seal/adoption.

**Global recovery permission versus account activation.** A signed global epoch approval authorizes only quiesce, old-writer stop, complete credential/session revocation, sealing, validating/restoring the exact whole snapshot inventory, publishing it with all surviving accounts frozen, and adopting the single successor. Before the first disruptive side effect, `Begin` verifies that approval against the complete immutable outer `RecoveryEnvelopeHash`, full sorted snapshot inventory digest/list, exact activation scope (possibly empty), source digest, old writer identity/generation/store, operation ID, action set, expiry, and nonce. Per-account approvals are not prerequisites for global restore. No global approval can unfreeze any account. Each account remains `restore_pending`/Free until a separate exact-one-account approval and a fresh guarded provider/deletion check succeeds.

**Forward-only recovery.** Before any external effect, an authorized abort may release reservations and leave the old writer unchanged. Persist `EFFECTS_MAY_HAVE_STARTED` before the first quiesce/stop request; from that point, including timeout/uncertain response, rollback to old writer or credential restoration is forbidden. Exact-operation resume continues the same source, full inventory, plan, writer, and epoch. If approvals or global authorization need replacement after effects, a newly signed continuation authorization binds the same epoch and the exact next phase; it cannot change scope or authorize rollback. A global approval failure before effects is a safe abort. Missing/bad account approval or later eligibility loss is recorded `WITHHELD_FROZEN`; finish the frozen restore and recover service when every inventory account is terminal. Later activation is a separate guarded one-account operation against the already-published exact data, with new approval and current checks; it never restores, seals, adopts, or republishes.

## Source preflight evidence

At baseline `8081632`, `internal/hosted/recovery/plan.go` lines 43–75 define `PlanInput`, `Plan`, and `planPayload`; `planPayload` contains only `FormatVersion`, `SnapshotSHA256`, `JournalWatermark`, `FenceGeneration`, canonical `ProviderObserved`, and `Accounts`. `newPlan` (lines 223–244) sorts accounts, JSON-marshals `payloadFromPlan`, and SHA-256 hashes those bytes; `LoadPlan` (lines 156–220) rechecks the same payload hash and canonical artifact bytes. That identity belongs to the artifact owned by the internal recovery package. `internal/hosted/contracts/backup.go` lines 269–315 separately define `contracts.RecoveryPlan` with `SourceSnapshot`, `JournalWatermark`, `Generation`, `ProviderTruthAt`, and `Accounts`; its unchanged `Validate` checks that `PlanHash` is SHA-shaped and validates plan fields, but does not recompute the hash. The source contains no helper that maps the artifact to the contract or computes the contract's hash. The proposed mapping and `CanonicalContractRecoveryPlanHash` above are therefore additive consumer requirements, not current implementation claims.

## Existing source and ownership

Reusable implementation already present: `backup.InspectSnapshot` validates exact manifest/artifact bytes and reports raw digest/build identity; `internal/hosted/recovery` has strict canonical legacy `recovery.Plan` artifact persistence via `CreatePlan`/`LoadPlan`; the concrete billing observer is read-only but only accepts current `restore_pending`; `service.AssembleWithDependencies` requires a shared journal and admission, then validates snapshot-cut/full-history reads and pending deletion intents; and `deletion.Journal` implements durable append, `ReadThrough`, and `Seal`. Do not duplicate these implementations or claim the inspector/observer is missing.

Important actual limits: `InspectSnapshot` owns and cleans its scratch copy before returning, so its returned inventory is not a durable Apply input. The existing billing observer cannot assess arbitrary candidate identities extracted from a snapshot. Service admission has no production factory. The current contracts have no capability for authenticated current-writer identity, active-generation inspection, atomic plan-wide recovery lease, durable successor adoption, or a separate post-fence evidence record. They also have no single-account activation guard spanning deletion/status mutation, provider refreads, final reread and row CAS; the account owner must supply and review that serialization boundary. `ReadThrough(...).Sealed` alone cannot establish adoption.

Ownership requests: T23.50 owns `internal/hosted/recovery/**` and `internal/cli/hosted_recovery.go`; backup owners add a pinned verified-snapshot lease API; billing owners add a narrow snapshot-candidate read-only observation API; deletion/journal owners add authenticated current-writer/ancestry and successor-adoption capabilities; service owners add epoch-aware admission wiring while preserving same-journal identity; a control-plane owner owns AWS stop/revocation/evidence adapters and their scoped role. These are dependency-linked owner patches. Planner code does not edit their files or permissions.

## Proposed private interfaces and records

Interfaces below are requested additions, not existing callable APIs. The shared contract additions proposed are `contracts.EvidenceRef{Authority, RecordID, Version}`, and `ExpectedSnapshotSHA256` plus `ApprovalRef` on `RecoveryPlanRequest`. The typed activation command belongs to the service-owned local RPC and does not reuse the plan-based `RecoveryApplyRequest`. All other capabilities and terminal ledgers stay in owner-local packages. `Verified*` values are constructed only inside `internal/hosted/recovery` after its verifier fetches and validates referenced records. Authority adapters return opaque evidence references/claims; they never return trusted serialized `Verified*` values. Durable records store references and digests, and the recovery verifier refetches the exact immutable object version/proof chain before consuming it. The CLI cannot deserialize a `Verified*` value into authority.

The backup-owner amendment supplies the artifact handoff that current `InspectSnapshot` does not return. Consume only `backup.SnapshotLeaseStore`, `*backup.VerifiedSnapshotLease`, `backup.VerifiedAccountCandidate`, `backup.SnapshotPinRef`, and `backup.PinnedSnapshotRef`; do not introduce recovery-local lease/candidate types. Exact proposed methods are `NewSnapshotLeaseStore(ctx, options, lifecycleAuthority)`, `Stage(ctx, sourcePath, InspectionOptions)`, `Candidate(ctx, accountID)`, `Pin(ctx, planRef, reservationVersion, expectedManifestSHA256) -> SnapshotPinRef`, `FindPinned(ctx, planRef, digest)`, `ResumePin(ctx, planRef, reservationVersion, digest)`, `CancelPin(ctx, exactAttempt backup.PinAttemptRef)`, `ResolvePinned(ctx, pinID, digest, planRef) -> PinnedSnapshotRef`, `ReopenPinned(ctx, ref)`, `Reconcile(ctx)`, `backup.RestoreVerified(ctx, lease, destination)`, and context-aware `Close`. `SnapshotPinRef` provides ID/digest/planRef/reservationVersion; the resolved `PinnedSnapshotRef` is process-local and never serialized. `VerifiedAccountCandidate` carries historical `CustomerBindingCandidate` and nullable `CheckoutAttempts`; `SnapshotCheckoutAttempt` preserves present-but-NULL and present-but-empty session refs as rows. These are untrusted historical candidates, not binding or eligibility authority.

```go
// Use backup.SnapshotLeaseStore, *backup.VerifiedSnapshotLease,
// backup.VerifiedAccountCandidate, backup.SnapshotPinRef and
// backup.PinnedSnapshotRef. PinRef is durable; PinnedSnapshotRef is process-local.
```

Backup `Stage` extracts/reuses the same one-pass verifier used by `InspectSnapshot`, writes verified bytes to owner-only bounded durable staging, and returns the private lease handle. Restore consumes only the pinned backup handle through the shared restore implementation. Neither consumer reopens the untrusted source after staging. The backup owner owns lease cleanup/reopen identity checks and preserves ordinary `Restore` validation through the shared implementation.

```go
// Shared additive fields in internal/hosted/contracts/backup.go only.
type EvidenceRef struct { Authority, RecordID, Version string }
type RecoveryPlanRequest struct {
    SnapshotPath string
    ExpectedSnapshotSHA256 string
    ApprovalRef EvidenceRef // immutable operator-case reference
}
```

```go
// New private recovery-owned types in internal/hosted/recovery; the existing
// contracts.RecoveryPlan and its Validate/serialization remain unchanged.
type LegacyPlanArtifactHash string
type RecoveryContractPlanHash string
type RecoveryEnvelopeHash string
type EpochRecordDigest string
type AccountRecordDigest string
type EvidenceRef = contracts.EvidenceRef
type RecoveryPlanKind string
const (
    EligibleRecoveryPlanKind RecoveryPlanKind = "ELIGIBLE"
    FrozenRecoveryPlanKind RecoveryPlanKind = "FROZEN_ONLY"
)
type FrozenAccountDisposition struct { AccountID, ReasonCode string }
type RecoveryEnvelopePayload struct {
    FormatVersion int
    Kind RecoveryPlanKind
    PlanRef, SnapshotPinID string
    ReservationVersion uint64
    SnapshotSHA256, SnapshotInventoryDigest string
    SnapshotAccountIDs []string // complete sorted restore inventory
    SnapshotBuildSHA256, SnapshotSchemaVersion string
    PlanApprovalRef EvidenceRef
    PlanApprovalDigest, PlanApprovalNonce string
    PlanApprovalExpiresAt time.Time
    ActivationAllowlist []string // exact sorted scope, possibly empty
    OperationID string
    OldWriter WriterIdentity
    LastSealed, SnapshotCut contracts.DeletionWatermark
    PrefixRef EvidenceRef
    Eligible *EligibleEnvelopePayload // iff Kind == ELIGIBLE
    Frozen *FrozenEnvelopePayload // iff Kind == FROZEN_ONLY
}
type recoveryContractPlanHashPayload struct {
    DomainVersion int `json:"domain_version"` // fixed at 1 for this additive hash domain
    SourceSnapshot string `json:"source_snapshot"`
    JournalWatermark contracts.DeletionWatermark `json:"journal_watermark"`
    Generation int64 `json:"generation"`
    ProviderTruthAt string `json:"provider_truth_at"` // normalized UTC RFC3339Nano
    Accounts []string `json:"accounts"` // sorted unique IDs
}
type VerifiedEligiblePlan struct { artifact Plan; artifactHash LegacyPlanArtifactHash; contractPlan contracts.RecoveryPlan; contractHash RecoveryContractPlanHash } // private fields; minted only after both validations
func createVerifiedEligiblePlan(ctx context.Context, artifactDir string, input PlanInput) (VerifiedEligiblePlan, error) {
    created, err := CreatePlan(ctx, artifactDir, input)
    if err != nil { return VerifiedEligiblePlan{}, err }
    artifact, err := LoadPlan(ctx, artifactDir, created.PlanHash)
    if err != nil { return VerifiedEligiblePlan{}, err }
    observed, err := time.Parse(time.RFC3339Nano, artifact.ProviderObserved)
    if err != nil || observed.UTC().Format(time.RFC3339Nano) != artifact.ProviderObserved { return VerifiedEligiblePlan{}, ErrPlanInvalid }
    plan := contracts.RecoveryPlan{
        SourceSnapshot: artifact.SnapshotSHA256, JournalWatermark: artifact.JournalWatermark,
        Generation: artifact.FenceGeneration, ProviderTruthAt: observed.UTC(),
        Accounts: append([]string(nil), artifact.Accounts...),
    }
    contractHash, err := CanonicalContractRecoveryPlanHash(plan)
    if err != nil { return VerifiedEligiblePlan{}, err }
    plan.PlanHash = string(contractHash)
    if err = plan.Validate(); err != nil { return VerifiedEligiblePlan{}, err }
    return VerifiedEligiblePlan{artifact: artifact, artifactHash: LegacyPlanArtifactHash(artifact.PlanHash), contractPlan: plan, contractHash: contractHash}, nil
}
type EligibleEnvelopePayload struct {
    LegacyPlanArtifactHash LegacyPlanArtifactHash
    ContractPlan contracts.RecoveryPlan // existing PlanHash field contains RecoveryContractPlanHash
    EligibleAccountIDs []string // sorted exact eligible projection
}
type FrozenEnvelopePayload struct { Dispositions []FrozenAccountDisposition }
func CanonicalContractRecoveryPlanHash(contracts.RecoveryPlan) (RecoveryContractPlanHash, error) // excludes PlanHash; domain_version=1
func canonicalRecoveryEnvelopeHash(RecoveryEnvelopePayload, *VerifiedEligiblePlan) (RecoveryEnvelopeHash, error)
type FrozenRecoveryPlan struct {
    Version int
    PlanRef, SnapshotPinID, SnapshotSHA256, SnapshotInventoryDigest string
    ReservationVersion uint64
    SnapshotBuildSHA256, SnapshotSchemaVersion string
    SnapshotAccountIDs []string // complete restored inventory, sorted
    OperationID, PlanApprovalDigest, PlanApprovalNonce string
    PlanApprovalExpiresAt time.Time
    PlanApprovalRef EvidenceRef
    ActivationAllowlist []string // exact approved scope, may be empty
    EligibleAccountIDs []string // invariant: empty
    Dispositions []FrozenAccountDisposition // complete inventory; no eligibility claim
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    LastSealed contracts.DeletionWatermark
    SnapshotCut contracts.DeletionWatermark
    PrefixRef EvidenceRef
    RecoveryEnvelopeHash RecoveryEnvelopeHash // outer canonical union hash; frozen payload has no legacy artifact or contract plan
}
type FrozenRecoveryPlan struct {
    Version int
    PlanRef, SnapshotPinID, SnapshotSHA256, SnapshotInventoryDigest string
    ReservationVersion uint64
    SnapshotBuildSHA256, SnapshotSchemaVersion string
    SnapshotAccountIDs []string // complete restored inventory, sorted
    OperationID, PlanApprovalDigest, PlanApprovalNonce string
    PlanApprovalExpiresAt time.Time
    PlanApprovalRef EvidenceRef
    ActivationAllowlist []string // exact approved scope, may be empty
    EligibleAccountIDs []string // invariant: empty
    Dispositions []FrozenAccountDisposition // complete inventory; no eligibility claim
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    LastSealed contracts.DeletionWatermark
    SnapshotCut contracts.DeletionWatermark
    PrefixRef EvidenceRef
    RecoveryEnvelopeHash RecoveryEnvelopeHash // outer canonical union hash; frozen payload has no legacy artifact or contract plan
}
func (p FrozenRecoveryPlan) Validate() error // exact sorted inventory/disposition coverage; zero eligible IDs; valid source/cut/writer/approval/pin bindings
type EligibleRecoveryPlan struct {
    Version int
    PlanRef, SnapshotPinID string
    ReservationVersion uint64
    SnapshotSHA256, SnapshotInventoryDigest string
    SnapshotBuildSHA256, SnapshotSchemaVersion string
    SnapshotAccountIDs []string // complete restored inventory, sorted
    OperationID, PlanApprovalDigest, PlanApprovalNonce string
    PlanApprovalExpiresAt time.Time
    PlanApprovalRef EvidenceRef
    ActivationAllowlist, EligibleAccountIDs []string // both exact, sorted
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    LastSealed, SnapshotCut contracts.DeletionWatermark
    PrefixRef EvidenceRef
    LegacyPlanArtifactHash LegacyPlanArtifactHash
    Plan contracts.RecoveryPlan // RecoveryContractPlanHash in existing PlanHash field; unchanged Validate
    RecoveryEnvelopeHash RecoveryEnvelopeHash // outer tagged-union hash; never overwrite Plan.PlanHash
}
type PlannedRecovery struct {
    Kind RecoveryPlanKind
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    Eligible *EligibleRecoveryPlan // non-nil only for ELIGIBLE
    Frozen *FrozenRecoveryPlan // non-nil only for FROZEN_ONLY
}
type RecoveryPlanBuilder interface {
    PlanRecovery(ctx context.Context, req contracts.RecoveryPlanRequest) (PlannedRecovery, error)
}
func (p *Planner) PlanRecovery(ctx context.Context, req contracts.RecoveryPlanRequest) (PlannedRecovery, error)
// Planner implements the existing contracts.RecoveryPlanner after that amendment.
func (p *Planner) Plan(ctx context.Context, req contracts.RecoveryPlanRequest) (contracts.RecoveryPlan, error)

type ImmutableEvidenceReader interface { ReadExactVersion(ctx context.Context, ref EvidenceRef, maxBytes int) ([]byte, error) }
type EvidenceTrustPolicy interface {
    VerifyAuthoritySignature(ctx context.Context, authority, keyVersion string, canonicalPayload, signature []byte) error
    KeyUsableAt(ctx context.Context, authority, keyVersion string, signedAt time.Time) error
}
type ApprovalBinding struct {
    SnapshotSHA256 string
    AccountIDs []string // sorted, unique, opaque IDs
    OperationID string
    Nonce string
    ExpiresAt time.Time
}
type EpochApprovalBinding struct {
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    SnapshotSHA256, SnapshotInventoryDigest string
    SnapshotAccountIDs []string // exact full restored inventory, sorted
    EligibleAccountIDs []string // exact eligible subset; empty for FROZEN_ONLY
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    OperationID, Nonce string
    Actions []string // fixed set: quiesce, stop, revoke, seal, restore-frozen, publish-frozen, adopt
    ExpiresAt time.Time
}
type AccountActivationBinding struct {
    CommittedEpochRecordDigest EpochRecordDigest
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    AccountID, SnapshotSHA256 string
    OperationID, Nonce string
    ExpiresAt time.Time
}
type RecoveryApprovalVerifier interface {
    VerifyPlan(ctx context.Context, ref EvidenceRef, expectedSnapshotSHA256 string) (VerifiedPlanApproval, error)
    VerifyEpoch(ctx context.Context, ref EvidenceRef, expected EpochApprovalBinding) (VerifiedEpochApproval, error)
    VerifyAccountActivation(ctx context.Context, ref EvidenceRef, expected AccountActivationBinding) (VerifiedAccountApproval, error)
}
type VerifiedPlanApproval struct { binding ApprovalBinding; digest, verifierID, keyVersion string }
type VerifiedEpochApproval struct { binding EpochApprovalBinding; digest, verifierID, keyVersion string }
type VerifiedAccountApproval struct { committedEpochRecordDigest EpochRecordDigest; envelopeHash RecoveryEnvelopeHash; accountID, operationID, nonce, digest string; expiresAt time.Time }
type ContinuationApprovalBinding struct { RecoveryEnvelopeHash RecoveryEnvelopeHash; CurrentEpochRecordDigest EpochRecordDigest; ExactNextPhase, OperationID, Nonce string; ExpiresAt time.Time }
type AccountTerminalState string
const (
    WithheldFrozen AccountTerminalState = "WITHHELD_FROZEN"
    ActiveEligible AccountTerminalState = "ACTIVE_ELIGIBLE"
    Deleted AccountTerminalState = "DELETED"
    Deleting AccountTerminalState = "DELETING"
)
type AccountTerminalRecord struct {
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    RecordDigest AccountRecordDigest
    AccountID string // opaque ID; encrypted/private record store only
    State AccountTerminalState
    ReasonCode string
    ApprovalDigest, ObservationDigest, BindingDigest, CheckoutSetDigest string
    ProviderSourceVersion string
    ObservedAt, ExpiresAt time.Time
    RestoreMarker string; EpochRecordDigest EpochRecordDigest
}

type Config struct {
    StateRoot string
    SnapshotOptions backup.SnapshotLeaseStoreOptions
    PlanStore RecoveryPlanStore // also implements backup.SnapshotPinLifecycleAuthority
    Approvals RecoveryApprovalVerifier
    Billing RecoverySnapshotBillingObserver
    Journal RecoveryJournalStateSource
    ControlPlane RecoveryControlPlane
    EvidenceReader ImmutableEvidenceReader
    TrustPolicy EvidenceTrustPolicy
    Applier RecoveryRestoreApplier
    Limits Limits
}
func NewProductionCoordinator(ctx context.Context, cfg Config) (*Coordinator, error)
func newCoordinatorForTest(deps testDependencies) (*Coordinator, error) // package-private; never registered in CLI/service

type RecoveryEpochPhaseGuard struct { envelopeHash RecoveryEnvelopeHash; currentRecordDigest EpochRecordDigest; operationID, exactNextPhase string } // private fields; created only after owner CAS; passed to one effect and its evidence verifier
func (g RecoveryEpochPhaseGuard) EnvelopeHash() RecoveryEnvelopeHash
func (g RecoveryEpochPhaseGuard) CurrentRecordDigest() EpochRecordDigest
func (g RecoveryEpochPhaseGuard) OperationID() string
func (g RecoveryEpochPhaseGuard) ExactNextPhase() string
type RecoveryEpochRecord struct {
    Version int; Sequence uint64; PriorDigest string
    RecordDigest EpochRecordDigest // SHA-256 of this exact record payload with RecordDigest omitted
    PlanKind, PlanRef, OperationID, PlanApprovalDigest, PlanApprovalNonce string
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    EpochApprovalRef EvidenceRef; EpochApprovalDigest, EpochApprovalNonce, SnapshotInventoryDigest string
    ContinuationApprovalRefs []EvidenceRef // exact phase approvals; each separately signs the same envelope and current record digest
    SnapshotAccountIDs, PlanEligibleAccountIDs []string
    OldWriterDigest, JournalStoreID string
    SnapshotCut contracts.DeletionWatermark; PrefixDigest, SnapshotPinID string
    ReservationVersion, PlanRecordVersion, PinAttemptVersion uint64
    Phase string; StopRef, RevocationRef, SealRef, RestoreRef, PublishRef, AdoptionRef, AdmissionRef string
    SuccessorDigest string; UpdatedAt time.Time
}
type RecoveryPlanStore interface {
    ReservePlanRef(ctx context.Context, operationID string) (PlanRefReservation, error) // unique 64 lowercase hex; immutable ReservationVersion plus CAS RecordVersion
    BeginPinAttempt(ctx context.Context, planRef string, reservationVersion uint64, leaseID, digest string) (backup.PinAttemptRef, error) // exact owner CAS to PIN_PENDING
    CommitPinAttempt(ctx context.Context, attempt backup.PinAttemptRef, pin backup.SnapshotPinRef) error // exact attempt version CAS to PINNED
    CancelPinAttempt(ctx context.Context, attempt backup.PinAttemptRef) error // only after backup proves pin file absent under store lock
    FindPinAttempt(ctx context.Context, planRef, digest string) (backup.PinAttemptRef, error)
    ListPinAttempts(ctx context.Context) ([]backup.PinAttemptRef, error)
    PersistReadyPlan(ctx context.Context, planRef string, reservationVersion uint64, canonicalEnvelope []byte, envelopeHash RecoveryEnvelopeHash) error // immutable READY CAS
    MarkAbandonedBeforeEffects(ctx context.Context, planRef string, reservationVersion uint64, reasonCode string) error // CAS only if no active PIN_PENDING attempt/effect exists
    LoadPlanByRef(ctx context.Context, planRef string) (canonicalEnvelope []byte, envelopeHash RecoveryEnvelopeHash, state string, err error)
    MarkRecoveredAdmissionConfirmed(ctx context.Context, envelopeHash RecoveryEnvelopeHash, planRef string, admission EvidenceRef) error
    // Exact methods required by backup.SnapshotPinLifecycleAuthority:
    ReconcilePin(ctx context.Context, pin backup.SnapshotPinRef) (backup.PinReconcileDecision, error) // KEEP,nil for live/pending; RELEASE only with exact terminal authority
    CompletePinRelease(ctx context.Context, authorization backup.PinReleaseAuthorization) error
}
type PlanRefReservation struct {
    PlanRef string
    ReservationVersion uint64 // immutable token for this unique reference
    RecordVersion uint64 // owner CAS version increments on every state change
}
// RecoveryPlanStore must persist PIN_PENDING attempts append-only or with
// equivalent durable CAS history. BeginPinAttempt is exact-tuple idempotent;
// canceled attempts are tombstoned, attemptVersion strictly increases, and a
// stale attempt can neither CommitPinAttempt nor CancelPinAttempt.
type AccountRecoveryRecord struct {
    RecoveryEnvelopeHash RecoveryEnvelopeHash
    EpochRecordDigest EpochRecordDigest // exact prior/current epoch record authorizing this account transition
    OperationID, AccountID string
    Outcome, ReasonCode, ActivationApprovalDigest, ActivationApprovalNonce string
    RecordDigest AccountRecordDigest // account-state payload digest; excluded from its own hash
    ObservationDigest string; ObservedAt, ExpiresAt time.Time
    CustomerBindingDigest, RecoveryMarker string
}

type SnapshotBillingProjection struct {
    AccountID string
    Eligible bool
    ReasonCode string
    ObservationDigest string
    CustomerBindingDigest string
    ProviderSourceVersion string
    ObservedAt time.Time
    ExpiresAt time.Time
}
type RecoverySnapshotBillingObserver interface {
    ObserveSnapshotIdentity(ctx context.Context, candidate backup.VerifiedAccountCandidate) (SnapshotBillingProjection, error)
}

type WriterObservation struct { Identity WriterIdentity; LastSealed contracts.DeletionWatermark; ObservedAt time.Time; Evidence EvidenceRef }
type RecoveryJournalStateSource interface {
    CurrentWriter(ctx context.Context) (WriterObservation, error)
    ReadAncestry(ctx context.Context, storeID string, from, through contracts.DeletionWatermark) (EvidenceRef, error)
    SealStoppedGeneration(ctx context.Context, guard RecoveryEpochPhaseGuard, generation int64, after contracts.DeletionWatermark) (EvidenceRef, error)
    AdoptSuccessor(ctx context.Context, guard RecoveryEpochPhaseGuard, predecessor WriterIdentity, generation int64, sealDigest string) (EvidenceRef, error)
    ReserveRecoveryEpoch(ctx context.Context, envelopeHash RecoveryEnvelopeHash, operationID string, oldWriter WriterIdentity) (EvidenceRef, error)
    CommitRecoveryEpoch(ctx context.Context, guard RecoveryEpochPhaseGuard, reservationRef, adoptionRef EvidenceRef) error
}

type RecoveryControlPlane interface {
    QuiesceService(ctx context.Context, guard RecoveryEpochPhaseGuard, writer WriterIdentity) (EvidenceRef, error)
    StopWriter(ctx context.Context, guard RecoveryEpochPhaseGuard, writer WriterIdentity) (EvidenceRef, error)
    RevokeEveryCredentialAndSession(ctx context.Context, guard RecoveryEpochPhaseGuard, writer WriterIdentity) (EvidenceRef, error)
}
type RecoveryEvidenceVerifier interface {
    VerifyWriter(ctx context.Context, observation WriterObservation) (VerifiedWriterIdentity, error)
    VerifyJournalPrefix(ctx context.Context, ref EvidenceRef, storeID string, from, through contracts.DeletionWatermark) (VerifiedJournalPrefix, error)
    VerifyEpochReservation(ctx context.Context, ref EvidenceRef, envelopeHash RecoveryEnvelopeHash, operationID string, writer WriterIdentity) (VerifiedEpochReservation, error)
    VerifyQuiesced(ctx context.Context, guard RecoveryEpochPhaseGuard, ref EvidenceRef, writer WriterIdentity) (VerifiedQuiescence, error)
    VerifyStopped(ctx context.Context, guard RecoveryEpochPhaseGuard, ref EvidenceRef, writer WriterIdentity) (VerifiedStop, error)
    VerifyRevocation(ctx context.Context, guard RecoveryEpochPhaseGuard, ref EvidenceRef, writer WriterIdentity) (VerifiedRevocation, error)
    VerifySeal(ctx context.Context, guard RecoveryEpochPhaseGuard, ref EvidenceRef, generation int64, after contracts.DeletionWatermark) (VerifiedSeal, error)
    VerifyAdoption(ctx context.Context, guard RecoveryEpochPhaseGuard, ref EvidenceRef, predecessor WriterIdentity, successor WriterIdentity, seal VerifiedSeal) (VerifiedAdoption, error)
}
type RecoveryRestoreApplier interface {
    StageAndReconcile(ctx context.Context, guard RecoveryEpochPhaseGuard, snapshot *backup.VerifiedSnapshotLease, seal VerifiedSeal, limits Limits) (StagedRestore, error)
    Publish(ctx context.Context, guard RecoveryEpochPhaseGuard, staged StagedRestore, expectedDestinationID string) (PublishedRestore, error)
}

type AccountActivationResult struct { State, ReasonCode string; RecordDigest AccountRecordDigest }
// Proposed additions under internal/hosted/service. The active service
// process owns this mutation guard and all SQL writes. The read-only billing
// observer is a separate dependency and is not a lock.
type AccountMutationGuard interface {
    WithAccountMutation(ctx context.Context, accountID string, fn func(AccountMutationView) error) error
}
type AccountMutationView interface {
    ReadCurrentAccountAndCheckoutDigest(ctx context.Context) (CurrentAccountView, error)
    ReadDeletionStateThrough(ctx context.Context, seal EvidenceRef) (DeletionDisposition, error)
    ObserveRestoredRecoveryAccount(ctx context.Context, expected CurrentAccountView, restoreMarker string) (SnapshotBillingProjection, error)
    CompareAndActivate(ctx context.Context, expected CurrentAccountView, expectedRestoreMarker string, expectedEligibleProjectionDigest string) error // refuses unless the exact row remains restore_pending and projection is eligible
}
type CurrentAccountView struct { AccountID, Status, CustomerBindingDigest, CheckoutSetDigest, RowDigest string }
type DeletionDisposition string
const (
    NoDeletion DeletionDisposition = "NONE"
    AccountDeleted DeletionDisposition = "DELETED"
    AccountDeleting DeletionDisposition = "DELETING"
)
type RecoveryActivationCommand struct { CommittedEpochRecordDigest EpochRecordDigest; RecoveryEnvelopeHash RecoveryEnvelopeHash; AccountID string; ApprovalRef EvidenceRef }
type RecoveryActivationHandler interface {
    HandleRecoveryActivation(ctx context.Context, command RecoveryActivationCommand) (AccountActivationResult, error)
}

type WriterIdentity struct { Provider, ScopeRef, WriterRef, BootRef, JournalStoreID string; Generation int64 }
// Exported result types have private fields and are created only by their authority adapters.
type VerifiedWriterIdentity struct { identity WriterIdentity; providerEvidence EvidenceRef; verifierID, digest string; verifiedAt time.Time }
type VerifiedJournalPrefix struct { from, through contracts.DeletionWatermark; storeID, digest string; verifiedAt time.Time }
type VerifiedEpochReservation struct { envelopeHash RecoveryEnvelopeHash; operationID, oldWriterDigest, lockDigest string }
type VerifiedSeal struct { watermark contracts.DeletionWatermark; generation int64; evidence EvidenceRef; digest string; verifiedAt time.Time }
type VerifiedAdoption struct { predecessorDigest, successorDigest, sealDigest, evidenceDigest string; generation int64; verifiedAt time.Time }
type VerifiedQuiescence struct { evidenceDigest string; operationID string; verifiedAt time.Time }
type VerifiedStop struct { evidenceDigest, writerDigest string; verifiedAt time.Time }
type VerifiedRevocation struct { evidenceDigest, writerDigest, inventoryDigest string; itemCount int; verifiedAt time.Time }
type StagedRestore interface { StagingID() string }
type PublishedRestore interface { DestinationID() string; ManifestSHA256() string }
type Limits struct { MaxManifestBytes, MaxArtifactBytes, MaxTotalBytes int64; MaxPlanAccounts, MaxSnapshotAccounts, MaxBrains, MaxCheckoutRefs, MaxEvidenceBytes, MaxCanonicalRecordBytes, MaxProviderGETs, MaxCLIOutputBytes int; MaxConcurrentProviderCalls int; ProviderTimeout, PhaseTimeout, MaxObservationAge time.Duration }
type BeginEpochRequest struct { RecoveryEnvelopeHash RecoveryEnvelopeHash; EpochApprovalRef EvidenceRef }
type ContinueEpochRequest struct { RecoveryEnvelopeHash RecoveryEnvelopeHash; ContinuationApprovalRef EvidenceRef }
type CommitEpochRequest struct { RecoveryEnvelopeHash RecoveryEnvelopeHash }
type EpochStatus struct { RecoveryEnvelopeHash RecoveryEnvelopeHash; OperationID, Phase string; RecordDigest EpochRecordDigest }

type RecoveryEpochCoordinator interface {
    Begin(ctx context.Context, req BeginEpochRequest) (EpochStatus, error)
    Continue(ctx context.Context, req ContinueEpochRequest) (EpochStatus, error)
    Commit(ctx context.Context, req CommitEpochRequest) (EpochStatus, error)
}
// Coordinator implements contracts.RecoveryPlanner for ordinary eligible plans
// and PlanRecovery for the eligible/frozen-only union. Begin loads and dispatches
// the persisted plan kind; FROZEN_ONLY cannot enter account activation.
```

`NewProductionCoordinator(ctx,cfg)` is the only production factory. It constructs `backup.NewSnapshotLeaseStore(ctx, cfg.SnapshotOptions, cfg.PlanStore)` so the very same recovery plan store is the durable lifecycle authority; it also binds the billing projection, journal authority, control-plane executor, evidence reader, and restore applier. It rejects missing or test-only implementations and never loads them from CLI input. The `RecoveryEvidenceVerifier` and approval verifier are recovery-package concrete implementations that consume exact-version `ImmutableEvidenceReader` output and verified operator-case verifier output; they construct the private-field `Verified*` values. `newCoordinatorForTest` is package-private. No serialized evidence can be unmarshaled directly into one of those values. Each production factory is a required reviewed handoff from the owning package; absence fails construction.

The cross-owner transaction follows backup revision `6a013b49b94e7f9286b782ad2827441d2e5bd737`: the recovery store first durably reserves a unique canonical 64-lowercase-hex `planRef` and returns immutable `reservationVersion` plus the current CAS `recordVersion`; the owner remains RESERVED during backup staging, then backup `Stage(ctx, sourcePath, InspectionOptions{ExpectedManifestSHA256: digest})` verifies/copies exact bytes. Backup `Pin(ctx, planRef, reservationVersion, digest)` acquires its per-ref lock and calls recovery `BeginPinAttempt`, an owner-side CAS to durable `PIN_PENDING` binding planRef, staged lease ID, digest, reservationVersion, and monotonic attemptVersion. This durable pending record blocks abandon/unrelated advancement. Backup then fsyncs the pin record/root and calls `CommitPinAttempt` for that exact attempt; only after owner CAS to `PINNED` does `Pin` return a durable `SnapshotPinRef`. Recovery atomically persists an immutable READY plan envelope containing planRef, pin ID, reservationVersion, digest, full inventory, exact ordinary-or-frozen plan, and canonical content/hash. The outer `RecoveryEnvelopeHash` includes the pin ID; the pin itself binds only planRef+leaseID+manifestDigest, so no self-reference exists. The global epoch approval is signed over the final `RecoveryEnvelopeHash` (never the legacy contract plan hash). If interrupted after `PIN_PENDING`, retry calls backup `ResumePin(planRef,reservationVersion,digest)`, which uses exact `FindPinAttempt` state and staged lease identity to finish or confirms the exact durable pin. If the pin fsynced but the recovery READY envelope did not, `FindPinned(planRef,digest)` returns the unique exact `SnapshotPinRef` needed to complete that same envelope. None/conflict remains a hard refusal; never silently restage or create a second pin. Before abandoning a never-started plan, recovery retains the exact `PinAttemptRef` returned by `BeginPinAttempt` (or exact `FindPinAttempt`) and passes that token to `CancelPin`; its planRef, leaseID, digest, reservationVersion, and attemptVersion bind the cancellation. Backup proves that exact pin file absent under lock and owner-cancels/tombstones only that attempt before `MarkAbandonedBeforeEffects` can pass. If the cancel response was lost, retrying the same canceled token is idempotent from its tombstone and cannot inspect or cancel a later attempt. Once the durable pin exists, cancellation refuses; use authoritative pre-effect abandonment and terminal pin release. After READY, retry loads the immutable envelope and performs `ResolvePinned(ctx,pinID,digest,planRef)` then `ReopenPinned`; it never rebuilds in place. Recovery's owner plan store must implement the exact `SnapshotPinLifecycleAuthority` CAS/version methods and `ReconcilePin` semantics below; pin/plan source authority remains separate from billing or writer authority.

Plan lifecycle states are `RESERVED`, `PIN_PENDING`, `PINNED`, `READY`, `APPLYING`, `FORWARD_RESUMABLE`, `ABANDONED_BEFORE_EFFECTS`, `COMMITTED`, `ADMISSION_CONFIRMED`, plus backup pin cleanup `RELEASING`/`RELEASED`. Keep exact staged lease/pin references through every source-consuming or forward-resumable phase. The lifecycle authority contract uses `BeginPinAttempt`, `CommitPinAttempt`, `CancelPinAttempt`, `FindPinAttempt`, `ListPinAttempts`, `ReconcilePin`, and `CompletePinRelease`; every attempt binds planRef, reservationVersion, leaseID, manifest digest, attemptVersion, and state. ReservationVersion is immutable; the plan owner recordVersion advances on every CAS. `BeginPinAttempt` is idempotent for only the exact tuple and rejects stale/different tuples; commit/cancel of stale attempts fails. Owner CAS `recordVersion` is checked on every mutation and increments monotonically; `reservationVersion` remains fixed across exact pin retries/cancellation, while `attemptVersion` increases per new attempt and canceled attempts are tombstoned. A PIN_PENDING owner record protects its staged lease from orphan cleanup. `ResumePin` repairs after process loss and `CancelPin(exactAttempt)` is the only return to RESERVED before that exact attempt’s pin file exists; the durable canceled-attempt tombstone makes retries idempotent without resolving a newer attempt. `ReconcilePin` returns typed `PinKeep,nil` for normal active/PIN_PENDING/PINNED pins (successful no-op), and typed `PinRelease` only when owner phase guard proves exact `ABANDONED_BEFORE_EFFECTS` or `COMMITTED` restore with durable admission and no source consumer. Backup `Reconcile` preserves owner-listed pending lease IDs and active pin bytes; it releases only by exact authorized tuple, tracks `RELEASING`/`RELEASED`, drains borrowers, and repairs cleanup/finalization after crashes. No TTL/age deletion of pins; only staged-but-unpinned records proven orphaned by backup reconciliation may use its bounded grace cleanup.

The billing owner adds a narrow `ObserveSnapshotIdentity(ctx, backup.VerifiedAccountCandidate)` read-only projection for planning, alongside a distinct `ObserveRestoredRecoveryAccount` projection for one-account activation. Both preserve the existing `ObserveRestorePending` observer unchanged. The planning API treats every backup candidate as a historical hint only and requires an authoritative customer/account relation; never use it to make an activation decision after the global admission pin is released. The activation API receives the service owner's current row view and exact restore marker from the shared account guard, then validates current account/status/customer binding and checkout rows, performs bounded provider reads/refreads, compares exact complete sets and source versions, and rereads current account/status/binding/checkout state before returning. It returns only `Eligible`, reason code, opaque binding/checkout digests, provider source version, observation/expiry times, and canonical digest; it performs no writes. Any missing, added, removed, conflicting, pending, unknown, unqueryable, or changed ref fails closed. Do not accept caller paths, historical snapshot customer values, or caller-supplied provider IDs in the activation API. The final service-side CAS still compares against the original guarded row view. Existing `ObserveRestorePending` behavior remains unchanged.

For `ObserveSnapshotIdentity`, do not route planning calls through `ObserveRestorePending`, which correctly accepts only current `restore_pending` rows. Snapshot identity fields are lookup candidates, not billing authority. The observer validates the backup-sealed candidate/digest and independently resolves the account/customer relationship before provider calls. For `ObserveRestoredRecoveryAccount`, derive identity only from the actual restored current row under the service guard; require row and restore marker match, check current account/customer binding and all current checkout rows, then perform bounded provider calls/refreads and a final reread. If the current row is absent or the mapping is ambiguous, fail closed. No new provider metadata key is assumed; the billing owner must name the existing verified mapping or keep either API fail-closed.

For an identity-verified candidate, make bounded read-only provider calls for current subscriptions/invoices and each exact snapshot checkout/session reference using the provider's supported read API. Any snapshot or current checkout ref that is missing, unknown, pending, nonterminal, duplicated, or unqueryable blocks; do not expire, delete, reconcile, or clear it. Compare current checkout row refs/state with the snapshot-derived set; conflict blocks. After provider I/O, reread account status/customer binding and checkout rows; require status/binding/ref digests unchanged from the pre-read, and refuse if any row changed, was added, disappeared, or became nonterminal. Verify current provider price/window/grace and source object versions, with `CustomerBindingDigest`, `ProviderSourceVersion`, `ObservedAt`, `ExpiresAt`, and canonical `ObservationDigest` in the return. No SQL, entitlement, checkout, or audit writes occur. If the provider lacks a read endpoint for an exact checkout reference, that candidate is blocked rather than treated as resolved.

`CurrentWriter` returns separate facts: active old writer identity/generation `G`, stable journal-store ID, last previously sealed historical watermark `H` (possibly older than `G`), observation time, and immutable evidence reference. The recovery-owned verifier validates the claim and constructs private `VerifiedWriterIdentity`. `ReadAncestry` returns an immutable reference; recovery re-fetches that exact version and `VerifyJournalPrefix` proves the durable chain from zero through snapshot cut `C`; it may include the unsealed active-generation prefix through `C`. It does not require or synthesize the future seal. Plan records `H`, `C`, and `G` separately and rejects a cut not present in verified ancestry. `ReserveRecoveryEpoch` is an authoritative durable CAS on the same generation control plane: after success, no second recovery epoch or regular writer start/generation advance is possible for this journal until the exact operation commits or an explicit reviewed abort completes. It is operation-owned and does not auto-expire into another writer's authority; its immutable reference is re-fetched and verified before use. After verified stop and complete revocation, `SealStoppedGeneration(guard, G, C)` creates the post-fence seal; independently verify `ReadThrough(zero)` complete through this exact seal `S` before restore. Persist `H`, `C`, `G`, `S`, and successor `G+1` as distinct values. Only after restore reconciliation may `AdoptSuccessor(guard, ...)` create the immutable `G+1` adoption record bound to predecessor identity, seal digest, successor identity, and generation. Generation write authority remains blocked while the reservation is held. `CommitRecoveryEpoch(guard, ...)` atomically commits terminal epoch/adoption and releases the reservation only to the exact successor. No value is inferred from a configured generation or empty prefix.

## Approval, plan, and exact scope

The signed plan operator case must bind raw snapshot SHA-256, sorted allowlisted account IDs (the list may be explicitly empty), one recovery `operation_id`, a cryptographically unique `approval_nonce`, and expiry. `VerifyPlan` returns a recovery-owned verified approval containing the canonical binding and verifier/key identity. `PlanRecovery` compares digest before provider calls, intersects snapshot IDs with that allowlist, and makes provider calls only for the sorted intersection. Snapshot accounts outside the approved set are neither queried nor placed in an activation scope; they remain in the exact full restore inventory and are terminally frozen. With one or more eligible accounts, persist the ordinary `RecoveryPlan` (which still must pass its unchanged `Validate`). With zero eligible accounts and complete valid evidence, persist the distinct `FrozenRecoveryPlan` above, with an empty `EligibleAccountIDs` and one disposition for every inventory account; this plan is executable through `Begin` only when the persisted type is `FROZEN_ONLY`. The legacy `Plan` method may return `ErrNoEligibleAccounts`, but the production CLI/service path must call `PlanRecovery` and retain the valid frozen-only envelope. Neither path records eligibility where none was established. Both plan payloads bind approval digest/nonce, operation ID, sorted allowlist, selected eligible IDs, raw manifest digest, separate build/schema provenance, exact inventory/digest, old writer identity/generation/store, last previously sealed watermark, snapshot cut, and verified existing journal-prefix reference. Any change to these changes the outer `RecoveryEnvelopeHash`; changing artifact inputs changes `LegacyPlanArtifactHash`; changing mapped contract fields changes `RecoveryContractPlanHash`; both change the outer hash.

Planning sequence: verify the signed plan operator case against the exact expected source digest and scope; durably reserve `planRef` and immutable `reservationVersion`; stage with per-call bounded `InspectionOptions` including `ExpectedManifestSHA256`; pin immediately using `(planRef,reservationVersion,digest)` and persist/retain its `SnapshotPinRef`; then read complete candidate inventory, active writer `W/G`, stable store identity, and historical sealed watermark `H`; verify ancestry from zero through snapshot cut `C` without sealing; query the read-only billing projection only for allowlisted candidate accounts, with fixed worker and call/byte limits; fail on ambiguous or incomplete evidence; assemble either valid `EligibleRecoveryPlan` with its unchanged validated `contracts.RecoveryPlan` or exact `FrozenRecoveryPlan` with zero eligible IDs and full terminal dispositions; atomically persist canonical immutable READY envelope/hash; return only when both READY record and owner-committed pin are durable. Empty allowlist is valid signed scope, produces zero provider calls, and may produce only FROZEN_ONLY. On pre-effect failure, cancel only exact `PinAttemptRef` tokens for PIN_PENDING attempts after backup proves that exact attempt has no durable pin; then mark abandonment and call reconciliation. A durable pin is never silently discarded.

The verified plan approval scope bounds provider calls. The executable plan may contain only eligible IDs in the sorted intersection of the plan allowlist and complete snapshot inventory; provider calls are made only inside this exact scope. `Begin` additionally requires an independently signed *global epoch approval* binding the `RecoveryEnvelopeHash`, raw source digest, full sorted restored inventory and canonical inventory digest, exact plan allowlist/selected scope, old writer identity/generation/store, operation ID, nonce, expiry, and the fixed global action set. That approval authorizes physical whole-database restore and fencing only; it carries no account activation authority. `Begin` reopens the pinned snapshot and re-verifies the exact manifest digest and complete inventory against the plan and epoch approval before any reservation, quiescence, stop, revocation, or seal. Bad/missing global approval fails before effects. Missing account approvals do not block global restore.

Each account activation requires a separate signed approval binding the committed epoch record digest, exact `RecoveryEnvelopeHash`, and exactly one account ID, source snapshot digest, operation ID, fresh nonce, and expiry. `Begin` atomically consumes the global approval nonce against the exact `RecoveryEnvelopeHash`. Each `ActivateOne` consumes a one-account nonce. Exact tuple retry resumes; nonce reuse with any changed tuple is replay. Initial approval expiry is checked at first consumption; an already-consumed approval may be used for exact crash resume only if its key has not been revoked. A missing, bad, expired, or revoked one-account approval creates a durable `WITHHELD_FROZEN` result; provider ineligibility, ambiguity, staleness, or deletion does likewise (or preserves `DELETED`/`DELETING`). These are terminal global-restore dispositions, not successful eligibility. An account withheld at epoch commit can later be activated only with a new approval and current checks. Rejected attempts are minimized private audit records; never log signed payloads or secrets.

## Serialized epoch and per-account state machine

Add private canonical records in the recovery owner package, with append-only hash chaining, owner-only permissions, strict versioned JSON, monotonic sequence, CAS on prior digest, fsync-before-effect, and atomic directory updates. `RecoveryEpochRecord` binds canonical envelope `RecoveryEnvelopeHash`, planRef, global approval ref/digest/nonce, exact source and full sorted inventory digest/list, operation ID, old writer identity/generation/store, H/C/prefix digest, pinned lease ID/digest, phase, effect-start marker, reservation, stop/revocation/seal/restore/publication/adoption references and verifier digests, successor, destination identity, terminal account inventory digest, sequence/time. `AccountRecoveryRecord` binds `RecoveryEnvelopeHash` and the exact epoch `EpochRecordDigest` for the account transition, exactly one account, optional activation approval digest/nonce, customer-binding and observation digests/times/expiry/source version, terminal state/reason, recovery marker, and last epoch-record digest. No customer IDs, token values, raw paths, credentials, or broad provider responses. The durable reservation has no auto-expiry that transfers write authority.

`Begin(envelopeHash, ...)` loads by exact `RecoveryEnvelopeHash`, revalidates exact current `W/G/store`, the plan-creation approval, the global epoch approval, full inventory, and pinned lease; validates the entire intended destination/scope before writing any effect marker; then atomically reserves `(operation, plan, global nonce)` and persists the initial record. Only after durable `EFFECTS_MAY_HAVE_STARTED` may it quiesce or fence. The reservation blocks any competing epoch/writer start until commit or a pre-effect safe abort. If no effect call was issued, safe abort verifies old writer/credentials unchanged, unsquiesces, and releases reservation. Once an effect may have started, only exact forward resume or a newly authorized same-epoch continuation is allowed; never restart old G or restore old credentials. A timeout is treated as uncertain effect, not safe abort.

Ordered global phases, with durable write before each external effect:

1. `EPOCH_RESERVED`: full global approval/source/inventory verified, exact plan bound, reservation held.
2. `SERVICE_QUIESCED`: supervisor quiesces exact service identity and verifies no writes.
3. `OLD_WRITER_STOPPED`: stop exact W/G and independently verify immutable provider evidence.
4. `OLD_CREDENTIALS_REVOKED`: enumerate every credential and issued session, revoke/deny all, independently verify full inventory/effective denial; unknown classes block forward progress.
5. `OLD_GENERATION_SEALED`: seal G after C only after stop/revocation; verify full history from zero through post-fence S; retain H, C, G, S, G+1 distinctly.
6. `RESTORE_STAGED_FROZEN`: restore only from pinned verified lease to fresh private same-volume destination. Preserve restore_pending/Free accounts and restore_pending subscriptions; replay all journal intents through S so deletes cannot be revived. Validate exact full inventory and freeze predicate. No account activation is applied to this destination during the global epoch.
7. `SUCCESSOR_ADOPTED`: after reconciliation, adopt exactly one successor G+1 bound to W/G, S, store, operation. Reservation prevents any other G+1 write.
8. `RESTORE_PUBLISHED`: persist intended destination identity, atomically publish the complete validated frozen database, verify identity after crash; service admission remains denied.
9. `EPOCH_COMMITTED`: require a terminal disposition for every account in the complete restored inventory, including accounts outside plan allowlist (which remain withheld frozen) and journal-deleted/deleting rows. Verify destination, replay, adoption, and all durable records; commit reservation only to successor. A missing/bad account approval is terminal frozen, never a reason to withhold host recovery. Only this terminal global epoch can authorize recovered-start admission.
10. `RECOVERED_ADMISSION_CONFIRMED`: the service independently verifies the committed epoch and persists the exact admission receipt before starting the recovered service. Only this state allows the pin-release handshake; if admission does not complete, retain the pin and resume from committed frozen destination.
11. `RELEASING` then `RELEASED`: under the exclusive phase guard, recovery persists the exact terminal disposition and invokes backup `SnapshotLeaseStore.Reconcile(ctx)`. Backup obtains `AuthorizePinRelease` and `CompletePinRelease` through the injected plan-store lifecycle authority and repairs its own exact `RELEASING`/`RELEASED` sequence. A crash in either release state retries cleanup; activation never needs the snapshot after admission because it uses only the current restored row and fresh provider/deletion checks.

`Commit` never requires all accounts to be active or currently provider-eligible; it requires every inventory account to be explicitly terminal. Whole database publishes frozen first, then recovered service starts. The terminal per-account account-state ledger must agree with preserved deletion journal truth: deletion wins. No “auto-exclude” changes signed scope; outside-scope IDs are explicitly `WITHHELD_FROZEN/NOT_IN_APPROVED_SCOPE` and receive no provider calls. A continuation approval can authorize only exact forward phase progress within the same full inventory and cannot alter per-account dispositions except through separately approved activation.

After `EPOCH_COMMITTED`, the **service-owned** `RecoveryActivationHandler.HandleRecoveryActivation` acts on one account in the already-published destination; it cannot call restore, replay, seal, adoption, or publication and no longer needs the backup snapshot pin. The active service process obtains the shared in-process per-account mutation guard also used by forget/delete/status mutation. Under that guard it checks the actual current account row and checkout-set digest, requires the exact epoch restore marker and `restore_pending`/Free status, obtains deletion state through committed S, verifies the one-account approval through the recovery authority, calls the billing owner's read-only `ObserveRestoredRecoveryAccount` using current restored-row identity, and performs a final current-row reread followed by the service-owned compare-and-swap. `AccountMutationGuard` is distinct from, and held around, the read-only provider observer; the observer's own lock is not the serialization boundary. Do not hold a SQL transaction open over provider I/O: retain the service-owned account guard, then open a short transaction for final reread/CAS. Any missing/bad approval, deleted/deleting state, or ineligible/ambiguous/stale/conflicting projection records `WITHHELD_FROZEN` and leaves the row frozen. The handler's dependencies are the recovery approval/epoch verifier, billing read-only projection, committed deletion authority, and `AccountMutationGuard`; the concrete guard and row CAS live inside `internal/hosted/service`, and SQL write authority stays there. A CLI `activate-one` command sends the typed command over an authenticated local RPC to the currently running service; it never opens the database or calls the billing observer directly. If the active service/RPC/guard is unavailable, activation is unavailable and the row remains frozen. Repeated activation is idempotent by `(epoch, account, nonce)`; a later attempt needs a new nonce/approval and current checks against the same restored data. These are requested owner changes; none is implemented by this proposal.
All global epoch record, recovery admission and per-account store APIs are keyed by `RecoveryEnvelopeHash` plus their own `EpochRecordDigest`; only account activation consumes a committed epoch record digest, and no epoch API passes outer hashes to legacy plan/fence methods. The concrete `FROZEN` target is the private restored destination with every surviving account `restore_pending`/Free and every subscription `restore_pending`, plus journal deletions replayed through `S`; old service remains quiesced. Every row remains frozen in the private destination and after publication. Before atomic publication, failed/canceled work leaves live destination unchanged. After publication but before epoch commit, service admission remains denied and no writer may start. This exact state is resumable; never auto-roll back by re-enabling the old writer. A crash around rename is resolved by comparing the persisted intended destination identity and exact manifest/restore digests. A crash around commit is resolved from the terminal epoch record; never assume timeout means rollback.

There is no recovery-package `Apply` endpoint for this operation; the global epoch path does not call legacy `Apply` or treat the outer hash as `contracts.RecoveryPlan.PlanHash`. `internal/hosted/service.RecoveryActivationHandler` is callable only after `EPOCH_COMMITTED`, accepts one `(committed_epoch_record_digest, envelope_hash, account_id, account_approval_ref)`, and owns the account guard plus final row CAS. The CLI reaches it only through the authenticated local service RPC, passing both committed epoch record digest and outer envelope hash. It cannot stop writers, seal, restore, adopt, republish, or admit startup. No public `apply-all` endpoint.

The append-only epoch record is hashed independently; its `RecordDigest` excludes itself and binds the outer `RecoveryEnvelopeHash`, independently signed global approval reference/digest, prior record digest, and exact stop, complete revocation, post-watermark seal, restore/publication, adoption, and admission references. Admission re-fetches those immutable references against the committed epoch record digest. `FenceReceipt.SufficientFor(contracts.RecoveryPlan)` remains only the contract-plan shape/binding check; its booleans and timestamp are compatibility summary only. Account activation must continue to enforce the existing single-account consistency/binding predicate where its additive service contract exposes it, but neither that predicate nor a receipt substitutes for current guarded billing/deletion checks or the global evidence references.

The `internal/cli/hosted_recovery.go` entrypoint sequence is `plan --snapshot --expected-manifest-sha256 --approval-ref` (returns a discriminated `ELIGIBLE` or `FROZEN_ONLY` planRef and `RecoveryEnvelopeHash`; for ELIGIBLE it may also display the legacy contract plan hash labeled explicitly), `begin --envelope-hash --epoch-approval-ref` (loads the immutable plan kind, accepts `FROZEN_ONLY` only as frozen restore, and verifies full inventory before any disruptive effect), `continue --envelope-hash --continuation-approval-ref` (same-epoch forward progress), `commit --envelope-hash` (requires full restore/adoption and terminal disposition for every inventory account), `activate-one --committed-epoch-record-digest --envelope-hash --account-id --approval-ref` (only after commit, sent through authenticated local RPC to the active service), and `status --envelope-hash` (redacted persisted phase). Every command returns nonzero on incomplete evidence. `begin/continue/commit` are resumable only under the persisted plan operation/global approval/continuation bindings; caller flags cannot override account scope, generation, provider identity, or operation ID. `Begin` dispatches only from the persisted plan kind and fails if a caller-supplied mode disagrees; no separate `BeginFrozen` endpoint is added.

## Startup paths this contract does not implement

`RecoveryAdmission` is a one-time recovered-start proof keyed by the exact committed `RecoveryEnvelopeHash` and `EpochRecordDigest` at sealed S. It re-verifies the exact outer hash and committed epoch-record digest against owner storage. It does not supply authenticated journal genesis and does not solve an ordinary process restart after successor G+1 has appended an unsealed tail. Do not fabricate an empty plan, generation zero, bootstrap seal, or reuse the previous recovery admission to accept that tail. Genesis needs a journal-owner authenticated durable genesis authorization bound to store and initial writer. Ordinary restart needs a separately reviewed startup handshake that verifies active writer/store/generation and the complete authorized chain/head including a valid unsealed G+1 tail. Current `service.AssembleWithDependencies` requires sealed-boundary evidence, so that restart path remains fail-closed until service/journal owners add and review the distinct path. Therefore this proposal is code-ready only for recovery planning, frozen whole-database recovery, committed recovered-start admission, and later single-account activation; it does not claim full hosted startup readiness.

## Production admission and AWS boundary

Request `service.RecoveryAdmission` construction as `recovery.NewAdmission(epochStore, activeWriterIdentitySource, journalIdentityVerifier, buildSHA)`; it uses the package-owned evidence verifier and cannot accept one from service config or CLI. Runtime identity must come from an owner-reviewed local supervisor/control-plane channel with peer authentication; no static environment generation, caller value, or app IMDS. `Admit(ctx, envelopeHash, committedRecordDigest, journal)` accepts only the exact `EPOCH_COMMITTED`, re-verifies stop, complete revocation, seal, publication, adoption, approval/plan/source provenance, all account records, and active successor identity, then checks the supplied journal is the adopted stable store. It returns the snapshot cut only after verified history reads establish the same sealed boundary; existing service assembly then retains its current full-history and intent checks. The same configured journal object/store must be wired into service, backup, fence, and admission. Any absent/stale evidence, mismatched store/build, nonterminal inventory disposition, or active competing epoch refuses recovered-start admission. This factory covers only the first service admission following a committed recovery epoch and its sealed boundary; it is not a general startup factory.

Preserve the hosted app's current IMDS denial. No instance profile, broad host credentials, default AWS credential chain, or app authority to stop instances/revoke credentials/adopt generations. A separately provisioned control-plane executable uses explicit scoped role credentials and the AWS CLI with fixed argument arrays, explicit region/profile, bounded context, scrubbed environment, and captured request IDs. For EC2 stop it verifies exact account/region/instance identity with `aws ec2 describe-instances`, calls `aws ec2 stop-instances`, waits with `aws ec2 wait instance-stopped`, then re-describes and persists immutable evidence references. Revocation must enumerate actual deployment credential/session classes at their owning authorities, revoke or deny each, and independently verify denial. EC2 stop does not establish revocation. The actual identity model, AWS role, policy scope, and credential classes remain owner inputs; no invented permissions or metadata key are proposed. The verifier retrieves provider evidence and checks exact subject/scope/version/status; CLI output or fake output alone is not evidence.

## Bounds and deterministic fake-only RED controls

Use these proposed hard ceilings (integrator owners may lower them; raising one requires a reviewed policy update): at most 100 accounts per recovery plan; 10,000 snapshot accounts/brains at the inspector boundary; 64 checkout refs per account; `SnapshotLeaseStoreOptions` require an explicit validated owner-only root and positive independent per-lease artifact, metadata and restore-scratch limits, plus aggregate retained artifact/metadata byte ceilings and `MaxLeases` across both staged leases and durable pins; per-lease artifact/scratch ceilings cannot exceed 1 TiB and metadata records cannot exceed 1 MiB; `InspectionOptions.MaxDeclaredBytes` is positive and at most 1 TiB and account/brain counts remain 1..10,000; 4 concurrent provider calls; 4,096 provider GETs per plan; each provider timeout at most 30 seconds, response at most 1 MiB, total response bytes at most 64 MiB; evidence record at most 1 MiB; canonical plan/epoch record at most 4 MiB; each operation invocation at most 30 minutes before returning resumable state; provider observation age at most 5 minutes at one-account activation; snapshot declared bytes capped by explicit SSD-backed configuration no greater than the inspector's 1 TiB absolute limit. Existing billing observer parser ceilings remain in force (JSON depth 32, nodes 100,000, event pages 32, invoices 16, invoice-line pages 4); aggregate provider GETs still obey the plan-wide cap. No default path or filesystem temp fallback. Each staged source is preflighted against the signed expected raw manifest digest and backup inspection limits before any provider call; approval and candidate projection do not prove source provenance beyond exact digest or provider truth. The external volume must enforce actual capacity; a configured byte ceiling alone is not a hard physical quota. Never truncate or partially accept work. Sort all IDs/references before hashing and writes. Per-provider/CLI calls have context deadlines and cancellation. No partial success is returned.

Fake-only tests must prove: raw-byte identity including whitespace changes; digest mismatch causes zero provider calls; unapproved and reordered/replayed account scope makes zero calls for out-of-scope IDs and changes/rejects the bound plan; nonce cannot replay across operation/plan/account while exact tuple retry resumes; every source/inventory/provenance field changes `RecoveryEnvelopeHash`; artifact input changes `LegacyPlanArtifactHash`; mapped contract fields change `RecoveryContractPlanHash`; epoch approval/record/effect evidence is excluded from outer hash and changes only its own canonical record/receipt digest; strict decoder rejects duplicate/unknown/trailing JSON; lease restart verifies pinned content and never reopens source; reserve→Stage→Pin→immutable-plan crash points resume or explicitly abandon the exact planRef without pin/hash cycles; frozen payload has no inner plan; outer hash excludes itself/global approval/effects; epoch record digest excludes itself; legacy `LegacyPlanArtifactHash` format remains byte-for-byte stable; additive `RecoveryContractPlanHash` deterministically covers mapped contract fields; pin ID, planRef, and digest mismatches refuse resolve/reopen; interrupted release waits for borrower drain and repairs RELEASING/RELEASED; tampered/replaced scratch is rejected; account/customer conflict or missing authoritative identity blocks before provider call; deleted/deleting snapshot account causes zero provider calls; snapshot/current checkout mismatch, added/disappeared/refread change, nullable/pending/unknown/unqueryable checkout, provider timeout, stale results never yield eligibility; planning does not call Seal and succeeds with a valid unsealed active `G` prefix through cut `C` while recording last seal `H` separately; active writer change between Plan and Begin blocks; stop/revocation failure prevents seal; seal/full-history mismatch prevents restore; missing/invalid global approval and full-inventory mismatch cause zero effects; zero eligible accounts preserve unchanged `RecoveryPlan.Validate`, return a valid approval-required `FROZEN_ONLY` envelope, and enter only `Begin` with persisted `FROZEN_ONLY` dispatch; empty allowlist makes zero provider calls and still cannot bypass global approval; no frozen-only envelope can create an activation request; missing/bad account approval and post-fence provider ineligibility become durable `WITHHELD_FROZEN`; permanent provider outage cannot prevent frozen publication/admission for unrelated accounts; every snapshot account has terminal ledger state; frozen whole restore publishes atomically; deleted/deleting intent always wins; activation for A holds the same in-process guard as forget/delete, rejects any row/checkout/deletion drift, and calls only the read-only observer while guarded before service-owned CAS; the observer's private lock alone cannot satisfy the guard; the local RPC cannot reach SQL directly; activation for A cannot alter B and never reruns restore/seal/adoption; concurrent Begin and activation serialize; crash before/after each persisted transition is idempotent; no `G+1` writer starts before full destination publication and committed epoch; configured `G+1`, empty prefix, fake provider success, signed approval/hash, copied lock/PID, or booleans alone never pass admission; IMDS/default credential access remains denied. These tests establish local state-machine behavior only, not live provider, IAM, stop/revocation, durable storage, or activation qualification.

## Current blockers and handoff boundary

Locally implementable after review: the recovery owner can add the coordinator/state records, exact-scope validation, pinned-lease consumer, zero-eligible frozen-only plan path, and terminal ledger. Backup proposal `6a013b49b94e7f9286b782ad2827441d2e5bd737` specifies the shared planRef/reservationVersion/PIN_PENDING/Pin/ResumePin/FindPinned/exact-attempt CancelPin/Reconcile lifecycle and exact typed `SnapshotPinLifecycleAuthority`; independent review is in progress. This remains a bounded proposal, not a cleared or implemented integration. The recovery proposal consumes those APIs and requires the durable owner CAS to protect pending attempts; no runtime API implementation is claimed. Billing owner must review both historical-candidate planning observation and current-restored-row activation projection; until authoritative account/customer identity exists, affected accounts remain frozen. Service owner must implement and inject the shared in-process `AccountMutationGuard`, use it across forget/delete/status changes and activation, own the final SQL CAS, and expose activation only through authenticated local RPC to the active service; none of these additions exists today. Journal owner must provide verified current-writer/ancestry plus durable seal/adoption capability. Service owner must also wire supervisor quiescence, shared journal identity, atomic full-destination publication, and committed-epoch admission. A separate control-plane owner must define actual credential/session inventory and scoped AWS authority. These are owner dependencies, not completed integrations; do not report production readiness until reviewed and landed.

Live-only gates remain: exact operator-approved source and account scope; fresh real provider observations; authoritative old-writer identity and stop; complete and effective revocation of every credential/session; post-fence durable journal seal; whole private restore and atomic publication; durable successor adoption; production shared journal wiring and scoped write credentials; physical storage limits/durability; and operator-controlled activation. No provider action, deployment, purge, spend, or activation is part of this proposal.
