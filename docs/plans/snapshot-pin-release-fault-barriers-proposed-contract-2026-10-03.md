# Snapshot pin/release deterministic crash seam proposal

Status: source preflight proposal only; no production protocol or authority change. Parent narrow pin-owner contract6588 requires real producer crash-prefix coverage that recovery-owned files cannot inject inside backup fsync/removal primitives.

Use only the existing `internal/hosted/testhooks.At` mechanism: ordinary builds link unconditional no-op without environment lookup; only explicit `hostedtest` binaries with inherited anonymous control pipes activate barriers. No environment/executable-name process-exit switch, new runtime API, mutable authority or credential access.

Proposed source ownership is limited to `internal/hosted/backup/snapshot_lease.go`, new backup-owned fault-barrier tests/probe, and append-only named phase constants/list checks in `internal/hosted/testhooks/testhooks.go` and `phases_test.go`. No pin authority, state enum, CAS, identity, cleanup, filesystem policy, shared contract or factory changes.

Named checkpoints, each only after the indicated existing operation succeeds:

- `snapshot_pin_pending_written`: after atomic local PIN_PENDING record write returns, before lease directory/root fsync.
- `snapshot_pin_pending_durable`: after lease directory/root fsync, before owner CommitPinAttempt.
- `snapshot_pin_owner_committed`: after successful owner CommitPinAttempt, before local PINNED promotion.
- `snapshot_pin_pinned_durable`: after successful local PINNED write and containing directory/root sync, before returning the pin.
- `snapshot_release_marker_durable`: after durable RELEASING marker tree/journal sync, before exact lease-directory removal.
- `snapshot_release_bytes_removed`: after exact identity-bound lease-directory removal succeeds, before RELEASED tombstone write.
- `snapshot_release_tombstone_written`: after exact RELEASED tombstone write succeeds, before removing the old release record and syncing marker/journal.
- `snapshot_release_tombstone_durable`: after successful final marker/journal sync, before returning to owner CompletePinRelease.

These eight boundaries do not individually inject inside writeLeaseRecord's temporary-file write/fsync/rename or within removeLeaseDirectory's per-file deletion. The frozen owner's temp-write/CAS crash matrix covers owner primitives only. If independent review requires producer primitive substeps to satisfy the frozen matrix, those are an explicit additional dependency rather than implied covered.

Acceptance: independent static checkpoint placement review; ordinary tagged/untagged pipe controls prove production cannot crash; compiled mutation removing an actual checkpoint causes a meaningful child test failure; root full local checks for the resulting exact combined source; pin-owner tagged child tests exercise real pair reopen at every reachable named boundary. No source acceptance until new tests prove activation/placement and all mandatory frozen requirements are accounted for. Existing source holds remain authoritative.
