# T23.52 manifest-v2 corrected-source review — hold receipt

Reviewed exact author source `b88594e03d284c0ed82e6b787dc88b0928a0fd94` in isolated full clone `/Volumes/BuildOffload/worktrees/serenity-manifest-v2-review-b88594-20261001`. The clone was made with `--no-hardlinks`; because the author repository itself referenced an alternate object directory, I repacked the reachable objects into this clone and removed only this clone's alternates file before review. The tree was clean at the pinned source before temporary runtime controls.

## Result

The previous typed-nil journal, nil-context, and missing real journal fixture issues are corrected in this source. `Create` and `Restore` use nil-interface checks, and `TestCreateManifestWatermarkComesFromFilesystemJournal` exercises a real filesystem journal entry against the manifest watermark.

This commit remains **on hold** for manifest JSON strictness. `readManifest` still accepts unknown JSON properties and duplicate object keys. A temporary test that added the valid top-level property `unexpected_security_mode` to an otherwise valid manifest demonstrated that `Restore` accepts it. Parent/coordinator already requested duplicate-key and unknown-field rejection; review the exact parser correction before any clearance.

Other source checks reviewed include v2/schema and inventory validation, checksum/length verification, bundle verification and local-head matching, `os.Root`-bounded reads, `Lstat` rejection of non-regular manifest/artifacts, private staging, destination no-overwrite guards, and `restore_pending` credential/session freezing. The manifest decoder's unambiguous closed-schema behavior is still an unmet gate.

## Validation

- `go test -race -p 1 ./internal/hosted/backup` — PASS, 18.628s, with external build and temp caches; immediately beforehand uptime loads were 3.66/4.98/5.70.
- Unknown-manifest-field runtime control — RED as intended: the test failed because Restore accepted the unsupported field. Temporary test source was removed.
- Checksum-validation removal control — RED as intended: `TestRestoreRejectsTamperedControlDBChecksum` failed with `err = <nil>`. The validation line was restored.
- Destination no-overwrite removal control — RED as intended: after temporarily disabling both Restore destination existence checks, `TestRestoreRefusesExistingDestination` failed because Restore published over the existing empty destination. Both checks were restored.
- Pre-clone bundle-head comparison removal control — the existing tampered-head test remained GREEN because the separate post-restore local-head comparison independently refused publication. This establishes the combined publication barrier, not the independent necessity of the pre-clone comparison.
- `git diff --check` and final status were clean after restoring all temporary source/test controls. No lasting production/test mutation was made.
- No AWS/S3/production calls or full T23.52 qualification were performed.

## Scope limits

This is a bounded library review. It does not clear the strict JSON parser hold or certify production adapters, backup scheduling/uploader wiring, IAM, S3, or production restore acceptance. The prior pinned-source receipt remains unchanged; this document supplements it for the author correction commit.
