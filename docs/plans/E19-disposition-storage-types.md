# E19 — SQLite disposition storage compatibility

Valid disposition JSON stored as SQLite TEXT was readable, but an update compared
it with a BLOB-bound snapshot. The storage-class mismatch caused repeated lost
compare-and-swap attempts even when the bytes had not changed. Normal application
writes already use BLOB; SQL JSON edits can produce TEXT in the same column.

- [x] T19.1 Compare readable disposition snapshots by exact bytes  Owner: pool  Est: 30m  verifies: [UC-026]  deps: [T14.1]  acc: [TEXT and BLOB rows can be decided and bookkept; changed bytes lose even when JSON is semantically equivalent; stale snapshots cannot append history; real CLI reproduces the old hang and completes after the fix; executed tests and negative controls are recorded]

The transaction now casts the stored snapshot to BLOB for comparison. Atomic
history insertion and the winning update are unchanged. This does not normalize
JSON or relax conflict detection and requires no schema migration.

Verified 2026-09-08: 1,742 passing race-test cases across 58 packages, six explicit
skips. Both changed packages passed vet and lint with zero issues. Two injected
comparison faults failed their tests. The baseline real CLI remained pending with
zero history until a three-second synthetic-process timeout; the fixed CLI
recorded the rejection once in 0.594 seconds, preserving its payload. No live
model calls were made. Evidence: `docs/evals/disposition-storage-types.json`.
