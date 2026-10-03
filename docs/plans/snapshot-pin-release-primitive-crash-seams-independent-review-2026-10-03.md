# Independent read-only preflight: snapshot pin release recovery

**Verdict: CLEAR for the document’s authority/recovery design correction at proposal commit `096c56d8753e2694cc086b5799d7757b072a2634` (tree `d2b14497aeb055e4fb92809fd58f32d870318e68`). Source freeze and implementation acceptance remain HOLD pending producer-owned crash controls, source changes, and root review.** This is a static review only; no source was edited and no build was run.

## Exact source inspected

- Current main: `bee4790cf9e53e858802b059f3e6f0af38153e41`, tree `8279a7451b79322d1edf2be9347e0fd6a754adff`; `internal/hosted/backup/snapshot_lease.go` SHA-256 `88a82db073ca75be07804615ef4c25c83cfeeebb7b947dafa461007fcccb900d`.
- Pin-owner implementation: `41a9cf4e8477a41e23b6ecd9aaa1217bbafdeb04`; `internal/hosted/recovery/pin_owner.go` SHA-256 `c26dcf880bbc7df98cbff6c73571dcd2712322dc5a8004554eaba8bebd05fb9a`.

The proposal now records the current main pin and explicitly marks the earlier `189cd...` pin historical. Its authority branches match the inspected owner API behavior.

## Findings

Branch A is the correct gate for a durable `RELEASING` marker without a `RELEASED` tombstone. The owner must freshly return `PinRelease` for the exact pin reference before any resumed `RemoveAll` or tombstone creation/repair. The local marker and checksum are recovery/integrity evidence, not current authority. The complete validated committed-attempt set must bind the lease ID, reservation and attempt versions, plan, and manifest to the marker; the returned release authorization must match pin, plan, digest, disposition, and owner release record version. This composes with the frozen API, where `ReconcilePin` receives the pin reference and `PinReleaseAuthorization` carries its release tuple. Owner `PinKeep`, errors, cancellation, or any tuple/identity uncertainty must prevent deletion and tombstone mutation.

The distinction in Branch B is necessary. `ReconcilePin` returns `PinKeep` after the owner reaches `RELEASED`, so requiring another positive `PinRelease` would strand a valid tombstone after a lost completion response. A fresh exact `CompletePinRelease` is the appropriate terminal acknowledgement: for `RELEASING`, the owner verifies the exact backup tombstone and records `RELEASED`; for exact already-`RELEASED`, it returns idempotent success. Branch B correctly requires a checksum-validated tombstone, complete local attempt correspondence, stable backup-root identity, and the live lease to be absent before that call. Marker cleanup only follows a nil owner response. A present, replaced, symlinked, or identity-mismatched lease must prevent cleanup.

The proposal also resolves the partial-delete retry failure. Current `finishReleaseJournal` calls `verifyRecordFiles` before retrying `RemoveAll`; an artifact already removed by an interrupted `RemoveAll` makes that retry fail. It also calls `CompletePinRelease` for an existing tombstone without first proving the live lease absent. `Reconcile` does validate the bounded owner attempt list and local pairs before entering `finishReleaseJournal`, but the current release-journal branches do not ask `ReconcilePin` before resumed deletion. The proposal’s Branch A/Branch B rules close those gaps while retaining fail-closed root and lease-directory identity checks.

The single-file fixture is a valid reachable partial-tree state test if it uses the described anchored root and inventory identity checks, then reopens the real store and verifies that production `Root.RemoveAll` completes the remaining deletion. It does not simulate an interruption inside Go’s opaque `os.Root.RemoveAll`, establish its unlink order, or prove power-loss persistence. The proposal states those limits accurately; do not label this as an internal-RemoveAll primitive crash test. Producer primitive checkpoints and their behavioral controls remain an implementation/source-acceptance gate.

One wording constraint should remain explicit during implementation: the frozen `PinReleaseAuthorization` does not itself carry lease ID or reservation/attempt versions. Those fields are bound by the complete validated `PinAttemptRef` and the exact local marker/record pair; the authorization itself is only the release pin/plan/digest/disposition/record-version tuple. Do not claim that the authorization value alone carries the full attempt tuple.

## Required implementation follow-through

1. Before resumed deletion or writing/repairing a new tombstone, re-read the exact release marker, validate the authoritative attempt list and local pair, revalidate root/lock and extant lease-directory identity, then call `ReconcilePin`; require `PinRelease` and exact authorization match.
2. Once durable `RELEASING` authority is freshly confirmed, allow the identity-bound `Root.RemoveAll` retry to tolerate inventory files already absent. Keep checksum/schema and directory-identity failures fatal. If the leaf is absent, validate the store root before tombstone persistence.
3. For a pre-existing valid `RELEASED` tombstone, prove exact full tuple and owner-attempt correspondence, prove the live lease is absent and store-root identity stable, call exact `CompletePinRelease`, and clear/sync the marker only on success. Do not require `ReconcilePin == PinRelease` for this already-terminal branch.
4. Preserve the proposal’s `hostedtest`-only hooks and single-file reachable-prefix fixture; clearly distinguish that fixture from an actual crash inside `Root.RemoveAll` and from power-loss durability.

The proposal remains **HOLD for source freeze** as its own status says. This review clears its authority/recovery design correction only; producer crash-matrix qualification, the two separate pin-owner implementation findings root reported, full-module requalification after fixes, and final source acceptance remain outstanding.
