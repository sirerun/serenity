# Snapshot cancellation absence proof — proposed source amendment

Status: coordinator-frozen `backup-pin-absence-v1` narrow amendment to historical producer contract `6a013b49b94e7f9286b782ad2827441d2e5bd737`; independently CLEAR at exact reviewed proposal `089f883142cafd88d724e033e1d06e08d53ddd1f` and adopted by backup owner. The complete durable recovery plan store remains separately proposed and unimplemented. No provider, deployment or capacity acceptance is inferred. Both producer and coordinator accept this proposed owner-local shape; it touches no shared contracts. Existing producer claim remains with the coordinator.

## Exact additive producer surface

All new source belongs to the already assigned `internal/hosted/backup/snapshot_lease*.go` glob and tests. Preserve public InspectSnapshot/Restore behavior. The current producer is new and unmerged; this amendment replaces its constructor signature and cancellation callback rather than leaving an optional fallback.

```go
// Both types have private representations and no public constructor or encoder.
type SnapshotStoreIdentity struct { state *snapshotStoreIdentityState }
type VerifiedPinAbsence struct { state *pinAbsenceState }

func PreflightSnapshotStoreIdentity(ctx context.Context, options SnapshotLeaseStoreOptions) (SnapshotStoreIdentity, error)
func NewSnapshotLeaseStore(ctx context.Context, options SnapshotLeaseStoreOptions, lifecycle SnapshotPinLifecycleAuthority, expected SnapshotStoreIdentity) (*SnapshotLeaseStore, error)
func (proof VerifiedPinAbsence) Consume(exact PinAttemptRef, expected SnapshotStoreIdentity) error

// Replace only this method in SnapshotPinLifecycleAuthority:
CancelPinAttempt(ctx context.Context, exact PinAttemptRef, proof VerifiedPinAbsence) error
```

`PreflightSnapshotStoreIdentity` validates options, context and the owner-only permission-enforced private root using the same no-follow policy as the store. It may create only the explicitly configured private root and stable `.store.lock`, using exclusive/validated creation, file fsync and root-directory fsync; never replace an existing lock. It captures actual root device/inode and stable regular lock device/inode. The returned opaque identity binds these exact owned filesystem objects and configured root; it is not a secret, signed approval, provider truth, or the recovery owner's UUID. Zero, invalid, decoded or root/lock-replaced identities fail. Do not expose private paths or identity fields as a public serialization protocol.

Production bootstrap order is explicit: preflight backup root/lock; construct and durably bind the recovery lifecycle owner to that exact opaque identity; construct the backup store with the required fourth argument; then permit pin lifecycle operations. The producer constructor reopens and verifies the anchored root and lock against the token before retaining it. It must refuse zero, stale, cross-root and cross-lock identities. There is no three-argument overload or inferred fallback. Every later store lock acquisition rechecks stable root and lock identity, including after blocking on the lock. The recovery factory and owner implementation remain separate work; this producer change does not create or trust a fake production authority.

## Locked absence and atomic consumption

`SnapshotLeaseStore.CancelPin` alone creates an absence proof. It holds the verified backup store lock and exact lease lock, reloads the exact attempt/lease record, and proves a regular owned STAGED record with no durable pin ID or PIN_PENDING/PINNED state for that exact tuple. Any conflicting pin, changed root/lock, stale attempt, different digest/lease/reservation, invalid record or uncertain absence fails closed without calling cancellation.

The private proof binds `(planRef, reservationVersion, leaseID, manifestSHA256, attemptVersion)`, the opaque store identity, the actual locked objects, and one live synchronous callback instance. Backup passes it to `CancelPinAttempt` before releasing either lock and expires it on every callback exit, including error, cancellation or panic cleanup. It cannot be used to cancel after the producer has durably entered PIN_PENDING; callers must resume/complete a pin and use separately authorized abandonment/release instead.

The recovery owner consumes the proof under its own CAS lock before tombstoning the exact pending attempt. `Consume` verifies exact tuple, expected root/lock identity and live callback scope, then atomically changes a single shared private state. All by-value aliases share that state: at most one consumption succeeds, and every alias is invalid after successful consumption or callback expiry. No claim is made that a shallow copy can be intrinsically rejected before first use, or that Go privacy protects against arbitrary unsafe code/process-memory compromise. Consumption never returns authority data for persistence. A zero, mismatched, cross-store, expired, already-consumed or decoded proof fails.

If the owner fails after consuming but before durable tombstoning, a retry asks backup for a new locked absence check/proof. If tombstoning completed but response was lost, the owner acknowledges the same permanent tombstone without affecting another attempt, after validating a fresh proof where cancellation is requested. Neither a raw exported PinAttemptRef nor any caller boolean, path, hash or EvidenceRef substitutes for this proof.

## Permanent cancellation and intentional retry

The exact canceled tuple `(planRef, reservationVersion, leaseID, manifestSHA256)` is permanently tombstoned. A delayed/retried Begin for that tuple cannot resurrect it. Intentional N+1 requires explicit fresh Stage/new leaseID under the still-active reservation and same expected digest; terminal reservations refuse all new attempts. AttemptVersion strictly increases. Delayed Cancel(N) can only acknowledge/refuse N's tombstone and cannot mutate N+1. Find/Resume use exact current attempts and never silently restage. This is a required lifecycle-owner behavior, tested in the producer's private fake but not evidence that a production owner store exists.

## Required qualification

Add genuine controls for root/lock substitution before and after constructor/lock acquisition; zero/cross-store identity; exact callback proof success; zero/mismatched/expired/decoded proof refusal; alias double-consumption; callback error/cancellation/panic expiry; no proof after durable PIN_PENDING/PINNED; permanent canceled-tuple refusal and fresh-lease N+1; and delayed old cancellation leaving N+1 intact. Privacy/decoder/reflection tests use safe Go APIs. Maintain current Stage/Candidate crash accounting, borrowed-byte protection, exact retained-byte restore, nullable candidate and metadata-budget controls.

At least two compiled behavioral mutants must fail intended assertions, including bypassing proof tuple/lifetime consumption and disabling canceled-tuple/identity enforcement. Setup/compiler errors do not count. Worker focused race/vet/lint at final coherent source; independent exact-head review and root full-module qualification before normal merge and exact landed proof. All load/lease/SSD rules remain mandatory.
