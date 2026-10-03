# Active pin-owner superblock freshness re-review

Verdict: **PRELIMINARY STATIC CLEAR for the active-superblock freshness finding at exact head `7c14eda470f555aa5b2d202bd236d95a32921a4c` (tree `0b6c0db29282101a782a1135c3645c4c8d96b044`).** This only clears the specific static finding reported at `b0a4c822f930b06b5444db51d7dc1c427f82655d`; it is not runtime qualification or the broader owner/producer contract acceptance. No Go commands were run.

## Correction verified

`pinOwnerWithLock` now acquires the owner and backup roots/locks, then calls `pinOwnerValidateSuperblock` before invoking any operation callback (`internal/hosted/recovery/pin_owner.go:706-731`). The validator reads and decodes the current bounded superblock from the acquired owner-root descriptor and requires the StoreID plus all captured owner-root, owner-lock, backup-root, and backup-lock identities to match (`:1436-1448`). This covers read and write callbacks, including `ListPinAttempts`, `ReservePinPlan`, and `ReconcilePin`. `pinOwnerAppend` repeats the validation before constructing and publishing a record (`:1482-1494`).

The new `TestPinOwnerActiveOperationsRefuseChangedSuperblock` exercises a genuine pair and an already-active owner against a future-version superblock, a bad v1 checksum, and a newly checksummed v1 superblock with a different StoreID. It invokes reserve, list, and reconcile after mutation; checks expected error classes and zero results; and verifies the superblock inode/bytes, owner-root entries, reservation entries, and existing record bytes/inodes remain unchanged (`internal/hosted/recovery/pin_owner_test.go:699-836`). This is inspection of the regression source, not a claim that it ran.

The earlier constructor-only decoder review remains separately CLEAR: valid future versions are classified unsupported before strict v1 decoding; malformed current-version structures remain corrupt. This re-review did not assess the unrelated backup journal-capacity test or producer WIP, and it does not close the combined crash-matrix or full owner/producer verification gates.

## Provenance

- Exact head: `7c14eda470f555aa5b2d202bd236d95a32921a4c`
- Exact tree: `0b6c0db29282101a782a1135c3645c4c8d96b044`
- Detached SSD review clone was clean at the exact head; `git diff --check` on the correction was clean.
- Static code/test inspection only. No source edits, Go commands, builds, or test execution.

External original report SHA256: `7a0790acc8983dca2267a4e6f52657a1d4d8c8ff09cb1bc77b267305c75e3266`. A later additional prepublication check was integrated from authora3526dc8; no fresh runtime result is inferred from this preliminary static report.
