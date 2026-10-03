# Backup-owned verified snapshot lease: proposed source contract

**Status:** proposal for owner and coordinator review; not a frozen API or implementation authorization. Source baseline is `a89adca` after PR #350. The independent recovery-planner consumer request is `29d3c13` in its author clone. The recovery proposal's separate global-availability hold remains in force. This document assigns no provider, deployment, purge, spend, activation, shared-contract, service, CLI, or module work.

## Decision

Add a backup-owned, bounded, durable lease over the exact private copies already verified by the current snapshot verifier. Recovery planning reads a bounded typed projection from that lease; restore consumes that same lease through the existing restore validation path. Neither consumer reopens the original snapshot source after `Stage` succeeds. `InspectSnapshot` and `Restore` keep their existing signatures and behavior; `InspectSnapshot` may continue using the shared verifier with its current short-lived scratch lifetime.

The lease does not establish account/customer authority, billing eligibility, provider truth, or writer-generation authority. Candidate values are historical snapshot inputs only. The billing owner must independently verify any binding and eligibility. There is no API to obtain a writable database, raw artifact path, or a caller-constructed verified value.

## Proposed API in `internal/hosted/backup`

The exact exported names can be adjusted during freeze, but preserve these shapes and invariants:

```go
type VerifiedSnapshotOptions struct {
	Inspection InspectionOptions // requires caller-supplied private ScratchRoot and exact digest
	LeaseRoot string             // explicit, validated, owner-only persistent root; no default
	MaxLeaseBytes int64          // <= Inspection.MaxDeclaredBytes; includes metadata/overhead
	MaxLeases int                // hard logical concurrency ceiling; no queue that ignores ctx
}

type SnapshotLeaseStore struct { /* private fields */ }

func NewSnapshotLeaseStore(ctx context.Context, options VerifiedSnapshotOptions) (*SnapshotLeaseStore, error)
func (s *SnapshotLeaseStore) Stage(ctx context.Context, sourcePath string) (*VerifiedSnapshotLease, error)
func (s *SnapshotLeaseStore) ReopenPinned(ctx context.Context, ref PinnedSnapshotRef) (*VerifiedSnapshotLease, error)

type SnapshotPinRef struct { /* private fields; no JSON/text unmarshaler */ }
func (p SnapshotPinRef) ID() string
func (p SnapshotPinRef) ManifestSHA256() string
func (p SnapshotPinRef) PlanRef() string

type PinnedSnapshotRef struct { /* private fields; produced by Pin, no JSON/text unmarshaler */ }

type VerifiedSnapshotLease struct { /* private fields */ }
func (l *VerifiedSnapshotLease) Inspection() SnapshotInspection
func (l *VerifiedSnapshotLease) Candidates(ctx context.Context) ([]VerifiedAccountCandidate, error)
func (l *VerifiedSnapshotLease) Pin(ctx context.Context, planRef string) (PinnedSnapshotRef, error)
func (l *VerifiedSnapshotLease) Restore(ctx context.Context, destination string) error
func (l *VerifiedSnapshotLease) Close(ctx context.Context) error

type VerifiedAccountCandidate interface {
	AccountID() string
	SnapshotStatus() string
	SnapshotCustomerRef() string
	CheckoutRefs() []string
	ManifestSHA256() string
	verifiedAccountCandidate() // private sealing method
}

// Package-private operations used by the existing public wrappers.
func inspectVerified(ctx context.Context, sourcePath string, options InspectionOptions, keep bool) (verifiedArtifacts, error)
func restoreVerified(ctx context.Context, lease *VerifiedSnapshotLease, destination string) error
```

`SnapshotPinRef` is the durable, opaque restart reference. Its printable ID is generated from at least 256 bits of cryptographic randomness, contains no path or account data, and is validated as canonical lowercase fixed-length hex. `PinnedSnapshotRef` is process-local and unforgeable outside this package; it is not serialized into plan JSON. The plan store persists only `SnapshotPinRef.ID()`, raw manifest SHA-256, and the plan reference. `ReopenPinned` requires that canonical ID and the expected digest/plan ref in a constructor-created `PinnedSnapshotRef` supplied by a backup-owned resolver. Therefore add:

```go
func (s *SnapshotLeaseStore) ResolvePinned(ctx context.Context, id, expectedManifestSHA256, planRef string) (PinnedSnapshotRef, error)
```

This resolver accepts identifiers, not paths or serialized handles. It succeeds only for a durable pin whose stored plan ref and exact manifest digest match; it does not grant mutation or eligibility authority. Callers must use `ResolvePinned` then `ReopenPinned`, never JSON-unmarshal a lease or fabricate a verified candidate. All returned slices and strings are immutable copies.

The recovery owner generates a random `planRef` before the snapshot handoff, pins the staged lease to it, then atomically persists the plan record containing the pin ID, digest, and plan ref. This ordering intentionally makes the crash boundary recoverable: a crash before plan persistence leaves an orphan pin, while a crash after persistence leaves a reopenable exact pin. A recovery-start reconciliation pass releases an orphan only after the recovery record store authoritatively confirms that the plan ref has no committed/in-progress plan and marks that ref abandoned. Backup does not guess from age or auto-delete a pin. A crash after the atomic plan write but before return is an idempotent retry: reload the same plan, resolve its pin, and return the same plan result. Never persist an executable plan that references an unpinned lease.

Pin, plan record, and orphan reconciliation form an owner handshake, not a cross-file transaction. The recovery store must reserve `planRef` uniquely before stage; a collision is fatal. The pin records an owner token plus digest, and `Pin` is idempotent only for the identical `(planRef, leaseID, manifestSHA256)` tuple. A different tuple is a conflict. Reconciliation may release only pins explicitly marked abandoned by the recovery store. No TTL silently converts a persisted plan's source authority into cleanup eligibility. Terminal plan retention policy and release are outside this API proposal and need an explicit recovery owner decision before source implementation.

## Candidate projection and data boundary

`Candidates(ctx)` projects one record per account from the copied read-only control database using a fixed query owned by backup. In this repository's current schema the bounded fields are account ID, snapshot status, `stripe_customer_id` as a historical candidate reference, and at most one `checkout_attempts` session reference (the table is keyed by account ID). The public projection calls that value `SnapshotCustomerRef`; it does not call it a verified binding. Checkout projection returns only the exact bounded session references needed by the billing owner, never request body, email, email hash, token, credentials, invoice bodies, or provider metadata. If supported source schema evolves to multiple checkout references, enforce a proposed ceiling of 64 per account and fail closed above it.

Reject duplicate/malformed account IDs, unknown statuses, oversized identifiers/references, duplicate checkout refs, invalid UTF-8/control text, missing/inconsistent schema, query/row/close errors, and context cancellation. Candidate count is at most `Inspection.MaxAccounts` and the existing hard 10,000 inspection ceiling. Sort accounts by ID and checkout refs bytewise before returning. Candidate records carry the exact raw manifest digest. They have no `Eligible`, `Free`, `CustomerBindingVerified`, `Current`, or similar field. Billing must treat every field as an untrusted historical lookup hint and perform fresh read-only verification; failed, missing, ambiguous, or conflicting evidence blocks.

`Inspection()` returns the existing metadata (`ManifestSHA256`, source/build/schema, journal cut, bounded inventories, artifact count/declared bytes) by value with deep-copied slices. It never returns source paths, lease paths, SQL handles, bundle paths, or mutable internal slices. Candidate projections do not expose arbitrary artifact reads. Restore is the only artifact consumer.

## Verification and same-byte guarantee

Extract the current `InspectSnapshot` verifier into one private routine used by `InspectSnapshot` and `Stage`. Preserve manifest digest validation over the exact raw `manifest.json` bytes, current `MaxDeclaredBytes`/account/brain ceilings, exact manifest-listed artifact names, regular-file/type and symlink refusal, bounded streaming copies, artifact digest checks, read-only DB inspection, bundle-head verification, and cleanup-on-error. `Stage` retains only the verified manifest, control DB, and manifest-listed brain bundles as exact private copies. It must not copy unlisted files or follow an arbitrary path supplied by a caller after validation.

The lease records SHA-256 for every retained artifact and the raw manifest, manifest source/build/schema provenance, journal cut, exact inventory, format version, random lease ID, and filesystem identity for its owned root/files. Hash strings require exactly 64 lowercase hexadecimal bytes. All metadata uses a strict versioned format: reject duplicate/unknown/trailing fields, invalid UTF-8, noncanonical IDs, and records above a fixed small ceiling. Open through an anchored `os.Root`/equivalent containment primitive; compare opened file identity, regular-file mode, size, and digest before each consumption. Detect root replacement, leaf replacement, truncation, extension, and in-place content tampering. Rehash the verified copies before `Restore` and verify every artifact before it is supplied to the existing restore path. No source reopen is permitted after successful `Stage`, even if the original path still exists.

The only accepted source path is the initial argument to `Stage`; it is untrusted and is validated/opened once using the current contained-root verifier. The returned lease and restore call do not contain or consult that path. Restore destination remains caller-selected under the existing Restore safety contract; the lease never interprets a destination as an artifact source.

## Bounds, storage ownership, and limits

Preserve the inspector's existing ceilings: declared bytes are positive and at most 1 TiB, accounts and brains are each 1..10,000, and caller configuration can lower those limits. The lease adds ceilings for retained metadata (1 MiB), candidate refs per account (64), simultaneous leases (default proposal: 1 until owner review establishes a supported higher value), and total retained lease bytes. Reject before allocation/copy when declared aggregate bytes, accounts, brains, per-account refs, active lease count, or configured total bytes exceed a limit. Bound reader buffers and copy loops; never truncate or return partial results. Use context checks between reads, hashes, DB rows, verification steps, fsyncs, and lock waits.

The lease root and all owned descendants must pass the existing private-directory ownership/mode checks and anchored containment checks; no system temp, search path, environment-selected fallback, or ambient workspace is allowed. Use an exclusive store lock with context-aware acquisition and inode-bound lock ownership; process-local mutexes alone do not coordinate restart/process concurrency. Verify root/marker/file identities before cleanup. Remove only entries created by this store and still matching recorded inode/type/owner identity; on an identity mismatch refuse cleanup and report it. Never recursively remove an unrecognized or replaced directory.

Before `Stage` reports success or `Pin` returns a durable reference, fsync every artifact and metadata file, then the lease directory and parent directory in the required order. Pin metadata is atomically replaced through a private temporary file and parent-directory fsync. Fsync failures are errors; a reference is not returned as durable until all required syncs succeed. `Close(ctx)` prevents new readers, waits for active readers, and then cleans up only unpinned ephemeral leases with verified ownership. It is idempotent. If canceled while borrowers remain, return `ctx.Err()` and retain the bytes/root identity; do not invalidate active reader descriptors, remove their files, or report cleanup complete. A later `Close`/store startup reconciliation retries after borrowers drain. Pinned leases remain until the explicit recovery-owner release/abandon handshake; closing a process handle never releases a durable pin.

Lease byte accounting is cooperative ownership accounting, not a hard physical quota. A configured 500 GB ceiling or a small fixture does not prove filesystem reservation, volume free capacity, or OS-enforced quota. T23.44 physical capacity remains open. Source code must fail closed when its own accounting ceiling is crossed, while deployment must separately establish a real capacity/quota mechanism and reserve sufficient headroom. No capacity claim follows from this API.

## Reader lifetime, cancellation, and errors

Every consuming operation checks `ctx.Err()` before work, during bounded loops, and before returning success. Context cancellation/deadline returns the context error (joined with any cleanup/sync error) and no partially verified projection/restore success. Digest mismatch, generation/lease identity mismatch, malformed pin record, root replacement, artifact mutation, unsupported schema, or expired/abandoned pin returns typed sentinel errors (`ErrSnapshotLeaseInvalid`, `ErrSnapshotLeaseNotFound`, `ErrSnapshotLeaseConflict`, `ErrSnapshotLeaseClosed`, `ErrSnapshotLeaseLimit`) wrapped with operation context but without raw paths or sensitive IDs in logs.

`Restore(ctx, destination)` serializes against `Close` and other restore attempts on the lease; `Close` cancels admission and waits for this borrow to finish. It invokes the same private restore core as public `Restore(ctx, source, destination)` but supplies already opened/contained verified bytes from the lease. The existing public `Restore` still validates an arbitrary source as before and continues to freeze restored account/subscription state to `restore_pending`; no recovery-specific eligibility or activation is added. Restore failures preserve the current destination no-overwrite and owned staging cleanup rules. Reader close, DB close/checkpoint, scratch cleanup, and directory close errors are propagated/joined; they are never discarded to turn refusal into success.

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
6. **Pin crash boundaries:** injected crash before pin fsync yields no returned durable ref; after durable pin but before plan write leaves a recoverable orphan; after plan write before method return reopens the identical bytes and ref; mismatched plan ref/digest, duplicate plan ref, and forged abandoned marker refuse. Abandon cleanup requires recovery-store confirmation and exact owned inode identity.
7. **Bounds and cancellation:** exact-at-limit succeeds, one-over-limit fails without partial projection/copy, concurrent lease ceiling is respected, and cancellation at source read, hash, DB iteration, bundle verification, fsync, pin, restore, and cleanup cannot return success or delete borrowed bytes.
8. **Shared verifier regression:** the same corrupt manifest/artifact/database/bundle vectors fail through both `InspectSnapshot` and `Stage`; a valid image produces identical manifest digest, source/build/schema, cut, account/brain inventories, and artifact hashes through both. Existing `InspectSnapshot` cleanup remains verified.
9. **Restore preservation:** lease restore follows the existing restore validation and freeze behavior, rejects occupied destinations, cleans only owned staging, and propagates DB/reader/close errors. It cannot mark an account eligible or publish a partially restored destination.

These tests can establish local source behavior only. They do not prove provider truth, filesystem physical quota, volume durability across power loss, production ownership identity, production pin reconciliation wiring, deployment, or hosted acceptance. The separate recovery availability hold and all T23.44 physical-capacity gates remain unchanged.
