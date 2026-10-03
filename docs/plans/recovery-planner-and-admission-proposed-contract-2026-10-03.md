# Recovery planner and production admission: proposed integration contract

**Status:** coordinator proposal for independent review; no authority to edit frozen interfaces, contact providers, deploy, purge, spend, or activate accounts. Source baseline is `8081632` (after PR #349). This proposal supplies the next production mechanism, not another inspection-only preview. Keep the existing contracts frozen until the owning integrators approve the additive handshakes below.

## Ruling on the three open contract questions

1. **Snapshot identity.** Set `contracts.RecoveryPlan.SourceSnapshot` to the lowercase SHA-256 of the exact raw `manifest.json` byte sequence returned by `backup.InspectSnapshot`; the existing object-ID validator accepts 64 lowercase hex characters. Do not hash a re-encoded JSON value. `ManifestV2.Source.BuildSHA` and `SchemaVersion` remain distinct provenance fields. Persist both beside the plan in the canonical plan document and include them in its canonical payload hash in an additive plan-store format revision. A reviewed additive `RecoveryPlanRequest.ExpectedSnapshotSHA256` is required: validate it before opening/copying artifacts and compare it with the inspector's digest before provider observation or plan publication. An operator signature, if an applicable verifier is accepted by its owner, binds this expected digest and exact account scope; it does not prove snapshot truth or current provider state. The later plan hash is the approval key for apply, never proof of authority by itself.
2. **No eligible accounts.** Do not weaken `RecoveryPlan.Validate` or fabricate an entitlement to satisfy its nonempty `Accounts` invariant. `Plan` returns a typed `ErrNoEligibleAccounts` and writes no `RecoveryPlan`, plan hash, or apply-capable artifact. The CLI may durably record a distinct `NoEligibleAccounts` terminal outcome bound to snapshot digest, provider observation cutoff, and reason codes, clearly marked non-executable. It must not make an empty plan pass validation. Missing, ambiguous, stale, or incomplete billing evidence is an error, not a zero-eligible success. Current cancelled/Free eligibility is allowed only when the current observer establishes it under existing policy.
3. **Fence/adoption evidence.** Keep `FenceReceipt.SufficientFor` as a shape-and-binding check; its booleans are not proof. A production receipt must reference independently retrievable, authenticated control-plane evidence for the stopped old instance, complete revocation/denial of every old writer credential and extant session, and the post-watermark durable journal seal. A second durable, authenticated adoption record must prove the successor generation and active writer identity after that fence. Require verifier-returned identity, scope, generation, timestamps, nonce/replay binding, and canonical record digest. A process lock, PID, elapsed lease, static generation, copied file, `ReadThrough(...).Sealed`, or signed approval/hash cannot establish any of these facts.

## Current seams and ownership

The frozen interfaces in `internal/hosted/contracts/backup.go` already define `RecoveryPlanner.Plan`, single-account `RecoveryApplier.Apply`, immutable plan/result binding, and `WriterFencer.Fence`. `contracts/fence.go` validates shape, watermark ordering, generation, booleans, and timestamp only. Do not edit these files in the first implementation patch.

Reusable implementations now exist and must be consumed rather than recreated:

- `internal/hosted/backup.InspectSnapshot` verifies the manifest and copied artifacts, checks schema and inventory, and reports the raw manifest digest and build identity separately. Its private scratch path and byte/account/brain bounds are explicit. T23.50 should call it and reuse its verified scratch copies.
- `internal/hosted/recovery` has a strict canonical immutable plan store, digest validation, and bounded storage. Extend its format in a reviewed additive change to persist manifest provenance and the outcome record; do not create a second parser/store.
- `internal/hosted/contracts.RecoveryBillingObserver` and `internal/hosted/billing` provide bounded, read-only restore-pending observations, including status/customer-binding reread protections. The observer is not a provider truth authority for unrelated states and must not be made to write SQL or unfreeze accounts.
- `internal/hosted/service.AssembleWithDependencies` requires a shared configured journal, build identity, and `RecoveryAdmission`; it admits before reading the journal, verifies the snapshot-cut read and full-history read through the same sealed boundary, reconciles pending deletion intents, then constructs/publishes service handlers. There is no production admission factory today.
- `internal/hosted/deletion.Journal` implements durable append, `ReadThrough`, and `Seal`. Existing tests establish that a seal can be read without proving a configured successor generation was adopted. The service and backup path must receive the same journal instance.

Ownership split for source follow-up: T23.50 owns `internal/hosted/recovery/**` and `internal/cli/hosted_recovery.go`; backup owners review `InspectSnapshot`/plan provenance handshakes; billing owners retain the observer and review any new state projection; hosted service owners wire the same journal and construct admission before `AssembleWithDependencies`; journal/storage owners implement a durable adoption capability. A dedicated privileged control-plane owner must implement AWS stop, identity, and revocation adapters. These are dependency-linked requests; the recovery planner must not edit those owners' files or silently take their permissions.

## Proposed private APIs

Add these APIs only after the owning interface changes receive independent review:

```go
type RecoveryEvidenceVerifier interface {
    VerifyOperatorPlan(ctx context.Context, signedCase []byte, snapshotSHA256 string, accountIDs []string) (VerifiedApproval, error)
    VerifyOldWriterStop(ctx context.Context, ref EvidenceRef, old WriterIdentity) (VerifiedStop, error)
    VerifyCredentialRevocation(ctx context.Context, ref EvidenceRef, old WriterIdentity) (VerifiedRevocation, error)
    VerifyJournalSeal(ctx context.Context, ref EvidenceRef, generation int64, after contracts.DeletionWatermark) (VerifiedSeal, error)
    VerifySuccessorAdoption(ctx context.Context, ref EvidenceRef, predecessor WriterIdentity, generation int64) (VerifiedAdoption, error)
}

type RecoveryControlPlane interface {
    StopOldWriter(ctx context.Context, expected WriterIdentity, operationID string) (EvidenceRef, error)
    RevokeOldWriterCredentialsAndSessions(ctx context.Context, expected WriterIdentity, operationID string) (EvidenceRef, error)
    AdoptSuccessor(ctx context.Context, old WriterIdentity, generation int64, seal contracts.DeletionWatermark, operationID string) (EvidenceRef, error)
}

type RecoveryCoordinator interface {
    Plan(ctx context.Context, req ApprovedRecoveryPlanRequest) (contracts.RecoveryPlan, error)
    Apply(ctx context.Context, req contracts.RecoveryApplyRequest) (contracts.RecoveryApplyResult, error)
}

type ActiveWriterIdentitySource interface {
    CurrentWriter(ctx context.Context) (WriterIdentity, error)
}
```

`EvidenceRef` is an opaque, non-secret provider record locator plus provider/account/region scope and immutable object/version identity; it contains no credentials, customer identifiers, raw host paths, or operator secrets. Every `Verified*` is a verifier-produced value with canonical record digest, verifier identity/key version, signed scope, exact expected subject/generation, observed-at time, expiry/freshness limit, unique operation nonce, and source version. The verifier fetches the referenced evidence itself using its read-only authority and rejects changed versions, wrong subject/scope, stale timestamps, missing subclaims, replayed nonce, and unverifiable chains. Do not accept caller-constructed `Verified*` values across the CLI boundary; persist verifier output and reverify before every phase that consumes it.

The public CLI stays a caller of the coordinator, not a provider principal. `recovery.NewProductionCoordinator(Config)` must require explicit private staging root, maximum snapshot bytes, freshness windows, admission-record verifier, `RecoveryEvidenceVerifier`, scoped `RecoveryControlPlane`, billing observer, backup inspector, plan store, journal, and restore applier. Constructor validation fails closed if any dependency/root/limit is absent. No constructor may fall back to `os.TempDir`, default AWS credentials, metadata credentials, an empty journal, or a fabricated generation.

### Plan path

1. `Plan(ctx, ApprovedRecoveryPlanRequest{SnapshotPath, ExpectedSnapshotSHA256, SignedCaseRef})` validates digest syntax, operator approval for that exact digest, and scoped opaque account IDs before touching provider state. If the existing signed-case verifier cannot establish a one-time approval bound to this exact source digest and scope, stop and request a reviewed verifier contract; do not infer approval from a plan hash.
2. Call `backup.InspectSnapshot` with configured private scratch and maximum declared artifact bytes. The inspector's result is authoritative only for the bytes it actually verified. Compare raw manifest digest exactly with `ExpectedSnapshotSHA256`; retain its `BuildSHA`, schema, watermark, and verified inventory as separate fields.
3. Read the complete configured journal history from zero and independently establish its terminal seal/current generation through the journal-owner capability. Require complete contiguous history through the snapshot watermark and reject a watermark absent from that history. Do not call `Seal` in Plan. Reject any journal tail whose seal/current writer cannot be established independently.
4. For every snapshot account in sorted order, observe current provider truth with the existing `RecoveryBillingObserver` under per-call timeout and rate cap. Enforce the maximum account count before calls. Do not include deleted/deleting accounts, unresolved pending checkout, unknown/multiple nonterminal subscriptions, stale state, provider timeout, or any ambiguous status as eligible. Preserve opaque account IDs only. Capture an aggregate observation time as the earliest observation expiry boundary, not the last response time.
5. Produce the existing frozen `RecoveryPlan` fields with `SourceSnapshot = raw manifest SHA256`, journal watermark, observed generation, earliest provider observation, and sorted eligible IDs. In the canonical private plan envelope also bind manifest build SHA/schema, approval verification digest/scope, per-account observation digests/times/outcomes, inspector bounds, and journal completeness/seal reference. The plan payload hash binds all these fields. Persist atomically with owner-only permissions and fsync file and parent directory before returning it. If the eligible set is empty, persist only the separate non-executable outcome described above and return `ErrNoEligibleAccounts`.

### Apply, fence, restore, and admission path

`Apply(ctx, RecoveryApplyRequest{PlanHash, AccountID})` only accepts one account and one plan hash. Load exact canonical bytes from the plan store, verify strict schema, hash, approval record, expected source digest, and account inclusion before side effects. Reobserve the account and fail if truth differs, is older than the configured freshness bound, or cannot be established. Do not extend freshness by reusing a previous plan observation.

Use a durable transaction journal keyed by `(plan_hash, account_id)` in the private recovery state store. Each record is append-only, canonical, hash chained, owner-only, fsynced before advancing. Store only opaque IDs and minimized evidence references/digests. Phases and required durable ordering:

| Phase | Durable transition before next effect | Idempotent retry rule |
|---|---|---|
| `PREPARED` | Bind exact plan/account, current observation digest, snapshot digest, approval digest, and unique operation ID | Same tuple resumes; any changed tuple is a hard conflict |
| `FROZEN` | Verify account remains frozen after restore; do not unfreeze | Already frozen is accepted only after reread proves exact account and state |
| `OLD_WRITER_STOPPED` | Control plane stops the exact old instance; independently verify record and persist `EvidenceRef` plus verifier result | Recheck same immutable record/version and exact instance identity |
| `OLD_CREDENTIALS_REVOKED` | Enumerate credential/session inventory from authority, revoke/deny every item, verify inventory completeness and effective denial, persist references and inventory digest | Repeat only idempotent revoke/deny; missing inventory or one live session blocks |
| `OLD_GENERATION_SEALED` | With prior phases verified, call the configured journal `Seal(plan.Generation)`; reread/verify complete history after plan watermark through returned seal; persist watermark and independently verifiable seal reference | Existing exact seal is reusable only if complete reread proves it; appended entries require a new verified seal |
| `RESTORED_RECONCILED` | Restore to fresh private destination; verify snapshot digest, schema, journal intents from zero through seal, deleted-account intents, and account remains frozen; never publish partial destination | Rebuild into a new scratch destination; publish by atomic same-filesystem rename only after full validation |
| `SUCCESSOR_ADOPTED` | Trusted coordinator durably authorizes generation `plan.Generation+1` for explicit successor writer identity and seal; independently verify adoption reference | Same operation/identity/generation returns same adoption record; conflicting adoption halts |
| `ACCOUNT_RECHECKED` | Reobserve billing, checkout, account status, credential epoch, and journal boundary; persist fresh evidence | Any mismatch resumes frozen; no compensating entitlement writes |
| `UNFROZEN` | One-account transaction unfreezes only after reloading verified phase chain and matching `RecoveryApplyResult.Consistent` | If already active, verify its exact recovery operation marker and all gates; otherwise fail closed |

Do not return `Unfrozen=true` until the final phase is durably committed and the result passes `RecoveryApplyResult.Consistent(plan, req)`. The result's `FenceReceipt` may mirror the verified generation, seal, stopped/revoked booleans, and verification time for compatibility, but admission consumes the persisted references and verifier-produced evidence, never those booleans. Any cancellation, provider outage, changed source/plan, incomplete inventory, failed fsync, inconsistent journal, unverified adoption, or uncertain SQL commit returns an error and leaves the account frozen. On uncertain final commit, reread account state and operation marker before retry.

Expose production `RecoveryAdmission` only through `recovery.NewAdmission(recordStore, evidenceVerifier, activeWriterIdentitySource, journalIdentityVerifier, buildSHA)`. `ActiveWriterIdentitySource` must obtain the actual running identity through a reviewed local supervisor/control-plane channel with peer authentication; it cannot return a static generation or trust caller-supplied environment/config. Because the hosted app is denied IMDS and must stay denied, this source cannot silently switch to metadata credentials. `Admit(ctx, journal)` loads the latest committed recovery record, re-verifies stop, all-item revocation/denial, seal, adoption, source/apply provenance, and exact active successor identity, then checks the supplied journal's storage identity is the adopted journal. It returns the snapshot-cut watermark only when the journal reads sealed through both that cut and the complete history; existing `AssembleWithDependencies` then performs its current full-history and intent checks. It rejects a generation number copied from config, absent successor writes, a different journal object/store, stale or invalid evidence, a different build when the record binds one, or any uncommitted account transition. Service construction must receive the same configured journal used by backup and fence paths.

## AWS boundary and privilege separation

The hosted application remains denied IMDS (`169.254.169.254`) as already enforced by its unit. Do not add an instance profile, widen that denial, grant the app broad host credentials, or use ambient/default credentials. Recovery CLI invokes a separately provisioned control-plane process/role with an explicit credentials file/profile and narrowly scoped role assumption. The application role remains unable to stop instances, revoke credentials, mutate journal generations, or adopt a successor.

The control-plane implementation must shell out only to the installed `aws` CLI using fixed argument arrays, no shell interpolation, explicit region/profile, bounded context, scrubbed environment, and captured request IDs. For instance stop, use `aws ec2 describe-instances` to verify account/region/instance ID and tags against the approved writer identity, `aws ec2 stop-instances`, then `aws ec2 wait instance-stopped`; persist immutable response/request references and re-describe state. For revocation, inventory every writer credential and issued session through their owning credential/session authorities, revoke/delete or add explicit deny for each, then independently describe/list and exercise an authorization-denied verification. Enumerate IAM access keys/roles, task/pod or host credentials, and app-issued sessions according to the actual deployment identity model; if any authority cannot enumerate a class, refuse to claim completeness. Do not assume EC2 stop revokes already-issued sessions.

No specific AWS role, account, region, policy, session issuer, or credential inventory is established by this proposal. The deployment/control-plane owner must propose least-privilege policies and identify the actual identity classes in a separately reviewed patch. AWS CLI output and local fake commands are not themselves proof: the verifier must retrieve provider records and validate subject, scope, status, and version. Apply continues to fail closed when this boundary is not configured.

## Bounded behavior and fake-only RED controls

Configuration requires hard maxima for manifest/artifact bytes, account count, brain count (reusing inspector defaults/limits rather than loosening them), parallel provider observations, total operation duration, and evidence freshness. Do not truncate source validation or continue with partial account lists when a limit is exceeded. Use a small fixed worker pool, per-provider timeouts, context cancellation, and deterministic sorted processing/persistence. Bound each AWS CLI invocation and output capture. No success result is emitted for incomplete work.

Before any provider integration or live qualification, the coordinator package must have fake-only RED controls proving:

- raw manifest digest equals the exact `manifest.json` bytes; changing whitespace changes identity; build SHA is never substituted; expected-digest mismatch causes zero provider calls and no plan artifact;
- build SHA/schema, approval scope, provider observation digests, and seal reference change the canonical plan hash; duplicate/unknown JSON keys, trailing data, path substitution, and plan tampering fail before effects;
- zero eligible accounts returns `ErrNoEligibleAccounts`, writes no executable plan, and leaves existing `RecoveryPlan.Validate` unchanged; unresolved checkout, deleted account, provider outage, stale observation, wrong price, and ambiguous subscriptions never become Free eligible;
- stop succeeds but one credential/session remains valid: no seal/adoption/unfreeze; a complete-looking but unverifiable or stale receipt: no progress; signed case/hash alone: no fence or provider truth;
- seal without a successor adoption write, successor configured but not adopted, visible empty successor prefix, mismatched journal storage identity, or incomplete history: production admission refuses startup;
- each crash cut immediately before/after every durable transition is retried without duplicate destructive effects; conflicting operation ID, nonce replay, changed evidence version, changed provider state, failed fsync, canceled context, and uncertain final SQL commit all leave account frozen;
- two accounts in one plan are independently applied; there is no apply-all path and one account's success cannot authorize the other;
- application process attempts IMDS/default AWS credentials and receives no authority; fake CLI argv has no shell metacharacter expansion and never includes secrets; fake CLI output alone cannot pass evidence verification.

These tests use fake filesystem/journal/provider/control-plane/verifier implementations only. They verify fail-closed behavior and durable state machine semantics. They do not qualify AWS permissions, real credential/session revocation, actual writer stop, durable storage guarantees, or hosted activation.

## Review handoffs and acceptance boundary

Locally implementable after exact-head review: additive request/provenance envelope, production coordinator behind injected interfaces, canonical transaction records, bounded phase runner, fake-only RED controls, CLI registration, and `RecoveryAdmission` record verifier. Separately reviewed integrator patches are required for the backup request digest handshake, billing policy surface if additional outcomes are needed, journal current-generation/adoption capability, hosted service and backup shared-journal factory, control-plane AWS CLI role and evidence store, and operator signature semantics. Keep changes additive and owner-local; do not edit frozen shared contracts or deployment/IAM policy in the planner patch.

Live-only evidence still required before hosted acceptance: the exact operator-approved snapshot and account set; actual provider truth freshness; complete old-instance stop proof; full inventory and effective revocation/denial of all old writer credentials and sessions; post-watermark durable seal; durable successor adoption/write authorization; production shared journal wiring with scoped credentials; physical storage limits/durability; and operator-controlled activation evidence. Inspector success, observer tests, signatures, plan hash, fake adapters, and local startup tests do not satisfy these gates. No live provider action, deployment, purge, spend, or activation is part of this proposal.
