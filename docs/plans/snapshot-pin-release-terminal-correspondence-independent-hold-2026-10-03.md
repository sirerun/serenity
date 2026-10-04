# Read-only source audit: terminal pin-attempt / producer-receipt correspondence

**Finding: confirmed permanent reconciliation conflict after a successful pin release.** This is a source-level blocker requiring a producer/owner contract correction and regression coverage. No build or source edit was performed.

## Exact behavior

`SnapshotPinOwner.ListPinAttempts` emits the plan head whenever `AttemptState` is `PENDING` **or** `COMMITTED`, without excluding owner state `RELEASED` (`internal/hosted/recovery/pin_owner.go:523-535`). `CompletePinRelease` transitions to `RELEASED` while preserving the committed attempt tuple/state (per the frozen event matrix and implementation).

At the beginning of every backup `Reconcile`, backup fetches that full list and calls `validateLocalPinAttemptPairs` before it processes the release journal (`internal/hosted/backup/snapshot_lease.go:1542-1559`). For every committed attempt whose live lease directory is absent, the pair validator requires a matching `.releases/<leaseID>` marker containing either exact `RELEASING` metadata or a `RELEASED` tombstone with matching lease ID, digest, plan, reservation version, attempt version, and nonempty pin ID (`1790-1804`).

The successful release path writes and fsyncs a `RELEASED` tombstone, calls owner `CompletePinRelease`, and then removes the whole journal marker (`finishReleaseJournal`, `snapshot_lease.go:2990-3017`; the in-loop path completes the owner at `1730` after `releaseLease` writes the tombstone at `2878-2888`). On the next `Reconcile`, the owner still lists the committed attempt but neither live lease nor release marker exists; the preflight pair validator returns `ErrSnapshotLeaseConflict` before journal processing. Thus even an ordinary fully successful release makes later reconciliation fail permanently. The same correspondence check also gates pinned-handle close/cleanup (`1886-1901`).

## Why omission from the owner list is not a safe workaround

The frozen `6588` owner contract requires the current pending/committed attempt to remain discoverable, with complete stable list validation; the owner record retains the committed tuple through `RELEASED`. The backup depends on that list for pending-attempt protection and exact local pairing. Silently omitting released committed attempts, weakening the pair validator to accept absence, or treating a missing local record as a release receipt would discard terminal correspondence and create an unproved cleanup state. A changed owner listing contract would require an explicit versioned status/receipt design, not an ad hoc filter.

## Minimal safe correction to contract and source

Keep a durable, exact producer terminal receipt for every committed attempt while the owner continues listing it. The existing `.releases/<leaseID>` `RELEASED` tombstone already carries the matching plan/digest/reservation/attempt/pin/disposition/record-version tuple and checksum, so the narrowest source correction is to retain that tombstone after `CompletePinRelease` rather than deleting its directory. The validator can then distinguish a fully completed release from a missing/corrupt marker on every later open, close, and reconciliation.

Retention must be accounted for as bounded durable lifecycle metadata: charge terminal receipts in aggregate metadata usage and reserve the required receipt headroom before deleting lease bytes or committing release completion. If configured limits cannot retain the exact receipt, refuse before destructive deletion. Validate all receipt fields and canonical bytes when retained; unknown, duplicate, mismatched, or malformed markers remain errors. Add regressions for: successful release followed by at least two `Reconcile` calls; reopen then `Reconcile`; `Close`/open after release; exact tuple mismatch/tamper; and capacity exhaustion refusing before lease-byte deletion. Existing crash-restart cases must still prove tombstone-before-owner-complete ordering and idempotent `CompletePinRelease`.

This change establishes local source correspondence only. It does not qualify full recovery authority, READY, provider behavior, runtime wiring, physical quota, or hosted acceptance. The prior pin-owner findings and producer crash-prefix gates remain open.

## Source references

- Owner list retention: `internal/hosted/recovery/pin_owner.go:523-535`.
- Reconcile order and pair preflight: `internal/hosted/backup/snapshot_lease.go:1542-1559`.
- Exact pair requirements: `snapshot_lease.go:1790-1804`.
- Release tombstone creation and sync: `snapshot_lease.go:2878-2888`.
- Owner completion then marker deletion: `snapshot_lease.go:2990-3017`.
- Aggregate release-journal metadata accounting: `snapshot_lease.go:2101-2142`.
- Pair validation also gates close: `snapshot_lease.go:1886-1901`.
