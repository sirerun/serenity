# Backup-owned verified snapshot lease: proposed source contract

**Status:** revised proposal responding to independent review HOLD `ac2367f7a033428217f68a0f1793fe40887716e2`; still not a frozen API or implementation authorization. Source baseline is `a89adca` after PR #350. The recovery-planner consumer request is pinned at `5168` in its author clone. Its zero-eligible and activation-guard corrections remain under that owner's review, and its separate global-availability hold remains in force. This document assigns no provider, deployment, purge, spend, activation, shared-contract, service, CLI, or module work.

## Decision

Add a backup-owned, bounded, durable lease over the exact private copies already verified by the current snapshot verifier. Recovery planning reads a bounded typed projection from that lease; restore consumes that same lease through the existing restore validation path. Neither consumer reopens the original snapshot source after `Stage` succeeds. `InspectSnapshot` and `Restore` keep their public signatures and successful data semantics; shared restore cleanup/close errors must propagate. `InspectSnapshot` may continue using the shared verifier with its current short-lived scratch lifetime.

The lease does not establish account/customer authority, billing eligibility, provider truth, or writer-generation authority. Candidate values are historical snapshot inputs only. The billing owner must independently verify any binding and eligibility. There is no API to obtain a writable database, raw artifact path, or a caller-constructed verified value.

## Proposed API in `internal/hosted/backup`

The exact exported names can be adjusted during freeze, but preserve these shapes and invariants:

```go
type SnapshotLeaseStoreOptions struct {
	LeaseRoot string // explicit, validated, owner-only persistent root; no default
	MaxArtifactBytesPerLease int64
	MaxMetadataBytesPerLease int64
	MaxRestoreScratchBytes int64
	MaxRetainedArtifactBytes int64
	MaxRetainedMetadataBytes int64
	MaxLeases int // every staging lease and every durable pin counts
}

type SnapshotLeaseStore struct { /* private fields */ }

func NewSnapshotLeaseStore(ctx context.Context, options SnapshotLeaseStoreOptions, lifecycle SnapshotPinLifecycleAuthority) (*SnapshotLeaseStore, error)
func (s *SnapshotLeaseStore) Stage(ctx context.Context, sourcePath string, options InspectionOptions) (*VerifiedSnapshotLease, error)
func (s *SnapshotLeaseStore) ResolvePinned(ctx context.Context, pinID, expectedManifestSHA256, planRef string) (PinnedSnapshotRef, error)
func (s *SnapshotLeaseStore) ReopenPinned(ctx context.Context, ref PinnedSnapshotRef) (*VerifiedSnapshotLease, error)
func (s *SnapshotLeaseStore) Reconcile(ctx context.Context) error

type SnapshotPinRef struct { /* private fields; no JSON/text unmarshaler */ }
func (p SnapshotPinRef) ID() string
func (p SnapshotPinRef) ManifestSHA256() string
func (p SnapshotPinRef) PlanRef() string

type PinnedSnapshotRef struct { /* private fields; produced by Pin, no JSON/text unmarshaler */ }

type VerifiedSnapshotLease struct { /* private fields */ }
func (l *VerifiedSnapshotLease) LeaseID() string
func (l *VerifiedSnapshotLease) Inspection() SnapshotInspection
func (l *VerifiedSnapshotLease) Candidate(ctx context.Context, accountID string) (VerifiedAccountCandidate, error)
func (l *VerifiedSnapshotLease) Pin(ctx context.Context, planRef, expectedManifestSHA256 string) (SnapshotPinRef, error)
func (l *VerifiedSnapshotLease) Close(ctx context.Context) error

// Only backup consumes retained artifacts. No general artifact-name/path reader is exposed.
func RestoreVerified(ctx context.Context, lease *VerifiedSnapshotLease, destination string) error

type VerifiedAccountCandidate interface {
	AccountID() string
	SnapshotStatus() string
	CustomerBindingCandidate() string
	CheckoutAttempts() []SnapshotCheckoutAttempt
	ManifestSHA256() string
	verifiedAccountCandidate() // private sealing method
}

type SnapshotCheckoutAttempt struct {
	HasSessionRef bool // true iff the database column was non-NULL
	SessionRef string // empty when NULL; the slice element itself means a row exists
}

// Configured by the recovery owner with its authoritative durable plan store.
// The production store constructor must bind that reviewed implementation;
// callers cannot authorize release by passing a bool/string flag to Reconcile.
type SnapshotPinLifecycleAuthority interface {
	ValidateReservedPlanRef(ctx context.Context, planRef string) (PlanRefReservation, error)
	AuthorizePinRelease(ctx context.Context, pin SnapshotPinRef) (PinReleaseAuthorization, error)
	CompletePinRelease(ctx context.Context, authorization PinReleaseAuthorization) error
}

type PlanRefState uint8
const PlanRefReserved PlanRefState = 1
type PlanRefReservation struct {
	PlanRef string
	State PlanRefState
	RecordVersion uint64
}
type PinReleaseDisposition uint8
const (
	PinAbandonedBeforeEffects PinReleaseDisposition = iota + 1
	PinCommittedRestoreComplete
)
type PinReleaseAuthorization struct {
	PinID string
	PlanRef string
	ManifestSHA256 string
	Disposition PinReleaseDisposition
	RecordVersion uint64
}

// Package-private operations used by the existing public wrappers.
func inspectVerified(ctx context.Context, sourcePath string, options InspectionOptions, keep bool) (verifiedArtifacts, error)
func restoreVerified(ctx context.Context, lease *VerifiedSnapshotLease, destination string) error
```

`SnapshotPinRef` is the durable, opaque restart reference returned directly by `Pin`; its getters expose the ID, raw manifest digest, and reserved plan ref needed for the immutable plan record. Its printable ID is generated from at least 256 bits of cryptographic randomness, contains no path/account data, and is canonical lowercase fixed-length hex. `PinnedSnapshotRef` is process-local and unforgeable outside backup; it is produced only by `ResolvePinned` and is never deserialized from plan JSON. `ResolvePinned(ctx, pinID, expectedDigest, planRef)` accepts identifiers, not paths or serialized handles, and succeeds only when all three match a durable pin. It does not grant eligibility or writer authority. Returned slices/strings are immutable copies.

The recovery protocol is fixed as follows. First the authoritative recovery store atomically reserves a globally unique canonical 64-byte lowercase-hex `planRef` in `RESERVED` state. Second, `Stage` verifies/copies the source using per-stage `InspectionOptions` and returns a lease bound to that one raw manifest digest. Third, `Pin(ctx, planRef, digest)` asks the injected lifecycle authority to validate the exact still-`RESERVED` reservation, then durably binds the staged lease to exactly `(planRef, leaseID, manifestSHA256)` and returns a `SnapshotPinRef`. Fourth, the recovery owner atomically persists one immutable READY plan containing `planRef`, `pinID`, digest, full verified inventory and canonical plan payload/hash. A plan approval binds that completed full plan hash; the pin never binds a hash that includes its own ID, so there is no cycle. No plan is executable/published before its pin is durable, and no approval/effect can consume a merely reserved or staged plan.

Crash recovery follows the same order. If the process stops before pin durability, the recovery owner may resume the exact reserved `planRef` and restage/reverify the same expected digest; no plan exists. If it stops after durable pin but before READY persistence, the reservation remains `RESERVED`; a retry resolves the unique pin by exact `(planRef,digest)`, verifies the retained bytes, and completes the same immutable plan. A different digest or second pin for the same plan ref is a conflict. If it stops after READY persistence, reload that immutable plan and resolve/reopen its exact pin; never rebuild or replace it in place. A crash after the atomic plan write but before return is an idempotent retry of that exact record. Approval references bind only the final full plan hash.

Pin lifecycle is an explicit trusted seam. The constructor requires a recovery-owner `SnapshotPinLifecycleAuthority`; the production factory binds only the reviewed authoritative plan store, never CLI input or a caller-supplied confirmation flag. `ValidateReservedPlanRef` must return a versioned claim for the exact unique `RESERVED` ref. `AuthorizePinRelease` succeeds only after the owner store's plan/epoch guard excludes a concurrent phase start and proves one of these exact dispositions: (a) this exact plan ref was durably marked `ABANDONED_BEFORE_EFFECTS`, or (b) global restore was `COMMITTED` with its admission record and no recovery operation still needs snapshot bytes. Pins remain through `RESERVED`, planning/READY, APPLYING, and every forward-resumable phase. Post-commit `ActivateOne` consumes current restored rows/provider evidence and does not reopen snapshot bytes, so the committed-restore disposition is sufficient for pin release. The authorization binds exact pin ID, plan ref, digest, disposition, and authoritative record version. It is not a boolean; only store reconciliation can consume it. `CompletePinRelease` releases the owner guard/CAS after local cleanup. Authorization/finalization are idempotent for the exact tuple and reject stale versions, another pin/digest, in-progress/active plan, or any phase where effects may have started. No TTL, age, missing file, local cleanup request, PID, or process-local lock is proof of abandonment/terminality.

`Reconcile(ctx)` lists store-owned staged and pinned records and calls that authority for each pin; authority errors preserve bytes and return an error. An in-progress or forward-resumable plan/pin is retained for exact retry. A pin release follows one durable crash-repairable sequence: (1) acquire the exclusive per-lease lifecycle lock, proving no active borrowers (borrowers hold a shared lock); (2) obtain the owner-authorized exact disposition/guard; (3) durably write `RELEASING` with exact pin ID, plan ref, digest, disposition and record version; (4) verify the recorded root/leaf inode, type, owner and marker, remove only those owned entries, and fsync the parent; (5) durably write `RELEASED`; (6) call `CompletePinRelease` to finalize the owner CAS/guard. A crash in `RELEASING` resumes only that exact authorized cleanup; a crash after `RELEASED` retries only owner finalization. Any identity, cleanup, sync, or authority error remains visible and never permits an unrelated pin cleanup. Foreign/replaced entries are never recursively removed.

Staged-but-never-pinned crash orphans are distinct: after startup holds the exclusive store lock and proves no active shared lease lock, backup may remove only a durable `STAGED` record not referenced by any pin, after a bounded grace period and inode/marker verification. A `PINNED` lease is never age-collected. This staged-orphan rule frees bytes without pretending to decide recovery-plan authority.

## Candidate projection and data boundary

`Candidate(ctx, accountID)` projects one record from the copied read-only control database using a fixed query owned by backup. In this repository's current schema the bounded fields are account ID, snapshot status, `stripe_customer_id` as a historical candidate reference, and the nullable `checkout_attempts.session_id`. The public projection calls the customer value a candidate, never a verified binding. Checkout attempts use a slice where no element means no row, while each element with `HasSessionRef=false` means a row existed and the SQL value was NULL; `HasSessionRef=true` plus an empty value preserves a present-but-empty ref. Thus NULL/empty pending attempts cannot collapse into “no checkout row.” The projection returns only necessary customer/session reference values, never request body, email, email hash, token, credentials, invoice bodies, or provider metadata. If the supported source schema evolves to multiple checkout rows, the proposed hard ceiling is 64 per account and excess rows fail closed.

Enforce byte ceilings *before scanning candidate text into Go strings*. Run fixed SQLite byte-length preflight queries over the immutable copied database, using `length(CAST(value AS BLOB))`, and reject oversized values before `Rows.Scan`; then run the bounded projection query. Candidate field ceilings are: account ID 128 bytes, customer-binding candidate 256 bytes, and each checkout session ref 256 bytes; plan refs are exactly 64 lowercase hex bytes. Every text value must be valid UTF-8 and contain no NUL/C0/C1 control bytes. A NULL or empty checkout ref is retained as pending evidence for the billing owner to block, not silently omitted. Reject duplicate/malformed account IDs, unknown statuses, duplicate refs, missing/inconsistent schema, query/row/close errors, and context cancellation. Candidate count is at most `Inspection.MaxAccounts` and the existing hard 10,000 inspection ceiling. Sort checkout refs bytewise when deriving any consumer digest. Candidate records carry the exact raw manifest digest. They have no `Eligible`, `Free`, `CustomerBindingVerified`, `Current`, or similar field. Billing treats all fields as untrusted historical lookup hints; failed, missing, ambiguous, or conflicting evidence blocks.

`Inspection()` returns the existing metadata (`ManifestSHA256`, source/build/schema, journal cut, bounded inventories, artifact count/declared bytes) by value with deep-copied slices. It never returns source paths, lease paths, SQL handles, bundle paths, or mutable internal slices. Candidate projections do not expose arbitrary artifact reads. Restore is the only artifact consumer.

## Verification and same-byte guarantee

Extract the current `InspectSnapshot` verifier into one private routine used by `InspectSnapshot` and `Stage`. Preserve manifest digest validation over the exact raw `manifest.json` bytes, current `MaxDeclaredBytes`/account/brain ceilings, exact manifest-listed artifact names, regular-file/type and symlink refusal, bounded streaming copies, artifact digest checks, read-only DB inspection, bundle-head verification, and cleanup-on-error. `Stage` retains only the verified manifest, control DB, and manifest-listed brain bundles as exact private copies. It must not copy unlisted files or follow an arbitrary path supplied by a caller after validation.

The lease records SHA-256 for every retained artifact and the raw manifest, manifest source/build/schema provenance, journal cut, exact inventory, format version, random lease ID, and filesystem identity for its owned root/files. Hash strings require exactly 64 lowercase hexadecimal bytes. All metadata uses a strict versioned format: reject duplicate/unknown/trailing fields, invalid UTF-8, noncanonical IDs, and records above a fixed small ceiling. Open through an anchored `os.Root`/equivalent containment primitive; compare opened file identity, regular-file mode, size, and digest before each consumption. Detect root replacement, leaf replacement, truncation, extension, and in-place content tampering. For `RestoreVerified`, open each retained artifact exactly once through the anchored root, hash and bounded-copy from that same open descriptor into an exclusive store-owned restore scratch file, verify bytes/count/digest from that descriptor, fsync/close it, then pass only the private scratch copy to Git/restore consumers. Do not hash a path and later reopen that pathname for Git. Git may reopen the private copy because its containing scratch root is store-owned, mode 0700, identity-pinned, exclusively locked against replacement, and never caller-selected. No source snapshot reopen is permitted after successful `Stage`, even if the original path still exists.

The only accepted source path is the initial argument to `Stage`; it is untrusted and is validated/opened once using the current contained-root verifier. The returned lease and restore call do not contain or consult that path. Restore destination remains caller-selected under the existing Restore safety contract; the lease never interprets a destination as an artifact source.

Use the explicit store-owned scratch root for the lease-backed restore path; no system temp or fallback is allowed there. Bound its per-invocation verified artifact copies by positive `MaxRestoreScratchBytes`, no greater than the hard 1 TiB ceiling; refuse before copying when the sum of input artifacts needed for restore would exceed it. Store the scratch root identity and all created file identities, and perform owned cleanup on every exit. `RestoreVerified` passes only fully hash-verified copied bundles to Git consumers. The existing public `Restore(ctx, source, destination)` retains its signature and source-validation behavior, sharing the same restore core while preserving its established wrapper lifetime; the lease-backed path always supplies its explicit owned scratch root. The shared core must propagate file/root close, DB checkpoint/close, and owned cleanup errors for both wrappers. The logical scratch byte ceiling is not proof of physical quota and cannot constrain untrusted same-UID changes outside the owned root; filesystem quota/capacity qualification remains a separate gate.

## Bounds, storage ownership, and limits

Preserve the inspector's existing ceilings: `InspectionOptions.MaxDeclaredBytes` is positive and at most 1 TiB; accounts and brains are each 1..10,000; caller configuration may lower them. The store is not bound to one expected digest: each `Stage` receives its own `InspectionOptions.ExpectedManifestSHA256`, and each resulting lease/pin carries that exact one digest and can bind to only one plan ref. Set positive hard ceilings independently: artifact bytes per lease at most 1 TiB; metadata bytes per lease at most 1 MiB; retained artifact bytes across all staged leases and durable pins; retained metadata bytes across those same records; and `MaxLeases` across every distinct retained lease ID in staging, staged, or pinned state. An open in-process handle/borrow of an already-counted pinned lease does not count a second time; borrowers have a separate lock/refcount. There is no default concurrency that bypasses these counters. At Stage preflight, declared artifact bytes must be no greater than each of `InspectionOptions.MaxDeclaredBytes`, `MaxArtifactBytesPerLease`, and remaining `MaxRetainedArtifactBytes`. Reserve the configured per-lease metadata ceiling before copying; actual encoded metadata must stay within it and aggregate retained metadata must stay within `MaxRetainedMetadataBytes`. Artifact and metadata accounting are separate: an image at the exact artifact limit is valid if its metadata independently fits the metadata limits. Reject before allocation/copy when counts, artifact bytes, metadata bytes, distinct retained lease count, or retained totals exceed a limit. Bound reader buffers and copy loops; never truncate or return partial results. Use context checks between reads, hashes, DB rows, verification steps, fsyncs, and lock waits.

The lease root and all owned descendants must pass the existing private-directory ownership/mode checks and anchored containment checks; no system temp, search path, environment-selected fallback, or ambient workspace is allowed. Use an exclusive store lock with context-aware acquisition and inode-bound lock ownership; process-local mutexes alone do not coordinate restart/process concurrency. Verify root/marker/file identities before cleanup. Remove only entries created by this store and still matching recorded inode/type/owner identity; on an identity mismatch refuse cleanup and report it. Never recursively remove an unrecognized or replaced directory.

Before `Stage` reports success or `Pin` returns a durable reference, fsync every artifact and metadata file, then the lease directory and parent directory in the required order. Pin metadata is atomically replaced through a private temporary file and parent-directory fsync. Fsync failures are errors; a reference is not returned as durable until all required syncs succeed. `Close(ctx)` prevents new readers, waits for active readers, and then cleans up only unpinned ephemeral leases with verified ownership. It is idempotent. If canceled while borrowers remain, return `ctx.Err()` and retain the bytes/root identity; do not invalidate active reader descriptors, remove their files, or report cleanup complete. A later `Close`/store startup reconciliation retries after borrowers drain. Pinned leases remain until the explicit recovery-owner release/abandon handshake; closing a process handle never releases a durable pin.

Lease byte accounting is cooperative ownership accounting, not a hard physical quota. A configured 500 GB ceiling or a small fixture does not prove filesystem reservation, volume free capacity, or OS-enforced quota. T23.44 physical capacity remains open. Source code must fail closed when its own accounting ceiling is crossed, while deployment must separately establish a real capacity/quota mechanism and reserve sufficient headroom. No capacity claim follows from this API.

## Reader lifetime, cancellation, and errors

Every consuming operation checks `ctx.Err()` before work, during bounded loops, and before returning success. Context cancellation/deadline returns the context error (joined with any cleanup/sync error) and no partially verified projection/restore success. Digest mismatch, lease identity mismatch, malformed pin record, root replacement, artifact mutation, unsupported schema, abandoned pin, or stale lifecycle authorization returns typed sentinel errors (`ErrSnapshotLeaseInvalid`, `ErrSnapshotLeaseNotFound`, `ErrSnapshotLeaseConflict`, `ErrSnapshotLeaseClosed`, `ErrSnapshotLeaseLimit`, `ErrSnapshotLeaseAbandoned`) wrapped with operation context but without raw paths or sensitive IDs in logs. Pins have no time expiry.

`RestoreVerified(ctx, lease, destination)` serializes against lease `Close` and other restore attempts; `Close` stops new borrows and waits for this restore borrow to finish. It invokes the same private restore core as public `Restore(ctx, source, destination)` but supplies only the descriptor-verified copied bytes under explicit store-owned scratch. The existing public `Restore` retains its signature, still validates an arbitrary source as before, and continues to freeze restored account/subscription state to `restore_pending`; no recovery-specific eligibility or activation is added. Restore failures preserve current destination no-overwrite and owned staging cleanup rules. Reader close, DB close/checkpoint, scratch cleanup, and directory close errors are propagated/joined; they are never discarded to turn refusal into success.

No lease API exposes `*sql.DB`, `*sql.Tx`, `Exec`, raw SQL, arbitrary artifact names/paths, source re-open, or a setter. The API returns only typed metadata/candidates and delegates restoration to backup. This keeps SQL writer authority and path construction inside backup.

## Exact source ownership after review and freeze

After independent review and root freeze, the backup-source followup may own only:

- `internal/hosted/backup/snapshot_inspect.go` for shared verifier extraction and additive lease types/implementation;
- `internal/hosted/backup/backup.go` only for a private shared restore-core extraction preserving existing `Restore` behavior;
- new `internal/hosted/backup/snapshot_lease.go` for lease store, durable pin, handle, bounds, and cleanup;
- new `internal/hosted/backup/snapshot_lease_test.go` for focused hostile controls;
- one new backup-owner implementation receipt under `docs/plans/`.

No shared `contracts`, recovery planner, billing observer, service wiring, CLI, schema migration, provider client, registry, installed script, or module/build-configuration edit belongs to that lane. Ownership of existing files is required explicitly from the coordinator before edits; this source proposal alone does not grant it.

## Required mutation RED controls

Tests must first demonstrate failure on the untouched baseline or an independently recorded vulnerable mutation, then show the corrected candidate refuses it. Fake-only local controls are required for:

1. **Discarded-byte handoff:** mutate/replace/delete the original source after `Stage`; candidate reads and `Restore` still use the staged exact verified bytes. Instrument the source to prove zero post-stage opens.
2. **Raw identity and digest:** whitespace-only raw manifest changes alter digest; wrong expected digest causes zero scratch/lease artifacts; uppercase, short, overlong, nonhex, or mismatched digest is rejected.
3. **Source and lease replacement/tamper:** replace snapshot root, scratch root, lease root, metadata, control DB, and bundle leaf; mutate same inode; truncate/extend artifact; replace with symlink/FIFO/device where supported. Every case fails before projection/restore success and cleanup refuses foreign/replaced paths.
4. **Forged handles and path injection:** JSON or caller-constructed ID/ref/candidate is not accepted as authority; malformed/noncanonical IDs, path separators, traversal, arbitrary artifact names, and caller-selected source reopen all fail. Resolve requires exact persisted plan ref and digest.
5. **Borrow/Close races:** a blocked candidate reader and restore borrower keep bytes alive while `Close(ctx)` waits; cancellation returns context error and leaves owned bytes for safe retry; a later close cleans only after borrowers finish. Reopen after close/cleanup is refused.
6. **Pin ordering, release, and crash boundaries:** plan-ref reservation must precede Stage; `Pin` rejects absent/non-RESERVED reservation, wrong manifest digest, duplicate plan ref, and second conflicting pin. Inject crashes before pin fsync, after durable pin/before plan persist, after READY persist/before response, after durable RELEASING, during owned cleanup, after RELEASED/before owner CAS, and after owner CAS; each retry either resumes the exact same digest/pin/plan or returns a typed refusal. Full plan hash approval has no dependency on the pin ID. Pins for active RESERVED, READY, APPLYING, and any forward-resumable phase remain; release is impossible without the authoritative versioned `ABANDONED_BEFORE_EFFECTS` or `COMMITTED` restore/admission authorization and owner guard. Mismatched/stale authorization, concurrent phase start, active borrower, replaced marker, and caller-supplied approval flag cannot release. No pin expires by time.
7. **Bounds and cancellation:** artifact and metadata caps are independent; exact artifact-at-limit plus metadata-within-its-separate-cap succeeds; one byte over either fails before copy/allocation. Aggregate counters include active staging and persistent pins, across concurrent store instances/processes. `MaxLeases` counts both active leases and pins. Candidate preflight rejects overlong SQLite strings before Go scans them; exact/over-limit ID/customer/session refs and 64/65 checkout rows are checked. Cancellation at source read, hash, DB preflight/iteration, bundle verification, fsync, pin, restore, reconciliation, and cleanup cannot return success or delete borrowed bytes.
8. **Nullable checkout preservation:** a missing row, row with SQL NULL session ID, row with empty non-NULL ID, and row with non-empty ID yield distinct projections. The NULL/empty cases remain present candidate attempts and are blocked as pending downstream; no absent-row inference is possible.
9. **Shared verifier regression:** the same corrupt manifest/artifact/database/bundle vectors fail through both `InspectSnapshot` and `Stage`; a valid image produces identical manifest digest, source/build/schema, cut, account/brain inventories, and artifact hashes through both. Existing `InspectSnapshot` cleanup remains verified.
10. **Restore preservation:** `RestoreVerified` follows the existing restore validation and freeze behavior, rejects occupied destinations, cleans only owned staging, and propagates DB/reader/close errors. It cannot mark an account eligible or publish a partially restored destination.

These tests can establish local source behavior only. They do not prove provider truth, filesystem physical quota, volume durability across power loss, production ownership identity, production pin reconciliation wiring, deployment, or hosted acceptance. The separate recovery availability hold and all T23.44 physical-capacity gates remain unchanged.
