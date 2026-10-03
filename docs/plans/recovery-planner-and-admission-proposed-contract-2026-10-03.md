# Recovery planner and production admission: proposed integration contract

**Status:** revised proposal following independent review; source baseline `8081632` after PR #349. This is not an approved interface change or provider/deployment/activation authority. Original proposal commit `05953ed` is preserved in history; this revision addresses its five blocking findings. Frozen interfaces remain unchanged until owner review and an additive amendment.

## Contract rulings

**Snapshot identity.** Set `contracts.RecoveryPlan.SourceSnapshot` to lowercase SHA-256 of the exact raw `manifest.json` byte sequence that `backup.InspectSnapshot` verified. Its 64 lowercase hex form satisfies the existing object-ID shape. Never hash re-encoded JSON. `ManifestV2.Source.BuildSHA` and `SchemaVersion` stay distinct metadata. Persist these with the plan and include them in the canonical payload hash in a reviewed plan-store format revision. `ExpectedSnapshotSHA256` is supplied with a reference to a verified operator case; the verifier returns the approved sorted account allowlist, nonce, and operation ID. Compare exact raw digest and scope before provider calls. Approval binds exact bytes and scope; it does not prove provider truth or old-writer fencing. A plan hash is an integrity key, not authority.

**Zero eligible accounts.** Preserve `RecoveryPlan.Validate`'s nonempty account invariant. Return typed `ErrNoEligibleAccounts` and persist no executable plan. An optional non-executable outcome record may bind the source digest, approval, provider observation cutoff, and reason codes. Missing, conflicting, stale, or incomplete evidence is an error, not a zero-eligible success. Never infer Free from missing customer linkage or a failed lookup.

**Plan-time journal facts versus future fence.** Plan is read-only. It records the observed active old writer generation `G`, authenticated writer identity and journal-store identity, snapshot cut, and ancestry/completeness through the snapshot cut using already durable journal history. It does not require a future seal, call `Seal`, or claim the active generation is sealed. The post-stop/post-revocation seal is obtained only in the later plan-wide recovery epoch. Re-observe active writer identity/generation before fencing; if changed from `G`, abort and require a newly approved plan.

**Plan-wide recovery versus account decisions.** One serialized recovery epoch owns stop, revocation, post-watermark seal, whole-snapshot restore/reconciliation, publication, and successor adoption. Per-account records own only that approved account's fresh billing decision and its frozen/active row transition. Success for one account never advances another account or commits the epoch. No successor writer starts while any planned account is unresolved or any staged data is unpublished. This prevents concurrent applies or account-level retries from racing a `G+1` writer into the old journal.

## Existing source and ownership

Reusable implementation already present: `backup.InspectSnapshot` validates exact manifest/artifact bytes and reports raw digest/build identity; `internal/hosted/recovery` has strict canonical plan persistence; the concrete billing observer is read-only but only accepts current `restore_pending`; `service.AssembleWithDependencies` requires a shared journal and admission, then validates snapshot-cut/full-history reads and pending deletion intents; and `deletion.Journal` implements durable append, `ReadThrough`, and `Seal`. Do not duplicate these implementations or claim the inspector/observer is missing.

Important actual limits: `InspectSnapshot` owns and cleans its scratch copy before returning, so its returned inventory is not a durable Apply input. The existing billing observer cannot assess arbitrary candidate identities extracted from a snapshot. Service admission has no production factory. The current contracts have no capability for authenticated current-writer identity, active-generation inspection, atomic plan-wide recovery lease, durable successor adoption, or a separate post-fence evidence record. `ReadThrough(...).Sealed` alone cannot establish adoption.

Ownership requests: T23.50 owns `internal/hosted/recovery/**` and `internal/cli/hosted_recovery.go`; backup owners add a pinned verified-snapshot lease API; billing owners add a narrow snapshot-candidate read-only observation API; deletion/journal owners add authenticated current-writer/ancestry and successor-adoption capabilities; service owners add epoch-aware admission wiring while preserving same-journal identity; a control-plane owner owns AWS stop/revocation/evidence adapters and their scoped role. These are dependency-linked owner patches. Planner code does not edit their files or permissions.

## Proposed private interfaces and records

Interfaces below are requested additions, not existing callable APIs. The only shared contract additions proposed are `contracts.EvidenceRef{Authority, RecordID, Version}`, `ExpectedSnapshotSHA256` and `ApprovalRef` on `RecoveryPlanRequest`, and per-account `ApprovalRef` on `RecoveryApplyRequest`; all other new capabilities stay in owner-local packages. `Verified*` values are constructed only inside `internal/hosted/recovery` after its verifier fetches and validates referenced records. Authority adapters return opaque evidence references/claims; they never return trusted serialized `Verified*` values. Durable records store references and digests, and the recovery verifier refetches the exact immutable object version/proof chain before consuming it. The CLI cannot deserialize a `Verified*` value into authority.

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
    Candidate(accountID string) (VerifiedAccountCandidate, error)
    OpenVerifiedArtifact(name string) (io.ReadCloser, contracts.ArtifactRef, error)
    LeaseID() string
    Close() error
}
type VerifiedSnapshotStore interface {
    Stage(ctx context.Context, source, expectedManifestSHA256 string, options InspectionOptions) (VerifiedSnapshotLease, error)
    Reopen(ctx context.Context, leaseID, expectedManifestSHA256 string) (VerifiedSnapshotLease, error)
    Pin(ctx context.Context, leaseID, owner string, expires time.Time) error
    Release(ctx context.Context, leaseID, owner string) error
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
type RecoveryApplyRequest struct {
    PlanHash string
    AccountID string
    ApprovalRef EvidenceRef // immutable one-account authorization reference
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
type RecoveryApprovalVerifier interface {
    VerifyPlan(ctx context.Context, ref EvidenceRef, expectedSnapshotSHA256 string) (VerifiedPlanApproval, error)
    VerifyAccount(ctx context.Context, ref EvidenceRef, planHash, accountID, operationID string) (VerifiedAccountApproval, error)
}
type VerifiedPlanApproval struct { binding ApprovalBinding; digest, verifierID, keyVersion string }
type VerifiedAccountApproval struct { planHash, accountID, operationID, nonce, digest string; expiresAt time.Time }

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
    PlanHash, PlanRef, OperationID, ApprovalDigest, ApprovalNonce string
    ApprovedAccounts []string; OldWriterDigest, JournalStoreID string
    SnapshotCut contracts.DeletionWatermark; PrefixDigest, SnapshotLeaseID string
    Phase string; StopRef, RevocationRef, SealRef, RestoreRef, PublishRef, AdoptionRef string
    SuccessorDigest string; UpdatedAt time.Time
}
type AccountRecoveryRecord struct {
    PlanHash, OperationID, AccountID string
    ApprovalDigest, ApprovalNonce string
    ObservationDigest string; ObservedAt, ExpiresAt time.Time
    Phase, ReasonCode, RecoveryMarker, EpochRecordDigest string
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
    ObserveSnapshotCandidate(ctx context.Context, candidate backup.VerifiedAccountCandidate) (SnapshotBillingProjection, error)
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
    ApplyAccount(ctx context.Context, staged StagedRestore, plan contracts.RecoveryPlan, req contracts.RecoveryApplyRequest, approval VerifiedAccountApproval) (contracts.RecoveryApplyResult, error)
    Publish(ctx context.Context, staged StagedRestore, expectedDestinationID string) (PublishedRestore, error)
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
type BeginEpochRequest struct { PlanHash string }
type CommitEpochRequest struct { PlanHash string }
type EpochStatus struct { PlanHash, OperationID, Phase, RecordDigest string }

type RecoveryEpochCoordinator interface {
    Begin(ctx context.Context, req BeginEpochRequest) (EpochStatus, error)
    Apply(ctx context.Context, req contracts.RecoveryApplyRequest) (contracts.RecoveryApplyResult, error)
    Commit(ctx context.Context, req CommitEpochRequest) (EpochStatus, error)
}
// Coordinator implements contracts.RecoveryPlanner and contracts.RecoveryApplier.
```

`NewProductionCoordinator` is the only production factory and must bind the owner-reviewed production snapshot store, billing projection, journal authority, control-plane executor, evidence reader, and restore applier; it rejects missing or test-only implementations and never loads them from CLI input. The `RecoveryEvidenceVerifier` and approval verifier are recovery-package concrete implementations that consume exact-version `ImmutableEvidenceReader` output and verified operator-case verifier output; they construct the private-field `Verified*` values. `newCoordinatorForTest` is package-private. No serialized evidence can be unmarshaled directly into one of those values. Each production factory is a required reviewed handoff from the owning package; absence fails construction.

The lease API lives in backup because `InspectSnapshot` currently cleans its scratch artifacts before returning. Apply and `RestoreVerified` consume only that lease, never reopen the untrusted source. `Candidate(accountID)` reads account identity/binding/checkout fields from the exact pinned copy; the sealed interface prevents callers from constructing verified candidates. `Pin` binds the lease to immutable plan/epoch and protects it across process restart. Reopen checks owner, lease ID, content digest, file identity, permissions, and expiry. Release follows terminal epoch/retention policy. Startup garbage collection removes only expired, unpinned, owner-created leases after verifying inode/marker identity. No system-temp fallback, source reopen, or unbounded orphan accumulation.

`ObserveSnapshotCandidate` is a new billing-owner API; do not route these calls through `ObserveRestorePending`, which correctly accepts only current `restore_pending` rows. It treats snapshot identity fields only as lookup candidates, not billing authority. Before provider I/O it validates the sealed candidate/digest, account ID shape, and reads current account/customer binding/status plus all current checkout rows for that account. A present conflicting binding, deleting/deleted status, or incompatible live row fails. Snapshot `deleted`/`deleting` statuses are excluded before provider I/O. Snapshot `restore_pending` and historical `active` rows may be candidates only after their customer binding is independently tied to the opaque account ID by an existing authoritative relation. If the current account is absent, provider-side proof of that relation is mandatory; if no such relation exists, return `ErrIdentityUnverified` without querying subscriptions. No new provider metadata key is assumed: the billing owner must name the existing verified mapping or keep the API fail-closed.

For an identity-verified candidate, make bounded read-only provider calls for current subscriptions/invoices and each exact snapshot checkout/session reference using the provider's supported read API. Any snapshot or current checkout ref that is missing, unknown, pending, nonterminal, duplicated, or unqueryable blocks; do not expire, delete, reconcile, or clear it. Compare current checkout row refs/state with the snapshot-derived set; conflict blocks. After provider I/O, reread account status/customer binding and checkout rows; require status/binding/ref digests unchanged from the pre-read, and refuse if any row changed, was added, disappeared, or became nonterminal. Verify current provider price/window/grace and source object versions, with `CustomerBindingDigest`, `ProviderSourceVersion`, `ObservedAt`, `ExpiresAt`, and canonical `ObservationDigest` in the return. No SQL, entitlement, checkout, or audit writes occur. If the provider lacks a read endpoint for an exact checkout reference, that candidate is blocked rather than treated as resolved.

`CurrentWriter` returns separate facts: active old writer identity/generation `G`, stable journal-store ID, last previously sealed historical watermark `H` (possibly older than `G`), observation time, and immutable evidence reference. The recovery-owned verifier validates the claim and constructs private `VerifiedWriterIdentity`. `ReadAncestry` returns an immutable reference; recovery re-fetches that exact version and `VerifyJournalPrefix` proves the durable chain from zero through snapshot cut `C`; it may include the unsealed active-generation prefix through `C`. It does not require or synthesize the future seal. Plan records `H`, `C`, and `G` separately and rejects a cut not present in verified ancestry. `ReserveRecoveryEpoch` is an authoritative durable CAS on the same generation control plane: after success, no second recovery epoch or regular writer start/generation advance is possible for this journal until the exact operation commits or an explicit reviewed abort completes. It is operation-owned and does not auto-expire into another writer's authority; its immutable reference is re-fetched and verified before use. After verified stop and complete revocation, `SealStoppedGeneration(G, C)` creates the post-fence seal; independently verify `ReadThrough(zero)` complete through this exact seal `S` before restore. Persist `H`, `C`, `G`, `S`, and successor `G+1` as distinct values. Only after restore reconciliation may `AdoptSuccessor` create the immutable `G+1` adoption record bound to predecessor identity, seal digest, successor identity, and generation. Generation write authority remains blocked while the reservation is held. `CommitRecoveryEpoch` atomically commits terminal epoch/adoption and releases the reservation only to the exact successor. No value is inferred from a configured generation or empty prefix.

## Approval, plan, and exact scope

The signed operator case must bind raw snapshot SHA-256, sorted allowlisted account IDs, one recovery `operation_id`, a cryptographically unique `approval_nonce`, and expiry. `VerifyPlan` returns a recovery-owned verified approval containing the canonical binding and verifier/key identity. `Plan` compares digest before provider calls, intersects snapshot IDs with that allowlist, and makes provider calls only for the sorted intersection. Snapshot accounts outside the approved set are neither queried nor included. The plan payload binds approval digest, nonce, operation ID, sorted allowlist, selected eligible IDs, raw manifest digest, separate build/schema provenance, per-account observation digest/time/expiry/customer-binding digest/provider source version, old writer identity/generation/store, last previously sealed watermark, snapshot cut, and verified existing journal-prefix reference. Any change to these changes the plan hash.

Planning sequence: acquire a short exclusive planning lease; obtain and pin `VerifiedSnapshotLease`; verify approval exact source digest/scope/freshness; read complete verified inventory; get active writer `W/G`, stable store identity, and last previously sealed watermark `H`; establish journal ancestry from zero through snapshot cut `C` (confirm `C` is present), without sealing; call the snapshot-candidate billing projection only for approved candidates, in a fixed-size worker pool; fail on ambiguity; assemble a deterministic plan with `RecoveryPlan.Generation=G` and `RecoveryPlan.JournalWatermark=C`; persist canonical bytes and plan digest atomically with file and parent-directory fsync; pin staged snapshot to plan; then return. The allowlist is the provider-call ceiling, even if the source has additional accounts. If no eligible account exists, atomically persist only the distinct non-executable outcome and release the lease when safe.

The verified plan approval scope bounds provider calls. Plan includes only eligible IDs in the intersection of snapshot IDs and that allowlist. Each account apply requires a second verified approval binding the exact plan hash, exactly one account ID, operation ID from the verified plan, fresh account nonce, and expiry. Plan approval cannot authorize another account. `Begin` reads operation ID and plan nonce from the verified plan envelope, atomically reserves `(operationID, planHash, nonce)`; exact retries with the same `PlanHash` resume that durable epoch. Each single-account `Apply` similarly reserves the account nonce to `(operationID, planHash, accountID)`. Exact retries return/resume that account record; reusing either nonce with any changed tuple is replay and fails. Plan reads before `Begin` do not consume the nonce. Verify expiry and key/revocation status when each nonce is first consumed; persist verified signature/key/binding digest and accepted time. Exact-operation crash retries may use that persisted approval after nominal expiry, but re-fetch and verify the signer/key has not revoked it. An expired, never-consumed account approval may be replaced by a new signed approval for the same plan/account/operation and a new nonce; it cannot change scope. Provider observations and stop/revocation/adoption facts have independent freshness/reverification rules at each consuming phase. Persist rejected attempts only in a minimized private audit record and never log signed payloads or secrets.

## Serialized epoch and per-account state machine

Add private canonical records in the recovery owner package, with append-only hash chaining, owner-only permissions, strict versioned JSON, monotonic sequence, CAS on prior digest, fsync-before-effect, and atomic directory updates. `RecoveryEpochRecord` fields: schema version, plan hash/ref, operation ID, plan approval digest/nonce, exact sorted eligible account set, old writer identity digest/generation/store ID, prior sealed watermark, snapshot cut and prefix digest, staged snapshot lease ID/digest, current phase, quiescence/stop/revocation/seal/restore/publication/adoption evidence references and verifier digests, successor identity/generation, durable reservation digest, update sequence/time. `AccountRecoveryRecord` fields: plan hash, one account ID, per-account approval digest/nonce, customer-binding digest, observation digest/time/expiry/source version, phase, opaque reason code, recovery marker, last durable epoch-record digest. The reservation has no automatic timeout that hands write authority to a different operation; process loss pauses and requires exact-operation resume or reviewed abort. No customer IDs, token values, raw paths, credentials, or broad provider responses.

`Begin` first revalidates `CurrentWriter` and requires exact plan identity `W/G/store`. It atomically consumes the plan approval nonce, calls the journal-owner `ReserveRecoveryEpoch` CAS, and writes the first durable epoch record before any stop/quiesce effect. It rejects another active epoch, changed plan/active writer, expired approval, consumed nonce, already-serving conflicting writer, missing pinned source lease, or disallowed state. Same plan hash resumes its exact operation ID/nonce; no caller-supplied operation ID can change it. The authoritative reservation stays held until commit or a reviewed terminal abort; a local file lock/lease never proves provider fencing or releases generation authority. Process loss pauses; the exact operation resumes after revalidating persisted provider evidence.

Ordered epoch phases, each persisted before the next external effect:

1. `EPOCH_RESERVED`: bind plan/source/approval, `W/G/store`, historical seal `H`, snapshot cut `C`, and operation ID; reserve the shared journal authority against any competing epoch or writer start.
2. `SERVICE_QUIESCED`: through the reviewed supervisor, quiesce the exact service bound to `W/G/store`, stop admission, and verify that identity is no longer writing. No app-local mutex or process-local claim suffices.
3. `OLD_WRITER_STOPPED`: stop exact writer `W/G` through control plane and independently verify immutable provider evidence. Any identity drift from Plan aborts before stop.
4. `OLD_CREDENTIALS_REVOKED`: enumerate every credential and issued session for `W`, revoke/deny all, independently verify complete inventory/effective denial, and persist inventory digest plus immutable references. Unknown classes block.
5. `OLD_GENERATION_SEALED`: after stop and revocation only, seal `G` after `C`; independently verify `ReadThrough(zero)` complete through this post-fence seal `S`. Persist `H`, `C`, `G`, `S`, and `G+1` as distinct values. If seal/history does not contain `C`, halt.
6. `RESTORE_STAGED_FROZEN`: using only the pinned verified lease, restore to a fresh private same-volume destination. Existing `backup.Restore` changes restored accounts to `restore_pending` with `plan_id=free` and subscriptions to `restore_pending`; preserve that fail-closed result. Replay every journal intent from zero through `S`, so deleted/deleting accounts stay unavailable and cannot be revived by snapshot state. Verify schemas/inventory and keep the service quiesced. The private destination is the actual freeze boundary; nothing is published or served.
7. `ACCOUNTS_APPLIED_STAGED`: for each eligible plan account, verify exactly one account approval; reread the sealed snapshot candidate and obtain a fresh provider projection; require customer-binding digest/source version to match that account's plan envelope, current observation age within the configured maximum and before its expiry, and current eligibility to remain true; pass one `contracts.RecoveryApplyRequest` to the existing single-account applier against the private staging DB; require `RecoveryApplyResult.Consistent`. The current billing observation can differ in mutable subscription details from the planning observation, but it must still prove eligibility under the same immutable account/customer binding and source version. Each call updates only that staged row. The live destination remains untouched and every partial staged result is unservable. Changed identity/truth or any account failure blocks the entire publication; retry reconstructs from the pinned bytes and replays exact idempotent records.
8. `SUCCESSOR_ADOPTED`: only after full-history reconciliation and every account decision succeeds, durably adopt the one successor identity at `G+1`, bound to `W/G`, `S`, journal store and operation. The generation reservation continues to block all writes until commit; adoption cannot start a service or writer.
9. `RESTORE_PUBLISHED`: atomically rename the complete staged destination into the configured live destination on the same filesystem. The rename publishes the full database as one unit; it never exposes a partial account set. Persist intended destination identity before rename and verify it after crash. Service remains stopped and admission denied.
10. `EPOCH_COMMITTED`: verify whole-destination identity, all per-account apply records, journal store and adoption; atomically commit the terminal epoch and release the journal reservation only to the adopted successor. A crash after publication but before commit keeps service stopped; exact-operation retry verifies the published destination and completes commit or fails closed. Only committed state permits `RecoveryAdmission` and service start.

The concrete `FROZEN` target is the private restored destination with every surviving account `restore_pending`/Free and every subscription `restore_pending`, plus journal deletions replayed through `S`; old service remains quiesced. Per-account applier changes happen only in that private destination. Before atomic publication, any failed/canceled attempt leaves the live destination unchanged and no staged row service-visible. After publication but before epoch commit, rows may contain individually applied active state, but service admission remains durably denied and no writer may start. This exact state is resumable; never auto-roll back by re-enabling the old writer. A crash around rename is resolved by comparing the persisted intended destination identity and exact manifest/restore digests. A crash around commit is resolved from the terminal epoch record; never assume timeout means rollback.

`Coordinator.Apply` implements `contracts.RecoveryApplier.Apply` and accepts exactly one `(plan_hash, account_id, account_approval_ref)` from the additive request field. It can only update that account's private staged row/record while its epoch is active. It cannot stop writers, seal, publish, adopt, or admit startup. Every call rechecks one distinct account approval, nonce, and allowlist membership; success cannot activate another account. No public `apply-all` endpoint. Whole-destination publication occurs only after all account-specific calls pass.

Continue to call `RecoveryApplyResult.Consistent(plan, req)` before accepting an account result. `FenceReceipt.SufficientFor` remains the existing shape/binding check; its booleans and timestamp are compatibility summary only. The epoch record separately persists and admission re-fetches the immutable stop, all-item revocation, and post-watermark seal references plus independently verified successor adoption. No field in the frozen result substitutes for those references.

The `internal/cli/hosted_recovery.go` entrypoint sequence is `plan --snapshot --expected-manifest-sha256 --approval-ref` (read-only provider planning; returns plan hash), `begin --plan-hash` (reserves global epoch), repeated `apply --plan-hash --account-id --approval-ref` (one account per invocation), `commit --plan-hash` (requires all account records, publication and adoption), and `status --plan-hash` (redacted persisted phase). Every command returns nonzero on incomplete evidence. `begin/apply/commit` are resumable only under the persisted plan operation/approval bindings; caller flags cannot override account scope, generation, provider identity, or operation ID.

## Production admission and AWS boundary

Request `service.RecoveryAdmission` construction as `recovery.NewAdmission(epochStore, activeWriterIdentitySource, journalIdentityVerifier, buildSHA)`; it uses the package-owned evidence verifier and cannot accept one from service config or CLI. Runtime identity must come from an owner-reviewed local supervisor/control-plane channel with peer authentication; no static environment generation, caller value, or app IMDS. `Admit(ctx, journal)` accepts only `EPOCH_COMMITTED`, re-verifies stop, complete revocation, seal, publication, adoption, approval/plan/source provenance, all account records, and active successor identity, then checks the supplied journal is the adopted stable store. It returns the snapshot cut only after verified history reads establish the same sealed boundary; existing service assembly then retains its current full-history and intent checks. The same configured journal object/store must be wired into service, backup, fence, and admission. Any absent/stale evidence, mismatched store/build, unresolved account, or active competing epoch refuses startup.

Preserve the hosted app's current IMDS denial. No instance profile, broad host credentials, default AWS credential chain, or app authority to stop instances/revoke credentials/adopt generations. A separately provisioned control-plane executable uses explicit scoped role credentials and the AWS CLI with fixed argument arrays, explicit region/profile, bounded context, scrubbed environment, and captured request IDs. For EC2 stop it verifies exact account/region/instance identity with `aws ec2 describe-instances`, calls `aws ec2 stop-instances`, waits with `aws ec2 wait instance-stopped`, then re-describes and persists immutable evidence references. Revocation must enumerate actual deployment credential/session classes at their owning authorities, revoke or deny each, and independently verify denial. EC2 stop does not establish revocation. The actual identity model, AWS role, policy scope, and credential classes remain owner inputs; no invented permissions or metadata key are proposed. The verifier retrieves provider evidence and checks exact subject/scope/version/status; CLI output or fake output alone is not evidence.

## Bounds and deterministic fake-only RED controls

Use these proposed hard ceilings (integrator owners may lower them; raising one requires a reviewed policy update): at most 100 accounts per recovery plan; 10,000 snapshot accounts/brains at the inspector boundary; 64 checkout refs per account; 4 concurrent provider calls; 4,096 provider GETs per plan; each provider timeout at most 30 seconds, response at most 1 MiB, total response bytes at most 64 MiB; evidence record at most 1 MiB; canonical plan/epoch record at most 4 MiB; each operation invocation at most 30 minutes before returning resumable state; provider observation age at most 5 minutes at account Apply; snapshot declared bytes capped by explicit SSD-backed configuration no greater than the inspector's 1 TiB absolute limit. Existing billing observer parser ceilings remain in force (JSON depth 32, nodes 100,000, event pages 32, invoices 16, invoice-line pages 4); aggregate provider GETs still obey the plan-wide cap. No default path or filesystem temp fallback. The external volume must enforce actual capacity; a configured byte ceiling alone is not a hard physical quota. Never truncate or partially accept work. Sort all IDs/references before hashing and writes. Per-provider/CLI calls have context deadlines and cancellation. No partial success is returned.

Fake-only tests must prove: raw-byte identity including whitespace changes; digest mismatch causes zero provider calls; unapproved and reordered/replayed account scope makes zero calls for out-of-scope IDs and changes/rejects the bound plan; nonce cannot replay across operation/plan/account while exact tuple retry resumes; every hashed provenance/evidence field affects canonical plan hash; strict decoder rejects duplicate/unknown/trailing JSON; lease restart verifies pinned content and never reopens source; tampered/replaced scratch is rejected; account/customer conflict or missing authoritative identity blocks before provider call; deleted/deleting snapshot account causes zero provider calls; snapshot/current checkout mismatch, added/disappeared/refread change, pending/unknown/unqueryable checkout, provider timeout, stale results never yield eligibility; planning does not call Seal and succeeds with a valid unsealed active `G` prefix through cut `C` while recording last seal `H` separately; active writer change between Plan and Begin blocks; stop/revocation failure prevents seal; seal/full-history mismatch prevents restore; staged restore uses only verified lease, lands live rows at `restore_pending`/Free with intents replayed, and never publishes partial data; account A apply does not alter account B's status/record; concurrent Begin and Apply calls serialize; crash before/after each persisted transition is idempotent; no `G+1` writer starts before full destination publication and committed epoch; configured `G+1`, empty prefix, fake provider success, signed approval/hash, copied lock/PID, or booleans alone never pass admission; IMDS/default credential access remains denied. These tests establish local state-machine behavior only, not live provider, IAM, stop/revocation, durable storage, or activation qualification.

## Current blockers and handoff boundary

Locally implementable after review: the recovery owner can add the coordinator/state records, exact-scope request validation, staged-lease consumer, per-account state, CLI entrypoints, and fake-only tests behind injected interfaces. The backup owner must first define and review the pinned verified-snapshot lease. Billing owner must establish an authoritative account/customer identity relation and review `ObserveSnapshotCandidate`; until it exists, affected candidates fail closed. Journal owner must provide verified current-writer/ancestry plus durable seal/adoption capability. Service owner must wire supervisor quiescence, shared journal identity, atomic full-destination publication, and committed-epoch admission. A separate control-plane owner must define actual credential/session inventory and scoped AWS authority. Do not report production integration clear until these owner patches are reviewed and landed.

Live-only gates remain: exact operator-approved source and account scope; fresh real provider observations; authoritative old-writer identity and stop; complete and effective revocation of every credential/session; post-fence durable journal seal; whole private restore and atomic publication; durable successor adoption; production shared journal wiring and scoped write credentials; physical storage limits/durability; and operator-controlled activation. No provider action, deployment, purge, spend, or activation is part of this proposal.
