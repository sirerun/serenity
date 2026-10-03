# Authenticated genesis and ordinary startup factory — proposed contract

**Status:** proposal only, based on source at `db7625cfb6e29e3c7c42531c004d22d72af31c22`. No production factory, trusted credential issuer, provider authority, or startup readiness is established by this document. Independent review and an explicit root seam freeze must precede any source assignment.

## Decision and scope

Hosted startup needs two authenticated paths that are distinct from off-host restore:

1. **Genesis** consumes a single-use, authenticated first-issuance authorization bound to this deployment and exact journal namespace. Durable authority state records the issuance before writable journal credentials or serving are possible. An empty journal is consistent with genesis only when that durable issuance exists; emptiness never establishes genesis.
2. **Ordinary restart** authenticates the existing durable genesis record, proves the former writer can no longer write, verifies the complete hash-linked journal ancestry, and issues a new uniquely identified writer in the already-authorized active generation. The active generation may end in an unsealed tail. A previously sealed generation may be resumed only by a separately authorized rotation to its immediate successor, linked to that seal.

Neither path accepts a restored snapshot, recovery plan, caller-selected watermark, empty directory, numeric generation zero, `dev`/boolean mode, or an old recovery COMMITTED result as startup authority. Off-host recovery remains the existing, separately owned `RecoveryAdmission` flow: it authenticates the snapshot watermark and old-writer fence, adopts a successor, and requires complete reads to terminate at the agreed seal before restored accounts can be admitted. It must not be widened to mean ordinary restart.

## What current source establishes

- `internal/hosted/service/service.go` defines `LifecycleDependencies{Journal, BuildSHA, Recovery}`. `NewWithDependencies` validates nonnil dependencies and a nonempty build string; `AssembleWithDependencies` invokes `Recovery.Admit`, verifies the cut and full-history reads, and rejects unless both reads are sealed at the same watermark. Legacy constructors fail closed. This is a recovery-shaped seam, not an authenticated deployment factory.
- `internal/hosted/contracts/deletion.go` defines the zero watermark as a cursor before generation 1, `DeletionJournal` (`AppendDeletion`, `ReadThrough`, `Seal`), and `DeletionRead{Entries, To, Sealed}`. Stored `JournalObject` has `WriterID`, but returned entries/read results do not expose writer provenance for issuer reconciliation.
- `internal/hosted/deletion/journal.go` recursively requires a preceding generation seal, verifies contiguous coordinates and hash links, rejects objects after a seal, and detects a tail changing during a read. `ReadThrough(zero)` can return a verified, unsealed active head. Its own comment says that an empty configured successor has no stored generation-start object and returns only the prior watermark, so activation must bind the adopted generation separately. `NewJournal`/`ProductionJournal` simply accept a store, writer string, generation and clock; they do not authenticate any of them. Conditional append protects a position, but does not by itself prevent two authorized live writers from successively appending different positions.
- `internal/hosted/service/lifecycle_deps_internal_test.go` and `lifecycle_deps_external_test.go` explicitly construct test generation 1, seal it, then construct generation 2. These fixtures prove mechanics only. `internal/cli/hosted.go` still returns `ErrStartupUnavailable` for `hosted serve`.
- `docs/plans/t23-48-journal-backup-assembly-contract-2026-10-01.md` explicitly says the production factory/admission mechanism is absent. `docs/launch/hosted-completion/interfaces.md` keeps the old-writer fence and complete-history proof as activation requirements. `internal/hosted/contracts/fence.go` has a recovery-plan `FenceReceipt`; it is bound to a recovery plan and is not a generic restart grant.

The resulting source gap is material: ordinary restart with a legitimate unsealed journal head is currently rejected by service assembly, and current returned journal data cannot prove which durable writer issuances authored the history. Do not relax the sealed check for every caller or infer provenance from a nonempty `WriterID`.

## Proposed construction seam

Place the production coordinator in a new `internal/hosted/startup` package. Only the trusted binary composition root may construct it, from deployment and release identities, the authenticated authority adapter, a constructor-bound runtime identity source, and attested build identity. The factory owns the only production path from those inputs to `service.LifecycleDependencies`; callers cannot supply a `DeletionJournal`, build string, writer ID, generation, or `RecoveryAdmission` as a substitute for authorization.

The trusted composition root binds a platform identity source when constructing the factory. `Restart` obtains its signed invocation from that source; callers cannot provide a flag, environment string, or alternate invocation:

```go
type RuntimeIdentitySource interface {
    CurrentInvocation(ctx context.Context) (SignedRuntimeInvocation, error)
}

func NewFactory(binding Binding, authority Authority, runtime RuntimeIdentitySource,
    build AttestedBuildIdentity) (*Factory, error)

func (f *Factory) Genesis(ctx context.Context, permit SignedGenesisPermit) (*service.Service, error)
func (f *Factory) Restart(ctx context.Context) (*service.Service, error)
```

`RuntimeIdentitySource` is constructor-bound only by the trusted binary composition root. It validates the provider's authenticated runtime channel and returns an untrusted-to-the-factory signed envelope bound to issuer, audience/deployment, namespace, unique instance and attempt IDs, expiry and nonce. `Restart` calls it with the startup context before journal I/O or authority preparation; source failure/cancellation fails startup. `Authority` verifies the envelope and consumes the attempt idempotently. Ambient environment, CLI/config values, machine names, and SQL rows are never runtime identity. `AttestedBuildIdentity` is likewise an explicit trusted constructor dependency. The `Prepared` and `WriterLease` implementations are the sole routes to scoped read-only and writable object clients; no SDK default credential chain is allowed.

`Genesis` requires authority to validate and durably consume a signed, expiring, audience-bound permit. `Restart` requires an already consumed genesis record for this exact deployment and journal namespace. A user flag or config value may select no authority path. A future recovery command remains with the recovery owner and cannot call either method with a recovery plan as a substitute.

The minimum proposed ports are shown below. New object/journal reader, position, observation, and writer-use types belong in `internal/hosted/contracts/deletion.go`; runtime identity, authority, reservation, lease, and inventory types belong in the new startup package. The service package owns its separate `StartupProof`/`StartupLease` consumption interfaces.

```go
type SignedGenesisPermit []byte // untrusted signed envelope; Authority must verify it
type SignedRuntimeInvocation []byte // untrusted platform identity envelope; Authority must verify it

type Binding struct {
    DeploymentID string // stable authority-owned service identity
    NamespaceID  string // stable provider-issued identity for the exact journal namespace
}

// Read capabilities intentionally exclude PutIfAbsent.
type JournalObjectReader interface {
    Get(ctx context.Context, key string) (body []byte, found bool, err error)
    ListAfter(ctx context.Context, prefix, startAfter string, limit int) (keys []string, more bool, err error)
}

type JournalObjectStore interface {
    JournalObjectReader
    PutIfAbsent(ctx context.Context, key string, body []byte) (created bool, err error)
}

type JournalWriterUse struct {
    Generation int64
    WriterID   string // globally unique issuance ID across all deployments/namespaces
}

type JournalPosition struct {
    ActiveGeneration       int64             // strictly positive, even when this generation has no objects
    LastObjectInGeneration DeletionWatermark // zero only for an empty active generation
    PredecessorSeal        DeletionWatermark // exact G-1 seal for G > 1
    GenesisIssuanceID      string            // required for G == 1
    SuccessorAllocationID  string            // required for G > 1
}

type JournalObservation struct {
    Read          DeletionRead
    Position      JournalPosition
    HistorySHA256 string // ordered hash of every canonical entry and seal from generation 1
    WriterUses    []JournalWriterUse // unique, first-object-use order; entries and seals included
}

type DeletionJournalReader interface {
    Observe(ctx context.Context) (JournalObservation, error) // read-only; no append/seal/write methods
}

type AttestedBuildIdentity interface {
    BuildID(ctx context.Context) (string, error) // immutable release identity from trusted binary metadata
}

type MutationRoute struct {
    PrincipalID       string
    CredentialClasses []string
    Issuers           []string
    DelegationChain   []string
    MutationActions   []string // object writes/deletes plus policy/credential delegation changes
    ResourceScope     string
    RenewalPolicy     string
    EnforcementPoint  string
    PolicyVersion     string
}

type MutationInventory struct {
    NamespaceID string
    Digest      string // canonical digest of the complete, authority-observed route set
    PolicyEpoch string // provider-enforced epoch while all non-lease mutation is denied
    Routes      []MutationRoute // complete and canonically sorted; no caller completeness flag
}

type Authority interface {
    PrepareGenesis(ctx context.Context, binding Binding, buildID string, permit SignedGenesisPermit) (Prepared, error)
    PrepareRestart(ctx context.Context, binding Binding, buildID string, invocation SignedRuntimeInvocation) (Prepared, error)
}

type Prepared interface {
    MutationInventory() MutationInventory // immutable provider-observed inventory after deny-policy readback
    ReadOnlyJournal(ctx context.Context) (contracts.DeletionJournalReader, error)
    VerifyHistoricalWriters(ctx context.Context, uses []contracts.JournalWriterUse) error
    ReserveHead(ctx context.Context, observed contracts.JournalObservation) (HeadReservation, error)
    Abort(ctx context.Context) error
}

type HeadReservation interface {
    Position() contracts.JournalPosition // opaque authority reservation's exact logical head
    VerifyObservation(ctx context.Context, observed contracts.JournalObservation) error
    ActivateWriter(ctx context.Context, observed contracts.JournalObservation) (WriterLease, error)
    Abort(ctx context.Context) error
}

type WriterLease interface {
    JournalStore() contracts.JournalObjectStore // writable only after head reservation activation
    WriterID() string                            // globally unique durable issuance identity
    Position() contracts.JournalPosition         // binds generation and first append predecessor
    Close(ctx context.Context) error             // revoke; errors preserve unknown state
}

```

These are proposed contracts, not current types. `Prepared` is an opaque, authority-created reservation. Its read-only journal is built over `JournalObjectReader`, whose only methods are `Get` and `ListAfter`; it cannot append, seal, or call `PutIfAbsent`. The service's staged admission receives `DeletionJournalReader`, never the writable `DeletionJournal`. Only after `HeadReservation.ActivateWriter` succeeds may `WriterLease.JournalStore` expose `JournalObjectStore` for construction of the writable `DeletionJournal`. No exported constructor accepts raw credential strings, caller booleans, local directory observations, or arbitrary stores. `WriterLease` wraps an explicitly authenticated, narrowly scoped object-store client; it must not use ambient/default cloud credentials. The factory calls its constructor-bound `AttestedBuildIdentity` on every startup attempt, validates the returned immutable release digest in the same form required by service `BuildSHA`, and passes it to Authority. It cannot be supplied or overridden through `Binding`, CLI, or config. `NamespaceID` is resolved from trusted provider configuration and cannot be a caller alias whose backing bucket/prefix can change after authorization.

The signed genesis permit contains a globally unique nonce; the authority durably binds that nonce and its complete signed-payload digest to exactly one genesis operation ID and namespace. Re-presenting the identical permit resolves that same record after a timeout or process crash; re-signing or reusing the nonce for another payload is rejected. Restart receives a signed platform invocation containing globally unique instance/attempt IDs; the authority binds its digest to one reservation and uses its namespace-level single-writer CAS to resolve an interrupted prior attempt. Neither identity comes from CLI/config input. Ambiguous responses are reconciled against those durable authority records, never by assuming a failed call had no effect or minting a second genesis. `Prepared` represents the durable record and reservation returned by that resolution. All methods take the startup context, reject nil/already-cancelled contexts before I/O, and honor cancellation across provider, journal, and service assembly work.

### Genesis authority and replay protection

The raw `SignedGenesisPermit` envelope must be authenticated by a trust root configured in the binary/runtime authority, with issuer, audience (`DeploymentID`), exact `NamespaceID`, generation 1, expiry, nonce, and request digest. The signing key source and clock are trusted injected dependencies with bounded validity; they do not come from request headers, journal objects, SQL, or the snapshot. Likewise `SignedRuntimeInvocation` must be authenticated against the configured workload-identity/control-plane trust source; strings copied from CLI, environment, or SQL are not attestations. The authority performs one serializable create-if-absent transition from `permit nonce unused` to a durable `GenesisPending(operationID, binding, permit digest, generation=1, globally unique writer issuance)`. The operation ID is authority-assigned by that nonce record and can only be resumed with the identical verified envelope and binding after a timeout or process crash. Reuse for another namespace, build, deployment, attempt, or digest is a hard replay refusal.

The authority record is the evidence that a first issuance happened; a filesystem/object listing that happens to be empty is never evidence. Before `ActivateWriter` returns, the durable record must identify the one genesis operation, stable namespace, generation, unique writer issuance and current authority state. The durable authority must expose read-after-ambiguous-write reconciliation. If durable issuance cannot be proven, startup remains unavailable. This protocol does not claim that an AWS/provider identity store or its durability semantics are currently implemented or qualified.

### Ordinary restart, fencing, and journal proof

Before returning a read-capable `Prepared`, `PrepareRestart` must acquire a durable exclusive namespace reservation and enforce a provider-side write-deny fence over the exact immutable namespace. The reservation is held while history is read, verified, activated, replayed by service, and until the new active writer lease is established; it cannot expire into an unlocked namespace on a timeout. The trusted authority must have an owner-reviewed, exhaustive `MutationInventory` for every principal, credential class, and control path able to mutate the journal or grant mutation ability. At minimum it inventories and closes:

- every previously issued journal writer principal and session, including assumed-role/workload sessions, nested role sessions, refresh/renewal paths and in-flight credentials;
- host/workload base credentials, instance profiles, metadata-service credentials, environment/config credential providers, and lateral roles able to assume a writer role (ambient/default SDK chains are disabled);
- alternate application/service principals, operators, break-glass or emergency principals, CI/automation, replication and maintenance identities, and broad wildcard/account/organization principals or token issuers with direct or delegable namespace mutation;
- bucket/account/access-point/resource policies, ACLs, version/delete-marker permissions, lifecycle/replication controls, object lock controls, and administrative permissions that can create a new writer, replace history, bypass conditional creation, delete versions, or alter the inventory/fence itself.

The inventory records principal/role identity, every credential and delegation route, allowed mutation verbs and exact resource scope, renewal/maximum lifetime, enforcement point and policy version. The owner must identify how every route is either revoked or denied and how policy prevents bypass during reservation and after activation. If the complete inventory, deny policy, or read-after-change confirmation is unavailable, no read-capable `Prepared` is returned. A local timeout, only revoking the last observed `WriterID`, or an `ErrDeletionJournalFenced` collision is insufficient. The trusted authority record also identifies the one genesis issuance, namespace, positive active generation, authorized writer issuance history, and current authority epoch.

Only a `JournalObjectReader` is available during this phase. Under the namespace-wide write-deny reservation, read the complete history from generation 1 and compute an observation that binds every canonical object hash, exact `WriterUses`, and logical `JournalPosition`. The list/get implementation must provide a consistent complete observation for this frozen namespace; if provider consistency cannot establish it, refuse startup. The observation is the `JournalObservation` defined in the proposed contracts block above. `Prepared.ReadOnlyJournal` is constructed from the authority-selected logical generation/allocation record; its `Observe` result reports that bound position while independently verifying the visible object chain. Active generation is never inferred solely from the last visible watermark.

For generation 1, the authority-backed genesis issuance ID is required even if the journal has no objects. For generation G>1, the authority-backed successor allocation ID is required and binds exactly G to the seal hash of G-1; `PredecessorSeal` must name that same seal. If G is empty, `LastObjectInGeneration` is zero while `ActiveGeneration=G` and `PredecessorSeal` remains the prior nonzero seal. `Read.To` is then expected to equal that predecessor seal, which is only the last visible object, not the active generation or its authority. This explicitly represents an authenticated empty immediate successor without inventing generation zero. For a nonempty active generation, `LastObjectInGeneration.Generation` must equal `ActiveGeneration` and `Read.To` must equal it. A visible later generation without its exact predecessor seal and allocation is corruption.

`Prepared.ReserveHead(observed)` persists an opaque reservation ID, namespace, active generation, last-object/predecessor-seal or genesis/allocation identity, history digest, writer-use digest, inventory digest and enforced policy epoch. It succeeds only while the same exclusive reservation and write-deny fence are held. `HeadReservation.ActivateWriter(ctx, observed)` atomically CASes that exact authority reservation and observation into one globally unique active writer epoch; it refuses any changed generation, seal, head, history/writer-use digest, namespace, inventory, or policy epoch. The write-deny policy is then changed so this exact new epoch is the only principal with journal mutation. This ordering makes the immutable head itself the cross-store reservation: no writer can append between the proof and authority CAS.

The activated `WriterLease.Position()` carries the reserved positive generation and exact first-append predecessor. The journal owner must provide a constructor such as `deletion.NewJournalAt(store, writerID, position)` that validates this position and initializes the append cursor from it; it cannot call the current lazy `NewJournal` path and silently reload/advance to an unreserved head. Its first append must conditionally create exactly the next key with the reserved previous hash (or sequence 1 linked to the predecessor seal for an empty successor), and fail closed if that key/head no longer matches. The exclusive active-writer epoch remains in force through full service replay and handler publication, and the same lease scopes every subsequent append. Do not seal an unsealed generation merely to make restart pass; sealing is an irreversible transition owned by an explicit rotation protocol.

The contracts owner should freeze these declarations as the additive read-only observation shape. `DeletionJournalReader.Observe` returns one full-history `JournalObservation`; it never exposes `PutIfAbsent`, `AppendDeletion`, or `Seal`. `WriterUses` is the unique set in first-object-use order over every verified entry AND seal. Repeated use of one issuance is represented once; the globally unique writer ID may never map to another generation, namespace, or issuance. Every nonempty history must produce a complete set; a partial read cannot be certified as full history. `internal/hosted/deletion` owns extracting writer uses from every verified stored object (including seals) and computing the complete history digest; `internal/hosted/contracts` owns the read-only interfaces and exact semantics. The authority reconciles every returned writer ID against one immutable durable issuance record bound to exact deployment, namespace, generation, and credential principal. `WriterID` values are globally unique across deployments and never reused, including after revocation; repeated objects by the same writer collapse to its one first-use record. Missing/incomplete read provenance, empty ID, a repeated ID with a conflicting generation/namespace/issuance, or authority-unknown writer use fails closed. This mapping is complete before writer activation, not inferred from SQL or arbitrary `WriterID` text.
## Service shared-seam request

Preserve `RecoveryAdmission` and its sealed same-boundary checks unchanged. Request the service owner to add a distinct authenticated `StartupProof`, read-only `JournalReader`, and `StartupLease` tuple to `LifecycleDependencies` (or an equivalently scoped constructor), mutually exclusive with off-host `RecoveryAdmission`; incomplete or mixed dependency sets return `ErrStartupUnavailable`. Do not add an exported `bool` such as `isRecovery`/`isGenesis` to control validation. The admission itself carries a complete expected journal head, not a caller-supplied watermark or mode flag.

For precision, the additive service contract should have this shape (names remain subject to owner review):

```go
type StartupProof interface {
    Position() contracts.JournalPosition
    VerifyObservation(ctx context.Context, observed contracts.JournalObservation) error
}

type StartupLease interface {
    Close(ctx context.Context) error // revoke exact writer authority after service work is joined
}

// LifecycleDependencies adds JournalReader contracts.DeletionJournalReader,
// Startup StartupProof, and Lease StartupLease. validate requires either the
// existing Recovery path OR the complete reader/proof/lease tuple, never both.
```

Before lease activation, staged admission accepts only `DeletionJournalReader`; it cannot append or seal. After activation the factory creates a writer journal from the exact lease and passes both the read-only reader/reservation and that writer journal to private service assembly. The service independently obtains a full observation through the read-only interface, recomputes `Position`, complete history digest, and all entry+seal writer uses, and calls the reservation's narrow `VerifyObservation` proof before replay. The reservation includes the positive active generation and either the genesis issuance or exact successor allocation/predecessor seal; therefore an empty authorized successor is distinct from a zero-generation journal. The exclusive writer epoch remains held while service replays deletion intents, performs provisioning recovery, checks cancellation, and publishes handlers/workers. Recovery retains its existing two sealed reads and snapshot watermark rules. Gateway and backup receive the exact same post-activation writer journal instance. The service owner should add `CloseContext(ctx)` (with the existing `Close()` delegating under a bounded shutdown policy) so it joins request/workers and then closes the exact `StartupLease`; cleanup errors must be returned and an uncertain revocation must leave the startup reservation unresolved.

This amendment affects `internal/hosted/service/service.go`, `internal/hosted/contracts/deletion.go`, and `internal/hosted/deletion/journal.go`; those files are not owned by this proposal lane. The service, contracts, and journal owners must accept the exact observation/reservation/first-append seam and ownership boundary before an implementation task is frozen. Existing test-only filesystem journals stay explicit test fixtures, never production fallback.

## Cancellation, crashes, and shutdown

| Boundary | Required result |
|---|---|
| Before authenticated permit/reservation | No journal credential or service side effect; return cancellation/authentication error. |
| Permit consumed, before journal read or writer activation | Durable `GenesisPending` is retained. Retry resolves the same authority-assigned operation ID and digest; another permit use cannot issue a second genesis. If state cannot be reconciled, stay unavailable. |
| Old-writer stop/revocation ambiguous | Do not read/activate a new writer or serve. Authority keeps the namespace fenced pending reconciliation. Never infer success from timeout. |
| During read-only `Observe` / historical-writer verification | Propagate context/backend error; issue no write credential. Incomplete chain, concurrent tail change, wrong writer, or namespace mismatch remains unavailable. |
| Writer activation response lost | Resolve the exact authority-assigned operation/attempt ID and expected head from the durable authority record. Reuse only the same lease or revoke it; never create a second active epoch on blind retry. |
| After lease activation, before service assembly/publication | On cancellation/error close constructed resources and revoke the exact staged lease. If revocation outcome is unknown, mark startup unresolved and block another writer until authority reconciliation. No listener/worker is published. |
| Graceful stop | Stop accepting work; cancel and join service workers; close Service; revoke its exact writer lease and verify. Ordinary restart leaves the generation unsealed unless an explicit serialized rotation owns the seal-and-successor transaction. |

No cleanup error may be discarded if it could leave credentials live. `Service.Close` today has no journal lease. The requested service-owned `StartupLease` field and `CloseContext` path must compose lease revocation after work is joined; do not rely on an outer wrapper or silently assume the existing `Close` revokes authority. Shutdown uses a fresh bounded cleanup context, not the already-cancelled serving context.

## Hostile controls required before source qualification

Each implementation test must show a genuine pre-fix RED with its exact old-source result captured before correction, then restore the fixture and show the focused corrected check pass. Fakes demonstrate protocol behavior only; they do not establish provider stop, revocation, namespace binding, or durable authority semantics.

1. Genesis with valid signed permit succeeds once; exact retry resumes the same authority-assigned operation ID. Restart obtains a signed invocation only from the constructor-bound runtime identity source and proves wrong issuer, audience, namespace, expired attempt, replayed attempt and cancellation fail before journal I/O. Replayed nonce with a different namespace, deployment, build, or digest is rejected. Missing, expired, wrong-audience, wrong-namespace, malformed, or wrong-signature permits cause zero write credentials and no service construction.
2. Empty journal with no durable genesis record is rejected. Empty journal after durable authorized genesis can resume that exact issuance after a simulated crash. An arbitrary nonempty directory cannot substitute for either record.
3. Ordinary restart on generation 1 with a real unsealed tail succeeds after the complete mutation inventory is fenced and preserves every entry/head hash; current `AssembleWithDependencies` must be recorded rejecting this unsealed shape today. A sealed generation with no persisted immediate-successor allocation cannot be reopened for writes. A genuinely empty G1 requires genesis issuance; an empty G+1 requires the exact prior seal, durable allocation ID, positive active generation, frozen-prefix proof, and a first append linked to that seal. It must never be represented as generation zero.
4. Wrong prior seal, missing middle object, malformed coordinates, unauthorized historical `WriterID`, missing entry/seal writer use, same writer ID mapped to multiple generations/issuances, conflicting writer-use mapping, unsealed earlier generation, post-seal tail, unexpected later generation, mutation despite the deny barrier, changed inventory/policy epoch, and a tail appended during proof/activation/first append all deny admission. Use real `Journal` mechanics for journal controls and an independent authority fake for issuer decisions.
5. Two concurrent prepares for one namespace yield one durable reservation and one active writer epoch. A stale prior credential and every enumerated alternate principal/role/session/default or metadata credential, break-glass/admin route, and mutation-capable policy/automation path are denied during the fence and after activation; test the actual access-policy gate, not only a same-sequence `PutIfAbsent` conflict. Incomplete inventory or an omitted credential class must refuse reservation.
6. Crash/retry barriers after nonce consumption, mutation inventory/fence, history observation, head reservation, activation, first append, service replay, and immediately before handler publication preserve one issuance and exact positive generation/head; never expose a service with unresolved authority. Include an authorized empty G+1 fixture whose last visible watermark is its predecessor seal.
7. Cancellation before I/O, during authority call, during each journal read page, between proof and activation, and after activation closes/revokes or leaves a durable unresolved fence. Assert no handler/worker publication and no credential reuse.
8. Type-level compile controls prove the prepared/read-only/staged path has no `PutIfAbsent`, `AppendDeletion`, or `Seal`; only an activated `WriterLease` creates the full writer journal. Recovery controls remain unchanged: a recovery COMMITTED record for a different snapshot/namespace cannot authorize genesis or restart; recovery with a valid plan still requires its pinned watermark, fenced old writer, successor and sealed complete boundary.

Current source already has mechanics controls for journal gaps, hash mismatch, generation gaps, post-seal tails and startup cancellation in `internal/hosted/contracts/contractstest/journal_suite.go`, `internal/hosted/deletion/journal_seal_resume_test.go`, and `internal/hosted/service/startup_cancellation_regression_test.go`. Those are useful foundations, not proof of the new authority path. No tests were run for this document-only assignment.

## Ownership and dependency-linked delivery gates

1. **Contract review — this lane:** review only this proposed document against main `db7625cf`; no source changes.
2. **Seam freeze — root + service/contracts/journal owners:** independently review the constructor-bound `RuntimeIdentitySource`, disjoint read-only/writer capabilities, `JournalObservation`/positive `JournalPosition`, globally unique entry+seal `WriterUses`, complete mutation-inventory fence, exact-head reservation/first-append handshake, and service-owned close/revoke order; retain the current recovery contract and publish an accepted interface decision. Until this is complete, no implementation is authorized.
3. **Preflight — assigned startup owner:** prove a separate SSD worktree and canonical source claim, enumerate exact ownership and protected boundaries, confirm a real durable authority, complete credential/principal/access-path inventory, provider-enforced write-deny/epoch policy, consistent frozen-namespace observation, and credential-revocation mechanism exists; freeze test fixtures. Stop if authority is not available; do not replace it with local files or a boolean.
4. **Implementation:** new `internal/hosted/startup` factory/authority adapter and its tests are owned by one startup implementation lane. `internal/hosted/deletion` owns the read-only observation and writer-use projection plus `NewJournalAt`; `internal/hosted/contracts` owns its contract tests; `internal/hosted/service` owns the typed admission/replay seam; `internal/cli` owns wiring only after the factory contract is accepted. Shared files require their owners' explicit task/claim and integration review. No one worker owns overlapping shared files by implication.
5. **Verification:** focused journal/service/startup hostile controls first; then the prescribed Go race, vet, lint and supported compilation gates under a verified shared build lease and load limits. Add actual provider-authority qualification as a separately authorized gate; local fakes cannot satisfy it. Preserve failures and distinguish local source, CI, provider, live deployment, and user acceptance evidence.
6. **Independent review:** a reviewer in a separate clone reruns every decisive RED and corrected control against exact candidate source; review must check the trusted construction/runtime-identity path, complete mutation inventory and policy enforcement, disjoint read-only capabilities, logical empty-successor position, reserved-head/first-append handshake, concurrent epoch transition, replay durability, cancellation cleanup, and no recovery/genesis conflation.
7. **Merge:** root checks current feed/board/PR discussions and exact reviewed head, then separately authorizes normal expected-head merge. A review or coordinator hold blocks merge.
8. **Landed verification:** verify the merged tree equals the reviewed candidate and the full diff is empty; update the accepted plan/roadmap through their owners. A docs proposal merge does not qualify provider authority, deployment, production startup, deletion, backup/restore, or overall hosted readiness.

## Open evidence boundary

The repository has no production startup factory, durable genesis issuer, current-writer lease/revocation adapter, or provider-qualified journal credentials in the inspected source. Existing `ProductionJournal` is a constructor over an injected store, not evidence of production authority. Therefore this contract is code-ready direction only; all provider identity, bucket policy, credential lifetime/revocation, durable replay, physical startup behavior, and deployment claims remain unproven.
