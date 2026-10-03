# Recovery planner and production admission: proposed integration contract

**Status:** proposed resolution of the corrected review's remaining hold; source baseline `8081632` after PR #349. This is not an approved interface change or provider/deployment/activation authority. Original commits `05953ed` and `29d3c13` remain in history. Re-review is required; frozen interfaces remain unchanged until owner review and additive amendments.

## Contract rulings

**Snapshot identity.** Set `contracts.RecoveryPlan.SourceSnapshot` to lowercase SHA-256 of the exact raw `manifest.json` byte sequence that `backup.InspectSnapshot` verified. Its 64 lowercase hex form satisfies the existing object-ID shape. Never hash re-encoded JSON. `ManifestV2.Source.BuildSHA` and `SchemaVersion` stay distinct metadata. Persist these with the plan and include them in the canonical payload hash in a reviewed plan-store format revision. `ExpectedSnapshotSHA256` is supplied with a reference to a verified operator case; the verifier returns the approved sorted account allowlist, nonce, and operation ID. Compare exact raw digest and scope before provider calls. Approval binds exact bytes and scope; it does not prove provider truth or old-writer fencing. A plan hash is an integrity key, not authority.

**Zero eligible accounts.** Preserve `RecoveryPlan.Validate`'s nonempty account invariant. Return typed `ErrNoEligibleAccounts` and persist no executable plan. An optional non-executable outcome record may bind the source digest, approval, provider observation cutoff, and reason codes. Missing, conflicting, stale, or incomplete evidence is an error, not a zero-eligible success. Never infer Free from missing customer linkage or a failed lookup.

**Plan-time journal facts versus future fence.** Plan is read-only. It records the observed active old writer generation `G`, authenticated writer identity and journal-store identity, snapshot cut, and ancestry/completeness through the snapshot cut using already durable journal history. It does not require a future seal, call `Seal`, or claim the active generation is sealed. The post-stop/post-revocation seal is obtained only in the later plan-wide recovery epoch. Re-observe active writer identity/generation before fencing; if changed from `G`, abort and require a newly approved plan.

**Plan-wide recovery versus account decisions.** One serialized recovery epoch owns stop, revocation, post-watermark seal, whole-snapshot restore/reconciliation, publication, and successor adoption. It covers the complete restored account inventory. Each snapshot account receives a durable terminal disposition: `WITHHELD_FROZEN` (with reason), or a journal-preserved `DELETED`/`DELETING` disposition; a successfully activated account has `ACTIVE_ELIGIBLE`. Missing/bad approval, provider ineligibility, ambiguity, stale evidence, or a current deletion intent means `WITHHELD_FROZEN` or the deletion disposition, never eligibility success. No account-local condition can block service for unrelated accounts after the global frozen restore. One-account activation cannot advance another account or repeat global restore/seal/adoption.

**Global recovery permission versus account activation.** A signed global epoch approval authorizes only quiesce, old-writer stop, complete credential/session revocation, sealing, validating/restoring the exact whole snapshot inventory, publishing it with all surviving accounts frozen, and adopting the single successor. Before the first disruptive side effect, `Begin` verifies that approval against the plan hash, full sorted snapshot inventory digest/list, exact plan/account scope, source digest, old writer identity/generation/store, operation ID, action set, expiry, and nonce. Per-account approvals are not prerequisites for `Begin` or global restore. No global approval can unfreeze any account. Each account remains `restore_pending`/Free until a separate exact-one-account approval and a fresh guarded provider/deletion check succeeds.

**Forward-only recovery.** Before any external effect, an authorized abort may release reservations and leave the old writer unchanged. Persist `EFFECTS_MAY_HAVE_STARTED` before the first quiesce/stop request; from that point, including timeout/uncertain response, rollback to old writer or credential restoration is forbidden. Exact-operation resume continues the same source, full inventory, plan, writer, and epoch. If approvals or global authorization need replacement after effects, a newly signed continuation authorization binds the same epoch and the exact next phase; it cannot change scope or authorize rollback. A global approval failure before effects is a safe abort. Missing/bad account approval or later eligibility loss is recorded `WITHHELD_FROZEN`; finish the frozen restore and recover service when every inventory account is terminal. Later activation is a separate guarded one-account operation against the already-published exact data, with new approval and current checks; it never restores, seals, adopts, or republishes.

## Existing source and ownership

Reusable implementation already present: `backup.InspectSnapshot` validates exact manifest/artifact bytes and reports raw digest/build identity; `internal/hosted/recovery` has strict canonical plan persistence; the concrete billing observer is read-only but only accepts current `restore_pending`; `service.AssembleWithDependencies` requires a shared journal and admission, then validates snapshot-cut/full-history reads and pending deletion intents; and `deletion.Journal` implements durable append, `ReadThrough`, and `Seal`. Do not duplicate these implementations or claim the inspector/observer is missing.

Important actual limits: `InspectSnapshot` owns and cleans its scratch copy before returning, so its returned inventory is not a durable Apply input. The existing billing observer cannot assess arbitrary candidate identities extracted from a snapshot. Service admission has no production factory. The current contracts have no capability for authenticated current-writer identity, active-generation inspection, atomic plan-wide recovery lease, durable successor adoption, or a separate post-fence evidence record. They also have no single-account activation guard spanning deletion/status mutation, provider refreads, final reread and row CAS; the account owner must supply and review that serialization boundary. `ReadThrough(...).Sealed` alone cannot establish adoption.

Ownership requests: T23.50 owns `internal/hosted/recovery/**` and `internal/cli/hosted_recovery.go`; backup owners add a pinned verified-snapshot lease API; billing owners add a narrow snapshot-candidate read-only observation API; deletion/journal owners add authenticated current-writer/ancestry and successor-adoption capabilities; service owners add epoch-aware admission wiring while preserving same-journal identity; a control-plane owner owns AWS stop/revocation/evidence adapters and their scoped role. These are dependency-linked owner patches. Planner code does not edit their files or permissions.

## Proposed private interfaces and records

Interfaces below are requested additions, not existing callable APIs. The shared contract additions proposed are `contracts.EvidenceRef{Authority, RecordID, Version}`, `ExpectedSnapshotSHA256` and `ApprovalRef` on `RecoveryPlanRequest`, plus a new single-account `AccountActivationRequest{EpochHash, AccountID, ApprovalRef}` / result pair if service needs a typed boundary. All other capabilities and terminal ledgers stay in owner-local packages. `Verified*` values are constructed only inside `internal/hosted/recovery` after its verifier fetches and validates referenced records. Authority adapters return opaque evidence references/claims; they never return trusted serialized `Verified*` values. Durable records store references and digests, and the recovery verifier refetches the exact immutable object version/proof chain before consuming it. The CLI cannot deserialize a `Verified*` value into authority.

The backup-owner amendment supplies the artifact handoff that current `InspectSnapshot` does not return:

```go
// New private implementations in internal/hosted/backup.
type VerifiedAccountCandidate interface {
    AccountID() string
    SnapshotStatus() string
    CustomerBinding() string
    CheckoutReferences() []string
    ManifestSHA256() string
    verifiedAccountCandidate() // package-private seal
}
type VerifiedSnapshotLease interface {
    Inspection() SnapshotInspection
    Candidate(ctx context.Context, accountID string) (VerifiedAccountCandidate, error)
    OpenVerifiedArtifact(ctx context.Context, name string) (io.ReadCloser, contracts.ArtifactRef, error)
    LeaseID() string
    Close(ctx context.Context) error // bounded, cancellable and idempotent
}
type VerifiedSnapshotStore interface {
    Stage(ctx context.Context, source, expectedManifestSHA256 string, options InspectionOptions) (VerifiedSnapshotLease, error)
    Reopen(ctx context.Context, leaseID, expectedManifestSHA256 string) (VerifiedSnapshotLease, error)
    Pin(ctx context.Context, leaseID, owner string, expires time.Time) error
    Release(ctx context.Context, leaseID, owner string) error // bounded, cancellable, idempotent
}
func RestoreVerified(ctx context.Context, lease VerifiedSnapshotLease, destination string) error
```

`Stage` extracts/reuses the same one-pass verifier used by `InspectSnapshot`, writes verified bytes to owner-only bounded durable staging, fsyncs content and directory, and returns the private lease handle; `InspectSnapshot` may continue to close its short-lived lease before returning its current metadata result. `RestoreVerified` consumes only the pinned handle and shared restore implementation. Neither method accepts an artifact path from the caller after verification. The backup owner owns lease cleanup/reopen identity checks and must preserve ordinary `Restore` validation through this shared implementation.

```go
// Proposed additive fields in internal/hosted/contracts/backup.go:
type RecoveryPlanRequest struct {
    SnapshotPath string
    ExpectedSnapshotSHA256 string
    ApprovalRef EvidenceRef // immutable operator-case reference
}
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
    SnapshotStore backup.VerifiedSnapshotStore
    Approvals RecoveryApprovalVerifier
    Billing RecoverySnapshotBillingObserver
    Journal RecoveryJournalStateSource
    ControlPlane RecoveryControlPlane
    EvidenceReader ImmutableEvidenceReader
    TrustPolicy EvidenceTrustPolicy
    Applier RecoveryRestoreApplier
    Limits Limits
}
func NewProductionCoordinator(cfg Config) (*Coordinator, error)
func newCoordinatorForTest(deps testDependencies) (*Coordinator, error) // package-private; never registered in CLI/service

type RecoveryEpochRecord struct {
    Version int; Sequence uint64; PriorDigest string
    PlanHash, PlanRef, OperationID, PlanApprovalDigest, PlanApprovalNonce string
    EpochApprovalDigest, EpochApprovalNonce, SnapshotInventoryDigest string
    SnapshotAccountIDs, PlanEligibleAccountIDs []string
    OldWriterDigest, JournalStoreID string
    SnapshotCut contracts.DeletionWatermark; PrefixDigest, SnapshotLeaseID string
    Phase string; StopRef, RevocationRef, SealRef, RestoreRef, PublishRef, AdoptionRef string
    SuccessorDigest string; UpdatedAt time.Time
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
type SnapshotIdentityCandidate interface {
    AccountID() string
    ManifestSHA256() string
    CustomerBindingCandidateDigest() string
    CheckoutReferences() []string
    SnapshotStatus() string
    verifiedSnapshotIdentityCandidate() // package-private seal
}
type RecoverySnapshotBillingObserver interface {
    ObserveSnapshotIdentity(ctx context.Context, candidate SnapshotIdentityCandidate) (SnapshotBillingProjection, error)
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
    StageAndReconcile(ctx context.Context, snapshot backup.VerifiedSnapshotLease, seal VerifiedSeal, limits Limits) (StagedRestore, error)
    Publish(ctx context.Context, staged StagedRestore, expectedDestinationID string) (PublishedRestore, error)
}

type AccountActivationRequest struct {
    EpochHash string
    AccountID string
    ApprovalRef EvidenceRef
}
type AccountActivationResult struct { State, ReasonCode, RecordDigest string }
type RecoveryAccountActivator interface {
    ActivateOne(ctx context.Context, req AccountActivationRequest) (AccountActivationResult, error)
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
// Coordinator implements contracts.RecoveryPlanner. Account activation is a separate single-account capability.
```

`NewProductionCoordinator` is the only production factory and must bind the owner-reviewed production snapshot store, billing projection, journal authority, control-plane executor, evidence reader, and restore applier; it rejects missing or test-only implementations and never loads them from CLI input. The `RecoveryEvidenceVerifier` and approval verifier are recovery-package concrete implementations that consume exact-version `ImmutableEvidenceReader` output and verified operator-case verifier output; they construct the private-field `Verified*` values. `newCoordinatorForTest` is package-private. No serialized evidence can be unmarshaled directly into one of those values. Each production factory is a required reviewed handoff from the owning package; absence fails construction.

The lease API lives in backup because `InspectSnapshot` currently cleans its scratch artifacts before returning. Apply and `RestoreVerified` consume only that lease, never reopen the untrusted source. `Candidate(ctx, accountID)` reads account identity/binding/checkout fields from the exact pinned copy; the sealed interface prevents callers from constructing verified candidates. All I/O and `Close`/`Release` are context-cancellable and bounded; close/release are idempotent. `Pin` binds the lease to immutable plan/epoch and protects it across process restart. Reopen checks owner, lease ID, content digest, file identity, permissions, and expiry. Persist a `PLAN_PREPARED` record containing canonical plan bytes/hash, lease ID, and operation ID with fsync; then idempotently pin by plan hash; then atomically publish the READY plan and mark the prepare record committed; only then return. On crash, reconciliation verifies the same bytes and retries pin/publication. An unresolved prepared lease is resumed exactly or explicitly abandoned before approvals/effects; a pinned orphan is never auto-deleted. Unpinned owner-created stage orphans may be collected after bounded grace and inode/marker verification. No system-temp fallback, source reopen, or unbounded orphan accumulation.

The billing owner adds a narrow `ObserveSnapshotIdentity(ctx, SnapshotIdentityCandidate) (SnapshotBillingProjection, error)` read-only projection in addition to preserving the existing `ObserveRestorePending` observer unchanged. `SnapshotIdentityCandidate` is privately constructed from the verified lease and contains account ID, raw manifest digest, customer-binding candidate digest, sorted checkout/session refs and snapshot status. The projection has an explicit conflict set and source-version/refread digests. It returns only `Eligible`, reason code, opaque binding digest, checkout-set digest, provider source version, observation/expiry times, and canonical digest; it performs no writes. The admission does not trust `Eligible` alone: it rechecks under the same account guard and requires all those bindings to match. The new API must preserve exact checkout semantics: account/status/customer-binding pre-read; reject conflict/deleted/deleting; call read-only provider endpoints for every exact checkout/session ref and current subscription; refread every ref; compare complete sorted sets and state/version; final account/status/binding/checkout reread must equal pre-read; any missing, added, removed, conflicting, pending, unknown, unqueryable, or changed ref fails closed. Do not add snapshot-path or caller-supplied provider IDs to this API. The existing `ObserveRestorePending` observer remains unchanged.

`ObserveSnapshotIdentity` is a new billing-owner API; do not route these calls through `ObserveRestorePending`, which correctly accepts only current `restore_pending` rows. It treats snapshot identity fields only as lookup candidates, not billing authority. Before provider I/O it validates the sealed candidate/digest, account ID shape, and reads current account/customer binding/status plus all current checkout rows for that account. A present conflicting binding, deleting/deleted status, or incompatible live row fails. Snapshot `deleted`/`deleting` statuses are excluded before provider I/O. Snapshot `restore_pending` and historical `active` rows may be candidates only after their customer binding is independently tied to the opaque account ID by an existing authoritative relation. If the current account is absent, provider-side proof of that relation is mandatory; if no such relation exists, return `ErrIdentityUnverified` without querying subscriptions. No new provider metadata key is assumed: the billing owner must name the existing verified mapping or keep the API fail-closed.

For an identity-verified candidate, make bounded read-only provider calls for current subscriptions/invoices and each exact snapshot checkout/session reference using the provider's supported read API. Any snapshot or current checkout ref that is missing, unknown, pending, nonterminal, duplicated, or unqueryable blocks; do not expire, delete, reconcile, or clear it. Compare current checkout row refs/state with the snapshot-derived set; conflict blocks. After provider I/O, reread account status/customer binding and checkout rows; require status/binding/ref digests unchanged from the pre-read, and refuse if any row changed, was added, disappeared, or became nonterminal. Verify current provider price/window/grace and source object versions, with `CustomerBindingDigest`, `ProviderSourceVersion`, `ObservedAt`, `ExpiresAt`, and canonical `ObservationDigest` in the return. No SQL, entitlement, checkout, or audit writes occur. If the provider lacks a read endpoint for an exact checkout reference, that candidate is blocked rather than treated as resolved.

`CurrentWriter` returns separate facts: active old writer identity/generation `G`, stable journal-store ID, last previously sealed historical watermark `H` (possibly older than `G`), observation time, and immutable evidence reference. The recovery-owned verifier validates the claim and constructs private `VerifiedWriterIdentity`. `ReadAncestry` returns an immutable reference; recovery re-fetches that exact version and `VerifyJournalPrefix` proves the durable chain from zero through snapshot cut `C`; it may include the unsealed active-generation prefix through `C`. It does not require or synthesize the future seal. Plan records `H`, `C`, and `G` separately and rejects a cut not present in verified ancestry. `ReserveRecoveryEpoch` is an authoritative durable CAS on the same generation control plane: after success, no second recovery epoch or regular writer start/generation advance is possible for this journal until the exact operation commits or an explicit reviewed abort completes. It is operation-owned and does not auto-expire into another writer's authority; its immutable reference is re-fetched and verified before use. After verified stop and complete revocation, `SealStoppedGeneration(G, C)` creates the post-fence seal; independently verify `ReadThrough(zero)` complete through this exact seal `S` before restore. Persist `H`, `C`, `G`, `S`, and successor `G+1` as distinct values. Only after restore reconciliation may `AdoptSuccessor` create the immutable `G+1` adoption record bound to predecessor identity, seal digest, successor identity, and generation. Generation write authority remains blocked while the reservation is held. `CommitRecoveryEpoch` atomically commits terminal epoch/adoption and releases the reservation only to the exact successor. No value is inferred from a configured generation or empty prefix.

## Approval, plan, and exact scope

The signed plan operator case must bind raw snapshot SHA-256, sorted allowlisted account IDs, one recovery `operation_id`, a cryptographically unique `approval_nonce`, and expiry. `VerifyPlan` returns a recovery-owned verified approval containing the canonical binding and verifier/key identity. `Plan` compares digest before provider calls, intersects snapshot IDs with that allowlist, and makes provider calls only for the sorted intersection. Snapshot accounts outside the approved set are neither queried nor placed in the executable plan; they remain in the exact full restore inventory and are terminally frozen. The plan payload binds approval digest, nonce, operation ID, sorted allowlist, selected eligible IDs, raw manifest digest, separate build/schema provenance, per-account observation digest/time/expiry/customer-binding digest/provider source version, old writer identity/generation/store, last previously sealed watermark, snapshot cut, and verified existing journal-prefix reference. Any change to these changes the plan hash.

Planning sequence: acquire a short exclusive planning lease; obtain and pin `VerifiedSnapshotLease`; verify approval exact source digest/scope/freshness; read complete verified inventory; get active writer `W/G`, stable store identity, and last previously sealed watermark `H`; establish journal ancestry from zero through snapshot cut `C` (confirm `C` is present), without sealing; call the snapshot-candidate billing projection only for approved candidates, in a fixed-size worker pool; fail on ambiguity; assemble a deterministic plan with `RecoveryPlan.Generation=G` and `RecoveryPlan.JournalWatermark=C`; persist canonical bytes and plan digest atomically with file and parent-directory fsync; pin staged snapshot to plan; then return. The allowlist is the provider-call ceiling, even if the source has additional accounts. If no eligible account exists, atomically persist only the distinct non-executable outcome and release the lease when safe; this outcome cannot enter `Begin`, restore, or produce admission. Recovery remains fail-closed until an operator provides a newly scoped executable plan or a separately reviewed frozen-only recovery path.

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

`Commit` never requires all accounts to be active or currently provider-eligible; it requires every inventory account to be explicitly terminal. Whole database publishes frozen first, then recovered service starts. The terminal per-account account-state ledger must agree with preserved deletion journal truth: deletion wins. No “auto-exclude” changes signed scope; outside-scope IDs are explicitly `WITHHELD_FROZEN/NOT_IN_APPROVED_SCOPE` and receive no provider calls. A continuation approval can authorize only exact forward phase progress within the same full inventory and cannot alter per-account dispositions except through separately approved activation.

After `EPOCH_COMMITTED`, `ActivateOne(ctx, epochHash, accountID, accountApprovalRef)` acts on one account in the already-published destination; it cannot call restore, replay, seal, adoption, or publication. It acquires the billing/service owner’s account guard, which serializes account deletion/forget, status changes, and activation. Under that guard it requires the row still matches this epoch’s exact restore marker/source digest and remains `restore_pending`/Free; checks current account/customer binding, all checkout refs and conflict/ref-read semantics using a new narrow snapshot identity read-only projection API; reads deletion intent/history through the already-committed S and refuses if deleted/deleting; verifies the one-account approval and current provider eligibility; then performs final reread and compare-and-swap of only that account’s entitlement/status while retaining the guard. Any missing/bad approval or ineligible/ambiguous/stale result records `WITHHELD_FROZEN` and leaves the row frozen. Provider projection follows the existing observer’s bounded calls, pre-read/current-row and checkout-ref checks, provider refreads, and final reread; it adds exact snapshot candidate identity mapping without weakening current observer. Forbid a second restore or epoch adoption while this API runs. Repeated activation is idempotent by `(epoch, account, nonce)`; a later attempt needs a new nonce/approval and current checks against the same restored data. Account guard API/locked observer variant is a billing/service owner amendment.
The concrete `FROZEN` target is the private restored destination with every surviving account `restore_pending`/Free and every subscription `restore_pending`, plus journal deletions replayed through `S`; old service remains quiesced. Every row remains frozen in the private destination and after publication. Before atomic publication, failed/canceled work leaves live destination unchanged. After publication but before epoch commit, service admission remains denied and no writer may start. This exact state is resumable; never auto-roll back by re-enabling the old writer. A crash around rename is resolved by comparing the persisted intended destination identity and exact manifest/restore digests. A crash around commit is resolved from the terminal epoch record; never assume timeout means rollback.

Replace the ambiguous apply-to-staging endpoint with `Coordinator.ActivateOne`, a recovery-owned service API callable only after `EPOCH_COMMITTED`. It accepts exactly one `(epoch_hash, account_id, account_approval_ref)` and only mutates that account under its serialized owner guard. It cannot stop writers, seal, restore, adopt, republish, or admit startup. No public `apply-all` endpoint.

The global epoch record separately persists and admission re-fetches immutable stop, all-item revocation, post-watermark seal, restore/publish and adoption references. `FenceReceipt.SufficientFor` remains only the existing shape/binding check; its booleans and timestamp are compatibility summary only. Account activation must continue to enforce the existing single-account consistency/binding predicate where its additive service contract exposes it, but neither that predicate nor a receipt substitutes for current guarded billing/deletion checks or the global evidence references.

The `internal/cli/hosted_recovery.go` entrypoint sequence is `plan --snapshot --expected-manifest-sha256 --approval-ref` (read-only provider planning; returns plan hash), `begin --plan-hash --epoch-approval-ref` (verifies full inventory before any disruptive effect), `continue --plan-hash --continuation-approval-ref` (same-epoch forward progress), `commit --plan-hash` (requires full restore/adoption and terminal disposition for every inventory account), `activate-one --epoch-hash --account-id --approval-ref` (only after commit), and `status --plan-hash` (redacted persisted phase). Every command returns nonzero on incomplete evidence. `begin/continue/commit` are resumable only under the persisted plan operation/global approval/continuation bindings; caller flags cannot override account scope, generation, provider identity, or operation ID.

## Startup paths this contract does not implement

`RecoveryAdmission` is a one-time recovered-start proof for a committed epoch at sealed S. It does not supply authenticated journal genesis and does not solve an ordinary process restart after successor G+1 has appended an unsealed tail. Do not fabricate an empty plan, generation zero, bootstrap seal, or reuse the previous recovery admission to accept that tail. Genesis needs a journal-owner authenticated durable genesis authorization bound to store and initial writer. Ordinary restart needs a separately reviewed startup handshake that verifies active writer/store/generation and the complete authorized chain/head including a valid unsealed G+1 tail. Current `service.AssembleWithDependencies` requires sealed-boundary evidence, so that restart path remains fail-closed until service/journal owners add and review the distinct path. Therefore this proposal is code-ready only for recovery planning, frozen whole-database recovery, committed recovered-start admission, and later single-account activation; it does not claim full hosted startup readiness.

## Production admission and AWS boundary

Request `service.RecoveryAdmission` construction as `recovery.NewAdmission(epochStore, activeWriterIdentitySource, journalIdentityVerifier, buildSHA)`; it uses the package-owned evidence verifier and cannot accept one from service config or CLI. Runtime identity must come from an owner-reviewed local supervisor/control-plane channel with peer authentication; no static environment generation, caller value, or app IMDS. `Admit(ctx, journal)` accepts only `EPOCH_COMMITTED`, re-verifies stop, complete revocation, seal, publication, adoption, approval/plan/source provenance, all account records, and active successor identity, then checks the supplied journal is the adopted stable store. It returns the snapshot cut only after verified history reads establish the same sealed boundary; existing service assembly then retains its current full-history and intent checks. The same configured journal object/store must be wired into service, backup, fence, and admission. Any absent/stale evidence, mismatched store/build, nonterminal inventory disposition, or active competing epoch refuses recovered-start admission. This factory covers only the first service admission following a committed recovery epoch and its sealed boundary; it is not a general startup factory.

Preserve the hosted app's current IMDS denial. No instance profile, broad host credentials, default AWS credential chain, or app authority to stop instances/revoke credentials/adopt generations. A separately provisioned control-plane executable uses explicit scoped role credentials and the AWS CLI with fixed argument arrays, explicit region/profile, bounded context, scrubbed environment, and captured request IDs. For EC2 stop it verifies exact account/region/instance identity with `aws ec2 describe-instances`, calls `aws ec2 stop-instances`, waits with `aws ec2 wait instance-stopped`, then re-describes and persists immutable evidence references. Revocation must enumerate actual deployment credential/session classes at their owning authorities, revoke or deny each, and independently verify denial. EC2 stop does not establish revocation. The actual identity model, AWS role, policy scope, and credential classes remain owner inputs; no invented permissions or metadata key are proposed. The verifier retrieves provider evidence and checks exact subject/scope/version/status; CLI output or fake output alone is not evidence.

## Bounds and deterministic fake-only RED controls

Use these proposed hard ceilings (integrator owners may lower them; raising one requires a reviewed policy update): at most 100 accounts per recovery plan; 10,000 snapshot accounts/brains at the inspector boundary; 64 checkout refs per account; 4 concurrent provider calls; 4,096 provider GETs per plan; each provider timeout at most 30 seconds, response at most 1 MiB, total response bytes at most 64 MiB; evidence record at most 1 MiB; canonical plan/epoch record at most 4 MiB; each operation invocation at most 30 minutes before returning resumable state; provider observation age at most 5 minutes at one-account activation; snapshot declared bytes capped by explicit SSD-backed configuration no greater than the inspector's 1 TiB absolute limit. Existing billing observer parser ceilings remain in force (JSON depth 32, nodes 100,000, event pages 32, invoices 16, invoice-line pages 4); aggregate provider GETs still obey the plan-wide cap. No default path or filesystem temp fallback. The external volume must enforce actual capacity; a configured byte ceiling alone is not a hard physical quota. Never truncate or partially accept work. Sort all IDs/references before hashing and writes. Per-provider/CLI calls have context deadlines and cancellation. No partial success is returned.

Fake-only tests must prove: raw-byte identity including whitespace changes; digest mismatch causes zero provider calls; unapproved and reordered/replayed account scope makes zero calls for out-of-scope IDs and changes/rejects the bound plan; nonce cannot replay across operation/plan/account while exact tuple retry resumes; every hashed provenance/evidence field affects canonical plan hash; strict decoder rejects duplicate/unknown/trailing JSON; lease restart verifies pinned content and never reopens source; tampered/replaced scratch is rejected; account/customer conflict or missing authoritative identity blocks before provider call; deleted/deleting snapshot account causes zero provider calls; snapshot/current checkout mismatch, added/disappeared/refread change, pending/unknown/unqueryable checkout, provider timeout, stale results never yield eligibility; planning does not call Seal and succeeds with a valid unsealed active `G` prefix through cut `C` while recording last seal `H` separately; active writer change between Plan and Begin blocks; stop/revocation failure prevents seal; seal/full-history mismatch prevents restore; missing/invalid global approval and full-inventory mismatch cause zero effects; missing/bad account approval before and provider eligibility loss after fence become durable `WITHHELD_FROZEN`; permanent provider outage cannot prevent frozen publication/admission for unrelated accounts; every snapshot account has terminal ledger state; frozen whole restore publishes atomically; deleted/deleting intent always wins; one-account activation checks current deletion/status/provider and checkout identity under account guard, leaves conflicts frozen, and never reruns restore/seal/adoption; activation for A cannot alter B; concurrent Begin and ActivateOne calls serialize; crash before/after each persisted transition is idempotent; no `G+1` writer starts before full destination publication and committed epoch; configured `G+1`, empty prefix, fake provider success, signed approval/hash, copied lock/PID, or booleans alone never pass admission; IMDS/default credential access remains denied. These tests establish local state-machine behavior only, not live provider, IAM, stop/revocation, durable storage, or activation qualification.

## Current blockers and handoff boundary

Locally implementable after review: the recovery owner can add the coordinator/state records, exact-scope request validation, staged-lease consumer, per-account terminal ledger and guarded activation, CLI entrypoints, and fake-only tests behind injected interfaces. The backup owner must first define and review the pinned verified-snapshot lease. Billing owner must establish an authoritative account/customer identity relation and review `ObserveSnapshotIdentity`; until it exists, affected candidates fail closed. Journal owner must provide verified current-writer/ancestry plus durable seal/adoption capability. Service owner must wire supervisor quiescence, shared journal identity, atomic full-destination publication, and committed-epoch admission. A separate control-plane owner must define actual credential/session inventory and scoped AWS authority. Do not report production integration clear until these owner patches are reviewed and landed.

Live-only gates remain: exact operator-approved source and account scope; fresh real provider observations; authoritative old-writer identity and stop; complete and effective revocation of every credential/session; post-fence durable journal seal; whole private restore and atomic publication; durable successor adoption; production shared journal wiring and scoped write credentials; physical storage limits/durability; and operator-controlled activation. No provider action, deployment, purge, spend, or activation is part of this proposal.
