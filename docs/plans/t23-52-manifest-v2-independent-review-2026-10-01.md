# T23.52 manifest-v2 independent review — pinned-source receipt

Reviewed exact source `dd87ddd2b246359870be8f28efec16ec48f11387` in isolated full clone `/Volumes/BuildOffload/worktrees/serenity-manifest-v2-independent-review-20261001` (full clone created with no hardlinks; detached checkout; source tree clean before review). This is a source-specific receipt, not final T23.52 acceptance.

## Result

The backup package's existing guard coverage is substantial, but this source is **not cleared**. Three known blockers remain: typed-nil deletion-journal interfaces pass the `journal == nil` guard; nil contexts are not rejected before context-aware work; and the tests do not exercise a real filesystem journal watermark implementation. In addition, manifest decoding is not strict: `readManifest` uses `json.Unmarshal` without rejecting unknown fields or duplicate JSON keys. Duplicate keys are resolved by Go's decoder using the later value, leaving security-sensitive manifest inputs ambiguous. Confirm whether closed-schema semantics are required by the v2 contract; if so this is a blocker to acceptance.

The implementation does validate v2 version and artifact metadata, checks the control database digest/length/schema and restored inventory, compares bundle heads, rejects symlinked manifest/artifacts and unsafe/duplicate inventory, stages restore data privately, publishes through rename, refuses existing destinations, and freezes restored credentials/accounts into `restore_pending` state. Tests cover control checksum tampering, bundle tampering and manifest head tampering, inventory mismatch, unsafe paths, overwrite refusal, future schema, and credential revocation. These checks do not establish production readiness or end-to-end backup qualification.

## Validation

- `go test -race -p 1 ./internal/hosted/backup` — PASS, 20.656s.
- Before the test, `uptime` reported load averages 4.75, 5.38, 5.96. The command used external `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, and `TMPDIR` under `/Volumes/BuildOffload`.
- No source or test mutations were made during validation. No AWS, S3, or production calls were made.
- Ajent polling instruction could not be completed: no Ajent tool was available and no `ajent.social` file was present in this clone.

## Scope limits

This is a bounded library review only. No production adapter, S3, IAM, scheduler, uploader integration, restore acceptance, or full T23.52 qualification is claimed. Author's reported package race result is not substituted for this independent run. Re-review the new exact source after the author correction; append a new receipt instead of altering this pinned-source history.
