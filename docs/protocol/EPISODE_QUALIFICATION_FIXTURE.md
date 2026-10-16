# Offline episode qualification fixture

This is a separate operator tool, not a hosted endpoint or authentication bypass.
It creates exactly two reserved `episode-<random-namespace>-1/2@serenity-episode-fixture.invalid`
accounts through the existing Store, one normal default brain each through the
Provisioner, and normal brain-scoped `memory:read,memory:write` credentials through
the Issuer. Free plans and ordinary quotas remain unchanged. It sends no email
and makes no email-verification assertion. Hosted-load's local-only guard is
unchanged. This package is not linked into the hosted binary.

**Source candidate only.** Parent must test and independently review the adapter,
child, and actual provider-proof producer together before invocation. No tests,
build, provider calls, live provisioning or deployment were run by this lane.
T3.7 is not production-qualified; fixture preparation does not complete T3.3.
Capture remains disabled.

## Authority boundary

`offline_operator.py` consumes an existing private root-owned proof packet. It
never generates provider evidence, reads credentials, creates a maintenance hold,
changes a service, issues a startup permit, repairs permissions, or calls a
provider. Its sole CLI input is the absolute proof-file path; there is no
maintenance boolean, environment override, test root, or fake-systemd CLI switch.
The reviewed parent launcher must use `/usr/bin/python3 -I` (isolated Python)
with a trusted minimal environment; the wrapper refuses non-isolated execution.

The parent first stages the established reviewed release-runtime transaction:
`maintenance.hold.json` phase `maintenance-ready`. That phase means the runtime
stopped writers and revoked permits. Parent then independently verifies the
provider session is authoritative **CLOSED**, lease-free, with concurrency zero,
and records fresh root-owned evidence bound to this exact hold/state/boot. The
provider producer must remain separately reviewed: root custody and a recorded
SHA do not themselves prove that a provider API was called correctly.

The adapter consumes that authority under the existing
`/var/lib/serenity-release/state.json.lock` flock. It retains the lock until the
child has exited and final checks finish. It verifies:

- Exact root-owned proof, hold, state, readiness receipt, deployed activation
  runtime, wrapper, child executable, installed service executable, provider
  receipt and producer bytes. Paths, modes, owners, link counts, size bounds,
  descriptor snapshots and hashes are checked. The existing runtime is loaded
  only from its verified bytes; no cached bytecode is loaded or written.
- Hold phase and every hold field, unchanged previous state, exact runtime
  `CONFIG` pins, default-off hosted configuration, explicit positive account cap,
  service UID/GID, data directory, boot ID and reviewed source commits.
- Root-owned persistent `90-serenity-release-guard.conf` and transaction
  `91-serenity-release-maintenance.conf` bytes, effective writer ExecCondition,
  read as the typed D-Bus `org.freedesktop.systemd1.Service.ExecCondition`
  property (`a(sasbttttuii)`) through `/usr/bin/busctl --system --json=short`.
  The pinned runtime's `SystemdBus` holds a `RefUnit` reference across the
  property read and releases it afterward. Exactly one command is allowed:
  executable `/usr/bin/python3`, argv exactly `[/usr/bin/python3,
  <pinned-runtime-path>, guard, <exact-unit>]`, and `ignore_errors=false`.
  Empty/multiple commands, display strings, wrapper arguments, ignored errors,
  unsupported JSON/type representations or unavailable bus access refuse.
  This requires busctl JSON support and parent verification against the actual
  host's read-only typed property receipt. The adapter also checks
  supervisor binding and Restart=no. Backup/timer conditions use the established
  runtime's typed systemd reader: the hold condition must be mandatory and
  negated, not positive or a trigger condition.
- Known loaded inactive/dead hosted, broker, backup and backup timer units; no
  start/stop job, PID/control PID or leftover cgroup process; no pending daemon
  reload, startup permits, pending release state or pending closeout.
- Root-only one-use proof consumption. A consumed packet cannot be rerun, even
  when a child fails. Recovery requires fresh provider evidence/packet/nonce.

The adapter starts the exact hashed child through Python subprocess `user`,
`group`, empty supplementary groups and `pass_fds`, as the existing `serenity`
service account. It never changes UID inside multithreaded Go. Standard input,
output and error are suppressed for the child; environment is minimal and Git
cannot load global/system user configuration. No Keychain or config-auth access
changes occur. The wrapper emits only constant content-free status text.

Inherited descriptors are, in order: root proof, hold, state, hosted config,
release lock, and a separate open description of that lock for contention
verification. The Go child checks root/private regular-file custody, read-only
proof descriptors, exact bytes, its own `/proc/self/exe` SHA, compiled source
revision, actual service UID/GID, boot, expiry and already-held lock inode. It
marks inherited descriptors close-on-exec before any Git subprocess. It validates
custody before subsequent mutations. Direct invocation without those descriptors
refuses. Non-Linux execution refuses.

The wrapper rechecks pinned paths and system state while the child runs and after
success, with a 120-second child bound and 180-second wrapper alarm. The child has
an at-most-110-second context bounded further by proof expiry. Failure retains the
maintenance hold and synthetic state for reconciliation; it never opens serving.
These checks fence cooperating reviewed release operations through the shared
lock and persistent startup guards. They do not claim to defeat a malicious root
operator. Root SIGKILL, power loss or uncertain subprocess state requires parent
reconciliation under the retained hold before any retry/start.

## Exact proof contract (version 1)

The strict JSON field set is defined in `offline_operator.py:PROOF_FIELDS` and
`custody_linux.go:custodyProof`. Unknown or duplicate wrapper JSON fields refuse.
All SHA256 values are lowercase 64-character hex; source commits are full
40-character lowercase hex. Paths are absolute/canonical, without whitespace.
Proof and provider receipt are private root-owned single-link regular files in a
private root directory. No plaintext credentials belong in either.

| Fields | Required binding |
| --- | --- |
| `version`, `purpose`, `operation` | `1`, `serenity-episode-fixture-offline-v1`, one of `prepare/reissue/revoke` |
| `nonce`, `issued_at`, `expires_at`, `boot_id` | Unique 256-bit nonce; integer UTC seconds; issued already, expiry in future, lifetime at most 300 seconds; current boot |
| `uid`, `gid` | Actual non-root existing serenity service account and primary group |
| `tool_source`, `tool_path`, `tool_sha256` | Exact reviewed child commit, immutable executable path and bytes; source is compiled into `main.sourceRevision` |
| `wrapper_path`, `wrapper_sha256` | Exact running reviewed wrapper file and bytes |
| `runtime_path`, `runtime_sha256` | Existing staged reviewed activation-runtime file and exact bytes, including CONFIG |
| `service_source`, `service_binary_path`, `service_binary_sha256` | Runtime CONFIG previous commit/executable; exact installed service target and `/usr/local/bin/serenity` link |
| `config_path`, `config_sha256`, `data_dir`, `account_cap` | Exact `/etc/serenity/hosted.json` bytes/data/cap; archive false and synthetic allowlist empty; absent/zero cap refuses |
| `private_dir` | Pre-created private service-owned output directory outside brains; never transferred |
| `hold`, `hold_sha256`, `state_sha256`, `ready_sha256` | Full exact hold object and bytes, unchanged previous state, staged maintenance-ready receipt |
| `lock_device`, `lock_inode` | Existing root release-state lock device/inode, not a replacement lock |
| `schema_sha256` | Reviewed supported schema-10 metadata fingerprint, independently matched to current DB |
| `authority_scope`, `authority_session_id`, `concurrency_target` | Exact reviewed provider scope/session and execution resource; receipt must match |
| `authority_receipt_path`, `authority_receipt_sha256` | Exact fresh root provider receipt bytes |
| `authority_generator_path`, `authority_generator_sha256` | Exact independently reviewed provider-evidence producer source, root-owned |

`hold` has exactly the established fields: `manifest_sha256`,
`executable_sha256`, `source`, `previous_state_sha256`, `expected_state_sha256`,
`supervisor`, `stage`, `backup_timer_prior_state`, `phase`, `timestamp`. Its phase
is `maintenance-ready`, timer prior state `active`, and source/manifest/executable/
states/supervisor/stage must match deployed runtime CONFIG. The runtime path is
`<stage>/activation-runtime.py`; current state matches `previous_state_sha256`.
The root readiness receipt must say `installed_pair=previous-unmodified`,
`held=true`, `services_started=false` and bind the same manifest/current state.
A completed deployment receipt is not interchangeable with this staged hold.

The separate authority receipt has exactly `version=1`, `observed_at`,
`status=CLOSED`, integer `active_leases=0`, integer `concurrency=0`, nonempty
`session_id`, `scope`, `concurrency_target` matching the proof, matching `boot_id`, `hold_sha256`, `state_sha256`, and 1–8
nonempty provider `request_ids`. Observation must be no more than 60 seconds
before proof issuance. Parent must obtain these facts from authoritative provider
responses, not simply write the expected values. A changed source/hold/current
state requires a correspondingly reviewed fresh packet, not edited historical
receipts. No fresh operational values are invented in this source packet.

### Schema and ownership before opening Store

The root wrapper checks service-owned private regular `control.db`, WAL and SHM
paths without opening SQLite as root. The service-UID child opens the existing DB
with `mode=ro`, requires exactly versions 1–10, the complete expected schema-10
object inventory, and the parent-reviewed schema fingerprint. Only then may it
call `Store.Open`, which would otherwise initialize/migrate storage. Older,
newer, missing, inconsistent or unsupported schemas refuse; nothing is migrated.

Fingerprint algorithm: select `type,name,tbl_name,coalesce(sql,'')` from
`sqlite_master`, excluding names matching `sqlite_%`, ordered by `type,name`.
Feed each UTF-8 field followed by one NUL byte to SHA256. This reads schema
metadata only. Parent must compare against the supported source schema, not bless
an arbitrary observed fingerprint. Any producer-side SQLite access must also use
the service UID/read-only mode so WAL/SHM ownership remains correct.

## Fixture custody and recovery

Manifest version 2 uses the dedicated suffix above; version-1 manifests refuse.
The private 0600 manifest binds namespace, exact account/brain/credential IDs,
configuration/source/data/cap and interruption hold epoch. Atomic replacement,
file fsync and directory fsync precede mutations. A private stable fixture flock
serializes operations; unsafe or unexplained private-directory state refuses.

Both identities are checked before either is changed. Foreign/colliding names,
IDs, credentials, extra brains, changed plans, browser/WorkOS/OAuth/partner/billing
adoption, symlinks, hardlinks and unsafe ownership/modes refuse. Queries inspect
only fixture identities, schema metadata and minimal counts. No customer SQL
writes or DB/config/credential dumps occur. All active accounts, including
partner accounts, count conservatively against the actual cap. Quota and
entitlement rows are not synthesized or raised.

- Persist reserved-address `account` intent before `Store.CreateAccount`. It
  generates the account ID internally; recovery may bind its same pristine row
  only under the prior valid manifest and **same exact maintenance hold bytes**.
- Persist a brain ID and `brain` intent before `Store.InsertBrain` allocates the
  row; normal `Provisioner.Provision` finishes that exact default brain under
  writer ownership. Never run global `Provisioner.Recover` on customer storage.
- Persist `key` intent and known credential IDs before `Issuer.Issue`. Exactly
  one matching additional credential under that same hold is an interrupted
  orphan. Record/revoke it through `Issuer.RevokeKey` before replacement. Unknown
  multiple/differently bound credentials refuse. Changed hold with pending
  intent refuses rather than inferring custody across a serving interval.
- Clear persisted plaintext key and record intent before reissue/revoke. Thus a
  crash after revocation does not require an already-revoked key to replay.
  No broad brain revoke or OAuth epoch mutation occurs.

`prepare` replay preserves valid keys. `reissue` replaces only owned keys.
`revoke` clears/revokes owned credentials while retaining synthetic accounts,
brain data and revoked credential rows inert; it is not physical purge. Partial
preparation remains recoverable under its same hold. A fully bound clean/inert
manifest can be reused under a new independently reviewed hold. Lost/corrupt
custody or a pending intent from another hold requires explicit reconciliation.
Crash-left private temporary files may retain old revoked key bytes; no secure
erasure is claimed. Keys never leave the private host manifest or enter logs.

## Parent verification and integration obligations

On an existing authorized isolated DGX/Linux worker, with Git, Go 1.26.9 and
private temporary storage:

```sh
GOMAXPROCS=2 go test -p 2 -count=1 -timeout 180s ./evals/episode-qualification
GOMAXPROCS=2 go test -race -p 2 -count=1 -timeout 180s ./evals/episode-qualification
GOMAXPROCS=2 go vet -p 2 ./evals/episode-qualification
python3 -m unittest discover -s evals/episode-qualification -p 'test_operator.py' -v
```

Parent builds the exact reviewed child source with
`-ldflags "-X main.sourceRevision=<exact-reviewed-commit>"`, hashes the result,
and pins it in the reviewed proof. The default unpinned build refuses. No
live CLI recipe or valid operational proof is supplied here.

Tests use temporary synthetic Store/Provisioner/Issuer instances and a filesystem/
systemd model. They cover provision/replay/interruption/revoke, caps and unrelated
rows, path privacy, same-hold recovery, custody loss, read-only schema refusal,
proof freshness/binding, file replacement, lock contention, permits, effective
conditions and running/unknown services. Model tests do not establish actual
systemd/provider behavior, service-UID launching, deployment or qualification.

Still required: fresh exact operational bindings and source-schema comparison;
provider-proof producer review; adapter/child review; DGX execution receipts;
actual Linux systemd/UID/FD qualification under authorized maintenance; verified
retained-state cleanup; separate signed release/fallback and HTTP qualification.
Parent owns these, merge and landed verification. Full real capture stays off.
