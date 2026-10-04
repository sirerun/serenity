# Preliminary static review — pin-owner/backup combined source

Verdict: **narrow static CLEAR for the corrected release-capacity and crash-fixture changes at this exact head**. This is not source qualification: no Go commands were run in this review lane, and full current-head build/race/vet/lint, compiled controls, and the complete producer matrix remain separate gates.

- Commit: `38f08ae6cf0c2dc3858a2b6a798733dda8cd9c60`
- Tree: `cbc0f135588839b87b6a663a76d53a6a722faf37`
- Detached review clone: `serenity-pin-owner-combined-static-review-38f08ae-20261003` (isolated external worktree)
- Working tree: clean
- Delta from the reviewed `9527b38eed8f621b9e633d245584874cdc909947`: one test-only change, capture JSON reader limit 2 MiB → 4 MiB in `internal/hosted/backup/snapshot_lease_hostedtest_crash_test.go`.

## Findings checked

The prior release-peak finding is statically closed. `ensureInitialReleasePeak` encodes the full candidate record and tombstone, uses `candidate.MetadataBytes` (including metadata charged from its manifest), and checks simultaneous current/candidate/temp/tombstone peaks with overflow-safe arithmetic (`snapshot_lease.go:3466-3506`). `ensureReleasePeakBeforeOwner` constructs a worst-valid-disposition candidate with `RecordVersion=MaxUint64` solely for conservative sizing; it does not treat those fields as release authority (`3509-3520`). Reconcile calls this guard before the owner release transition in the live PINNED path (`1817-1825`); the resume path guards before its authorization/reconciliation work as well (`1714-1726`). Later deletion/publication guards remain in place. The independent source-frozen tests still need to prove these paths dynamically.

The crash matrix now has 40 listed barrier cases (15 pin, 25 release, including five temp-prefix cases). For each barrier it kills the paused child, reconstructs test authority only from the child’s reported actual attempt and release authorization, snapshots the retained tree, and reopens via the real constructor/preflight path. A refusal must leave the tree and owner attempt unchanged and perform no owner completion; success checks the expected pending, pinned, or released terminal state (`snapshot_lease_hostedtest_crash_test.go:205-489`). The five partial-temp fixtures validate the captured record tuple/state and actual root/directory/temp-file identities before writing a strict prefix of bytes from the actual encoded temp payload (`537+`). This removes the earlier fabricated-PinID concern. The test-only capture hook runs after temp creation and before the barrier; the normal-build implementation is a no-op (`snapshot_lease_capture_hostedtest.go`, `snapshot_lease_capture_normal.go`, and `snapshot_lease.go` call sites). The new 4 MiB `LimitReader` is bounded and accommodates JSON/base64 framing for the bounded record payload; it does not change production code.

The terminal receipt capacity test is correctly framed as a storage-capacity test, not owner authority: it writes actual encoded tombstone records, confirms charged metadata/counts, and confirms Stage refuses without changing inventory or usage (`snapshot_lease_journal_capacity_test.go`). I do not require 4,096 owner-authenticated terminal records; the documented owner-side plan/attempt limits make that an invalid fixture. Real-pair release ordering and refusal coverage must remain separately qualified.

## Remaining gate

The earlier `9f5c7c4...` HOLD finding is superseded for the corrected source by this static review; its historical report remains preserved. This result does not clear the producer matrix, runtime crash behavior, compiled behavioral mutants, or any factory/READY/provider/production acceptance. No source was changed and no build or test was run.

External original report SHA256: `2d5ee38823e65e2e199f4f3b5fd00a433e960ceee006ee4facf53bf172456483`. Coordinator records implementation completion only. Current producer tests, compiled behavioral controls, full integrated checks and final exact-head review remain open; no merge or runtime acceptance follows from this report.
