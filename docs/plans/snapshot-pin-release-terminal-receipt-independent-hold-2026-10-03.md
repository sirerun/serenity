# Independent review — terminal release receipt amendment

**Verdict: HOLD before source freeze.** The amendment has the right correspondence rule and refuses unsafe work, but its claimed existing bounded journal enumeration ceiling is not present in the source. The amendment must name and enforce one before it can safely promise persistent receipts.

Reviewed exact document commit `118ee79de4d1b9ceccdaefe2d6c8a7a41e627938`, `docs/plans/snapshot-pin-release-terminal-receipt-proposed-amendment-2026-10-03.md`.

## Supported design

Retaining the existing exact checksummed `.releases/<leaseID>/release.json` is the minimal compatible fix: it carries the tuple required by `validateLocalPinAttemptPairs`, does not create a new authority/API/wire arm, and allows exact idempotent `CompletePinRelease` after reopening. The document correctly rejects omission from the owner list, permissive missing-pair validation, receipt fabrication, automatic repair, and compaction. It also correctly requires metadata headroom before destructive deletion and blocks Stage when finite retained capacity is exhausted.

## Blocking bound mismatch

The amendment says receipts remain within an “existing bounded enumeration ceiling,” but current producer source has no bounded `.releases` directory scan. `usageLocked` uses `os.ReadDir(journal)` at `internal/hosted/backup/snapshot_lease.go:2110`; `finishReleaseJournal` independently uses unbounded `os.ReadDir(journal)` at `2891-2906`. These allocate the full entry list before receipt validation. The existing `maxSnapshotPinAttempts = 4096` (`snapshot_lease.go:48`) bounds the owner-list length, not the journal directory enumeration. The separate `MaxLeases` maximum also does not cap terminal receipt entries once live leases are removed.

Persistent receipts make this a lasting boundary: memory consumed by arbitrary or accumulated journal entries cannot be claimed bounded by metadata-byte accounting, because both current scans enumerate names before calculating/checking metadata. The phrase “existing bounded enumeration ceiling” therefore overstates current enforcement.

## Required document correction before freeze

Specify a fixed finite maximum for retained release-journal entries using an already-frozen protocol ceiling (the current `maxSnapshotPinAttempts` value 4096 is the source-grounded candidate), including valid terminal receipts, in-progress release records, and unknown entries. Require bounded `ReadDir(limit+1)`/equivalent enumeration in **both** `usageLocked` and `finishReleaseJournal`, reject overflow or unknown/unrecognized entries before partial processing, and ensure Stage refuses before publication once the cap is reached. Add the corresponding over-limit directory controls. This does not need a new user option or wire field.

The prescribed metadata preflight should also say it reserves the complete peak transition footprint before any lease-byte deletion: live record, in-progress journal record, and final retained receipt, each charged until safely replaced/removed. A check that only accounts the final tombstone after deleting the live lease would not meet the document's own fail-before-destruction rule.

No builds or source edits were made. The producer/owner terminal-correlation bug remains open; the previous pin-owner source findings and other crash-matrix gates also remain open.
