# T23.52 AWS CLI retention adapter development contract

Coordinator freeze v1 on main340 6d62fe8. This source-only extension implements the existing Python Storage protocol; no planner semantics, deployment wiring, timer, production credential acquisition, approval authority or AWS action is included. Supersedes the design addendum's scratch-file and inherited-environment proposals.

## Scope and constructor

Implement new deploy/hosted/backup_retention_awscli.py and deploy/hosted/tests/test_backup_retention_awscli.py only, plus the worker's own receipt. Import/construction never starts a process or contacts a service. No boto3 dependency. POSIX process-group support is mandatory; unsupported platforms fail closed.

AWSCLIStorage requires explicit executable, region, bucket, expected_owner, credential_env, command_timeout_seconds, stdout_limit_bytes and stderr_limit_bytes; no policy defaults. Region is exactly us-west-2. Bucket is a strict 3..63-character lowercase alphanumeric/hyphen DNS-label subset, no leading/trailing hyphen. Expected owner is exactly 12 ASCII digits. Executable is an explicit absolute canonical existing regular executable, trusted root/current-owner with no group/other writes; trusted executable and environment provenance remain caller/activation obligations. Do not discover executables, accept profiles/endpoint URLs/arbitrary arguments, or refresh credentials.

Timeout is an integer 1..120 seconds; stdout limit integer 1..8 MiB, stderr integer 1..64 KiB; bools are not integers here. credential_env contains exactly AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_SESSION_TOKEN, nonempty bounded strings without NUL/control characters (access/secret <=256 bytes, token <=16 KiB). These are explicitly supplied temporary credentials, never logged, written to files, put in argv or claimed authentic merely from shape. Constructor copies configuration so later caller dictionary mutations cannot redirect calls.

## Controlled child boundary

No ambient environment is inherited. Supply only those three credential values and fixed controls: AWS_CONFIG_FILE=/dev/null, AWS_SHARED_CREDENTIALS_FILE=/dev/null, AWS_EC2_METADATA_DISABLED=true, AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true, AWS_CLI_AUTO_PROMPT=off, AWS_PAGER empty, AWS_MAX_ATTEMPTS=1, LC_ALL=C. No HOME repurposing. Null config and explicit temporary credentials prevent profile/config, credential-process, container/IMDS and endpoint discovery from ambient state; real identity, session validity and IAM remain external qualification gates.

Every invocation uses an argv list with shell disabled, explicit --region us-west-2, --no-cli-pager, --no-paginate, --output json, --cli-error-format json and --no-cli-auto-prompt. Create a new owned process group/session. Stream stdin/stdout/stderr with a bounded selector pump, wall-clock deadline, and exact owned-group termination/reaping on limits, timeout or failure. Never buffer arbitrary output before checking limits; never leave the child's descendants running on cancellation. EOF/deadline behavior must also handle a child that exits while descendants retain a pipe. Use no temporary request files.

AWS CLI2.35.14 was locally checked to recognize structured error format. Offline skeleton validation with synthetic credentials and null config accepted --delete file:///dev/stdin and returned the expected DeleteObjects output schema; no S3 request was made. Unsupported CLI flags or output conventions fail closed. Official references: https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-parameters-file.html and https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-error-format.html . Bootstrap's unpinned awscli-2 package still needs actual deployed compatibility qualification.

## Protocol mapping and scope

All methods compare supplied bucket/expected_owner to constructor scope before spawning. Inventory prefix must be exactly snapshots/. Object keys must satisfy the existing retention library's snapshot datetime-prefix rules, valid UTF-8, <=1024 bytes and no control characters; retain its valid directory-marker objects. IDs and markers are opaque bounded nonempty strings, never shell text; continuation key markers stay in the same snapshot namespace. Paired markers are either both absent or both supplied. Part markers are valid positive integer service markers; no bool/negative values.

list_object_versions maps to s3api list-object-versions with expected owner, exact prefix, --max-keys 1000 and the explicit two markers. list_multipart_uploads maps equivalently with --max-uploads 1000 and key/upload markers. list_parts uses exact key/upload ID, --max-parts 1000 and optional part marker. One CLI process per protocol page; no automatic pagination.

delete_versions requires 1..1000 unique exact Key/VersionId entries, no extra fields or missing/null IDs, every key within scope. Preserve explicit string version ID null, if returned by S3, rather than treating it as missing. Serialize Objects and Quiet:false as strict JSON, bounded to4 MiB, and feed a private stdin pipe to --delete file:///dev/stdin. This avoids shell/argv length issues and scratch-path races. Include expected owner. A response must report exactly the requested successful Key/VersionId set with no duplicates, omissions, foreign confirmations or Errors; Quiet success or CLI exit alone cannot acknowledge deletion.

abort_multipart maps only to abort-multipart-upload with explicit bucket/owner/key/upload. Empty stdout is valid only for this operation; a successful attempt never proves all parts gone. The unchanged retention executor performs post-abort ListParts verification. All inventory operations require valid response objects; no absent/malformed page can become an empty inventory.

## Parsing and failures

Strict UTF-8 JSON, unique object keys, finite values, one document and object root; bounded depth and fields. Validate response page arrays, bool IsTruncated and required paired continuation markers with nonrepeating progress delegated to existing planner. Missing required members or malformed arrays/markers refuse. Preserve fields needed by the existing protocol.

Only the fixed invoked list_parts command with exit254 and strict structured stderr top-level Code exactly NoSuchUpload maps to UploadNotFound. AWS structured stderr has no OperationName field; do not require or infer one. Other operation/status/code, malformed or oversized stderr, AccessDenied, unavailable credentials, unknown flags, timeout and malformed success all become sanitized AdapterError; no raw stdout/stderr, credential value or exception chaining is exposed. Unexpected stderr on a nominal success fails closed. Do not change the existing planner to swallow adapter errors.

## Ownership and qualification

Worker uses a new isolated full SSD clone and canonical source claim R-hosted-backup-retention-cli before edits, plus its own unique report claim. Own only the two new files and receipt; existing backup_retention.py, backup.sh, bootstrap, services/timers, completion helpers, frozen Go contracts and registry remain read-only. Coordinator integrates, reviews and verifies; no standalone executable main or scheduler.

Use a private fake CLI subprocess, explicit synthetic credentials and owned SSD temp profiles. Prove exact argv, scope, every manual marker, JSON stdin/Quiet:false, no ambient endpoint/profile/credential inheritance, all response/error branches, duplicate/oversize/deep/trailing data, per-version confirmation and valid null version IDs. Invalid configuration/scope launches zero children. Prove timeout, output ceilings, retained descendant pipes and owned-group cleanup through deterministic barriers (no synchronization sleeps); preserve genuine RED mutations for scope, strict parser, error classifier, environment isolation and delete confirmations, then restore exact source. Existing completion and retention unit suites must pass. Fake subprocess tests qualify local adapter behavior only, not a real bucket, installed CLI compatibility, credential provenance or service atomicity.

T23.52 remains partial: COMPLETE upload/download integration, reviewed purge authority, real IAM/lifecycle, scheduling/alarms, all-version/multipart verification and authorized live 31-day qualification remain open. T23.48's journal authority and live conditional-write qualification remain separate. No AWS request, provider mutation, purge, spend, deployment or activation is authorized by this development freeze.
