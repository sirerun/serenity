# Recovery snapshot-pin owner: proposed narrow source contract

**Status:** proposal for independent review only. This is a narrow durable owner for backup snapshot-pin lifecycle records. It does not implement or qualify the full recovery plan store, READY envelope, approval, journal, epoch, provider, startup, or admission authority. The full recovery plan-store proposal `618c224b0f962ffefff05555c30ebc34971cd8d7` remains HOLD. No Go source assignment, source freeze, production readiness, READY, full-store CLEAR, or hosted acceptance follows from this document.

This proposal derives its scope from the accepted recovery artifact-to-contract mapping proposal `2c26c6c04cdf274b1d8cf6b658c976053869fbeb`, the accepted verified snapshot producer contract `6a013b49b94e7f9286b782ad2827441d2e5bd737`, and the coordinator-frozen `backup-pin-absence-v1` amendment `9823cfa6c13caa768a47a7a2ef8bd8b6220b6a02`. The current landed producer source is main commit `c2d5a5439675fb7a964aadedb1a0a461f0fce386` (PR 355); its Go bytes are the root-qualified producer bytes. The base producer approval and absence amendment remain component-scoped; neither makes this owner or the whole recovery path ready.

## Ownership boundary

The recovery package owns one private, durable source of truth for reservation identity, backup pin-attempt history, pin commitment, cancellation tombstones, pre-effect abandonment, and exact pin-release completion. It implements `backup.SnapshotPinLifecycleAuthority`. It may reserve a plan reference and bind the exact lease and manifest digest, but this slice has no method that creates or persists READY bytes, accepts approval, starts effects, marks a restore committed, or proves runtime admission.

The eventual full recovery store must extend and reuse this same store UUID, owner root, superblock, per-reservation append-only history, versions, pin records, cancellation tombstones, and release records. It must not add a second recovery pin owner, parallel pin database, sidecar reservation namespace, or migration that drops old attempt history. Future full-store records use the same versioned record envelope and owner CAS sequence. Any incompatible wire change requires a separately reviewed migration and cannot silently rebind pins.

The owner accepts only backup-owned opaque types. It never duplicates backup lease files or manufactures `SnapshotPinRef`/`VerifiedPinAbsence`; it never accepts a caller boolean, path, JSON proof, digest-shaped string, evidence reference, or test verifier as authority. Tests may use package-private fixtures. Production constructs only this concrete owner from the owner-controlled root and exact backup store identity.

## Concrete narrow surface and bootstrap

The owner constructor has a mandatory, nonzero identity parameter and no path-only or optional fallback. The exact public owner types are:

```go
type SnapshotPinOwnerOptions struct {
    OwnerRoot string
    BackupLeaseRoot string
    MaxPlans int
    MaxOwnerMetadataBytes int64
}

type PinOwnerState string
const (
    PinOwnerReserved PinOwnerState = "RESERVED"
    PinOwnerPinPending PinOwnerState = "PIN_PENDING"
    PinOwnerPinned PinOwnerState = "PINNED"
    PinOwnerAbandonedBeforeEffects PinOwnerState = "ABANDONED_BEFORE_EFFECTS"
    PinOwnerReleasing PinOwnerState = "RELEASING"
    PinOwnerReleased PinOwnerState = "RELEASED"
)

// Opaque return value. Only this package can construct a nonzero value.
type PinOwnerReservation struct {
    planRef string
    operationID string
    reservationVersion uint64
    recordVersion uint64
    state PinOwnerState
}
func (r PinOwnerReservation) PlanRef() string
func (r PinOwnerReservation) OperationID() string
func (r PinOwnerReservation) ReservationVersion() uint64
func (r PinOwnerReservation) RecordVersion() uint64
func (r PinOwnerReservation) State() PinOwnerState
func (r PinOwnerReservation) Valid() bool

type PinAbandonReason string
const (
    PinAbandonOperatorRequested PinAbandonReason = "OPERATOR_ABORT_BEFORE_READY"
    PinAbandonSetupFailed PinAbandonReason = "SETUP_FAILURE_BEFORE_READY"
)

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

There are no other exported fields or constructors for `PinOwnerReservation`; its zero value is invalid, all getters return zero values, and `Valid()` returns false. For a nonzero reservation, `Valid()` is true exactly when planRef is 64 lowercase hex, operationID matches the stated ASCII pattern, reservationVersion and recordVersion are positive, and state is one of the six v1 states; it does not assert freshness. A returned reservation exposes the exact `PlanRef()` and immutable `ReservationVersion()` required by `VerifiedSnapshotLease.Pin`/`ResumePin`, plus the exact `RecordVersion()` required for owner CAS. Every mutating owner method rejects a locally invalid value, stale record version, wrong operation ID, or state mismatch. Getters return values and cannot mutate owner state.

`SnapshotPinOwnerOptions` has exactly four fields: `OwnerRoot string`, `BackupLeaseRoot string`, `MaxPlans int`, and `MaxOwnerMetadataBytes int64`. Both roots must be absolute, already-clean paths, must differ, and are selected by the recovery production factory rather than CLI or request input. `BackupLeaseRoot` must be byte-for-byte equal to `backup.SnapshotLeaseStoreOptions.LeaseRoot` passed to preflight and to the backup constructor. The exact backup options supplied to preflight and construction have fields `LeaseRoot string`, `MaxArtifactBytesPerLease int64`, `MaxMetadataBytesPerLease int64`, `MaxRestoreScratchBytes int64`, `MaxRetainedArtifactBytes int64`, `MaxRetainedMetadataBytes int64`, and `MaxLeases int`; validate them with the landed backup validation, without changing their values or interpreting zero as unlimited. `MaxPlans` is in `[1,256]`; `MaxOwnerMetadataBytes` is in `[1,268435456]`. History/event/attempt limits and the encoding version are fixed protocol constants, not caller-adjustable options. `expected` is the required third constructor argument of opaque type `backup.SnapshotStoreIdentity`; it is never stored in `SnapshotPinOwnerOptions`, serialized, reflected, or replaced by a zero/default token. The owner supports only the same GOOS and filesystem validation matrix as the landed backup/privatefs implementation (Darwin and Linux, with `privatefs.ValidateDirectory` acceptance); other platforms or filesystem policies fail closed. There is no configurable support bypass.

`ReservePinPlan` accepts only operation IDs matching `[A-Za-z0-9._:-]{1,128}`. The ID is the sole retry key because the narrow method has no other caller payload: the first call permanently binds that ID to one planRef, reservationVersion 1, and recordVersion 1; every exact retry resolves the same planRef and reservationVersion, then returns the latest durable state and recordVersion (including terminal states). A retry appends nothing. Callers repeat the same operationID after Pin/Resume to obtain the current CAS token; the originally returned value remains a stale token and is rejected for mutation. Reusing an operation ID can never create a second plan. New reservations use a fresh cryptographic 64-lowercase-hex planRef, check it against every retained reservation, and use reservationVersion 1; planRefs and operation IDs are never recycled or deleted. AttemptVersion, not reservationVersion, advances for intentional fresh-lease retry N+1. This makes the incarnation tuple exact without an unbounded global counter.

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

The producer-owned values crossing this interface have these exact public fields and getters at main `c2d5a54`:

```go
type PinAttemptRef struct {
    PlanRef string
    LeaseID string
    ManifestSHA256 string
    ReservationVersion uint64
    AttemptVersion uint64
    State PinAttemptState
}
const (
    PinAttemptPending PinAttemptState = 1
    PinAttemptCommitted PinAttemptState = 2
)

type SnapshotPinRef struct { /* all fields private */ }
func (p SnapshotPinRef) ID() string
func (p SnapshotPinRef) ManifestSHA256() string
func (p SnapshotPinRef) PlanRef() string
func (p SnapshotPinRef) ReservationVersion() uint64

type PinReleaseAuthorization struct {
    PinID string
    PlanRef string
    ManifestSHA256 string
    Disposition PinReleaseDisposition
    RecordVersion uint64
}
const (
    PinAbandonedBeforeEffects PinReleaseDisposition = 1
    PinCommittedRestoreComplete PinReleaseDisposition = 2
)
type PinReconcileDecision struct {
    Action PinReconcileAction
    Authorization PinReleaseAuthorization
}
const (
    PinKeep PinReconcileAction = 1
    PinRelease PinReconcileAction = 2
)

type SnapshotStoreIdentity struct { /* all fields private */ }
type VerifiedPinAbsence struct { /* all fields private */ }
func (p VerifiedPinAbsence) Consume(exact PinAttemptRef, expected SnapshotStoreIdentity) error

func PreflightSnapshotStoreIdentity(ctx context.Context, options SnapshotLeaseStoreOptions) (SnapshotStoreIdentity, error)
func NewSnapshotLeaseStore(ctx context.Context, options SnapshotLeaseStoreOptions, lifecycle SnapshotPinLifecycleAuthority, expected SnapshotStoreIdentity) (*SnapshotLeaseStore, error)
func (l *VerifiedSnapshotLease) Pin(ctx context.Context, planRef string, reservationVersion uint64, expectedManifestSHA256 string) (SnapshotPinRef, error)
func (s *SnapshotLeaseStore) ResumePin(ctx context.Context, planRef string, reservationVersion uint64, expectedManifestSHA256 string) (SnapshotPinRef, error)
func (s *SnapshotLeaseStore) CancelPin(ctx context.Context, exact PinAttemptRef) error
```

`PinAttemptRef` has six caller-readable fields. Its exact persisted attempt tuple is `(PlanRef, ReservationVersion, LeaseID, ManifestSHA256, AttemptVersion)`; a live commit/cancel also checks the exact `State`. The producer's private `sameAttempt` compares those five tuple fields and intentionally does not compare State, so the owner must separately require `State == PinAttemptPending` before cancellation or proof consumption and must compare `State` to its exact durable attempt status on all other operations. `PinReleaseAuthorization` carries no reservation or attempt version; the owner binds its `RecordVersion` to a terminal record whose entire predecessor chain resolves uniquely to the exact reservation/attempt/lease/digest/pin tuple. The producer accepts the authorization only when pin ID, plan ref, digest and supported disposition match its durable record and RecordVersion is positive; the owner must perform the stronger full-history comparison before issuing it and again at `CompletePinRelease`.

The production factory ordering is mandatory:

1. Resolve the backup root from owner-controlled configuration and call `backup.PreflightSnapshotStoreIdentity(ctx, backupOptions)`.
2. Open the recovery pin owner with that exact opaque token. Before it serves methods, the owner creates or validates its own private root, stable lock, store UUID, and a durable binding to the backup root and stable lock device/inode observed through no-follow private filesystem checks. The opaque token itself is held only as a typed in-memory value and is never serialized, reflected, or treated as a public identity format.
3. Construct the backup store using the required fourth argument: `backup.NewSnapshotLeaseStore(ctx, backupOptions, owner, expectedIdentity)`. This constructor checks that the token is nonzero, its private root path equals `backupOptions.LeaseRoot`, and its private root/lock device/inodes equal the actual root and stable lock. Any mismatch, replaced root or lock, zero token, cancellation, or constructor error makes the entire factory unavailable; there is no path-based or three-argument fallback and no automatic rebinding of an existing owner.
4. After the backup constructor returns, reopen/revalidate the backup root and lock against the tuple persisted by the owner. The factory publishes neither component until both the backup token check and this final tuple check succeed. The owner never reads private token fields: successful producer construction proves the retained token matches the same actual tuple that the owner captured and persisted; the final check closes the interval after construction. On every restart repeat preflight, owner binding validation, mandatory four-argument construction, and final tuple check before enabling operations.

The backup token is intentionally opaque and has no wire encoding. The durable binding records only filesystem object identity needed to reject replacement; `VerifiedPinAbsence.Consume(exact, expected)` receives the retained typed token and requires pointer identity with the callback's token state. The factory must not persist the token's Go memory representation. The post-construction tuple check is part of the identity proof, not an optional audit. Do not weaken opacity or infer equality from a caller-supplied path.

The owner root and backup snapshot root are distinct identities. Each root has a stable regular `.store.lock` inode and device binding. The recovery owner stores its own root identity and lock identity in its superblock; its backup binding stores the backup root/lock identity. Neither identity is a UUID substitute for the other. Lock acquisition is cross-process and context-aware. Revalidate both roots and both stable lock objects before blocking, after lock acquisition, before every mutation, and after durable publication. A lock-path replacement while waiting fails closed.

## Durable states and exact lifecycle behavior

The complete owner-state enum for this v1 component is exactly `RESERVED`, `PIN_PENDING`, `PINNED`, `ABANDONED_BEFORE_EFFECTS`, `RELEASING`, and `RELEASED`. The complete event enum is exactly `RESERVE`, `PIN_BEGIN`, `PIN_COMMIT`, `PIN_CANCEL`, `ABANDON`, `RELEASE_BEGIN`, and `RELEASE_COMPLETE`. The complete `attempt_state` enum is empty, `PENDING`, `COMMITTED`, or `CANCELED`. The complete v1 `release_disposition` enum is empty or `ABANDONED_BEFORE_EFFECTS`; `COMMITTED_RESTORE_COMPLETE` is a producer enum value but is invalid for this narrow owner's v1 records and can never be returned by this owner. The complete v1 `reason_code` enum is empty, `OPERATOR_ABORT_BEFORE_READY`, or `SETUP_FAILURE_BEFORE_READY`. No other event/state/value is silently accepted.

The record-field matrix below is normative for every event. “Carry” means copy the exact latest predecessor value; “empty” means empty string for text, zero for integers, or the empty enum member. `attempt_high_water` never decreases. Every event carries the exact `store_id`, `plan_ref`, `reservation_version`, `operation_id`, incremented `record_version`, and predecessor digest; only the event-specific fields below may differ.

| Event | Required predecessor | Result state | Attempt high-water/version and attempt fields | Pin ID | Release fields | Reason |
| --- | --- | --- | --- | --- | --- | --- |
| `RESERVE` | no predecessor | `RESERVED` | both counters 0; lease/digest/state empty | empty | disposition empty, release version 0 | empty |
| `PIN_BEGIN` | `RESERVED` or `PIN_PENDING` exact idempotent retry; for append after cancellation current state is `RESERVED` | `PIN_PENDING` | append increments both high-water and attemptVersion by 1, writes fresh nonempty lease ID and lowercase SHA-256 digest, state `PENDING`; exact retry appends nothing | empty | empty/0 | empty |
| `PIN_COMMIT` | exact current `PIN_PENDING`/`PENDING` attempt | `PINNED` | carry high-water, attemptVersion, lease and digest; state becomes `COMMITTED` | exact nonempty producer Pin ID | empty/0 | empty |
| `PIN_CANCEL` | exact current `PIN_PENDING`/`PENDING` attempt plus consumed absence proof | `RESERVED` | carry high-water, attemptVersion, lease and digest; state becomes `CANCELED` | empty | empty/0 | empty |
| `ABANDON` | exact current `RESERVED` with no live attempt, or `PINNED` with exact committed attempt | `ABANDONED_BEFORE_EFFECTS` | carry all existing counters and attempt fields unchanged (all zero/empty if no attempt) | carry exact committed pin ID only for prior `PINNED`, otherwise empty | empty/0 | exactly one allowed reason |
| `RELEASE_BEGIN` | exact `ABANDONED_BEFORE_EFFECTS` record with committed pin | `RELEASING` | carry exact committed attempt | carry exact pin ID | disposition `ABANDONED_BEFORE_EFFECTS`; release version equals this event's recordVersion | empty |
| `RELEASE_COMPLETE` | exact `RELEASING` record and matching backup RELEASED tombstone | `RELEASED` | carry exact committed attempt | carry exact pin ID | carry disposition and original `RELEASE_BEGIN` recordVersion unchanged | empty |

`PIN_BEGIN` is the only event that can increase attempt counters; its only valid append predecessor is a `RESERVED` head whose previous latest attempt, if any, is permanently `CANCELED`. `PIN_COMMIT` and `PIN_CANCEL` require `PIN_PENDING`; `ABANDON` is forbidden from `PIN_PENDING`, from any live attempt, and after release begins. `RELEASE_BEGIN` and `RELEASE_COMPLETE` are only the transitions above. Idempotent retry returns the exact already-recorded result and does not create a second event.

The larger recovery plan-store proposal names `READY`, `APPLYING`, `FORWARD_RESUMABLE`, `COMMITTED`, and `ADMISSION_CONFIRMED`; these are reserved for a future explicitly versioned full-store extension. A v1 pin owner encountering a record version it does not support returns `ErrPinOwnerUnavailable`, never mutates/rebinds/cleans it, and returns no release authority. Backup callers must retain the bytes on this error. The future store must extend this same root/store ID/history; it cannot make v1 interpret unfamiliar state as terminal.

| Current owner state | Exact operation | Durable result |
| --- | --- | --- |
| absent | `RESERVE` / `ReservePinPlan(operationID)` | Append record 1 in `RESERVED`; attempt high-water/version are zero and all attempt, pin, release, and reason fields are empty/zero. A retry of the same operationID returns that exact reservation without appending. |
| `RESERVED` | `PIN_BEGIN` / `BeginPinAttempt` | Append `PIN_PENDING`; increment attempt high-water and use that value as AttemptVersion; write nonempty leaseID and manifest digest; attempt state is PENDING; pin/release/reason fields empty. This CAS is durable before backup writes its pin file. |
| `PIN_PENDING` | exact `PIN_BEGIN` retry | Return the same PENDING tuple and versions without appending or changing any version. A changed tuple conflicts. |
| `PIN_PENDING` | `PIN_COMMIT` / `CommitPinAttempt` | Append `PINNED`; preserve exact attempt tuple, change attempt state to COMMITTED, write nonempty producer pin ID, keep release/reason fields empty. This call follows backup pin-file and containing-directory fsync. Exact retry is idempotent; mismatch is refused. |
| `PIN_PENDING` | `PIN_CANCEL` / `CancelPinAttempt` | Verify exact tuple and PENDING state, consume the exact callback proof, append `RESERVED` with attempt state CANCELED and the canceled tuple retained; pin/release/reason fields empty. No cancellation can succeed after durable producer pin. |
| `RESERVED` or `PINNED` | `ABANDON` / `MarkAbandonedBeforeEffects` | Fresh record-version CAS to `ABANDONED_BEFORE_EFFECTS`, with one nonempty reason enum. Preserve the last attempt high-water and tuple: no-attempt reservation has all attempt fields empty/zero; a canceled attempt keeps its CANCELED tuple and no pin; a PINNED reservation keeps its COMMITTED tuple and pin ID. Release fields are empty. Requires complete history with no READY/effect marker and no conflicting live attempt. |
| active owner with no terminal abandonment | `ReconcilePin(exactPin)` | Return `PinKeep` with zero authorization and do not append an event. Unknown state, mismatch, corrupt history, or incomplete inventory returns an error and never authorizes cleanup. |
| `ABANDONED_BEFORE_EFFECTS` with exact committed pin | first `RELEASE_BEGIN` / `ReconcilePin(exactPin)` | Append `RELEASING`; preserve exact committed attempt/pin; set disposition `ABANDONED_BEFORE_EFFECTS`; set `release_record_version` equal to this new recordVersion. Return PinRelease authorization with exactly the recordVersion persisted here. Exact retry while RELEASING returns the same authorization/version without appending. |
| `RELEASING` | backup records/releases exact bytes, then `RELEASE_COMPLETE` / `CompletePinRelease(exactAuth)` | Only after backup's exact RELEASED tombstone, append owner `RELEASED`; preserve the full attempt/pin/disposition tuple and preserve release_record_version as the authorizing RELEASE_BEGIN version. A wrong auth version or tuple conflicts. Lost response is resolved by rereading this record. |

Attempt history is permanent for the lifetime of the reservation. Each attempt is one of `PENDING`, `COMMITTED`, or `CANCELED`; public `PinAttemptRef.State` maps only PENDING and COMMITTED, as the integrated producer API defines. CANCELED exists only in owner history and is never emitted by `ListPinAttempts` as a live attempt.

`FindPinAttempt(planRef, digest)` returns the exact current attempt for that reservation and digest. If the latest matching attempt was canceled, it returns conflict, not not-found, so a caller cannot mistake cancellation for authority to retry the same tuple. `BeginPinAttempt` for any tombstoned tuple always refuses. An intentional N+1 is allowed only while the reservation remains active and only after explicit backup `Stage` supplies a fresh lease ID under the same expected digest. AttemptVersion increases exactly once; delayed Cancel(N), Begin(N), Resume(N), or commit(N) cannot mutate N+1. Terminal reservations reject all future attempts.

`ListPinAttempts` validates the entire bounded owner history before returning anything. It returns every current PENDING and COMMITTED attempt in stable lexicographic `(PlanRef, ReservationVersion, AttemptVersion, LeaseID)` order. Duplicate plan refs, duplicate attempt tuples, unknown enum values, state/pin inconsistencies, malformed IDs/digests, a gap/fork/checksum mismatch, or any unreadable reservation fails the complete call. It never returns a partial list. Canceled tombstones remain in history even though they are omitted from the live list.

`CommitPinAttempt` requires the current PENDING row to match every `PinAttemptRef` field, including planRef, reservationVersion, attemptVersion, leaseID, digest and PENDING state. It then checks every field actually exposed by `SnapshotPinRef`: pin ID, planRef, digest and reservationVersion. The attemptVersion and leaseID remain bound by the exact pending attempt row because the producer's `SnapshotPinRef` intentionally does not expose those fields. Only the unique current PENDING attempt may commit. Exact retry after a lost response returns success after verifying the committed pin; a different pin, attempt or reservation is conflict. No hash-shaped value or raw snapshot path proves that a backup pin was fsynced.

`ReconcilePin` validates the complete reservation history and exact pin tuple before deciding. On an active `RESERVED`/`PIN_PENDING`/`PINNED` reservation it returns `PinKeep` with zero authorization and does not append. On an exact abandoned pin, `ReconcilePin` durably appends `RELEASE_BEGIN` before returning `PinRelease`; the authorization RecordVersion equals that newly appended record. Subsequent calls in `RELEASING` return the identical authorization without appending. `CompletePinRelease` appends `RELEASE_COMPLETE` only after the producer has durably recorded the exact RELEASED tombstone; its new current recordVersion is distinct from, and does not replace, the authorization RecordVersion preserved in `release_record_version`. Corrupt/ambiguous/unsupported history returns an error and grants nothing. `COMMITTED` means only that the backup snapshot pin was durably committed; it never means the restore or epoch is committed. Since this owner does not contain a concrete epoch/admission proof owner, `PinCommittedRestoreComplete` always yields `PinKeep`/error. It cannot synthesize a full-restore release.

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

The v1 event/state/field matrix is exhaustive. Every event preserves store/plan/reservation identity; recordVersion increments exactly once; operationID stays unchanged. All omitted fields are canonical empty strings or zero. The most recent attempt tuple remains present after cancellation so the tombstone cannot be forgotten.

| event | transition | attempt fields | pin ID | release fields | reason code |
| --- | --- | --- | --- | --- | --- |
| `RESERVE` | absent → `RESERVED` | high-water 0; attemptVersion 0; lease/digest/state empty | empty | disposition empty; version 0 | empty |
| `PIN_BEGIN` | `RESERVED` → `PIN_PENDING` | high-water increments by 1; attemptVersion equals high-water; new 64-hex lease/digest; `PENDING` | empty | empty; 0 | empty |
| `PIN_COMMIT` | `PIN_PENDING` → `PINNED` | preserve exact PENDING tuple; set `COMMITTED` | new exact producer 64-hex pin ID | empty; 0 | empty |
| `PIN_CANCEL` | `PIN_PENDING` → `RESERVED` | preserve exact tuple; set `CANCELED` | empty | empty; 0 | empty |
| `ABANDON` | `RESERVED` or `PINNED` → `ABANDONED_BEFORE_EFFECTS` | preserve empty tuple if none; otherwise preserve last CANCELED tuple from RESERVED or exact COMMITTED tuple from PINNED | empty from RESERVED; exact pin ID from PINNED | empty; 0 | exactly one nonempty value below |
| `RELEASE_BEGIN` | exact pinned abandonment → `RELEASING` | exact last COMMITTED tuple | exact pin ID | `ABANDONED_BEFORE_EFFECTS`; `release_record_version` equals this recordVersion | empty |
| `RELEASE_COMPLETE` | `RELEASING` → `RELEASED` | exact tuple from RELEASE_BEGIN | exact pin ID | same disposition; preserve RELEASE_BEGIN version | empty |

The complete v1 enums are:

- Event: `RESERVE`, `PIN_BEGIN`, `PIN_COMMIT`, `PIN_CANCEL`, `ABANDON`, `RELEASE_BEGIN`, `RELEASE_COMPLETE`.
- State and exported `PinOwnerState`: `RESERVED`, `PIN_PENDING`, `PINNED`, `ABANDONED_BEFORE_EFFECTS`, `RELEASING`, `RELEASED`.
- Attempt state: empty, `PENDING`, `COMMITTED`, `CANCELED`.
- Release disposition: empty, `ABANDONED_BEFORE_EFFECTS`. The producer's `COMMITTED_RESTORE_COMPLETE` enum is refused in this narrow v1 owner.
- Reason code and `PinAbandonReason`: empty, `OPERATOR_ABORT_BEFORE_READY`, `SETUP_FAILURE_BEFORE_READY`. A reason string is audit metadata, never proof.

The larger full-store vocabulary `READY`, `APPLYING`, `FORWARD_RESUMABLE`, `COMMITTED`, `ADMISSION_CONFIRMED` is future-only. A v1 owner encountering a future wire version or state returns `ErrPinOwnerUnavailable`; it cannot mutate, list partially, rebind, or grant release, and backup must retain bytes. The later full store must append its reviewed higher-version records to this same owner root/history; a v1 owner never writes placeholders or drops unrecognized fields.

No map is serialized. The checksum is SHA-256 over `"serenity.recovery.snapshot-pin-owner.v1\0"` followed by the exact compact JSON encoding of those payload fields in the listed order, without the checksum field. The final record appends `checksum_sha256` last. The previous digest is 64 zeroes only for a reservation's first record; every later record binds the digest of the exact previous canonical full record. The parser rejects noncanonical re-encoding even when the JSON meaning is equivalent.

### Complete linked v1 golden vectors

The standalone superblock vector above is followed here by one exact reservation history containing every current v1 authority event: initial reservation, PIN_PENDING attempt N, cancellation tombstone N, intentional fresh-lease attempt N+1, pin commit, pre-effect abandonment, release authorization, and release completion. `previous_sha256` is the SHA-256 of the exact previous full record bytes including its `checksum_sha256`; each listed `record_sha256` is the digest stored as the next record's predecessor. The chain uses reservationVersion 1, recordVersion 1–8, operationID `op-001`, and fixed example IDs solely as golden data. The exact compact bytes are the JSONL rows below, with no trailing newlines in each record file.

| Record | Event | Previous full-record SHA-256 | This full-record SHA-256 | Payload checksum SHA-256 |
| --- | --- | --- | --- | --- |
| 01 | `RESERVE` | `0000000000000000000000000000000000000000000000000000000000000000` | `19a19db4826344cb6a09702ce1ce311fb2ec9aa28df21e542ad5582144eaaad7` | `0d6c13a6cba21bfabe6f84f3ad0ca7060f3843f21e02e7cb44e72b574712d4fa` |
| 02 | `PIN_BEGIN` | `19a19db4826344cb6a09702ce1ce311fb2ec9aa28df21e542ad5582144eaaad7` | `ef5e2a1b21eba195ee1964794a1f3120fe78953f4bec540a590d79803b831116` | `04675bb0ecb2b826486e72fcf05b99b1132a832886d0157a97df191d545f134d` |
| 03 | `PIN_CANCEL` | `ef5e2a1b21eba195ee1964794a1f3120fe78953f4bec540a590d79803b831116` | `c5d64c664c9e294cbba3be2af08fd2fc578f8b9f483c1a402354c486306d1a8f` | `9fbd909715a0f2e4058b43477903aba6b6e723be776751ffda633037fe11a251` |
| 04 | `PIN_BEGIN` | `c5d64c664c9e294cbba3be2af08fd2fc578f8b9f483c1a402354c486306d1a8f` | `4ab7781d15579d68ad8c69baafaad3840a96d6b1b9dc6d19714f9f2d4fbdf5a3` | `38bf57736b47bc106820bd6a784ab6eba8d3727e01037d7509f8df7fafb5fd3e` |
| 05 | `PIN_COMMIT` | `4ab7781d15579d68ad8c69baafaad3840a96d6b1b9dc6d19714f9f2d4fbdf5a3` | `0813afd97163c680013c40b573df46b1c85240207d5a42a514aa66326e4a19a3` | `507d37dfd95ab8f39f20f8c33cbfc2cdfb97b81535b1967b370c2492f16831e6` |
| 06 | `ABANDON` | `0813afd97163c680013c40b573df46b1c85240207d5a42a514aa66326e4a19a3` | `5341aaf36a79eea01aa9ed2aa1231302f5cbaa36982242fe4e97b7ceb8efaf48` | `e9182706582b70d8a1b272a8b00e74dcf4559ed6e3f205c32fa1a7bfbe13428f` |
| 07 | `RELEASE_BEGIN` | `5341aaf36a79eea01aa9ed2aa1231302f5cbaa36982242fe4e97b7ceb8efaf48` | `094c14e0feec28a5d05a57450109abd3d246f3d8818dfa7596464785470fd0d8` | `c0abe790755bde24e288a7b7c861747d7313d05c3b457100b4556fd3dd29408e` |
| 08 | `RELEASE_COMPLETE` | `094c14e0feec28a5d05a57450109abd3d246f3d8818dfa7596464785470fd0d8` | `9874a1678ad667206f8c9f526310e6bc115b2122e9a6f69068c1360e5c6c7997` | `d37602def3518a529978d9d3b4e358b0b2dd95f7aa92fef30d4851524c5aca70` |

```jsonl
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":1,"previous_sha256":"0000000000000000000000000000000000000000000000000000000000000000","operation_id":"op-001","event":"RESERVE","state":"RESERVED","attempt_high_water":0,"attempt_version":0,"lease_id":"","manifest_sha256":"","attempt_state":"","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"0d6c13a6cba21bfabe6f84f3ad0ca7060f3843f21e02e7cb44e72b574712d4fa"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":2,"previous_sha256":"19a19db4826344cb6a09702ce1ce311fb2ec9aa28df21e542ad5582144eaaad7","operation_id":"op-001","event":"PIN_BEGIN","state":"PIN_PENDING","attempt_high_water":1,"attempt_version":1,"lease_id":"4444444444444444444444444444444444444444444444444444444444444444","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"PENDING","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"04675bb0ecb2b826486e72fcf05b99b1132a832886d0157a97df191d545f134d"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":3,"previous_sha256":"ef5e2a1b21eba195ee1964794a1f3120fe78953f4bec540a590d79803b831116","operation_id":"op-001","event":"PIN_CANCEL","state":"RESERVED","attempt_high_water":1,"attempt_version":1,"lease_id":"4444444444444444444444444444444444444444444444444444444444444444","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"CANCELED","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"9fbd909715a0f2e4058b43477903aba6b6e723be776751ffda633037fe11a251"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":4,"previous_sha256":"c5d64c664c9e294cbba3be2af08fd2fc578f8b9f483c1a402354c486306d1a8f","operation_id":"op-001","event":"PIN_BEGIN","state":"PIN_PENDING","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"PENDING","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"38bf57736b47bc106820bd6a784ab6eba8d3727e01037d7509f8df7fafb5fd3e"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":5,"previous_sha256":"4ab7781d15579d68ad8c69baafaad3840a96d6b1b9dc6d19714f9f2d4fbdf5a3","operation_id":"op-001","event":"PIN_COMMIT","state":"PINNED","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"507d37dfd95ab8f39f20f8c33cbfc2cdfb97b81535b1967b370c2492f16831e6"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":6,"previous_sha256":"0813afd97163c680013c40b573df46b1c85240207d5a42a514aa66326e4a19a3","operation_id":"op-001","event":"ABANDON","state":"ABANDONED_BEFORE_EFFECTS","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"","release_record_version":0,"reason_code":"OPERATOR_ABORT_BEFORE_READY","checksum_sha256":"e9182706582b70d8a1b272a8b00e74dcf4559ed6e3f205c32fa1a7bfbe13428f"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":7,"previous_sha256":"5341aaf36a79eea01aa9ed2aa1231302f5cbaa36982242fe4e97b7ceb8efaf48","operation_id":"op-001","event":"RELEASE_BEGIN","state":"RELEASING","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"ABANDONED_BEFORE_EFFECTS","release_record_version":7,"reason_code":"","checksum_sha256":"c0abe790755bde24e288a7b7c861747d7313d05c3b457100b4556fd3dd29408e"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":8,"previous_sha256":"094c14e0feec28a5d05a57450109abd3d246f3d8818dfa7596464785470fd0d8","operation_id":"op-001","event":"RELEASE_COMPLETE","state":"RELEASED","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"ABANDONED_BEFORE_EFFECTS","release_record_version":7,"reason_code":"","checksum_sha256":"d37602def3518a529978d9d3b4e358b0b2dd95f7aa92fef30d4851524c5aca70"}
```

The checksums and predecessor hashes were computed independently in Python and verified with OpenSSL SHA-256 over the exact domain-prefixed payload bytes and exact full-record bytes. The external verification receipt retains the generated bytes, hash inputs, and OpenSSL output.

Expected negative outcomes against this vector chain:

| Mutation or retry | Required outcome |
| --- | --- |
| exact Reserve retry for `op-001` | Return the same planRef/reservationVersion and latest state/recordVersion; append nothing |
| `PIN_BEGIN` exact retry for attempt N or N+1 | Return the same attempt and versions; append nothing |
| Begin the canceled N tuple again | `ErrPinOwnerConflict`; recordVersion remains 3 and the N+1 record remains current |
| delayed Cancel(N) after N+1 exists, with a fresh exact proof for N | Acknowledge only N's existing tombstone; recordVersion and N+1 remain unchanged |
| Cancel with any tuple field changed or State other than PENDING | `ErrPinOwnerConflict`; consume no mismatched proof and append nothing |
| commit N+1 with a different pin ID/plan/digest/reservationVersion | `ErrPinOwnerConflict`; append nothing |
| `MarkAbandonedBeforeEffects` with stale recordVersion 4 instead of current 5 | `ErrPinOwnerConflict`; state remains PINNED |
| `RELEASE_BEGIN` from PINNED without ABANDON | `ErrPinOwnerConflict`; no authorization |
| `CompletePinRelease` with RecordVersion other than 7 or a changed tuple/disposition | `ErrPinOwnerConflict`; state remains RELEASING |
| request `PinCommittedRestoreComplete` under this narrow owner | `PinKeep`/`ErrPinOwnerUnavailable`; no authorization |
| unknown event/state, checksum, predecessor, or trailing byte | `ErrPinOwnerInvalid`/`ErrPinOwnerCorrupt`; all pins stay protected |

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
| I/O/fsync/rename/lock/unlock uncertainty | wrapped OS error or `ErrPinOwnerOutcomeUnknown`; reread exact history on retry, never retarget |
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
