# T23.52 AWS CLI storage adapter design addendum

This coordinator-selected design supersedes the boto3 proposal in `t23-48-52-journal-retention-adapter-discovery-2026-10-01.md`. It describes an unwired source-only adapter; no AWS invocation was made.

## Boundary and API proposal

Create `deploy/hosted/backup_retention_awscli.py`, implementing the existing `backup_retention.Storage` protocol, and a separate `deploy/hosted/tests/test_backup_retention_awscli.py`. Keep both outside the existing planner and deployment wiring. Proposed constructor:

```python
AWSCLIStorage(*, executable: str, region: str, bucket: str,
              expected_owner: str, temp_root: str,
              command_timeout_seconds: int = 60,
              stdout_limit_bytes: int = 64 * 1024 * 1024,
              stderr_limit_bytes: int = 64 * 1024)
```

Exact defaults and ceilings remain coordinator policy to freeze. The executable must be an explicit absolute path and the remaining scope values nonempty and validated. Each protocol call still supplies `bucket` and `expected_owner`; mismatches fail before spawning. Inventory operations must use only the exact `snapshots/` prefix; object keys for parts/abort and every delete key must match that namespace. The adapter constructs no client, discovers no executable, loads no credentials, and triggers no operation at import/construction time.

Each child receives a fixed argv list with `shell=False`, an explicit `--region`, `--no-cli-pager`, `--no-paginate`, `--output json`, and `--cli-error-format json`. Do not accept arbitrary CLI args, profile names, endpoint URLs, shell fragments, or caller-selected prefixes. Runtime credential/config/environment provenance must be explicitly reviewed before later wiring; this helper must not allow endpoint overrides from environment/config to silently redirect requests. The official CLI references document these flags, the single-page behavior of `--no-paginate`, and machine-readable JSON error format ([pagination](https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-pagination.html), [structured errors](https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-error-format.html)).

Map existing protocol methods directly to AWS CLI v2 `s3api` operations:

- `list_object_versions`: `list-object-versions --bucket B --expected-bucket-owner O --prefix snapshots/ --max-keys 1000`, plus both `--key-marker` and `--version-id-marker` when continuing. Return the exact response fields (`Versions`, `DeleteMarkers`, `IsTruncated`, `NextKeyMarker`, `NextVersionIdMarker`) to the planner. [The CLI reference documents these markers and expected-owner flag.](https://docs.aws.amazon.com/cli/latest/reference/s3api/list-object-versions.html)
- `list_multipart_uploads`: `list-multipart-uploads --bucket B --expected-bucket-owner O --prefix snapshots/ --max-uploads 1000`, with both key and upload-ID markers when continuing; preserve the exact truncation and next-marker response.
- `list_parts`: `list-parts --bucket B --expected-bucket-owner O --key K --upload-id U --max-parts 1000`, with the optional part-number marker. Only this operation may translate a definitive `NoSuchUpload` response into the protocol's `UploadNotFound`. [The command is paginated and supports expected owner.](https://docs.aws.amazon.com/cli/latest/reference/s3api/list-parts.html)
- `delete_versions`: reject empty batches and batches over 1000; require exact nonempty key and `VersionId` for every entry and enforce the fixed prefix. Serialize a strict JSON `{"Objects":[...],"Quiet":false}` request to a mode-0600 file under an owned private temp directory and pass `--delete file://PATH` as one argv item. Pass `--expected-bucket-owner O`. Quiet must remain false so the planner can verify each requested version's outcome; any `Errors` response or omitted success confirmation fails. [DeleteObjects accepts up to 1000 identifiers; verbose mode reports per-item success, while quiet omits it.](https://docs.aws.amazon.com/cli/latest/reference/s3api/delete-objects.html)
- `abort_multipart`: `abort-multipart-upload --bucket B --expected-bucket-owner O --key K --upload-id U`. Treat this as an attempt only; the planner must re-list parts and verify that no parts remain.

Use strict, size-bounded JSON parsing for stdout and structured stderr: reject duplicate keys, trailing documents, invalid UTF-8, oversized payloads, malformed roots, missing required members, and invalid pagination shapes. Never convert a malformed or truncated page into an empty inventory. For failures, parse stderr only when exit status is 254 and the JSON top-level `Code` is exactly `NoSuchUpload` on `list_parts`; otherwise return a safe generic adapter error without raw stderr. AWS documents 254 as a service-error status and warns that 255 is a general catch-all, not a stable classifier ([return codes](https://docs.aws.amazon.com/cli/latest/topic/return-codes.html)); structured JSON errors use a top-level `Code` ([error format](https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-error-format.html)). Unknown CLI versions/status/output conventions fail closed.

Avoid `subprocess.run(capture_output=True)` followed by a size check, because it buffers arbitrary output before checking. Use a bounded streaming pump for stdout/stderr, fixed wall-clock timeout, and owned process-group termination/reaping if the timeout or byte ceiling trips. Request JSON lives only in the mode-0700 temp root and the adapter removes only the exact private files/directories it created. Use `--no-cli-auto-prompt` as an additional noninteractive safeguard if it is supported by the deployment's pinned CLI v2; otherwise set `AWS_CLI_AUTO_PROMPT=off` in the controlled child environment.

## Local controls

Run only against an isolated fake executable that records argv and emits fixture documents; never inherit developer AWS profiles or credentials. Prove exact args and JSON request content for each method; bucket/owner/prefix/key mismatch is rejected with zero child processes; all manual markers pass through and the CLI is invoked once per protocol page; only the exact ListParts structured `NoSuchUpload` plus expected CLI status maps to `UploadNotFound`; `AccessDenied`, malformed or wrong-operation error JSON, status 255, configuration errors, timeout and oversized outputs remain errors; duplicate/trailing/malformed/truncated success JSON fails closed; delete has `Quiet:false`, the <=1000 limit and exact per-version confirmations; command timeout/output-limit handling kills and reaps only the owned process group; and scratch JSON permissions/cleanup hold for success and every failure path. These tests establish local argument translation and parser behavior only.

## Explicit limitations

This is not a purge runner or authority mechanism. The class contains a destructive delete primitive because it implements the frozen `Storage` seam, but it must remain unwired. Do not add `purge.py`, service/timer installation, backup.sh changes, download integration, IAM/credential configuration, endpoint URLs, production CLI entry point, plan signing/approval, or object deletion in this slice. The source-only adapter does not prove which account/role the real executable uses; freeze an explicit child environment/config policy and production owner/account provenance before wiring.

T23.52 still requires manifest-bound COMPLETE upload/download integration, exact reviewed dry-run/apply authority, all versions/delete markers and multipart cleanup with verification, installed hourly scheduling (task57), runtime proof (task64), and disposable-resource qualification of the 31-day window (task66). T23.48's current-main Go journal adapter still needs its separate current stack/IAM/conditional-write and generation-adoption qualifications. No result in this report marks either task accepted.
