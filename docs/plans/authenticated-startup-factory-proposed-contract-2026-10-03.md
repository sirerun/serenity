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
- `internal/hosted/deletion/journal.go` recursively requires a preceding generation seal, verifies contiguous coordinates and hash links, rejects objects after a seal, and detects a tail changing during a read. `ReadThrough(zero)` can return a verified, unsealed active head. `NewJournal`/`ProductionJournal` simply accept a store, writer string, generation and clock; they do not authenticate any of them. Conditional append protects a position, but does not by itself prevent two authorized live writers from successively appending different positions.
- `internal/hosted/service/lifecycle_deps_internal_test.go` and `lifecycle_deps_external_test.go` explicitly construct test generation 1, seal it, then construct generation 2. These fixtures prove mechanics only. `internal/cli/hosted.go` still returns `ErrStartupUnavailable` for `hosted serve`.
- `docs/plans/t23-48-journal-backup-assembly-contract-2026-10-01.md` explicitly says the production factory/admission mechanism is absent. `docs/launch/hosted-completion/interfaces.md` keeps the old-writer fence and complete-history proof as activation requirements. `internal/hosted/contracts/fence.go` has a recovery-plan `FenceReceipt`; it is bound to a recovery plan and is not a generic restart grant.

The resulting source gap is material: ordinary restart with a legitimate unsealed journal head is currently rejected by service assembly, and current returned journal data cannot prove which durable writer issuances authored the history. Do not relax the sealed check for every caller or infer provenance from a nonempty `WriterID`.

## Proposed construction seam

Place the production coordinator in a new `internal/hosted/startup` package. Only the trusted binary composition root may construct it, from the deployment identity, release identity, authenticated authority adapter, and explicit journal client provider. The factory owns the only production path from those inputs to `service.LifecycleDependencies`; callers cannot supply a `DeletionJournal`, build string, writer ID, generation, or `RecoveryAdmission` as a substitute for authorization.

Expose separate operations, never `Start(ctx, mode bool)` or a caller-supplied mode enum:

```go
func (f *Factory) Genesis(ctx context.Context, permit SignedGenesisPermit) (*service.Service, error)
func (f *Factory) Restart(ctx context.Context) (*service.Service, error)
```

`Genesis` requires authority to validate and durably consume a signed, expiring, audience-bound permit. `Restart` requires an already consumed genesis record for this exact deployment and journal namespace. A user flag or config value may select no authority path. A future recovery command remains with the recovery owner and cannot call either method with a recovery plan as a substitute.

The minimum proposed ports are:

```go
type SignedGenesisPermit []byte // untrusted signed envelope; Authority must verify it
type SignedRuntimeInvocation []byte // untrusted platform identity envelope; Authority must verify it

type Binding struct {
    DeploymentID string // stable authority-owned service identity
    NamespaceID  string // stable provider-issued identity for the exact journal namespace
    BuildID      string // attested release identity, not a config/CLI value
}

type Authority interface {
    PrepareGenesis(ctx context.Context, binding Binding, permit SignedGenesisPermit) (Prepared, error)
    PrepareRestart(ctx context.Context, binding Binding, invocation SignedRuntimeInvocation) (Prepared, error)
}

type Prepared interface {
    ReadOnlyJournal(ctx context.Context) (contracts.JournalObjectStore, error)
    VerifyHistoricalWriters(ctx context.Context, uses []contracts.JournalWriterUse) error
    ActivateWriter(ctx context.Context, head contracts.DeletionWatermark) (WriterLease, error)
    Abort(ctx context.Context) error
}

type WriterLease interface {
    JournalStore() contracts.JournalObjectStore // capability scoped to this namespace/generation
    WriterID() string                            // unique to this durable writer issuance
    Generation() int64
    Close(ctx context.Context) error             // stop authority + revoke; errors preserve unknown state
}
```

These are proposed contracts, not current types. `Prepared` is an opaque, authority-created reservation. Its implementation is the only trusted path to `WriterLease`; no exported constructor accepts raw credential strings, caller booleans, local directory observations, or arbitrary stores. `WriterLease` wraps an explicitly authenticated, narrowly scoped object-store client; it must not use ambient/default cloud credentials. `Binding.BuildID` comes from the signed/embedded release identity and must be a valid immutable release digest (the existing service field is named `BuildSHA`). `NamespaceID` is resolved from trusted provider configuration and cannot be a caller alias whose backing bucket/prefix can change after authorization.

The signed genesis permit contains a unique nonce; the authority durably binds that nonce and its complete signed-payload digest to exactly one genesis operation ID and namespace. Re-presenting the identical permit resolves that same record after a timeout or process crash; re-signing or reusing the nonce for another payload is rejected. Restart receives a signed platform invocation containing a unique instance/attempt identity; the authority binds its digest to one reservation and uses its namespace-level single-writer CAS to resolve an interrupted prior attempt. Neither ID comes from CLI/config input. Ambiguous responses are reconciled against those durable authority records, never by assuming a failed call had no effect or minting a second genesis. `Prepared` represents the durable record and reservation returned by that resolution. All methods take the startup context, reject nil/already-cancelled contexts before I/O, and honor cancellation across provider, journal, and service assembly work.

### Genesis authority and replay protection

The raw `SignedGenesisPermit` envelope must be authenticated by a trust root configured in the binary/runtime authority, with issuer, audience (`DeploymentID`), exact `NamespaceID`, generation 1, expiry, nonce, and request digest. The signing key source and clock are trusted injected dependencies with bounded validity; they do not come from request headers, journal objects, SQL, or the snapshot. Likewise `SignedRuntimeInvocation` must be authenticated against the configured workload-identity/control-plane trust source; strings copied from CLI, environment, or SQL are not attestations. The authority performs one serializable create-if-absent transition from `permit nonce unused` to a durable `GenesisPending(startupID, binding, permit digest, generation=1, writer issuance)`. Only the exact same ID and digest may resume that transition after a timeout or process crash. Reuse for another namespace, build, deployment, ID, or digest is a hard replay refusal.

The authority record is the evidence that a first issuance happened; a filesystem/object listing that happens to be empty is never evidence. Before `ActivateWriter` returns, the durable record must identify the one genesis operation, stable namespace, generation, unique writer issuance and current authority state. The durable authority must expose read-after-ambiguous-write reconciliation. If durable issuance cannot be proven, startup remains unavailable. This protocol does not claim that an AWS/provider identity store or its durability semantics are currently implemented or qualified.

### Ordinary restart, fencing, and journal proof

`PrepareRestart` must use a trusted control-plane identity for the prior process/writer and establish all of the following before returning a read-capable `Prepared`:

- a serialized startup reservation prevents another new process from being authorized concurrently;
- the prior process is stopped or otherwise unable to continue service work;
- every credential/session that can write this exact journal namespace for the prior writer is revoked or expired with provider-confirmed enforcement, including in-flight/renewable credentials; an elapsed local timeout or an `ErrDeletionJournalFenced` collision alone is insufficient;
- the authenticated authority record says which genesis issuance, namespace, active generation and writer IDs are legitimate;
- authority reads are bound to the immutable namespace identity, and every journal request uses the exact explicitly scoped identity.

With the read-only store, construct a read-only journal view and scan from `DeletionWatermark{}`. Require the read to prove a contiguous hash chain from generation 1, every earlier generation sealed, no tail beyond a seal, and one terminal head in the authority-selected active generation. For ordinary same-generation restart, that terminal head may be unsealed (including a genuinely empty generation 1 only when genesis authority exists). If the terminal generation is sealed, do not append there: require a durable rotation record binding its seal hash to exactly generation+1 before a new lease can be activated. A visible later generation without that predecessor seal is corruption. Do not seal an unsealed generation merely to make restart pass; sealing is an irreversible generation transition owned by an explicit rotation protocol.

Reconcile every historical `(generation, writerID)` against durable issuance records before writer activation. Current `DeletionRead` cannot do this, so request an additive contract field and reader implementation:

```go
type JournalWriterUse struct {
    Generation int64
    WriterID   string
}
// DeletionRead adds WriterUses []JournalWriterUse in first-use order,
// covering every entry and seal object verified by this read.
```

`internal/hosted/deletion` owns extracting these uses from already verified stored objects; `internal/hosted/contracts` owns the additive shape and its semantics. Missing, duplicate-conflicting, empty, or authority-unknown writer uses fail closed. A writer ID is unique to one startup issuance; reusing a prior ID is refused. The active authority record binds the new `WriterID`, generation, namespace, startup ID and credential principal, and the object-store client enforces that exact scope on every write. Old writer credentials must remain unable to write after the new lease is issued. The conditional object create remains a final integrity/race check, not the single-writer authority.

After the complete read and writer reconciliation, `ActivateWriter(ctx, head)` atomically compare-and-sets the authority's active writer epoch against the prepared reservation, issuing the new narrowly scoped write client. It must reject if the head or expected generation changed after verification. The returned journal is constructed from that lease's store/writer ID/generation, never from config. Startup then replays all verified deletion intents against the existing control DB, refuses orphan terminal/deleting state as current service logic requires, and finishes provisioning recovery. Publish handlers/workers only after assembly returns success and a final `ctx.Err()` check.

## Service shared-seam request

Preserve `RecoveryAdmission` and its sealed same-boundary checks unchanged. Request the service owner to add a distinct authenticated startup admission capability to `LifecycleDependencies` (or an equivalently scoped constructor), mutually exclusive with off-host `RecoveryAdmission`; absence or both capabilities return `ErrStartupUnavailable`. Do not add an exported `bool` such as `isRecovery`/`isGenesis` to control validation. The admission itself carries a complete expected journal head, not a caller-supplied watermark or mode flag.

For precision, the additive service contract should have this shape (names remain subject to owner review):

```go
type AuthenticatedJournalStartup interface {
    Admit(ctx context.Context, journal contracts.DeletionJournal) (contracts.DeletionRead, error)
}

type StartupLease interface {
    Close(ctx context.Context) error // revoke exact writer authority after service work is joined
}

// LifecycleDependencies adds Startup AuthenticatedJournalStartup and Lease StartupLease.
// validate requires exactly one of Startup or Recovery, plus the existing journal/build identity.
```

In the authenticated branch, service assembly independently calls `ReadThrough` from zero, requires the result to end at the admitted head, and replays the full verified deletion history before handlers/workers publish. A verified unsealed head is permitted only through this capability; the recovery branch retains its existing two sealed reads and snapshot watermark rules. The service owner must specify how authority reserves the verified head through writer activation/first append so a concurrent tail cannot be ignored. The exact leased journal instance remains shared with Gateway and backup. The service owner should add `CloseContext(ctx)` (with the existing `Close()` delegating under a bounded shutdown policy) so it joins request/workers and then closes the exact `StartupLease`; cleanup errors must be returned and an uncertain revocation must leave the startup reservation unresolved.

This amendment affects `internal/hosted/service/service.go` and `internal/hosted/contracts/deletion.go`; those files are not owned by this proposal lane. The service and contracts owners must accept the exact seam and ownership boundary before an implementation task is frozen. Existing test-only filesystem journals stay explicit test fixtures, never production fallback.

## Cancellation, crashes, and shutdown

| Boundary | Required result |
|---|---|
| Before authenticated permit/reservation | No journal credential or service side effect; return cancellation/authentication error. |
| Permit consumed, before journal read or writer activation | Durable `GenesisPending` is retained. Retry resolves the same startup ID and digest; another permit use cannot issue a second genesis. If state cannot be reconciled, stay unavailable. |
| Old-writer stop/revocation ambiguous | Do not read/activate a new writer or serve. Authority keeps the namespace fenced pending reconciliation. Never infer success from timeout. |
| During `ReadThrough` / historical-writer verification | Propagate context/backend error; issue no write credential. Incomplete chain, concurrent tail change, wrong writer, or namespace mismatch remains unavailable. |
| Writer activation response lost | Resolve the exact startup ID and expected head from the durable authority record. Reuse only the same lease or revoke it; never create a second active epoch on blind retry. |
| After lease activation, before service assembly/publication | On cancellation/error close constructed resources and revoke the exact staged lease. If revocation outcome is unknown, mark startup unresolved and block another writer until authority reconciliation. No listener/worker is published. |
| Graceful stop | Stop accepting work; cancel and join service workers; close Service; revoke its exact writer lease and verify. Ordinary restart leaves the generation unsealed unless an explicit serialized rotation owns the seal-and-successor transaction. |

No cleanup error may be discarded if it could leave credentials live. `Service.Close` today has no journal lease. The requested service-owned `StartupLease` field and `CloseContext` path must compose lease revocation after work is joined; do not rely on an outer wrapper or silently assume the existing `Close` revokes authority. Shutdown uses a fresh bounded cleanup context, not the already-cancelled serving context.

## Hostile controls required before source qualification

Each implementation test must show a genuine pre-fix RED with its exact old-source result captured before correction, then restore the fixture and show the focused corrected check pass. Fakes demonstrate protocol behavior only; they do not establish provider stop, revocation, namespace binding, or durable authority semantics.

1. Genesis with valid signed permit succeeds once; exact retry resumes the same startup ID. Replayed nonce with a different ID, namespace, deployment, build, or digest is rejected. Missing, expired, wrong-audience, wrong-namespace, malformed, or wrong-signature permits cause zero write credentials and no service construction.
2. Empty journal with no durable genesis record is rejected. Empty journal after durable authorized genesis can resume that exact issuance after a simulated crash. An arbitrary nonempty directory cannot substitute for either record.
3. Ordinary restart on generation 1 with a real unsealed tail succeeds after old writer stop/revocation and preserves every entry/head hash; current `AssembleWithDependencies` must be recorded rejecting this unsealed shape today. A sealed current generation without a persisted successor rotation cannot be reopened for writes.
4. Wrong prior seal, missing middle object, malformed coordinates, unauthorized historical `WriterID`, conflicting writer-use mapping, unsealed earlier generation, post-seal tail, unexpected later generation, and a tail appended during the proof all deny admission. Use real `Journal` mechanics for journal controls and an independent authority fake for issuer decisions.
5. Two concurrent prepares for one namespace yield one active writer epoch. A stale prior credential is rejected after new activation; the test must exercise the object-store authorization gate, not merely a same-sequence `PutIfAbsent` conflict.
6. Crash/retry barriers after nonce consumption, old-writer revocation, history verification, lease activation, service assembly, and immediately before handler publication preserve one issuance and never expose a service with unresolved authority.
7. Cancellation before I/O, during authority call, during each journal read page, between proof and activation, and after activation closes/revokes or leaves a durable unresolved fence. Assert no handler/worker publication and no credential reuse.
8. Recovery controls remain unchanged: a recovery COMMITTED record for a different snapshot/namespace cannot authorize genesis or restart; recovery with a valid plan still requires its pinned watermark, fenced old writer, successor and sealed complete boundary.

Current source already has mechanics controls for journal gaps, hash mismatch, generation gaps, post-seal tails and startup cancellation in `internal/hosted/contracts/contractstest/journal_suite.go`, `internal/hosted/deletion/journal_seal_resume_test.go`, and `internal/hosted/service/startup_cancellation_regression_test.go`. Those are useful foundations, not proof of the new authority path. No tests were run for this document-only assignment.

## Ownership and dependency-linked delivery gates

1. **Contract review — this lane:** review only this proposed document against main `db7625cf`; no source changes.
2. **Seam freeze — root + service/contracts owners:** independently review the shared `LifecycleDependencies` and `DeletionRead.WriterUses` amendments, retain the current recovery contract, and publish an accepted interface decision. Until this is complete, no implementation is authorized.
3. **Preflight — assigned startup owner:** prove a separate SSD worktree and canonical source claim, enumerate exact ownership and protected boundaries, confirm a real durable authority and credential-revocation mechanism exists, and freeze test fixtures. Stop if authority is not available; do not replace it with local files or a boolean.
4. **Implementation:** new `internal/hosted/startup` factory/authority adapter and its tests are owned by one startup implementation lane. `internal/hosted/deletion` owns writer-use projection; `internal/hosted/contracts` owns its contract tests; `internal/hosted/service` owns the typed admission/replay seam; `internal/cli` owns wiring only after the factory contract is accepted. Shared files require their owners' explicit task/claim and integration review. No one worker owns overlapping shared files by implication.
5. **Verification:** focused journal/service/startup hostile controls first; then the prescribed Go race, vet, lint and supported compilation gates under a verified shared build lease and load limits. Add actual provider-authority qualification as a separately authorized gate; local fakes cannot satisfy it. Preserve failures and distinguish local source, CI, provider, live deployment, and user acceptance evidence.
6. **Independent review:** a reviewer in a separate clone reruns every decisive RED and corrected control against exact candidate source; review must check the trusted construction path, concurrent epoch transition, replay durability, cancellation cleanup, and no recovery/genesis conflation.
7. **Merge:** root checks current feed/board/PR discussions and exact reviewed head, then separately authorizes normal expected-head merge. A review or coordinator hold blocks merge.
8. **Landed verification:** verify the merged tree equals the reviewed candidate and the full diff is empty; update the accepted plan/roadmap through their owners. A docs proposal merge does not qualify provider authority, deployment, production startup, deletion, backup/restore, or overall hosted readiness.

## Open evidence boundary

The repository has no production startup factory, durable genesis issuer, current-writer lease/revocation adapter, or provider-qualified journal credentials in the inspected source. Existing `ProductionJournal` is a constructor over an injected store, not evidence of production authority. Therefore this contract is code-ready direction only; all provider identity, bucket policy, credential lifetime/revocation, durable replay, physical startup behavior, and deployment claims remain unproven.
