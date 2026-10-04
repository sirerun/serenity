# Pin-owner and snapshot producer preliminary static review

Verdict: **HOLD for the release peak-capacity preflight at exact `9f5c7c487e4673f76666c3ea9497b117011dcd3e` (tree `f9d550dedf8a8412be9f4f0ca6f243cb18e762e`).** The active-owner superblock correction from exact `7c14eda470f555aa5b2d202bd236d95a32921a4c` remains statically clear. This is a preliminary source checkpoint only; no Go commands were run, and full source qualification remains open.

## Finding: initial release headroom can undercount, and its guard follows owner RELEASE_BEGIN

`ensureInitialReleasePeak` in `internal/hosted/backup/snapshot_lease.go:3457-3496` estimates `m` as `len(candidateRaw)` and checks the max of `base+old+m`, `base+2*m`, and `base+m+tomb`. This does not match the producer's retained-metadata accounting:

- `encodeLeaseRecord` sets `MetadataBytes` to manifest bytes plus serialized record bytes (`:3151-3190`), and the record decoder verifies that same equation (`:3380-3387`).
- `usageLocked` charges a live `RELEASING` record by `r.MetadataBytes` (`:2463-2479`) and also charges a journal `RELEASING` marker by that `MetadataBytes` (`:2551-2557`). It additionally charges tombstone and in-progress temporary bytes (`:2535-2584`).
- During the release transition, the code writes the live `RELEASING` record and marker record, then checks `ensureReleasePeakCapacity` before deleting the live tree (`:3516-3568`). At that point `usageLocked` charges both full candidate `MetadataBytes` values; the guard then adds the tombstone bytes (`:3701-3713`).

The early guard can therefore accept a metadata budget based on raw record lengths that the later guard rejects after the owner release transition and producer journal mutations have started. Specifically, `Reconcile` calls the mutating owner `ReconcilePin` before calling `releaseLease` (`snapshot_lease.go:1817-1856`; owner `pin_owner.go:598-613` appends `RELEASE_BEGIN`). `releaseLease` invokes the incomplete initial guard only afterward (`snapshot_lease.go:3509-3524`). If full simultaneous headroom is unavailable, the later check can refuse while the owner is already `RELEASING` and the backup has written its live and marker `RELEASING` records, though the pinned artifact is not deleted. A full journal at 4096 entries has the analogous ordering: the capacity check occurs in the same late initial guard after owner `RELEASE_BEGIN`.

The frozen terminal-receipt amendment requires the complete live-record, marker-record, final receipt, and temporary-write peak to be reserved before starting release, in addition to unrelated retained metadata, with checked arithmetic. Rework the first guard to use the actual metadata charges (including manifest bytes) and all simultaneous temporary/retained phases. Move or add an equivalent preflight before the owner can durably transition to `RELEASING`; preserve a second check immediately before destructive live-tree deletion. Add a real owner/producer fixture with a nonempty manifest and a configured budget that fits the current early estimate but is below the complete conservative peak. Assert refusal before owner `RELEASE_BEGIN`, before any backup `RELEASING` record or marker publication, with original owner history, backup record, journal inventory, pin bytes, and receipt inventory unchanged. Add the same boundary at 4096 direct journal entries. The new `snapshot_lease_journal_capacity_test.go` covers overflow-before-filtering, malformed direct entries, and Stage refusal at 4096 retained receipts, but it does not cover release refusal before the owner transition; I inspected the test source only and did not execute it.

## Other reviewed surfaces

Static tracing found the constructor/factory still requires the exact opaque backup identity and final activation gate. Cancellation creates its proof only while the store and exact lease locks are held; proof consumption binds the exact attempt and captured root/lock identities and is one-shot. Owner `PIN_PENDING` remains fail-closed without guessing a pin ID. Release terminal processing preserves the exact receipt directory; the terminal branch checks the exact retained owner tuple and absent live lease before acknowledgement, while resume requires exact owner authorization and the matching producer record. The direct journal scan is bounded to 4097 entries for the 4096-entry protocol ceiling, identity-checks its root and journal descriptors before/after enumeration, counts entries before filtering, and refuses unknown/noncanonical/symlink/non-directory children untouched. The test-only journal-capacity fixture matches those refusal cases at source level.

No additional static finding was established in these paths from this checkpoint. Release durability remains unqualified: source hooks exist for release boundaries, but the required per-boundary child crash/reopen/refusal matrix is still incomplete (pause/kill alone does not establish exact restart behavior). The terminal receipt amendment and source owner/producer assignment remain distinct from any startup factory, READY authority, provider, or physical-quota acceptance.

## Provenance

- Exact source head: `9f5c7c487e4673f76666c3ea9497b117011dcd3e`
- Exact source tree: `f9d550dedf8a8412be9f4f0ca6f243cb18e762e`
- Detached SSD clone was clean at the reviewed head; `git diff --check` over the reviewed integration was clean.
- Static source/test inspection only; no source edits, builds, or tests. The separately active foreign build lease was left untouched.

External original report SHA256: `ecd0c0de94db5afd2f670bc8baf7395e43304f8c83ee6ae39c71d8758dfda697`. Coordinator accepts the complete-peak and pre-authority ordering findings for correction under the producer source claim. The planned conservative encoded preflight is capacity evidence only; it grants no release authority, and fresh exact owner authorization remains required afterward.
