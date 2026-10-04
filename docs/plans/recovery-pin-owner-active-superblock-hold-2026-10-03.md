# Pin-owner active superblock freshness audit

Verdict: **HOLD for active-owner superblock freshness at exact head `b0a4c822f930b06b5444db51d7dc1c427f82655d` (tree `501b1a1f8b8d4bcace25eb59fb457ef534f6439a`).** This is a separate finding from the earlier decoder correction: the constructor classifies unsupported future superblocks correctly, but an already-open activated owner does not re-read the superblock when it services later operations. Static inspection only; no Go commands were run.

## Finding

`pinOwnerReadSuperblock` is called during `OpenSnapshotPinOwner` (`internal/hosted/recovery/pin_owner.go:233`) and during its definition there is no other call site. The constructor pins the decoded StoreID in memory at `:257` and validates reservation history at `:261`. Later public operations go through `pinOwnerWithLock` (`:706-728`), which checks the owner is activated, calls `pinOwnerAcquire`, then immediately invokes the operation callback. `pinOwnerAcquire` (`:736-801`) reopens and locks the owner/backup roots and validates their root/lock device+inode identities, but never reads or decodes the current `superblock.json`.

Consequently, after a successful open, changing the on-disk superblock to a future-version, malformed, or different-but-valid checksummed v1 superblock does not make an existing owner fail closed before `ListPinAttempts`, `ReservePinPlan`, `FindPinAttempt`, or lifecycle operations proceed. For example, a valid v1 superblock with a different StoreID can coexist temporarily with the old in-memory `o.storeID`; `pinOwnerLoadAll` validates each record against that cached value (`:1595-1596`), so a reservation can be appended under the stale StoreID despite the current superblock naming a different store. A subsequent fresh open can then reject that history against the new superblock. The future-version constructor regression added in the decoder correction only reopens a fresh owner and therefore does not exercise this active-instance path.

## Required correction and regression

Before each public operation is allowed to read or mutate owner history, validate the current superblock from the already acquired owner-root descriptor while the owner lock is held. Require the current record to decode as supported v1, pass its checksum/canonical checks, and match the exact StoreID and root/lock identity captured by the activated owner. On any missing, malformed, future-version, or identity-mismatched superblock, return an error with zero operation result and do not append or alter reservation history. Keep this check inside the serialized acquire/use path so list and read-only lifecycle queries receive the same protection as writes.

Add public-API regressions using a genuine verified pair: activate/open an owner, alter the superblock to each unsupported/corrupt/different-valid-identity case, then call at least `ListPinAttempts` and a mutation such as `ReservePinPlan`. Assert fail-closed error classification, zero output, unchanged superblock and reservation bytes during the attempted operations, and no new history files. Include a compiled behavioral control with the active-operation revalidation removed; it must show the regression fails before restoring exact source.

## Scope and provenance

This does not revoke the narrow static CLEAR for the constructor's future-version decoder fix. It adds a distinct active-instance freshness HOLD; no source, runtime, provider, or full recovery acceptance is claimed. Exact head and tree are recorded above. The detached review clone was clean; `git diff --check` was clean. No source edits, tests, or builds were performed.

External original report SHA256: `31f354606e56327e3b55725da378cbe877ab0d03ffa3db2aef284ef46dca7e64`. Coordinator accepts this finding for correction under the existing narrow owner source claim.
