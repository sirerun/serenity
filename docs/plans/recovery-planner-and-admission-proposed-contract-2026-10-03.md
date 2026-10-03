# Recovery planner and production admission: proposed integration contract

**Status:** proposed resolution of the corrected review's remaining hold; source baseline `8081632` after PR #349. This is not an approved interface change or provider/deployment/activation authority. Original commits `05953ed` and `29d3c13` remain in history. Re-review is required; frozen interfaces remain unchanged until owner review and additive amendments.

## Contract rulings

**Snapshot identity.** Set `contracts.RecoveryPlan.SourceSnapshot` to lowercase SHA-256 of the exact raw `manifest.json` byte sequence that `backup.InspectSnapshot` verified. Its 64 lowercase hex form satisfies the existing object-ID shape. Never hash re-encoded JSON. `ManifestV2.Source.BuildSHA` and `SchemaVersion` stay distinct metadata. Persist these with the plan and include them in the canonical payload hash in a reviewed plan-store format revision. `ExpectedSnapshotSHA256` is supplied with a reference to a verified operator case; the verifier returns the approved sorted account allowlist, nonce, and operation ID. Compare exact raw digest and scope before provider calls. Approval binds exact bytes and scope; it does not prove provider truth or old-writer fencing. A plan hash is an integrity key, not authority.

**Zero eligible accounts.** Preserve `RecoveryPlan.Validate`'s nonempty account invariant and never manufacture an eligible account to satisfy it. When source, inventory, journal ancestry, and global plan approval are valid but the approved provider candidate set is empty (including all candidates ineligible or all accounts outside activation scope), persist a distinct executable `FrozenRecoveryPlan`. It binds the same exact raw snapshot, complete inventory, journal cut/ancestry, old writer, plan approval, and planRef/pin as an ordinary plan, has zero activation-eligible IDs, and carries only opaque per-account frozen/deleted disposition reasons. It can enter only the existing `Begin` path, which dispatches by the persisted `PlanKind` and requires a separate exact global epoch approval and can never produce `RecoveryApplyRequest` or account activation authority. Missing/ambiguous source, identity, ancestry, approval, or inventory evidence remains an error. Never infer Free or eligibility from missing customer linkage or failed lookup.

**Plan-time journal facts versus future fence.** Plan is read-only. It records the observed active old writer generation `G`, authenticated writer identity and journal-store identity, snapshot cut, and ancestry/completeness through the snapshot cut using already durable journal history. It does not require a future seal, call `Seal`, or claim the active generation is sealed. The post-stop/post-revocation seal is obtained only in the later plan-wide recovery epoch. Re-observe active writer identity/generation before fencing; if changed from `G`, abort and require a newly approved plan.

**Plan-wide recovery versus account decisions.** One serialized recovery epoch owns stop, revocation, post-watermark seal, whole-snapshot restore/reconciliation, publication, and successor adoption. It covers the complete restored account inventory. Each snapshot account receives a durable terminal disposition: `WITHHELD_FROZEN` (with reason), or a journal-preserved `DELETED`/`DELETING` disposition; a successfully activated account has `ACTIVE_ELIGIBLE`. Missing/bad approval, provider ineligibility, ambiguity, stale evidence, or a current deletion intent means `WITHHELD_FROZEN` or the deletion disposition, never eligibility success. No account-local condition can block service for unrelated accounts after the global frozen restore. One-account activation cannot advance another account or repeat global restore/seal/adoption.

**Global recovery permission versus account activation.** A signed global epoch approval authorizes only quiesce, old-writer stop, complete credential/session revocation, sealing, validating/restoring the exact whole snapshot inventory, publishing it with all surviving accounts frozen, and adopting the single successor. Before the first disruptive side effect, `Begin` verifies that approval against the complete immutable plan hash, full sorted snapshot inventory digest/list, exact activation scope (possibly empty), source digest, old writer identity/generation/store, operation ID, action set, expiry, and nonce. Per-account approvals are not prerequisites for global restore. No global approval can unfreeze any account. Each account remains `restore_pending`/Free until a separate exact-one-account approval and a fresh guarded provider/deletion check succeeds.

**Forward-only recovery.** Before any external effect, an authorized abort may release reservations and leave the old writer unchanged. Persist `EFFECTS_MAY_HAVE_STARTED` before the first quiesce/stop request; from that point, including timeout/uncertain response, rollback to old writer or credential restoration is forbidden. Exact-operation resume continues the same source, full inventory, plan, writer, and epoch. If approvals or global authorization need replacement after effects, a newly signed continuation authorization binds the same epoch and the exact next phase; it cannot change scope or authorize rollback. A global approval failure before effects is a safe abort. Missing/bad account approval or later eligibility loss is recorded `WITHHELD_FROZEN`; finish the frozen restore and recover service when every inventory account is terminal. Later activation is a separate guarded one-account operation against the already-published exact data, with new approval and current checks; it never restores, seals, adopts, or republishes.

## Existing source and ownership

Reusable implementation already present: `backup.InspectSnapshot` validates exact manifest/artifact bytes and reports raw digest/build identity; `internal/hosted/recovery` has strict canonical plan persistence; the concrete billing observer is read-only but only accepts current `restore_pending`; `service.AssembleWithDependencies` requires a shared journal and admission, then validates snapshot-cut/full-history reads and pending deletion intents; and `deletion.Journal` implements durable append, `ReadThrough`, and `Seal`. Do not duplicate these implementations or claim the inspector/observer is missing.

Important actual limits: `InspectSnapshot` owns and cleans its scratch copy before returning, so its returned inventory is not a durable Apply input. The existing billing observer cannot assess arbitrary candidate identities extracted from a snapshot. Service admission has no production factory. The current contracts have no capability for authenticated current-writer identity, active-generation inspection, atomic plan-wide recovery lease, durable successor adoption, or a separate post-fence evidence record. They also have no single-account activation guard spanning deletion/status mutation, provider refreads, final reread and row CAS; the account owner must supply and review that serialization boundary. `ReadThrough(...).Sealed` alone cannot establish adoption.

Ownership requests: T23.50 owns `internal/hosted/recovery/**` and `internal/cli/hosted_recovery.go`; backup owners add a pinned verified-snapshot lease API; billing owners add a narrow snapshot-candidate read-only observation API; deletion/journal owners add authenticated current-writer/ancestry and successor-adoption capabilities; service owners add epoch-aware admission wiring while preserving same-journal identity; a control-plane owner owns AWS stop/revocation/evidence adapters and their scoped role. These are dependency-linked owner patches. Planner code does not edit their files or permissions.

## Proposed private interfaces and records

Interfaces below are requested additions, not existing callable APIs. The shared contract additions proposed are `contracts.EvidenceRef{Authority, RecordID, Version}`, and `ExpectedSnapshotSHA256` plus `ApprovalRef` on `RecoveryPlanRequest`. The typed activation command belongs to the service-owned local RPC and does not reuse the plan-based `RecoveryApplyRequest`. All other capabilities and terminal ledgers stay in owner-local packages. `Verified*` values are constructed only inside `internal/hosted/recovery` after its verifier fetches and validates referenced records. Authority adapters return opaque evidence references/claims; they never return trusted serialized `Verified*` values. Durable records store references and digests, and the recovery verifier refetches the exact immutable object version/proof chain before consuming it. The CLI cannot deserialize a `Verified*` value into authority.

The backup-owner amendment supplies the artifact handoff that current `InspectSnapshot` does not return. Consume backup-owned lease/pin types directly; do not introduce a recovery-local lease/store or candidate type. Exact proposed APIs are `NewSnapshotLeaseStore(ctx, options, lifecycleAuthority)`, `SnapshotLeaseStore.Stage(ctx, sourcePath, InspectionOptions)`, `VerifiedSnapshotLease.Candidate(ctx, accountID)`, `VerifiedSnapshotLease.Pin(ctx, planRef, expectedManifestSHA256) -> SnapshotPinRef`, `SnapshotLeaseStore.ResolvePinned(ctx, pinID, expectedDigest, planRef) -> PinnedSnapshotRef`, `SnapshotLeaseStore.ReopenPinned(ctx, PinnedSnapshotRef)`, `SnapshotLeaseStore.Reconcile(ctx)`, `backup.RestoreVerified(ctx, lease, destination)`, and context-aware `Close`. The process-local pinned capability is not serialized. `VerifiedAccountCandidate` carries historical `CustomerBindingCandidate` and nullable `CheckoutAttempts`; `SnapshotCheckoutAttempt` preserves present-but-NULL and present-but-empty session refs instead of collapsing them into no row. No returned candidate field means verified account/customer authority. The recovery plan store implements `backup.SnapshotPinLifecycleAuthority` and is supplied to `NewSnapshotLeaseStore`; it alone authorizes pin and release transitions.

```go
// Use backup.SnapshotLeaseStore, *backup.VerifiedSnapshotLease,
// backup.VerifiedAccountCandidate, backup.SnapshotPinRef and
// backup.PinnedSnapshotRef. PinRef is durable; PinnedSnapshotRef is process-local.
```

Backup `Stage` extracts/reuses the same one-pass verifier used by `InspectSnapshot`, writes verified bytes to owner-only bounded durable staging, and returns the private lease handle. Restore consumes only the pinned backup handle through the shared restore implementation. Neither consumer reopens the untrusted source after staging. The backup owner owns lease cleanup/reopen identity checks and preserves ordinary `Restore` validation through the shared implementation.

```go
// Proposed additive fields in internal/hosted/contracts/backup.go:
type RecoveryPlanRequest struct {
    SnapshotPath string
    ExpectedSnapshotSHA256 string
    ApprovalRef EvidenceRef // immutable operator-case reference
}
// RecoveryPlan.Validate remains unchanged. This owner-local executable union
// gives the CLI a distinct non-activation path when the approved set is empty.
type RecoveryPlanKind string
const (
    EligibleRecoveryPlanKind RecoveryPlanKind = "ELIGIBLE"
    FrozenRecoveryPlanKind RecoveryPlanKind = "FROZEN_ONLY"
)
type FrozenAccountDisposition struct { AccountID, ReasonCode string }
type FrozenRecoveryPlan struct {
    Version int
    PlanRef, SnapshotPinID, SnapshotSHA256, SnapshotInventoryDigest string
    SnapshotBuildSHA256, SnapshotSchemaVersion string
    SnapshotAccountIDs []string // complete restored inventory, sorted
    OperationID, PlanApprovalDigest, PlanApprovalNonce string
    ActivationAllowlist []string // exact approved scope, may be empty
    EligibleAccountIDs []string // invariant: empty
    Dispositions []FrozenAccountDisposition // complete inventory; no eligibility claim
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    LastSealed contracts.DeletionWatermark
    SnapshotCut contracts.DeletionWatermark
    PrefixRef EvidenceRef
    PlanHash string
}
func (p FrozenRecoveryPlan) Validate() error // exact sorted inventory/disposition coverage; zero eligible IDs; valid source/cut/writer/approval/pin bindings
type PlannedRecovery struct {
    Kind RecoveryPlanKind
    PlanHash string
    Eligible *contracts.RecoveryPlan // non-nil only for ELIGIBLE
    Frozen *FrozenRecoveryPlan // non-nil only for FROZEN_ONLY
}
type RecoveryPlanBuilder interface {
    PlanRecovery(ctx context.Context, req contracts.RecoveryPlanRequest) (PlannedRecovery, error)
}
func (p *Planner) PlanRecovery(ctx context.Context, req contracts.RecoveryPlanRequest) (PlannedRecovery, error)
// Proposed in contracts/backup.go; recovery.EvidenceRef aliases this type.
type EvidenceRef struct { Authority string; RecordID string; Version string }
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
    PlanHash, SnapshotSHA256, SnapshotInventoryDigest string
    SnapshotAccountIDs []string // exact full restored inventory, sorted
    PlannedAccountIDs []string // exact RecoveryPlan.Accounts subset
    OldWriterDigest, JournalStoreID string
    OldWriterGeneration int64
    OperationID, Nonce string
    Actions []string // fixed set: quiesce, stop, revoke, seal, restore-frozen, publish-frozen, adopt
    ExpiresAt time.Time
}
type AccountActivationBinding struct {
    EpochDigest, PlanHash, AccountID, SnapshotSHA256 string
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
type VerifiedAccountApproval struct { epochHash, planHash, accountID, operationID, nonce, digest string; expiresAt time.Time }
type AccountTerminalState string
const (
    WithheldFrozen AccountTerminalState = "WITHHELD_FROZEN"
    ActiveEligible AccountTerminalState = "ACTIVE_ELIGIBLE"
    Deleted AccountTerminalState = "DELETED"
    Deleting AccountTerminalState = "DELETING"
)
type AccountTerminalRecord struct {
    AccountID string // opaque ID; encrypted/private record store only
    State AccountTerminalState
    ReasonCode string
    ApprovalDigest, ObservationDigest, BindingDigest, CheckoutSetDigest string
    ProviderSourceVersion string
    ObservedAt, ExpiresAt time.Time
    RestoreMarker, EpochDigest, RecordDigest string
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

type RecoveryEpochRecord struct {
    Version int; Sequence uint64; PriorDigest string
    PlanKind, PlanHash, PlanRef, OperationID, PlanApprovalDigest, PlanApprovalNonce string
    EpochApprovalDigest, EpochApprovalNonce, SnapshotInventoryDigest string
    SnapshotAccountIDs, PlanEligibleAccountIDs []string
    OldWriterDigest, JournalStoreID string
    SnapshotCut contracts.DeletionWatermark; PrefixDigest, SnapshotPinID string
    Phase string; StopRef, RevocationRef, SealRef, RestoreRef, PublishRef, AdoptionRef, AdmissionRef string
    SuccessorDigest string; UpdatedAt time.Time
}
type RecoveryPlanStore interface {
    ReservePlanRef(ctx context.Context, operationID string) (planRef string, err error) // unique 64 lowercase hex, durable RESERVED
    PersistReadyPlan(ctx context.Context, planRef string, canonicalPlan []byte, planHash string) error // immutable READY CAS
    MarkAbandonedBeforeEffects(ctx context.Context, planRef, reasonCode string) error // only after proving no effect could start
    LoadPlanByRef(ctx context.Context, planRef string) (canonicalPlan []byte, planHash string, state string, err error)
    MarkRecoveredAdmissionConfirmed(ctx context.Context, planRef string, admission EvidenceRef) error
    // Exact methods required by backup.SnapshotPinLifecycleAuthority:
    ValidateReservedPlanRef(ctx context.Context, planRef string) (backup.PlanRefReservation, error)
    AuthorizePinRelease(ctx context.Context, pin backup.SnapshotPinRef) (backup.PinReleaseAuthorization, error)
    CompletePinRelease(ctx context.Context, authorization backup.PinReleaseAuthorization) error
}
type AccountRecoveryRecord struct {
    PlanHash, OperationID, AccountID string
    Outcome, ReasonCode, ActivationApprovalDigest, ActivationApprovalNonce string
    ObservationDigest string; ObservedAt, ExpiresAt time.Time
    CustomerBindingDigest, RecoveryMarker, EpochRecordDigest string
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
    SealStoppedGeneration(ctx context.Context, generation int64, after contracts.DeletionWatermark, operationID string) (EvidenceRef, error)
    AdoptSuccessor(ctx context.Context, predecessor WriterIdentity, generation int64, sealDigest, operationID string) (EvidenceRef, error)
    ReserveRecoveryEpoch(ctx context.Context, planHash, operationID string, oldWriter WriterIdentity) (EvidenceRef, error)
    CommitRecoveryEpoch(ctx context.Context, reservationRef, adoptionRef EvidenceRef, operationID string) error
}

type RecoveryControlPlane interface {
    QuiesceService(ctx context.Context, writer WriterIdentity, operationID string) (EvidenceRef, error)
    StopWriter(ctx context.Context, writer WriterIdentity, operationID string) (EvidenceRef, error)
    RevokeEveryCredentialAndSession(ctx context.Context, writer WriterIdentity, operationID string) (EvidenceRef, error)
}
type RecoveryEvidenceVerifier interface {
    VerifyWriter(ctx context.Context, observation WriterObservation) (VerifiedWriterIdentity, error)
    VerifyJournalPrefix(ctx context.Context, ref EvidenceRef, storeID string, from, through contracts.DeletionWatermark) (VerifiedJournalPrefix, error)
    VerifyEpochReservation(ctx context.Context, ref EvidenceRef, planHash, operationID string, writer WriterIdentity) (VerifiedEpochReservation, error)
    VerifyQuiesced(ctx context.Context, ref EvidenceRef, writer WriterIdentity, operationID string) (VerifiedQuiescence, error)
    VerifyStopped(ctx context.Context, ref EvidenceRef, writer WriterIdentity) (VerifiedStop, error)
    VerifyRevocation(ctx context.Context, ref EvidenceRef, writer WriterIdentity) (VerifiedRevocation, error)
    VerifySeal(ctx context.Context, ref EvidenceRef, generation int64, after contracts.DeletionWatermark) (VerifiedSeal, error)
    VerifyAdoption(ctx context.Context, ref EvidenceRef, predecessor WriterIdentity, successor WriterIdentity, seal VerifiedSeal) (VerifiedAdoption, error)
}
type RecoveryRestoreApplier interface {
    StageAndReconcile(ctx context.Context, snapshot *backup.VerifiedSnapshotLease, seal VerifiedSeal, limits Limits) (StagedRestore, error)
    Publish(ctx context.Context, staged StagedRestore, expectedDestinationID string) (PublishedRestore, error)
}

type AccountActivationResult struct { State, ReasonCode, RecordDigest string }
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
type RecoveryActivationCommand struct { EpochHash, AccountID string; ApprovalRef EvidenceRef }
type RecoveryActivationHandler interface {
    HandleRecoveryActivation(ctx context.Context, command RecoveryActivationCommand) (AccountActivationResult, error)
}

type WriterIdentity struct { Provider, ScopeRef, WriterRef, BootRef, JournalStoreID string; Generation int64 }
// Exported result types have private fields and are created only by their authority adapters.
type VerifiedWriterIdentity struct { identity WriterIdentity; providerEvidence EvidenceRef; verifierID, digest string; verifiedAt time.Time }
type VerifiedJournalPrefix struct { from, through contracts.DeletionWatermark; storeID, digest string; verifiedAt time.Time }
type VerifiedEpochReservation struct { planHash, operationID, oldWriterDigest, lockDigest string }
type VerifiedSeal struct { watermark contracts.DeletionWatermark; generation int64; evidence EvidenceRef; digest string; verifiedAt time.Time }
type VerifiedAdoption struct { predecessorDigest, successorDigest, sealDigest, evidenceDigest string; generation int64; verifiedAt time.Time }
type VerifiedQuiescence struct { evidenceDigest string; operationID string; verifiedAt time.Time }
type VerifiedStop struct { evidenceDigest, writerDigest string; verifiedAt time.Time }
type VerifiedRevocation struct { evidenceDigest, writerDigest, inventoryDigest string; itemCount int; verifiedAt time.Time }
type StagedRestore interface { StagingID() string }
type PublishedRestore interface { DestinationID() string; ManifestSHA256() string }
type Limits struct { MaxManifestBytes, MaxArtifactBytes, MaxTotalBytes int64; MaxPlanAccounts, MaxSnapshotAccounts, MaxBrains, MaxCheckoutRefs, MaxEvidenceBytes, MaxCanonicalRecordBytes, MaxProviderGETs, MaxCLIOutputBytes, MaxRetainedSnapshotLeases int; MaxConcurrentProviderCalls int; ProviderTimeout, PhaseTimeout, MaxObservationAge time.Duration }
type BeginEpochRequest struct { PlanHash string; EpochApprovalRef EvidenceRef }
type ContinueEpochRequest struct { PlanHash string; ContinuationApprovalRef EvidenceRef }
type CommitEpochRequest struct { PlanHash string }
type EpochStatus struct { PlanHash, OperationID, Phase, RecordDigest string }

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

The cross-owner transaction is exact: recovery durably reserves one unique canonical lowercase 64-hex `planRef`; backup `Stage(ctx, sourcePath, InspectionOptions{ExpectedManifestSHA256: ...})` verifies/copies the source; a durable owner-store PREPIN compare-and-swap must exclude abandonment while backup `Pin(ctx, planRef, digest)` is between reservation validation and pin fsync; after fsync, the owner store transitions the exact tuple to PINNED. Backup lease Pin returns `SnapshotPinRef`; recovery atomically publishes the immutable READY plan containing `planRef`, pin ID, digest, complete verified inventory, and canonical payload/hash. The plan hash includes pin ID and digest; the pin does not contain that hash, avoiding a self-reference cycle. Both ordinary and `FrozenRecoveryPlan` use this sequence; approval binds the completed full plan hash. The recovery plan store implements the backup lifecycle authority and is injected into the store. The final backup correction must include: (a) a unique-pin lookup by `(planRef,digest)` to repair crashes after pin fsync and before READY plan persistence, since `ResolvePinned` requires the not-yet-persisted pinID; (b) a durable PREPIN guard/CAS so `MarkAbandonedBeforeEffects` cannot race with `Pin`; and (c) a typed `KEEP` outcome for active/normal pins during `Reconcile`, which is a successful no-op and not an authority error. These methods/state claims are not yet present in backup revision 312; exact names/types remain pending its owner correction and review. Before READY, a crash may resume only the same reservation/digest, find the unique pin, reverify, and finish; any second pin or different digest is a conflict. It may not silently restage after a durable pin. After READY persistence, retry loads the immutable plan and calls `ResolvePinned(ctx,pinID,digest,planRef)` then `ReopenPinned`; it never rebuilds in place. No executable plan is returned until both pin and plan are durable.

Retention states are `RESERVED`, `STAGING`, `PINNING`, `PINNED`, `PLANNING`, `READY`, `APPLYING`, `FORWARD_RESUMABLE`, `ABANDONED_BEFORE_EFFECTS`, `COMMITTED`, and backup-local `RELEASING`/`RELEASED`. Keep the pin in every reserved/planning/ready/applying/forward-resumable state. Backup `Reconcile` receives a typed `KEEP` (successful no-op) for any normal active pin; it must not surface that as an authority failure. Release authority succeeds only when the recovery store proves exact `ABANDONED_BEFORE_EFFECTS` or `COMMITTED` global restore plus a durable recovered-admission record, and no source consumer remains under its exclusive plan/epoch guard. The owner CAS keeps a `PINNING` plan non-abandonable until the pin is durable or the exact interrupted pin attempt is owner-reconciled. The backup store `Reconcile(ctx)` holds the exclusive per-lease lock (borrowers use shared locks), obtains `AuthorizePinRelease` for that exact `(pinID, planRef, digest)`, writes `RELEASING`, verifies/removes only its own exact entries and fsyncs, writes `RELEASED`, then calls `CompletePinRelease`; retries repair only that tuple. A closed process handle never releases a persistent pin. Reconcile may age-clean only verified staged-but-unpinned crash orphans after proving there are no borrowers and no pin reference; it never time-deletes pins. Recovery and backup proposals must freeze these exact shared lifecycle semantics together.

The billing owner adds a narrow `ObserveSnapshotIdentity(ctx, backup.VerifiedAccountCandidate)` read-only projection for planning, alongside a distinct `ObserveRestoredRecoveryAccount` projection for one-account activation. Both preserve the existing `ObserveRestorePending` observer unchanged. The planning API treats every backup candidate as a historical hint only and requires an authoritative customer/account relation; never use it to make an activation decision after the global admission pin is released. The activation API receives the service owner's current row view and exact restore marker from the shared account guard, then validates current account/status/customer binding and checkout rows, performs bounded provider reads/refreads, compares exact complete sets and source versions, and rereads current account/status/binding/checkout state before returning. It returns only `Eligible`, reason code, opaque binding/checkout digests, provider source version, observation/expiry times, and canonical digest; it performs no writes. Any missing, added, removed, conflicting, pending, unknown, unqueryable, or changed ref fails closed. Do not accept caller paths, historical snapshot customer values, or caller-supplied provider IDs in the activation API. The final service-side CAS still compares against the original guarded row view. Existing `ObserveRestorePending` behavior remains unchanged.

For `ObserveSnapshotIdentity`, do not route planning calls through `ObserveRestorePending`, which correctly accepts only current `restore_pending` rows. Snapshot identity fields are lookup candidates, not billing authority. The observer validates the backup-sealed candidate/digest and independently resolves the account/customer relationship before provider calls. For `ObserveRestoredRecoveryAccount`, derive identity only from the actual restored current row under the service guard; require row and restore marker match, check current account/customer binding and all current checkout rows, then perform bounded provider calls/refreads and a final reread. If the current row is absent or the mapping is ambiguous, fail closed. No new provider metadata key is assumed; the billing owner must name the existing verified mapping or keep either API fail-closed.

For an identity-verified candidate, make bounded read-only provider calls for current subscriptions/invoices and each exact snapshot checkout/session reference using the provider's supported read API. Any snapshot or current checkout ref that is missing, unknown, pending, nonterminal, duplicated, or unqueryable blocks; do not expire, delete, reconcile, or clear it. Compare current checkout row refs/state with the snapshot-derived set; conflict blocks. After provider I/O, reread account status/customer binding and checkout rows; require status/binding/ref digests unchanged from the pre-read, and refuse if any row changed, was added, disappeared, or became nonterminal. Verify current provider price/window/grace and source object versions, with `CustomerBindingDigest`, `ProviderSourceVersion`, `ObservedAt`, `ExpiresAt`, and canonical `ObservationDigest` in the return. No SQL, entitlement, checkout, or audit writes occur. If the provider lacks a read endpoint for an exact checkout reference, that candidate is blocked rather than treated as resolved.

`CurrentWriter` returns separate facts: active old writer identity/generation `G`, stable journal-store ID, last previously sealed historical watermark `H` (possibly older than `G`), observation time, and immutable evidence reference. The recovery-owned verifier validates the claim and constructs private `VerifiedWriterIdentity`. `ReadAncestry` returns an immutable reference; recovery re-fetches that exact version and `VerifyJournalPrefix` proves the durable chain from zero through snapshot cut `C`; it may include the unsealed active-generation prefix through `C`. It does not require or synthesize the future seal. Plan records `H`, `C`, and `G` separately and rejects a cut not present in verified ancestry. `ReserveRecoveryEpoch` is an authoritative durable CAS on the same generation control plane: after success, no second recovery epoch or regular writer start/generation advance is possible for this journal until the exact operation commits or an explicit reviewed abort completes. It is operation-owned and does not auto-expire into another writer's authority; its immutable reference is re-fetched and verified before use. After verified stop and complete revocation, `SealStoppedGeneration(G, C)` creates the post-fence seal; independently verify `ReadThrough(zero)` complete through this exact seal `S` before restore. Persist `H`, `C`, `G`, `S`, and successor `G+1` as distinct values. Only after restore reconciliation may `AdoptSuccessor` create the immutable `G+1` adoption record bound to predecessor identity, seal digest, successor identity, and generation. Generation write authority remains blocked while the reservation is held. `CommitRecoveryEpoch` atomically commits terminal epoch/adoption and releases the reservation only to the exact successor. No value is inferred from a configured generation or empty prefix.

## Approval, plan, and exact scope

The signed plan operator case must bind raw snapshot SHA-256, sorted allowlisted account IDs (the list may be explicitly empty), one recovery `operation_id`, a cryptographically unique `approval_nonce`, and expiry. `VerifyPlan` returns a recovery-owned verified approval containing the canonical binding and verifier/key identity. `PlanRecovery` compares digest before provider calls, intersects snapshot IDs with that allowlist, and makes provider calls only for the sorted intersection. Snapshot accounts outside the approved set are neither queried nor placed in an activation scope; they remain in the exact full restore inventory and are terminally frozen. With one or more eligible accounts, persist the ordinary `RecoveryPlan` (which still must pass its unchanged `Validate`). With zero eligible accounts and complete valid evidence, persist the distinct `FrozenRecoveryPlan` above, with an empty `EligibleAccountIDs` and one disposition for every inventory account; this plan is executable through `Begin` only when the persisted type is `FROZEN_ONLY`. The legacy `Plan` method may return `ErrNoEligibleAccounts`, but the production CLI/service path must call `PlanRecovery` and retain the valid frozen-only envelope. Neither path records eligibility where none was established. Both plan payloads bind approval digest/nonce, operation ID, sorted allowlist, selected eligible IDs, raw manifest digest, separate build/schema provenance, exact inventory/digest, old writer identity/generation/store, last previously sealed watermark, snapshot cut, and verified existing journal-prefix reference. Any change to these changes the plan hash.

Planning sequence: verify the operator case's exact source digest/scope/freshness; durably reserve a unique random `planRef` bound to its operation; call backup `Stage(ctx, sourcePath, options)` with `options.ExpectedManifestSHA256` and verify the raw digest; immediately durably `Pin(ctx, planRef, digest)` and retain its opaque pin ID; then read complete inventory, active writer `W/G`, stable store identity, and last previously sealed watermark `H`; establish journal ancestry from zero through snapshot cut `C` (confirm `C` is present), without sealing; call the snapshot identity billing projection only for approved candidates, in a fixed-size worker pool; fail on ambiguity; assemble either a valid ordinary plan with `RecoveryPlan.Generation=G` and `RecoveryPlan.JournalWatermark=C`, or a frozen-only plan with zero eligible accounts and a terminal frozen/deletion disposition for each inventory ID; atomically persist canonical immutable READY plan bytes/hash including `planRef`, pin ID, and digest, then return. The allowlist is the provider-call ceiling, even if the source has additional accounts. If source, identity, ancestry, approval, or inventory evidence is incomplete, persist no executable plan; transition only a not-yet-PINNING reservation to `ABANDONED_BEFORE_EFFECTS`. A `PINNING` tuple must first be reconciled with backup; then use the authorized release path only after the owner store proves abandonment and no effect has started.

The verified plan approval scope bounds provider calls. The executable plan may contain only eligible IDs in the sorted intersection of the plan allowlist and complete snapshot inventory; provider calls are made only inside this exact scope. `Begin` additionally requires an independently signed *global epoch approval* binding plan hash, raw source digest, full sorted restored inventory and canonical inventory digest, exact plan allowlist/selected scope, old writer identity/generation/store, operation ID, nonce, expiry, and the fixed global action set. That approval authorizes physical whole-database restore and fencing only; it carries no account activation authority. `Begin` reopens the pinned snapshot and re-verifies the exact manifest digest and complete inventory against the plan and epoch approval before any reservation, quiescence, stop, revocation, or seal. Bad/missing global approval fails before effects. Missing account approvals do not block global restore.

Each account activation requires a separate signed approval binding the committed epoch digest, exact plan hash, exactly one account ID, source snapshot digest, operation ID, fresh nonce, and expiry. `Begin` atomically consumes the global approval nonce. Each `ActivateOne` consumes a one-account nonce. Exact tuple retry resumes; nonce reuse with any changed tuple is replay. Initial approval expiry is checked at first consumption; an already-consumed approval may be used for exact crash resume only if its key has not been revoked. A missing, bad, expired, or revoked one-account approval creates a durable `WITHHELD_FROZEN` result; provider ineligibility, ambiguity, staleness, or deletion does likewise (or preserves `DELETED`/`DELETING`). These are terminal global-restore dispositions, not successful eligibility. An account withheld at epoch commit can later be activated only with a new approval and current checks. Rejected attempts are minimized private audit records; never log signed payloads or secrets.

## Serialized epoch and per-account state machine

Add private canonical records in the recovery owner package, with append-only hash chaining, owner-only permissions, strict versioned JSON, monotonic sequence, CAS on prior digest, fsync-before-effect, and atomic directory updates. `RecoveryEpochRecord` binds canonical plan/ref, global approval ref/digest/nonce, exact source and full sorted inventory digest/list, operation ID, old writer identity/generation/store, H/C/prefix digest, pinned lease ID/digest, phase, effect-start marker, reservation, stop/revocation/seal/restore/publication/adoption references and verifier digests, successor, destination identity, terminal account inventory digest, sequence/time. `AccountRecoveryRecord` binds epoch/plan, exactly one account, optional activation approval digest/nonce, customer-binding and observation digests/times/expiry/source version, terminal state/reason, recovery marker, and last epoch-record digest. No customer IDs, token values, raw paths, credentials, or broad provider responses. The durable reservation has no auto-expiry that transfers write authority.

`Begin` revalidates exact current `W/G/store`, the plan approval, the global epoch approval, full inventory, and pinned lease; validates the entire intended destination/scope before writing any effect marker; then atomically reserves `(operation, plan, global nonce)` and persists the initial record. Only after durable `EFFECTS_MAY_HAVE_STARTED` may it quiesce or fence. The reservation blocks any competing epoch/writer start until commit or a pre-effect safe abort. If no effect call was issued, safe abort verifies old writer/credentials unchanged, unsquiesces, and releases reservation. Once an effect may have started, only exact forward resume or a newly authorized same-epoch continuation is allowed; never restart old G or restore old credentials. A timeout is treated as uncertain effect, not safe abort.

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
The concrete `FROZEN` target is the private restored destination with every surviving account `restore_pending`/Free and every subscription `restore_pending`, plus journal deletions replayed through `S`; old service remains quiesced. Every row remains frozen in the private destination and after publication. Before atomic publication, failed/canceled work leaves live destination unchanged. After publication but before epoch commit, service admission remains denied and no writer may start. This exact state is resumable; never auto-roll back by re-enabling the old writer. A crash around rename is resolved by comparing the persisted intended destination identity and exact manifest/restore digests. A crash around commit is resolved from the terminal epoch record; never assume timeout means rollback.

There is no recovery-package `Apply` endpoint for this operation. `internal/hosted/service.RecoveryActivationHandler` is callable only after `EPOCH_COMMITTED`, accepts one `(epoch_hash, account_id, account_approval_ref)`, and owns the account guard plus final row CAS. The CLI reaches it only through the authenticated local service RPC. It cannot stop writers, seal, restore, adopt, republish, or admit startup. No public `apply-all` endpoint.

The global epoch record separately persists and admission re-fetches immutable stop, all-item revocation, post-watermark seal, restore/publish and adoption references. `FenceReceipt.SufficientFor` remains only the existing shape/binding check; its booleans and timestamp are compatibility summary only. Account activation must continue to enforce the existing single-account consistency/binding predicate where its additive service contract exposes it, but neither that predicate nor a receipt substitutes for current guarded billing/deletion checks or the global evidence references.

The `internal/cli/hosted_recovery.go` entrypoint sequence is `plan --snapshot --expected-manifest-sha256 --approval-ref` (returns a discriminated `ELIGIBLE` or `FROZEN_ONLY` plan ref/hash), `begin --plan-hash --epoch-approval-ref` (loads the immutable plan kind, accepts `FROZEN_ONLY` only as frozen restore, and verifies full inventory before any disruptive effect), `continue --plan-hash --continuation-approval-ref` (same-epoch forward progress), `commit --plan-hash` (requires full restore/adoption and terminal disposition for every inventory account), `activate-one --epoch-hash --account-id --approval-ref` (only after commit, sent through authenticated local RPC to the active service), and `status --plan-hash` (redacted persisted phase). Every command returns nonzero on incomplete evidence. `begin/continue/commit` are resumable only under the persisted plan operation/global approval/continuation bindings; caller flags cannot override account scope, generation, provider identity, or operation ID. `Begin` dispatches only from the persisted plan kind and fails if a caller-supplied mode disagrees; no separate `BeginFrozen` endpoint is added.

## Startup paths this contract does not implement

`RecoveryAdmission` is a one-time recovered-start proof for a committed epoch at sealed S. It does not supply authenticated journal genesis and does not solve an ordinary process restart after successor G+1 has appended an unsealed tail. Do not fabricate an empty plan, generation zero, bootstrap seal, or reuse the previous recovery admission to accept that tail. Genesis needs a journal-owner authenticated durable genesis authorization bound to store and initial writer. Ordinary restart needs a separately reviewed startup handshake that verifies active writer/store/generation and the complete authorized chain/head including a valid unsealed G+1 tail. Current `service.AssembleWithDependencies` requires sealed-boundary evidence, so that restart path remains fail-closed until service/journal owners add and review the distinct path. Therefore this proposal is code-ready only for recovery planning, frozen whole-database recovery, committed recovered-start admission, and later single-account activation; it does not claim full hosted startup readiness.

## Production admission and AWS boundary

Request `service.RecoveryAdmission` construction as `recovery.NewAdmission(epochStore, activeWriterIdentitySource, journalIdentityVerifier, buildSHA)`; it uses the package-owned evidence verifier and cannot accept one from service config or CLI. Runtime identity must come from an owner-reviewed local supervisor/control-plane channel with peer authentication; no static environment generation, caller value, or app IMDS. `Admit(ctx, journal)` accepts only `EPOCH_COMMITTED`, re-verifies stop, complete revocation, seal, publication, adoption, approval/plan/source provenance, all account records, and active successor identity, then checks the supplied journal is the adopted stable store. It returns the snapshot cut only after verified history reads establish the same sealed boundary; existing service assembly then retains its current full-history and intent checks. The same configured journal object/store must be wired into service, backup, fence, and admission. Any absent/stale evidence, mismatched store/build, nonterminal inventory disposition, or active competing epoch refuses recovered-start admission. This factory covers only the first service admission following a committed recovery epoch and its sealed boundary; it is not a general startup factory.

Preserve the hosted app's current IMDS denial. No instance profile, broad host credentials, default AWS credential chain, or app authority to stop instances/revoke credentials/adopt generations. A separately provisioned control-plane executable uses explicit scoped role credentials and the AWS CLI with fixed argument arrays, explicit region/profile, bounded context, scrubbed environment, and captured request IDs. For EC2 stop it verifies exact account/region/instance identity with `aws ec2 describe-instances`, calls `aws ec2 stop-instances`, waits with `aws ec2 wait instance-stopped`, then re-describes and persists immutable evidence references. Revocation must enumerate actual deployment credential/session classes at their owning authorities, revoke or deny each, and independently verify denial. EC2 stop does not establish revocation. The actual identity model, AWS role, policy scope, and credential classes remain owner inputs; no invented permissions or metadata key are proposed. The verifier retrieves provider evidence and checks exact subject/scope/version/status; CLI output or fake output alone is not evidence.

## Bounds and deterministic fake-only RED controls

Use these proposed hard ceilings (integrator owners may lower them; raising one requires a reviewed policy update): at most 100 accounts per recovery plan; 10,000 snapshot accounts/brains at the inspector boundary; 64 checkout refs per account; 4 concurrent provider calls; 4,096 provider GETs per plan; each provider timeout at most 30 seconds, response at most 1 MiB, total response bytes at most 64 MiB; evidence record at most 1 MiB; canonical plan/epoch record at most 4 MiB; each operation invocation at most 30 minutes before returning resumable state; provider observation age at most 5 minutes at one-account activation; snapshot declared bytes capped by explicit SSD-backed configuration no greater than the inspector's 1 TiB absolute limit. Existing billing observer parser ceilings remain in force (JSON depth 32, nodes 100,000, event pages 32, invoices 16, invoice-line pages 4); aggregate provider GETs still obey the plan-wide cap. No default path or filesystem temp fallback. The external volume must enforce actual capacity; a configured byte ceiling alone is not a hard physical quota. Never truncate or partially accept work. Sort all IDs/references before hashing and writes. Per-provider/CLI calls have context deadlines and cancellation. No partial success is returned.

Fake-only tests must prove: raw-byte identity including whitespace changes; digest mismatch causes zero provider calls; unapproved and reordered/replayed account scope makes zero calls for out-of-scope IDs and changes/rejects the bound plan; nonce cannot replay across operation/plan/account while exact tuple retry resumes; every hashed provenance/evidence field affects canonical plan hash; strict decoder rejects duplicate/unknown/trailing JSON; lease restart verifies pinned content and never reopens source; reserve→Stage→Pin→immutable-plan crash points resume or explicitly abandon the exact planRef without pin/hash cycles; pin ID, planRef, and digest mismatches refuse resolve/reopen; interrupted release waits for borrower drain and repairs RELEASING/RELEASED; tampered/replaced scratch is rejected; account/customer conflict or missing authoritative identity blocks before provider call; deleted/deleting snapshot account causes zero provider calls; snapshot/current checkout mismatch, added/disappeared/refread change, nullable/pending/unknown/unqueryable checkout, provider timeout, stale results never yield eligibility; planning does not call Seal and succeeds with a valid unsealed active `G` prefix through cut `C` while recording last seal `H` separately; active writer change between Plan and Begin blocks; stop/revocation failure prevents seal; seal/full-history mismatch prevents restore; missing/invalid global approval and full-inventory mismatch cause zero effects; zero eligible accounts preserve unchanged `RecoveryPlan.Validate`, return a valid approval-required `FROZEN_ONLY` envelope, and enter only `Begin` with persisted `FROZEN_ONLY` dispatch; empty allowlist makes zero provider calls and still cannot bypass global approval; no frozen-only envelope can create an activation request; missing/bad account approval and post-fence provider ineligibility become durable `WITHHELD_FROZEN`; permanent provider outage cannot prevent frozen publication/admission for unrelated accounts; every snapshot account has terminal ledger state; frozen whole restore publishes atomically; deleted/deleting intent always wins; activation for A holds the same in-process guard as forget/delete, rejects any row/checkout/deletion drift, and calls only the read-only observer while guarded before service-owned CAS; the observer's private lock alone cannot satisfy the guard; the local RPC cannot reach SQL directly; activation for A cannot alter B and never reruns restore/seal/adoption; concurrent Begin and activation serialize; crash before/after each persisted transition is idempotent; no `G+1` writer starts before full destination publication and committed epoch; configured `G+1`, empty prefix, fake provider success, signed approval/hash, copied lock/PID, or booleans alone never pass admission; IMDS/default credential access remains denied. These tests establish local state-machine behavior only, not live provider, IAM, stop/revocation, durable storage, or activation qualification.

## Current blockers and handoff boundary

Locally implementable after review: the recovery owner can add the coordinator/state records, exact-scope validation, pinned-lease consumer, zero-eligible frozen-only plan path, and terminal ledger. Backup proposal `31209b4c` now specifies the shared planRef/Pin/resolve/reconcile lifecycle and typed `SnapshotPinLifecycleAuthority`; independent review remains pending. The pin-before-READY crash path still needs a bounded unique `FindPinnedByPlanRef(planRef,digest)` (or equivalent) lookup because the recovery plan does not yet contain the pin ID. Do not freeze the coordinated lane until that is resolved. Billing owner must review both historical-candidate planning observation and current-restored-row activation projection; until authoritative account/customer identity exists, affected accounts remain frozen. Service owner must implement and inject the shared in-process `AccountMutationGuard`, use it across forget/delete/status changes and activation, own the final SQL CAS, and expose activation only through authenticated local RPC to the active service; none of these additions exists today. Journal owner must provide verified current-writer/ancestry plus durable seal/adoption capability. Service owner must also wire supervisor quiescence, shared journal identity, atomic full-destination publication, and committed-epoch admission. A separate control-plane owner must define actual credential/session inventory and scoped AWS authority. These are owner dependencies, not completed integrations; do not report production readiness until reviewed and landed.

Live-only gates remain: exact operator-approved source and account scope; fresh real provider observations; authoritative old-writer identity and stop; complete and effective revocation of every credential/session; post-fence durable journal seal; whole private restore and atomic publication; durable successor adoption; production shared journal wiring and scoped write credentials; physical storage limits/durability; and operator-controlled activation. No provider action, deployment, purge, spend, or activation is part of this proposal.
