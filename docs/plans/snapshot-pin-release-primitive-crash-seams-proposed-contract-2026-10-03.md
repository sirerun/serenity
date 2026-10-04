# Snapshot pin/release primitive crash seams: source-grounded correction

**Status: HOLD for source freeze.** This is a doc-only design correction. It does not alter recovery-pin-owner-v1 (`6588d950610a9fca704d9afcd342f4079f64144e`), authorize producer source changes, or assert that the existing outer crash hooks satisfy its primitive-prefix matrix.

**Source pins inspected:** current `main` `bee4790cf9e53e858802b059f3e6f0af38153e41` (tree `8279a7451b79322d1edf2be9347e0fd6a754adff`); pin-owner integration review baseline `f555fc72e47f0cd3287e059a1f93d1b9189d5df1`, containing freeze `recovery-pin-owner-v1`; backup producer WIP inspected read-only at `454af2028889a567e0a05c38b62765a6cdfc5846` (tree `aade8acbbd9f20ee96cd267b88bb04199d382382`); pin-owner WIP `41a9cf4e8477a41e23b6ecd9aaa1217bbafdeb04` (tree `d2cbfec951d60f3f3cd9032540820ff8020bb6bd`). The earlier main pin `189cdabe89ff296cfab42c72d7e3b8af030f332c` is historical, not current. The prior independent HOLD is the review of proposal commit `f16d8b89e74b3f6a4ff08f7944c030a0b88f3647` (review-report SHA-256 `c6b59c1bc8f6ae41bb0c45b30167fd2a3d0cedd3e97d74c966c2bfcf56d4ec04`).

## Frozen hook boundary

Use the existing private `internal/hosted/testhooks.At` mechanism only. Ordinary builds link an unconditional no-op and do not inspect environment state. Only binaries built with `hostedtest` and started with the existing inherited anonymous control/status pipes may pause or exit at named phases. Do not add environment flags, executable-name tests, public hook APIs, HTTP handlers, mutable authority, or production cleanup switches. Add phase names only through the existing testhooks phase catalogue and its phase-list tests.

The eight outer lifecycle checkpoints in `f16d8b89` cover successful-operation boundaries, but cannot be described as producer-primitive coverage: `writeLeaseRecord` performs temp creation, a single `f.Write(raw)`, `f.Sync`, rename and directory sync before the current outer checkpoint; `removeLeaseDirectory` calls `os.Root.RemoveAll(id)` as one stdlib operation and syncs the store root after it returns. A post-return hook cannot interrupt those inner operations.

## Exact producer objects and ordering

The inspected producer WIP uses a stable lease root identity (`rootDevice`, `rootInode`) and a root lock file `.store.lock`; each lease is a direct child named by its canonical lease ID and contains `.lease.lock`, `lease.json`, `manifest.json`, `control.db`, and one `<brain-id>.bundle` file for each nonempty brain listed by the manifest. `lease.json` stores the complete canonical lease record, including exact artifact names, digests, lengths and device/inode identities. No artifact deletion may be inferred from a caller path or from an unverified directory listing.

Pin ordering is:

1. `BeginPinAttempt` commits the owner-side `PIN_PENDING` attempt.
2. `writeLeaseRecord(<root>/<lease-id>, state=PIN_PENDING)` atomically replaces `lease.json`. Its private temp basename is `.lease.tmp-<random-id>` inside an `os.Root` opened on the exact lease directory after root/lease identity checks. The helper writes the complete JSON, fsyncs the temp file, rechecks the captured directory identity, renames the temp over `lease.json`, and fsyncs the lease directory. `finishPinLocked` then fsyncs the lease directory and store root.
3. Only after those backup-owned bytes are durable does the producer call owner `CommitPinAttempt`.
4. The producer replaces `lease.json` with state `PINNED` through the same temp/write/fsync/rename/directory-sync sequence, then fsyncs the lease directory and store root. It returns the pin only after those syncs.

On restart, a durable local `PIN_PENDING` record is reconciled against `FindPinAttempt`: a still-pending attempt stays pending; a committed owner attempt is promoted to local `PINNED`. A crash after owner commit and before local promotion must not mint a new attempt or pin identity.

Release ordering is:

1. Before starting release, `releaseLease` validates the exact authorization and verifies the complete artifact set. It writes local `lease.json` as `RELEASING`.
2. It creates `.releases/<lease-id>`, writes that marker's own `lease.json` as `RELEASING`, syncs the marker tree and `.releases` directory. This durable marker binds lease ID, pin ID, plan reference, digest, reservation/attempt versions, disposition and owner release record version.
3. `removeLeaseDirectory` rechecks store-root device/inode and exact lease-directory identity, calls `os.Root.RemoveAll(lease-id)`, then fsyncs the store root.
4. After successful root sync, `writeReleaseTombstone` creates `.release.tmp-<random-id>` inside the marker root, writes/fsyncs it, renames it to `release.json`, and syncs the marker directory. The tombstone repeats the exact pin/release tuple and `RELEASED` state. The producer removes the marker's old `lease.json`, syncs the marker and `.releases`, then calls owner `CompletePinRelease`. Only afterward may it remove the release marker and sync `.releases`.

## Primitive checkpoint proposal

For `writeLeaseRecord` state transitions and `writeReleaseTombstone`, add testhooks checkpoints immediately after each successful real operation, so a child can exit with the actual filesystem at that prefix:

- exclusive temp-file creation;
- a deterministic actual partial temp write, if that checkpoint is required (the current single `Write` call has no controllable mid-call stop);
- complete temp write;
- temp-file fsync and close;
- final inode/root recheck;
- atomic rename/publish;
- temp parent-directory sync;
- outer lease-directory sync and store-root sync;
- the owner `CommitPinAttempt` return, before local `PINNED` replacement;
- local `PINNED` file publication and each containing-directory/store-root sync;
- durable `RELEASING` live record, marker directory creation, marker record publication, marker-tree sync and `.releases` sync;
- exact lease-byte removal return, root sync, tombstone temp creation/write/fsync/publish/marker sync, old marker `lease.json` removal, marker sync, `.releases` sync, owner `CompletePinRelease`, and final marker removal.

These phases describe checkpoints, not new state values or authority. Each must be reached only after the named operation succeeded. Tests must assert exact bytes, exact owner tuple, captured inode/root identities, and recovery disposition after reopening. A process-crash test does not establish power-loss durability; fsync ordering remains a separately reviewed property.

A deterministic partial-write prefix can be produced without a production environment switch by a package-private helper available only to `hostedtest` tests. It must open the already captured root/lease directory through `os.Root`, re-stat and compare device/inode identities, derive the exact temporary name grammar and canonical prospective record from real fixture state, open exclusively, write a strict prefix of those bytes, and block/exit only through `testhooks.At`. The helper may not accept an arbitrary absolute path, record, authority result or replacement root. Alternatively, the producer owner can refactor its real helper into an internal step function shared by production and tests, with production passing the existing no-op checkpoint. Independent review must ensure tests exercise the actual serialization and path rules rather than a hand-written fake record.

## Partial `RemoveAll` coverage without replacing the production algorithm

`os.Root.RemoveAll` is a single stdlib call; there is no safe callback for each internal unlink. Do not replace the anchored production recursive deletion algorithm just to make it injectable. Instead, a `hostedtest`-only recovery fixture can create one real, reachable deletion prefix:

1. Run the real release path in a child until the exact durable `.releases/<lease-id>/lease.json` RELEASING marker, then terminate at the existing inherited-pipe barrier before `RemoveAll`.
2. With the child stopped, a test-only helper opens the fixture's captured store root with `os.OpenRoot`, verifies the recorded root device/inode, opens the canonical lease-ID child, verifies its exact recorded lease-directory device/inode, reads the durable `lease.json` file inventory, and chooses one inventory-listed regular artifact (`control.db`, `manifest.json`, or a named bundle). It opens/identifies that entry beneath the same `os.Root`, verifies its recorded device/inode, removes only that basename through `Root.Remove`, then crashes the helper child at a named inherited-pipe barrier before directory/root sync.
3. Reopen the same store and owner history. This is a genuine partial artifact-removal state produced by anchored filesystem operations, without changing or substituting the production `Root.RemoveAll` implementation. Keep `.lease.lock` and release metadata intact for this first controlled prefix; add additional prefixes only with their own identity proofs.

The helper must not use `os.RemoveAll`, string-concatenated untrusted paths, symlink-following APIs, or the live process's external environment to choose a deletion target. It receives expected identity from the same private fixture that created the pinned lease, and verifies that identity at each open. The test also records what the real producer's `RemoveAll` recovery is expected to do with the resulting partial tree; the helper cannot claim to exercise the Go runtime's internal unlink order or prove all filesystem crash persistence modes.

## Blocking recovery mismatch

The inspected backup producer WIP's `finishReleaseJournal` currently calls `verifyRecordFiles(live, r)` before retrying `removeLeaseDirectory` when a durable `RELEASING` marker exists and the lease directory still exists. `verifyRecordFiles` requires every original artifact in `lease.json` to remain. Therefore the exact partial-removal fixture above currently fails before retrying the identity-bound `Root.RemoveAll`. The same routine calls `CompletePinRelease` from a local `RELEASED` tombstone without independently verifying that the live lease directory is absent and its root identity remains stable. Recovery must not infer current release authority from local records: checksums, tombstones, and `ListPinAttempts` plus `validateLocalPinAttemptPairs` establish integrity and exact local attempt correspondence, but do not establish current owner state. The backup producer WIP is `454af2028889a567e0a05c38b62765a6cdfc5846`; the distinct pin-owner WIP is `41a9cf4e8477a41e23b6ecd9aaa1217bbafdeb04`. Both expose `ReconcilePin(ctx, SnapshotPinRef)` and `CompletePinRelease`. In that pin-owner implementation, `ReconcilePin` returns `PinKeep` after owner state is already `RELEASED`, while `CompletePinRelease` accepts an exact already-released authorization idempotently; the producer must preserve this distinction to recover a crash after owner completion but before local marker cleanup.

Do not conceal this with a test helper that instead deletes `lease.json` or manually fabricates a tombstone. Before source freeze, producer and pin-owner reviewers must choose and freeze one recovery rule:

- **Branch A — durable `RELEASING` marker, no `RELEASED` tombstone:** before resumed artifact deletion or creation/repair of a new `RELEASED` tombstone, call trusted `ReconcilePin(ctx, exact SnapshotPinRef)` and require a fresh positive `PinRelease` decision with authorization exactly matching pin ID, plan reference, manifest digest, disposition, and release `RecordVersion`. Also require the complete validated local attempt set to match the exact lease ID, reservation version, and committed attempt tuple represented by the marker. Local markers, tombstones, and serialized prior authorization are integrity/recovery evidence only; `ListPinAttempts` plus `validateLocalPinAttemptPairs` proves local attempt correspondence, not current owner release authority. An owner error, cancellation, `PinKeep`, unknown/incomplete or mismatched attempt history, or mismatched authorization denies deletion and tombstone creation without removing bytes or clearing metadata. After the fresh exact `PinRelease`, recovery may tolerate missing inventory-listed artifacts caused by interrupted deletion and idempotently resume anchored `Root.RemoveAll`. Always validate the captured store-root identity and, whenever the lease directory remains, its exact recorded device/inode and non-symlink identity. If the directory is absent, still validate stable store-root identity before writing the tombstone. Persist/fsync that tombstone, then call `CompletePinRelease` with the exact authorization; this call is the trusted owner transition/acknowledgment and may succeed from either `RELEASING` or exact already-`RELEASED` state. Clear the marker only after `CompletePinRelease` returns nil.
- **Branch B — an existing valid `RELEASED` tombstone:** do not call `ReconcilePin` and require `PinRelease` here, because the owner correctly returns `PinKeep` once release is already complete. First validate the tombstone's checksum and full tuple against the release record, validate the complete local attempt set, verify the live lease directory is absent, and verify the captured store-root identity remains stable. Then call trusted `CompletePinRelease` with the exact tombstone authorization as the current owner terminal acknowledgment. Its exact already-`RELEASED` path is idempotent nil; if owner remains `RELEASING`, it verifies the exact backup release tombstone before committing `RELEASED`. Owner errors, mismatched/unknown history or authorization, a present/replaced/symlinked live lease, or root-identity mismatch deny marker clearing. Remove and sync the marker only after the owner call returns nil. A local tombstone alone is never authority.
- If that rule is not accepted, amend `recovery-pin-owner-v1` and source delivery explicitly to define the terminal behavior for partial backup-byte deletion, including why leaving the owner `RELEASING` while the snapshot bytes are incomplete is safe and how operator recovery proceeds. “The helper does not test it” is not an acceptable substitute for the frozen crash requirement.

This proposal does not authorize source changes to the producer or pin owner; it requests an exact owner decision and independent review. It does not alter root/lease identity checks, owner CAS protocol, pin absence proof, release disposition, tombstone schema, or directory-removal algorithm. For Branch A, the fresh positive `ReconcilePin` result is required before resumed deletion/new tombstone; `CompletePinRelease` is the trusted terminal transition/acknowledgment after durable tombstone persistence. Branch B must use idempotent exact `CompletePinRelease` rather than a `ReconcilePin` positive-release check. Neither branch derives authority from local disk alone.

## Review gate

Independent review must confirm the listed file names, metadata and operation order against the current exact producer source before freezing any test/source change. It must also resolve the partial-removal recovery rule and state whether the tagged test helper's single-file unlink is sufficient reachable-prefix evidence under 6588. Until then, keep this lane HOLD. No build, source ownership claim, producer authority, provider/live acceptance, PR or merge is established by this document.
