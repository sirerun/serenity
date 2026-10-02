# T23.52 publisher source milestone — 2026-10-02

**Status: partial implementation evidence only; T23.52 remains open.** This receipt records a bounded publisher/downloader source seam. It does not establish task acceptance, provider qualification, production recovery authority, a usable installed backup path, or retention/purge completion.

## Source and ownership

- Repository source base: `c3d491b03980310d7d59688d092b8119640e5250`.
- Implementation commit: `b2b1a7982508d89f23f68f4e77191b247e015f46` on `hosted/t23.52-publisher-20261001`.
- Implementation files: `deploy/hosted/backup_publish.py` (SHA-256 `30e3ac48150784b5946ef4fe9b92269d2786d3fd8f0c556e15623d529cf6559d`) and `deploy/hosted/tests/test_backup_publish.py` (SHA-256 `c4c84add463d18643ad5ef7e8b6b09b87d48ad5223daaa62507e0c5bc4ee05e3`).
- The implementation is isolated to the two newly assigned files. Existing completion, retention, shell, systemd, journal, task registry, and qualification sources were not changed.

## Implemented boundary

`publish_snapshot(snapshot, snapshot_prefix, *, staging_root, storage, limits)` validates an existing v2 snapshot and its artifact set/checksums from caller-owned private staging. It refuses any already-present prefix inventory, streams artifacts and the manifest through the `Storage` protocol using bounded chunks and create-if-absent semantics, reads back and verifies remote bytes, then writes the hash-bound `COMPLETE` marker last. An ambiguous final write yields a success receipt only after readback proves the exact marker and all content. It has no legacy timestamp-completion fallback.

`download_snapshot(snapshot_prefix, *, staging_parent, storage, limits)` checks `COMPLETE`, the bound manifest, complete remote inventory and each listed artifact while writing bounded content into a newly created private stage. It reuses the existing completion verifier and returns only that verified staging path. It does not restore, activate, or return deletion-journal authority.

The explicit frozen `Limits` type has no implicit defaults. The supported fixture envelope includes 1,000 brains plus `control.db` (1,001 artifacts; 1,003 object keys including manifest and completion record), with configured per-snapshot total artifact bytes up to 30 GiB and bounded object, manifest, completion, JSON complexity, chunk, and staging sizes. A 100-account × 10-brain rich fixture succeeds. This is a code/test envelope, not evidence that a 30 GiB volume is provisioned or that the complete 500 GB aggregate Scale quota is supported operationally.

`Storage` is only a transport contract: complete all-version/delete-marker prefix inventory, atomic `put_if_absent`, bounded `read_bytes`, and bounded `read_to`. Tests use a fake implementation. No AWS adapter, credential handling, provider API behavior, S3 atomicity/version semantics, or pagination implementation is delivered or qualified.

## Validation performed

Using the existing owned private APFS test fixture at `/Volumes/SerenityPrivateFixture20261001/tmp` with resource warnings treated as errors:

```sh
SERENITY_RECOVERY_TEST_TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp PYTHONDONTWRITEBYTECODE=1 TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp python3 -W error::ResourceWarning -m unittest deploy.hosted.tests.test_backup_publish deploy.hosted.tests.test_backup_completion -v
ruff check deploy/hosted/backup_publish.py deploy/hosted/tests/test_backup_publish.py
git diff --check
```

Result: 38 unit tests passed (18 publisher tests and 20 existing completion-helper tests); Ruff and whitespace checks passed. Fake-transport tests cover 1,000-brain success, interruption across publication phases, ambiguous final-write reconciliation, incomplete/readback failure, changed staging, pre-existing prefix/delete marker, invalid paths/extras/symlinks, byte/object bounds, malformed and over-complex JSON, and bounded/failed download cleanup. These are local fixture tests only; they are not production or live qualification.

## Remaining acceptance gates

- Implement and qualify a real bounded transfer adapter, including exhaustive/paginated all-version and delete-marker inventory and proven atomic create-if-absent behavior.
- Establish trusted deletion-journal provenance and production recovery admission/factory wiring before any backup/restore path can claim journal authority. The new module intentionally does not infer authority from bytes, hashes, or booleans.
- Integrate the publisher into an authorized backup runner, with single-run admission, real staging/headroom and cleanup behavior, metrics, installed service/timer ownership, and task57/task64 integration evidence. The installed `backup.sh` and units remain unchanged.
- Implement the task's purge runner plus installable hourly service/timer, dry-run/apply destructive-action authority, all-version/delete-marker/multipart/pagination behavior, deletion-journal retention, IAM/alarms requirements, and task66 live qualification. No object deletion or deployment occurred here.
- Resolve T23.48/T23.49 dependency facts and complete T23.52's full acceptance matrix, including retention age/cutoff guarantees and operational qualification.

No runtime, provider, deployment, restore, or delete operation was performed. Do not interpret this partial source milestone as satisfying T23.52.
