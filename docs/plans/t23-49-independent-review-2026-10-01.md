# T23.49 backup candidate independent review (2026-10-01)

Reviewed the isolated receipt tree at `20657af391a3d885bfcae2a659f537bdb577bbad` (code parent `8d5cd65fd59cf625f19a74c7f0067c0d043e5a6e`). This review covers only the backup package candidate. **T23.49 remains PARTIAL; this is not an acceptance or deployment qualification.**

## Findings

The package implementation satisfies the inspected local backup/restore invariants. `Create` refuses absent canonical Git history, unsafe `.git` entries, and dirty working trees; it inventories ready brains from the staged control DB and emits sorted brain entries with bundle/control DB SHA-256 and byte lengths. `Restore` validates manifest shape and safe single-component artifact names, rejects symlink/non-regular manifest and artifact entries, verifies copied bytes and bundle heads, and compares the complete ready-brain inventory with the restored DB before publication. Relevant code is in [backup.go](../../internal/hosted/backup/backup.go) and manifest validation in [contracts/backup.go](../../internal/hosted/contracts/backup.go). Tests exercise truncated/tampered artifacts and manifests, unsupported schemas, inventory mismatch, symlinks, legacy-v1 refusal, and credential/session/OAuth invalidation in [backup_test.go](../../internal/hosted/backup/backup_test.go).

Restore checks the manifest schema range, then opens the exact verified copy with SQLite read-only/immutable mode for integrity and schema checks before calling `store.Open`; at this revision schema 9 is supported and the future-schema test exercises `store.SchemaVersion+1` (10). Restore freezes accounts and subscriptions, deletes sessions/login tokens/OAuth rows, and revokes client credentials before atomically publishing the private staged tree.

Git bundle restore uses `gitrun.CloneBundle`, which requires a regular local bundle, clones in a fresh workspace with Git ancestor discovery capped, quarantines global/system configuration, and disables network transports. Backup-side Git inspection/bundling uses the hardened quarantine runner. The backup package race suite passed: `go test -race -count=1 ./internal/hosted/backup` (`ok`, 32.056s) on the exact receipt tree.

## Blocking integration and scope gaps

- **No production deletion journal is wired.** `Create` requires a `DeletionJournal`, reads its watermark before copying, and fails on nil/read errors. The production caller has not been updated and package tests use a fake journal. The integration request documents the missing service/gateway field and adapter and explicitly rejects a fabricated empty watermark. Until the real T23.48 journal is assembled and service-level tests prove its watermark is used, this is only a package capability, not a truthful production backup. See [integration-request.md](../launch/evidence/T23.49/integration-request.md) and the T23.49 partial receipt.
- **No `COMPLETE` record or upload/download verification exists in this package.** The local manifest binds artifact hashes and lengths, but there is no separate completion object binding the manifest hash. That requirement appears under the later hosted-backup storage task (T23.52), so it remains a downstream storage/integration gate rather than a defect in T23.49's local manifest implementation.
- **Coordinated maintenance remains a caller obligation.** `Create` does not acquire the exclusive writer/service maintenance lock; its comments require the caller to coordinate ownership, SQLite capture, and canonical Git flush. Existing package tests do not establish assembled live-write consistency. Preserve and verify the service maintenance lock and `Pool.FlushAll` when wiring the production caller.

## Handoff

Keep T23.49 PARTIAL until the real journal and existing service assembly are integrated and tested under the coordinated backup window. Keep T23.52's upload completion marker separate: require download-side verification of the completion record, manifest hash, and artifact checksums before restore. No implementation files were changed during this review.
