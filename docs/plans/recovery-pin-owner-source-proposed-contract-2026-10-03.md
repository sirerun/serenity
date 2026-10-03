# Recovery snapshot-pin owner: proposed narrow source contract

**Status:** proposal for independent review only. This is a narrow durable owner for backup snapshot-pin lifecycle records. It does not implement or qualify the full recovery plan store, READY envelope, approval, journal, epoch, provider, startup, or admission authority. The full recovery plan-store proposal `618c224b0f962ffefff05555c30ebc34971cd8d7` remains HOLD. No Go source assignment, source freeze, production readiness, READY, full-store CLEAR, or hosted acceptance follows from this document.

This proposal derives its scope from the accepted recovery artifact-to-contract mapping proposal `2c26c6c04cdf274b1d8cf6b658c976053869fbeb`, the accepted verified snapshot producer contract `6a013b49b94e7f9286b782ad2827441d2e5bd737`, and the coordinator-frozen `backup-pin-absence-v1` amendment `9823cfa6c13caa768a47a7a2ef8bd8b6220b6a02`. The integrated producer at `e67b5fb` supplies the exact current API surface described below. The base producer approval and absence amendment remain component-scoped; neither makes this owner or the whole recovery path ready.

## Ownership boundary

The recovery package owns one private, durable source of truth for reservation identity, backup pin-attempt history, pin commitment, cancellation tombstones, pre-effect abandonment, and exact pin-release completion. It implements `backup.SnapshotPinLifecycleAuthority`. It may reserve a plan reference and bind the exact lease and manifest digest, but this slice has no method that creates or persists READY bytes, accepts approval, starts effects, marks a restore committed, or proves runtime admission.

The eventual full recovery store must extend and reuse this same store UUID, owner root, superblock, per-reservation append-only history, versions, pin records, cancellation tombstones, and release records. It must not add a second recovery pin owner, parallel pin database, sidecar reservation namespace, or migration that drops old attempt history. Future full-store records use the same versioned record envelope and owner CAS sequence. Any incompatible wire change requires a separately reviewed migration and cannot silently rebind pins.

The owner accepts only backup-owned opaque types. It never duplicates backup lease files or manufactures `SnapshotPinRef`/`VerifiedPinAbsence`; it never accepts a caller boolean, path, JSON proof, digest-shaped string, evidence reference, or test verifier as authority. Tests may use package-private fixtures. Production constructs only this concrete owner from the owner-controlled root and exact backup store identity.

## Concrete narrow surface and bootstrap

The owner constructor has a mandatory, nonzero identity parameter and no path-only or optional fallback:

```go
func OpenSnapshotPinOwner(
    ctx context.Context,
    options SnapshotPinOwnerOptions,
    expected backup.SnapshotStoreIdentity,
) (*SnapshotPinOwner, error)

func (s *SnapshotPinOwner) ReservePinPlan(
    ctx context.Context,
    operationID string,
) (PinOwnerReservation, error)

func (s *SnapshotPinOwner) MarkAbandonedBeforeEffects(
    ctx context.Context,
    exact PinOwnerReservation,
    reason PinAbandonReason,
) (PinOwnerReservation, error)
```

`SnapshotPinOwner` implements the integrated producer interface exactly:

```go
type SnapshotPinLifecycleAuthority interface {
    BeginPinAttempt(ctx context.Context, planRef string, reservationVersion uint64, leaseID, manifestSHA256 string) (PinAttemptRef, error)
    CommitPinAttempt(ctx context.Context, attempt PinAttemptRef, pin SnapshotPinRef) error
    CancelPinAttempt(ctx context.Context, attempt PinAttemptRef, proof VerifiedPinAbsence) error
    FindPinAttempt(ctx context.Context, planRef, manifestSHA256 string) (PinAttemptRef, error)
    ListPinAttempts(ctx context.Context) ([]PinAttemptRef, error)
    ReconcilePin(ctx context.Context, pin SnapshotPinRef) (PinReconcileDecision, error)
    CompletePinRelease(ctx context.Context, authorization PinReleaseAuthorization) error
}
```

The production factory ordering is mandatory:

1. Resolve the backup root from owner-controlled configuration and call `backup.PreflightSnapshotStoreIdentity(ctx, backupOptions)`.
2. Open the recovery pin owner with that exact opaque token. Before it serves methods, the owner creates or validates its own private root, stable lock, store UUID, and a durable binding to the backup root and stable lock device/inode observed through no-follow private filesystem checks. The opaque token itself is held only as a typed in-memory value and is never serialized, reflected, or treated as a public identity format.
3. Construct the backup store using the required fourth argument: `backup.NewSnapshotLeaseStore(ctx, backupOptions, owner, expectedIdentity)`. This constructor must succeed against the same preflight token and current root/lock objects before the factory publishes either component to callers. Any mismatch, replaced root or lock, zero token, cancellation, or constructor error makes the entire factory unavailable; there is no path-based or three-argument fallback and no automatic rebinding of an existing owner.
4. Pass the same token value held by the owner to the backup store. On every owner restart, preflight again, compare the observed backup root/lock identity to the durable binding, reopen the owner, and require the backup constructor's identity recheck before enabling operations.

The backup token is intentionally opaque and has no wire encoding. The durable binding records only filesystem object identity needed to reject replacement; `VerifiedPinAbsence.Consume(exact, expected)` receives the retained typed token and requires the same token state. The factory must not persist the token's Go memory representation. If source review determines that the producer API cannot prove the tuple written by the owner matches the opaque token, add a narrowly reviewed backup-owned binding verifier before source freeze; do not weaken opacity or infer equality from a caller-supplied path.

The owner root and backup snapshot root are distinct identities. Each root has a stable regular `.store.lock` inode and device binding. The recovery owner stores its own root identity and lock identity in its superblock; its backup binding stores the backup root/lock identity. Neither identity is a UUID substitute for the other. Lock acquisition is cross-process and context-aware. Revalidate both roots and both stable lock objects before blocking, after lock acquisition, before every mutation, and after durable publication. A lock-path replacement while waiting fails closed.

## Durable states and exact lifecycle behavior

The narrow store recognizes the following owner states: `RESERVED`, `PIN_PENDING`, `PINNED`, `ABANDONED_BEFORE_EFFECTS`, `RELEASING`, and `RELEASED`. Its decoder also recognizes the later full-store state names `READY`, `APPLYING`, `FORWARD_RESUMABLE`, `COMMITTED`, and `ADMISSION_CONFIRMED` so those records remain part of one future history; this narrow implementation treats them as active/unsupported authority states and returns `PinKeep` or a fail-closed error. It cannot create any of them.

| Current owner state | Exact operation | Durable result |
| --- | --- | --- |
| absent | `ReservePinPlan(operationID)` | One random 64-lowercase-hex `planRef`, one nonzero immutable `reservationVersion`, owner record version 1, and reservation headroom committed. Exact lost-response retry with the same operation binding returns the same reservation. |
| `RESERVED` | `BeginPinAttempt` for a fresh lease ID and exact manifest digest | Append `PIN_PENDING`; exact tuple `(planRef, reservationVersion, leaseID, digest)` receives one nonzero `attemptVersion`. This CAS must be durable before backup writes its pin file. |
| `PIN_PENDING` | exact `BeginPinAttempt` retry | Return the same PENDING tuple and versions without appending or changing any version. A changed tuple conflicts. |
| `PIN_PENDING` | `CommitPinAttempt(exactAttempt, exactPin)` | Verify every attempt field and all pin fields; append COMMITTED and move owner state to `PINNED`. This call follows backup pin-file and containing-directory fsync. Exact retry is idempotent; mismatch is refused. |
| `PIN_PENDING` | `CancelPinAttempt(exactAttempt, proof)` | Under owner CAS lock, verify the complete current tuple, then consume the exact callback-scoped `VerifiedPinAbsence` with the retained `SnapshotStoreIdentity`. Append permanent CANCELED tombstone; return owner state to `RESERVED`. No cancellation can succeed after a durable producer pin exists. |
| `RESERVED` or `PINNED` | `MarkAbandonedBeforeEffects(exactReservation, reason)` | Only a fresh owner record-version CAS may append terminal `ABANDONED_BEFORE_EFFECTS`, and only when complete history contains no READY/effect marker, no unknown state, and no conflicting live attempt. No caller boolean or caller-created proof is accepted. |
| pinned active state | `ReconcilePin(exactPin)` | Return `PinKeep` with zero authorization. Unknown state, mismatch, corrupt history, or incomplete inventory returns an error and never authorizes cleanup. |
| exact `ABANDONED_BEFORE_EFFECTS` | `ReconcilePin(exactPin)` | Return `PinRelease` for `PinAbandonedBeforeEffects`, bound to exact pin ID, plan ref, digest, and owner record version. This owner never returns `PinCommittedRestoreComplete`. |
| `ABANDONED_BEFORE_EFFECTS` | backup `RELEASING` then `RELEASED`; owner `CompletePinRelease(exactAuth)` | Accept completion only for the exact release authorization and expected owner version. Append or acknowledge the exact `RELEASED` tombstone. Lost responses are resolved by rereading this same record, never minting another authorization. |

Attempt history is permanent for the lifetime of the reservation. Each attempt is one of `PENDING`, `COMMITTED`, or `CANCELED`; public `PinAttemptRef.State` maps only PENDING and COMMITTED, as the integrated producer API defines. CANCELED exists only in owner history and is never emitted by `ListPinAttempts` as a live attempt.

`FindPinAttempt(planRef, digest)` returns the exact current attempt for that reservation and digest. If the latest matching attempt was canceled, it returns conflict, not not-found, so a caller cannot mistake cancellation for authority to retry the same tuple. `BeginPinAttempt` for any tombstoned tuple always refuses. An intentional N+1 is allowed only while the reservation remains active and only after explicit backup `Stage` supplies a fresh lease ID under the same expected digest. AttemptVersion increases exactly once; delayed Cancel(N), Begin(N), Resume(N), or commit(N) cannot mutate N+1. Terminal reservations reject all future attempts.

`ListPinAttempts` validates the entire bounded owner history before returning anything. It returns every current PENDING and COMMITTED attempt in stable lexicographic `(PlanRef, ReservationVersion, AttemptVersion, LeaseID)` order. Duplicate plan refs, duplicate attempt tuples, unknown enum values, state/pin inconsistencies, malformed IDs/digests, a gap/fork/checksum mismatch, or any unreadable reservation fails the complete call. It never returns a partial list. Canceled tombstones remain in history even though they are omitted from the live list.

`CommitPinAttempt` requires the current PENDING row to match every `PinAttemptRef` field, including planRef, reservationVersion, attemptVersion, leaseID, digest and PENDING state. It then checks every field actually exposed by `SnapshotPinRef`: pin ID, planRef, digest and reservationVersion. The attemptVersion and leaseID remain bound by the exact pending attempt row because the producer's `SnapshotPinRef` intentionally does not expose those fields. Only the unique current PENDING attempt may commit. Exact retry after a lost response returns success after verifying the committed pin; a different pin, attempt or reservation is conflict. No hash-shaped value or raw snapshot path proves that a backup pin was fsynced.

`ReconcilePin` validates the complete reservation history and exact pin tuple before deciding. It returns `PinKeep` for RESERVED/PENDING/PINNED, every future live/unknown consumer phase, and every incomplete/corrupt state. It may release only an exact pin whose complete current history durably says `ABANDONED_BEFORE_EFFECTS`. `COMMITTED` means only that the backup snapshot pin was durably committed; it never means the restore or epoch is committed. Since this owner does not contain a concrete epoch/admission proof owner, `PinCommittedRestoreComplete` always yields `PinKeep`/error. It cannot synthesize a full-restore release.

The current producer's `PinReleaseAuthorization` exposes pin ID, plan ref, manifest digest, disposition, and owner recordVersion; it does not expose reservationVersion or attemptVersion. Therefore the owner must validate that this exact recordVersion is the terminal record for the unique current pin row whose complete prior history contains the matching reservationVersion, attemptVersion, lease ID, digest, and pin ID. If more than one history row could match, if the terminal record is not linked to that exact prior digest, or if any tuple field conflicts, return `PinKeep`/error. The owner never reconstructs a weaker “same plan and digest” release from authorization fields alone.

The recovery owner must implement the missing full-store `PINNED → ABANDONED_BEFORE_EFFECTS` terminal transition before backup `Close`/`Reconcile` can clean those bytes. The transition requires the exact fresh owner record version and a complete validated history with no READY state or effects marker. It is never inferred from a missing READY file, caller intent, age, failed downstream operation, or absence of a provider response. If a future full store can create READY/effects, it must independently verify its typed no-effects proof before that transition; this narrow store has no API for doing so and therefore refuses such a transition when any READY/effect/future consumer record exists.

Backup `Close` only closes a handle for PINNED, PIN_PENDING, READY, or other protected records. It does not delete bytes based on a stale cached state. The recovery owner exposes explicit terminal abandonment and exact reconciliation; `Close` does not itself assert absence of READY/effects or turn a missing pin into authorization. A valid pending-without-pin attempt can be canceled only through the backup's locked absence callback and one-shot proof consumption.

## Cancellation proof and retry boundary

`CancelPinAttempt(ctx, exact, proof)` verifies the exact durable PENDING attempt and current reservation under the recovery owner's own cross-process CAS lock before calling `proof.Consume(exact, expectedIdentity)`. It consumes only after every owner-side tuple/state check and before writing the tombstone. `Consume` is inside the synchronous backup callback while the backup store and lease locks still pin their verified inodes. All value aliases share the producer's one-shot state; at most one consume succeeds, and every alias expires on callback return, including context error, owner error, or panic cleanup.

The cancellation tuple is exactly `(planRef, reservationVersion, leaseID, manifestSHA256, attemptVersion)`. The permanent tombstone is committed before cancellation returns. On a lost response, a retry obtains a fresh backup absence callback/proof for the same tuple; the owner consumes that fresh proof, observes the existing tombstone, and acknowledges without changing the current reservation or attempt. If N+1 exists, this old retry cannot change N+1. No record is removed or overwritten to “undo” cancellation. A serialized, zero, stale, wrong-store, cross-lock, mismatched, expired, already-consumed, or alias-replayed proof fails closed. There is no public proof constructor or identity getter.

Context is checked on entry, during bounded history loading, before lock acquisition, after blocking, before proof consumption, before each write/sync/rename, and between records. Cancellation before any possible commit returns the context error without mutation. After a write may have become durable, the owner rereads exact history under lock: it returns idempotent success only when that exact requested transition is present and verified; otherwise it returns an outcome-uncertain error that names no replacement tuple. It never reports rollback merely because the caller's context expired after fsync.

## Canonical durable wire and bounds

The owner root has one checksummed `superblock.json`, one stable regular `.store.lock`, and per-reservation immutable records at `reservations/<planRef>/<20-digit-recordVersion>.json`. The superblock v1 payload uses this exact order: `version`, `store_id`, `owner_root_device`, `owner_root_inode`, `owner_lock_device`, `owner_lock_inode`, `backup_root_device`, `backup_root_inode`, `backup_lock_device`, `backup_lock_inode`. All identity values are canonical unsigned decimal integers. Its checksum is SHA-256 over `"serenity.recovery.snapshot-pin-owner-superblock.v1\0"` plus compact JSON of those fields, and `checksum_sha256` is appended last. Its parser has the same strict duplicate/unknown/trailing/canonical checks as record loading.

Superblock golden vector:

```json
{"version":1,"store_id":"11111111111111111111111111111111","owner_root_device":10,"owner_root_inode":20,"owner_lock_device":10,"owner_lock_inode":21,"backup_root_device":10,"backup_root_inode":30,"backup_lock_device":10,"backup_lock_inode":31,"checksum_sha256":"de4868ebd763e062c2effd459f51d35dd0b5eb56d92af560a027aa3f143cddae"}
```

A rebuildable index may accelerate lookup but is not authority. Per-reservation history from version 1 through its exact head is authoritative; any gap, duplicate, fork, unexpected regular file, missing head record, or index/history mismatch either rebuilds only the index from the validated history or makes the affected store unavailable. Unknown files are never deleted by name or guessed prefix.

Wire version 1 is one compact UTF-8 JSON object, no leading/trailing whitespace or newline, with fixed struct field order, all fields present, no `null`, no duplicate/unknown keys, no floating-point numbers, canonical decimal unsigned integers, lowercase ASCII enum strings, exact lowercase 32-hex `store_id`, exact lowercase 64-hex plan/lease/pin/digest/hash values, and bounded operation IDs matching `[A-Za-z0-9._:-]{1,128}`. The payload order is:

```text
version, store_id, plan_ref, reservation_version, record_version,
previous_sha256, operation_id, event, state, attempt_high_water,
attempt_version, lease_id, manifest_sha256, attempt_state, pin_id,
release_disposition, release_record_version, reason_code
```

No map is serialized. The checksum is SHA-256 over `"serenity.recovery.snapshot-pin-owner.v1\0"` followed by the exact compact JSON encoding of those payload fields in the listed order, without the checksum field. The final record appends `checksum_sha256` last. The previous digest is 64 zeroes only for a reservation's first record; every later record binds the digest of the exact previous canonical full record. The parser rejects noncanonical re-encoding even when the JSON meaning is equivalent.

Golden vector for a committed pin record (the prior digest is fixture data; it stands for the preceding exact PENDING record):

```json
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":7,"record_version":3,"previous_sha256":"3333333333333333333333333333333333333333333333333333333333333333","operation_id":"op-001","event":"PIN_COMMITTED","state":"PINNED","attempt_high_water":1,"attempt_version":1,"lease_id":"4444444444444444444444444444444444444444444444444444444444444444","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"6666666666666666666666666666666666666666666666666666666666666666","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"4fe1e7968cc68d6703c8024aad1d9aab00835c9b1c420e5e26f1fb153cdd90bb"}
```

An independent implementation test must reproduce the exact payload bytes and checksum above, then reject reordered fields, extra whitespace, trailing bytes, duplicate/unknown fields, uppercase hex, leading-zero numbers, `null`, changed digest, wrong predecessor, unknown state/event, and every truncated prefix. Additional golden vectors cover the superblock, genesis reservation, pending pin, canceled tuple N, fresh tuple N+1, pre-effect abandonment, release authorization, RELEASING, RELEASED, and future full-store READY/effect state records. Hash tests prove each authority-bearing field changes the digest and checksum is excluded from its own input.

Hard narrow-store bounds (all checked before allocation, decode, append, or hash) are:

- One record and one superblock at most 4 KiB each; one reservation history at most 256 records; at most 32 attempts per reservation including canceled tombstones.
- `MaxPlans` must be positive and no greater than 256. Aggregate owner metadata limit must be positive and no greater than 256 MiB. These are ceilings, not physical quota evidence.
- The maximum live-attempt response is 4096 refs, matching the producer ceiling. The whole response is validated and buffered within that bound; overflow or truncation returns an error with no partial list.
- Every integer is checked for overflow. Counts and record lengths are read/bounded before JSON decoding. No zero-as-unlimited, silent truncation, or default temp path is allowed.

Before creating a reservation, reserve logical space for its entire permitted owner-history headroom (`256 × 4 KiB`) plus its superblock/index share. Before every event, charge the exact new record and temporary metadata against the aggregate cap while retaining enough reserved headroom for required pin-commit/cancel and terminal release records. A mutation that cannot guarantee its required next lifecycle records refuses before changing state. In-progress temp records are charged until exact atomic publication or verified cleanup. Canceled tuples and RELEASED owner records are not compacted or dropped to free capacity. This is logical accounting only: it is not a filesystem quota, expanded Git/checkout bound, SQLite/WAL/index bound, destination output limit, or physical capacity guarantee.

## Atomic filesystem and crash protocol

The configured owner root must be absolute, canonical, private, permission-enforced, owner-only and on the approved filesystem. Every component is opened no-follow with `os.Root`/the repository's `privatefs` policy; reject symlinks, nonregular lock/record files, hard-link/replacement changes, foreign owner, widened permissions, device changes, path traversal, and filesystem types outside the configured support matrix. Create a missing root/lock only through the explicit preflight/factory path with exclusive creation, fsync the new lock, then fsync the parent. Never replace an existing lock. Capture root and stable-lock device/inode. Revalidate them before and after blocking on the interprocess lock and before and after every mutation. Propagate unlock, close, write, fsync, rename, and directory-sync errors.

For each CAS: load and validate the complete bounded history under the stable interprocess lock; compare exact expected versions and tuples; encode the canonical next record; verify aggregate reservation; create a unique same-root temporary regular file with no-follow/exclusive semantics; write all bytes; fsync file; verify the captured temp inode; atomically publish without replacement at the exact next-version name; fsync the reservation directory and every affected parent/root; then update any rebuildable index and fsync its directory. Return success only after required syncs. Reopen the published record and verify its checksum and exact predecessor before acknowledging. Before any backup pin-file write, the `PIN_PENDING` history entry and all containing directory entries are already durable.

Startup and `ListPinAttempts` scan and validate all retained histories under the owner lock. A temp file may be removed only if its exact captured inode is still present, its reservation path/root identity is still the captured owner root, and no committed history record references it. Unknown or replaced entries remain untouched and force refusal. A crash or error after a possible publication is resolved by rereading history; no cleanup is based on filename prefix alone.

Required child-process crash injection points: before/after temp creation, partial write, complete write, file fsync, no-replace publish, reservation-directory fsync, root-directory fsync, index write/sync, owner `PIN_PENDING` durable boundary, backup pin-file write/fsync/root sync, owner `PINNED` commit, proof consumption, cancellation tombstone publish/sync, abandonment CAS, backup `RELEASING`, exact lease-byte deletion, backup `RELEASED` tombstone, and owner `CompletePinRelease`. On restart, every prefix either reconstructs the same exact idempotent transition or refuses with the pin retained. No guessed rollback, rebind, new attempt, or duplicate release is allowed.

## Error and retry contract

The recovery package exports stable sentinels `ErrPinOwnerInvalid`, `ErrPinOwnerNotFound`, `ErrPinOwnerConflict`, `ErrPinOwnerLimit`, `ErrPinOwnerCorrupt`, `ErrPinOwnerUnavailable`, and `ErrPinOwnerOutcomeUnknown`. Lifecycle errors also wrap the matching backup sentinels where applicable, so backup callers can use `errors.Is` without parsing strings. Retain underlying OS errors for diagnosis:

| Condition | Result |
| --- | --- |
| malformed IDs, digest, versions, enum, noncanonical wire, bad checksum/history | `ErrPinOwnerInvalid` or `ErrPinOwnerCorrupt` (and `backup.ErrSnapshotLeaseInvalid` where crossing the backup interface); no mutation |
| absent plan/current attempt | `ErrPinOwnerNotFound` and `backup.ErrSnapshotLeaseNotFound`; canceled latest tuple is conflict, never absence |
| stale record/reservation/attempt version, changed tuple/pin, duplicate operation binding, tombstoned Begin, terminal reservation | `ErrPinOwnerConflict` and `backup.ErrSnapshotLeaseConflict`; no mutation |
| configured record/history/plan/aggregate/list ceiling reached | `ErrPinOwnerLimit` and `backup.ErrSnapshotLeaseLimit`; no mutation; existing owner bytes retained |
| context canceled before possible publication | exact context error; no mutation |
| I/O/fsync/rename/lock/unlock uncertainty | wrapped OS error or `ErrOutcomeUnknown`; reread exact history on retry, never retarget |
| changed root/lock inode, corrupt/forked history, unknown owner transition or partial list | `ErrPinOwnerUnavailable` or `ErrPinOwnerCorrupt`; fail closed, keep all pins |
| valid live or unsupported terminal state in `ReconcilePin` | `PinKeep` and zero authorization |
| only exact pre-effect abandonment | `PinRelease` with exact `PinAbandonedBeforeEffects` authorization |
| any attempted committed-restore release in this narrow owner | `PinKeep` or fail-closed error; no authorization |

An exact retry is idempotent only for the same operation ID, reservationVersion, attemptVersion, leaseID, digest, pin ID, disposition, and recordVersion. A new record version is created only for a new valid transition. Lost responses are resolved by querying the same tuple and history head. Delayed N operations cannot alter N+1. Record, counter, disk-space, lock, context, or sync uncertainty never justifies inventing a new planRef, attempt, lease, pin, release authorization, or owner incarnation.

## Required independent controls before source freeze

Independent review must clear this narrow owner contract and its relation to the open `618c` full-store proposal before any Go source assignment. Source qualification later requires:

- golden record/superblock bytes and checksums; canonical parser rejection cases; exact field/version/hash domain separation; overflow-before-allocation controls; aggregate headroom exhaustion before first record and at each lifecycle transition;
- fresh and repeated reserve, simultaneous reserve collision, exact PENDING/COMMITTED retries, changed reservation/digest/lease conflicts, `Find` and full bounded `List`, and permanent canceled tuple N with fresh-lease N+1;
- one-shot absence proof exact tuple, wrong owner/root/lock, invalid/zero token, mismatch, alias double consume, cancellation before/after consumption, context cancellation, panic/callback expiry, proof reuse and a delayed Cancel(N) after N+1;
- Stage → Pin → Close → FindPinned/Reopen/Restore and the missing `PINNED → ABANDONED_BEFORE_EFFECTS` transition; no deletion on stale handle state, no pending-without-proof cleanup, no READY/effect state accepted as pre-effect;
- complete-list hostile states, duplicates, conflicting attempts, committed-owner/local-STAGED mismatch, unknown future state and corrupted unrelated history all fail before any removal; only exact valid owner history can authorize cleanup;
- private-root/lock identity replacement before/after blocking, lock waiter across replacement, symlink/FIFO/device/hard-link/nonregular opens, permissions/owner/device changes, every fsync/rename/delete crash edge, ENOSPC, short writes, unlock/close error propagation, and multi-process CAS races;
- at least two compiled behavioral RED controls after successful compilation, including one that disables exact absence-proof consumption/tombstone checks and another that changes release/identity/fail-closed behavior. Compiler, setup, fixture, unrelated panic, missing API or lease-hold errors do not count as behavioral REDs.

Production owner tests cannot substitute for real approval, journal ancestry, epoch history, effect evidence, provider evidence, destination publication, service admission, or owner-controlled physical quota. Those separate owner interfaces and reviews remain open. A passing narrow pin owner would establish only its own durable lifecycle contract; it would not make full recovery-store `618c` CLEAR or permit READY, provider effects, production recovery, deployment, spend, purge, or hosted acceptance.
