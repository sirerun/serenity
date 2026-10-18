# Same-release fixture maintenance owner — contract v1

This source owns same-release fixture qualification. It does not release a new
binary, authorize live operations, enable capture, erase customer brains or
restore old canonical data. The owner assigned this source reconciliation to the
current integration session. Historical private transition templates remain
preserved evidence; selected generic mechanisms are adopted here as the future
versioned source owner, rather than creating another active private scheduler.

## Authority and deployment boundary

Reuse reviewed root-owned code, exact private packet pins, existing systemd
custody and the established service-UID/FD handoff. No new signing service, key,
policy, KMS operation or grant CLI is introduced. A generic runtime has
`CONFIG = None` and refuses CLI use. A renderer produces an inert private packet;
rendering is neither approval nor execution. An independently reviewed, finite,
exact operational packet and its trusted owner execution channel remain required
before any real invocation. A local JSON flag or hash cannot authenticate who
approved it, claim CLOSED authority or establish namespace completeness.

Current source preflight used a read-only release-lock observation on the
existing host: installed/highwater8, the existing service pair, healthy normal
services/timer, no hold/pending, capture off. Private observations, host identifiers,
configuration, credentials, review inputs and rendered packets stay outside Git.
The observation is not a cold grant. Deployment and current version-bound host,
provider and service-UID/FD qualification remain separate obligations.

## Source ownership and files

- Runtime producer: `runtime.py`, extracted generic systemd custody support and
  `observe_closed.py`, with runtime/authority-focused tests.
- Fixture producer: `collect_fixture_binding.py`, `collect_cold_ready.py`,
  `render_fixture_proof.py` and their tests.
- Consumers: existing `evals/episode-qualification/offline_operator.py` and
  `custody_linux.go`, plus existing and new interoperability tests.
- Integration owner: this contract, provenance, `render_runtime.py`, joined
  fixtures, plan/evidence and independent review/delivery.

No rendered old6-to8 packet, approval, saved observation, credential or private
account/principal/host constant is imported. Generic donor mechanisms and their
hashes are recorded in provenance; copied source is adapted and independently
reviewed. Root never opens SQLite as root or emits keys. Metadata inspection and
fixture mutation retain the existing service UID and private namespace.

## Exact runtime seam and packet

The runtime exposes `Transaction(config, root=Path('/'), runner=None, uid=0,
gid=0, clock=time.time)`, `enter()`,
`closeout(restore_receipt_path, authorization_path=None)`, `guard(unit)`,
`supervisor_exit()` and read-only `cold_ready()`. Test injection is import-only;
CLI is root-only, isolated Python, real root filesystem and configured bounded
supervisor. No environment-variable test or authority override is accepted.
The runner must preserve typed systemd command/path/argv checks, no shell input,
finite command time/output bounds, and actual cgroup/unit-job quiescence.

The canonical private declaration is sorted compact JSON with exact keys:
`version, purpose, operation_id, issued_at, expires_at, boot_id, stage,
supervisor, service, fixture, authority`. Version1 and purpose
`serenity.fixture-maintenance/v1` are mandatory. Operation ID is64 lowercase hex;
stage is `/var/lib/serenity-release/maintenance-<operation_id>`; supervisor is the
exact bounded `serenity-release-<operation_id>.service`. Lifetime is positive and
at most3600seconds. Expiry never clears a hold or implicitly restarts services.
Every mutating action rechecks time/boot/pins; lost authority retains the hold.

`service` has exact keys `source, binary_path, binary_sha256, state_sha256,
broker_sha256, config_sha256, installed_sequence, highest_sequence,
release_manifest_sha256, startup_runtime_path, startup_runtime_sha256, unit_files`. Sources are40 lowercase hex, hashes64, sequences positive
with installed<=highest. Binary path names the exact unchanged installed artifact.
No previous/target aliases, release preparation, commit/recover, pair publication,
config edits, signing or binary install occurs. State bytes and highwater remain
unchanged even on failure/closeout. Capture/workos/billing remain false and
synthetic-brain enrollment remains empty. `startup_runtime_path` and its hash pin
the existing permanent startup guard's root-owned runtime. `unit_files` is a
complete bounded array of exact `{path, sha256}` pins for the four actual unit
fragments and every current drop-in; foreign extra drop-ins refuse. Paths are
limited to those fixed units' systemd directories. Typed ExecCondition must
match the exact pinned baseline runtime before entry; temporary guards use this
operation's runtime, then closeout restores exact baseline bytes.

`fixture` has exact keys `tool_source, tool_path, tool_sha256, wrapper_path,
wrapper_sha256, uid, gid, data_dir, private_dir, account_cap, schema_sha256, unit`.
UID/GID are the actual nonroot service account. Source/tool identity is independent
of service source. Private dir is service-owned0700 outside canonical brain roots;
unit is `serenity-episode-fixture-<operation_id>.service`. Existing exactly-two
synthetic-account custody, source-adoption and reissue limits are preserved.
The runtime does not read credential bytes. Unknown/unsettled child outcomes or
control/SQLite sidecars refuse closeout; metadata-only service-UID reads must
honor exact complete family/schema state rather than discard sidecars.

`authority` has exact keys `generator_path, generator_sha256, scope, session_id,
concurrency_target, restore_concurrency, expectations`. Expectations are a strict parameterized input
to the adopted authenticated read-only collector, not a replacement receipt.
Its exact keys are `cli_path, cli_sha256, account, principal, region, stack,
function, version, code_sha256, table, namespace, table_arn, lineage,
witness_bucket, metadata_sha256, tip_sha256, minimum_revision`. Endpoint regions and identity,
published version/code and configured table/namespace/lineage/witness bucket,
exact metadata and independent TIP hashes must all match. The collector must perform real authenticated
provider reads in its production path, verify CLOSED/no lease/no prepared or
unfinished witness/concurrency0, and recheck metadata/tip. Injected fixtures do
not establish live authority. Unknown expectation fields refuse. Public source contains no private account,
principal, function, region, CLI or namespace constant. Reuse the donor's
read-only META/TIP/configuration/concurrency rechecks, bounded45second collection,
regional endpoints and sanitized request IDs; never persist CLI diagnostics or
raw authority records. `observe_closed.observe(expectations, runner=None)`
returns the donor's detailed sanitized observation, with production authenticated
AWS CLI reads and import-only bounded injected runner for tests.

CONFIG contains exactly that declaration plus `manifest_sha256`, the hash of
its exact bytes. Runtime path is `<stage>/runtime.py`; its output hash is bound
externally by operational review/proofs, avoiding a self-referential hash. The
renderer exclusively creates an owned0700 output containing declaration/runtime;
it never runs them, signs, uploads or includes an approval assertion.

## Durable hold and unit custody

Reuse `/var/lib/serenity-release/state.json.lock` and established permanent
startup guards. Entry acquires exclusive existing lock ownership, verifies exact
current pair and clean/no-pending baseline, snapshots exact prior owned guard/
timer settings, installs durable fail-closed guards before mutation, publishes
an owned hold, stops/drains timer/backup/hosted/broker and revokes all permits.
An already active backup or child must drain within bounds; no PID-only success.
Only owned temporary files may change. Foreign holds, unknown guards/permits,
unknown pending files and unsupported prior unit arrangements refuse.

The new hold has exact keys `version, purpose, operation_id, manifest_sha256,
service_source, executable_sha256, state_sha256, broker_sha256, config_sha256,
supervisor, stage, backup_timer_prior_state, phase, timestamp, expires_at`.
Purpose is `serenity.fixture-maintenance/v1`; phase for fixture admission is
`maintenance-ready`, prior timer state is `active`. No transition-shaped previous/
expected fields appear. Runtime verifies unchanged service identity on every
phase and uses durable atomic owned record publication, explicit recovery and
fresh owner admission. Failure retains gates; no destructive old-state rollback.

Cold readiness checks all four normal units loaded/inactive/dead, zero jobs/PIDs,
empty actual cgroups, exact typed guards, active bounded supervisor, boot, stable
lock, no permits/pending and unchanged pair. It returns content-free v2 readiness:
exact keys `version, purpose, operation_id, manifest_sha256, state_sha256,
hold_sha256, service_source, executable_sha256, held, services_started`;
version2, purpose above, held=true, services_started=false. Creation/inspection
must never manufacture provider CLOSED authority. The separately collected fresh
authenticated authority receipt binds that exact hold/state/boot.

Closeout first reconciles exact child termination and custody outcomes, including
required credential revocation, private inventory, account/data retention and
consumed nonce records. Unknown or interrupted outcomes stay held. Existing nonce
files and journals are preserved. Restore only byte-pinned owned temporary guards
and the qualified prior timer/service arrangement; verify unchanged pair and
actual readiness before releasing the matching hold. Service start must obey the
held permits/supervisor protocol. A durable closeout intent makes interruption
recoverable; a missing hold alone is never proof of success. Fresh exact operational
authority is needed for resumed closeout after expiry; no implicit expiry bypass.
The exact renewal and restoration seams below apply. Cloud restore remains with
its existing authenticated owner; host source never freezes/thaws capacity.

## Expiry recovery and authority restoration

Fixture admission always uses the original declaration window and CLOSED receipt.
Closeout uses original authority only while that window/supervisor remains valid.
Otherwise its optional `authorization_path` names a NEW privately reviewed root-
owned0600 regular single-link closeout-only record. Exact keys: `version, purpose,
operation_id, manifest_sha256, hold_sha256, state_sha256, executable_sha256,
broker_sha256, config_sha256, boot_id, runtime_sha256, entry_baseline_sha256,
supervisor, issued_at, expires_at`. Version1, purpose `serenity.fixture-closeout-authorization/v1`;
all old hold/declaration/current service hashes and current runtime must match.
New lifetime is positive, <=300seconds, current boot only. Supervisor may be the
original still-qualified bounded unit or exact newly reviewed
`serenity-release-<operation_id>-closeout-<64hex>.service`; it must actually be
active and bounded through the new deadline. Source does not create unreviewed
supervisors or extend a unit silently. As with the original packet, root file
ownership/hash binds input to the trusted owner channel; fields do not authenticate
an independent approval. Runtime never treats this record as fixture permission.

Under the existing EX lock, verify exact retained hold/current pair/child outcomes,
then persist `<stage>/closeout-pending.json`, a durable closeout intent binding
original hold and declaration plus
this authorization hash/effective supervisor/deadline. Keep ORIGINAL hold bytes
unchanged throughout closing. Intent presence fences ALL new fixture proofs,
even if their old hold hash/window still matches. Rebind only owned transient
supervisor guards if needed, using the durable intent as the forward-recovery
identity. Guard(unit) can admit only closeout permits under that exact original
or renewed closeout authority. Unknown/partial authorization or progress refuses;
no expiry bypass boolean, declaration edit, arbitrary old-hold adoption or fixture
retry is allowed. A missing hold may resume only with its exact durable closeout
record proving prior release plus fresh owner authorization; never infer success.

Before any guard mutation, durable `baseline.json` freezes the expected original
hold bytes/timestamp and a service-UID custody snapshot. The pinned fixture tool's
mutually exclusive `--fixture-custody-snapshot <data_dir> <private_dir>
<config_sha256> <account_cap>` mode returns only canonical content-free
`{version:1, private_inventory_sha256, fixture_scope_sha256, schema_sha256}`.
It uses bounded WAL-aware read-only/query-only SQLite transactions, exact schema,
validated fixture identities and stable private inventory. It never migrates,
changes canonical/control/credential rows, emits keys, discards sidecars or opens
SQLite as root. SQLite read bookkeeping may create/change WAL/SHM; this mode does
not promise zero filesystem effects. Snapshot hashes do not confer authority.

A known-empty variant requires an exactly empty existing owned0700 private
directory, zero current accounts in the entire reserved fixture email domain
regardless of case, exact schema and no foreign-key/orphan ambiguity. Its digest
binds this variant, config and account cap. Missing directory, lock-only state,
partial manifest and ambiguous identity refuse. This is current scoped absence,
not proof that historical renamed/deleted fixture aliases or credentials never
existed. Existing fresh namespace and email-hash collision checks still govern
prepare independently; no cleanup or revocation is inferred from absence.

A crash before hold publication, or after exact hold publication but before
maintenance readiness, can resume after expiry only with fresh closeout-only
authorization binding `entry_baseline_sha256` and the exact deterministic
expected original hold hash. Under the existing lock, require the durable baseline,
unchanged declaration/current service pair, only known old/new owned guards, no
request/proof/consumed/outcome/child history, no readiness record (including
symlink/partial ambiguity), and an unchanged service-UID custody snapshot.
For an absent hold, persist closeout intent BEFORE publishing only the
deterministic EXPIRED original hold. A matching already-published original hold
is preserved byte-for-byte; this adjacent interrupted-entry path requires the
same fresh closeout-only authority and nonexecution proof. The old expiry and closeout intent both deny new fixture admission. This
entry-abort path may omit a final revoke only after proving nonexecution and
unchanged baseline fixture custody; preexisting fixture credentials are preserved,
not opportunistically revoked. Unknown custody stays held for separate explicit
reconciliation. This path is distinct from missing hold AFTER completed release,
which still requires the exact durable closeout record and fresh authorization.

`restore_receipt_path` is a root-owned0600 regular single-link result from the
existing authenticated external restore/observation channel. Exact keys:
`version, purpose, operation_id, manifest_sha256, hold_sha256, state_sha256,
boot_id, observed_at, scope, session_id, concurrency_target, concurrency, phase,
revision, metadata_sha256, tip_sha256, request_ids, generator_path,
generator_sha256`. Version1, purpose `serenity.fixture-authority-restored/v1`.
It binds the unchanged original hold/state/boot/operation and the exact generator,
scope, original closed session and target. Concurrency equals explicitly reviewed
`authority.restore_concurrency` (positive bounded capacity, never an arbitrary
new target). Observation is <=60seconds old, no future timestamp; revision is
at least expectations.minimum_revision, exact namespace/lineage/configuration
remains qualified, metadata and independent TIP agree, no unfinished witness,
phase is CLOSED or ACTIVE. ACTIVE may have its valid normal runtime lease.

The adopted read-only generator adds `observe_restored(expectations, binding,
restore_concurrency, runner=None)` to collect that authenticated current state;
it checks expected account/principal/region/version/code/table/namespace/lineage
and rechecks META/TIP/concurrency, but does NOT require old CLOSED metadata/TIP
hashes after normal operation resumes. Its production path does provider reads;
no local status flag or mock observation is a live restore receipt.

Ordering is explicit: CLOSED/concurrency0 plus drained normal units for fixture
admission -> known child outcomes and final credential revoke -> exact closeout
intent -> authenticated capacity-restored confirmation -> held permits/start of
the unchanged broker/hosted -> actual process executable/identity and readiness
checks -> owned baseline guard restoration and durable completion -> exact hold
release -> prior timer restoration and final arrangement checks. After capacity
restore/start, do NOT apply CLOSED/no-lease checks: readiness must exercise the
actual pinned service's startup/lease readiness instead. Expired/changed restore
receipts can be refreshed by the trusted owner against the SAME held identity;
unknown provider outcomes or missing receipts keep the hold. Every critical
record publication, guard mutation, unit action, parent sync and hold release is
forward-recoverable from exact saved/actual state; torn final records and silent
cleanup are not accepted. Completion remains unfinished until prior timer and
normal arrangement are actually restored. No old binary/data restoration occurs.

## Strict v3 fixture handoff

Keep v1/v2 proof semantics unchanged. V3 purpose is
`serenity-episode-fixture-offline-v3`; operations are prepare/reissue/revoke or
adopt-source. Base fields equal existing `PROOF_FIELDS` minus `hold`, plus
`maintenance_hold`. Adoption adds only existing `source_adoption`, with identical
exact-old-source/manifest validation; other operations must omit it. Null, extra,
duplicate and cross-version fields refuse in Python AND Go. Existing six FD
identities remain proof/hold/state/config/lock/probe; root lock/inode/probe/boot,
time<=300seconds, private consumed nonce, pinned artifacts and service-UID checks
are unchanged. V3 uses exactly the new hold/readiness schema above; its stage,
manifest, installed service and runtime CONFIG must match both directions.

Existing CLOSED receipt schema and freshness/scope/session checks remain. The
trusted generator/collection channel supplies authenticated evidence; a caller
cannot gain authority from a proof-shaped JSON file. Root adapter loads only
already-pinned runtime bytes, matches service source to current service fields
and tool source separately, and checks current cold units before/after child.
It does not weaken old assertions by assigning current state to old previous
fields. Fresh proof and nonce are required after any failed/uncertain child.
After an admitted child exits and final custody rechecks, the root wrapper
publishes a private durable `fixture-outcome-<nonce>.json` under this stage, with
exact keys `version, purpose, operation_id, nonce, operation, proof_sha256,
tool_source, tool_sha256, completed_at, sequence, result`. Version1, purpose above;
result is success only after actual child zero exit and custody checks. Refusal,
interruption and uncertainty cannot become success. All consumed nonces must
have reconciled outcomes and a qualified final revoke before closeout; unknown
or absent outcomes stay held. This is a trusted wrapper's observed event, not a
self-authenticating cleanup flag. Account/private inventory qualification remains
inside the pinned service-UID tool and its existing exactly-two-account gates.

## Acceptance and limits

Before implementation dispatch, independent read-only design review accepts the
contract/API/authority/file split. Before merge, use actual producer->Python
adapter->Go consumer fixtures, original v1/v2 regressions, source adoption/reissue/
revoke, current-pair drift, expiry, authority drift/live writers, unsupported
sidecars, unknown guards, finite child drain and interrupted entry/closeout at
record/rename/fsync/unit boundaries. Include exact preservation of state/pair,
no old binary/data resurrection and no capture change. Linux root/service-UID/
FD and systemd checks must be labeled separately from mocked runner checks.
Versioned source tests never stand in for live authority or installed acceptance.

Fixed operation records include `manifest.json`, `runtime.py`,
`maintenance-ready.json`, `closeout-pending.json`, `closed.json`, original
baseline guard snapshots and the already specified consumed-nonce/outcome files.
Any temporary atomic-publication names are fixed, operation-owned, content-free
and explicitly reconciled; they never authorize removal of unknown artifacts.
All final records publish atomically after sync; actual filesystem/unit state
and exact known record bytes determine forward recovery.

Outcome ordering refinement (independently reviewed before implementation):
`sequence` is a positive integer (not bool), allocated under the existing EX
release lock. All prior consumed nonces, saved proofs and terminal outcomes must
form an exact bijection with unique contiguous sequences1..N; allocateN+1 only
then. Missing, uncertain or otherwise unreconciled outcomes BLOCK a new child
and sequence allocation. Closeout requires known successful outcomes and final
highest-sequence revoke. Wallclock completion time and nonce ordering never
establish event order. No implicit reconciliation fills a missing success.

The v3 wrapper exclusively saves exact content-free proof bytes as
`fixture-proof-<nonce>.json` before consumption; outcome publication may use only
its fixed `fixture-outcome-<nonce>.json.pending` stage with deterministic owned
recovery, never an arbitrary cleanup. A v3 revoke child additionally qualifies
its private directory as exactly private manifest.json/fixture.lock, exactly two
complete retained account/brain identities with no pending/key bytes and exact
current source/target, no unknown credential rows and all recorded credentials
revoked. Only its successful service-UID tool exit after close/checks can seal
a successful revoke outcome. Root never reads keys or queries SQLite itself.
The existing capacity-one authority is this source cohort's only accepted
restore_concurrency; another capacity needs its own qualification, not an input
that silently expands this operational envelope.

Reviewed recovery refinements (MAINTOWN-R1/R2/R3): a matching published hold
without readiness uses the same strictly verified no-child abort boundary as
pre-hold interruption. Late closeout resumes must account for supervisor-exit
containment stopping normal units: a durable `reholding` phase, exact saved
release identity and fresh authority precede republication of the original hold,
owned guards, held permits, restart and actual readiness. Missing hold never
implies completion.

Before auth-bearing progress records, publish immutable content-addressed
admission snapshots. Pending progress staging names bind their expected canonical
record hash (or exact prior/phase/admission reconstruction tuple), so zero-byte
and partial writes remain uniquely recoverable after renewal. Reconstruct only
bounded finite legal transitions from exact durable records and complete
admissions under FRESH authority; completing old bytes never executes an expired
grant. Incomplete admission stages remain retained, ineligible evidence; a fresh
complete admission may proceed only when no dependent progress binds the
incomplete one. Unknown bytes are never truncated or silently deleted.
