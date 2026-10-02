# T23.52 AWS CLI publisher transport readiness

**Status: viable transport design, no adapter or live qualification approved.** The publisher supports artifacts up to 500 GB, which needs multipart upload. Its read interface is key-only and does not explicitly pin object versions. S3 conditional writes alone do not guarantee an all-version no-overwrite property.

## Evidence and pin

PR344 landed as 763eb7b; the reviewed source tree is 3277f6aa59b3eea70716a726e54e16bd47c6437f (available in the read-only assembly checkout). The seam is deploy/hosted/backup_publish.py: RemoteObject has key, version_id, delete_marker; Storage.read_bytes and read_to take only a key.

deploy/hosted/backup_retention_awscli.py provides the reusable process and credential controls: explicit trusted executable and temporary credentials; fixed region/expected owner; sanitized environment; argv without a shell; bounded manual pages, output, and time; anchored process cleanup. Its receipt says it is unwired; local offline parser evidence is for CLI 2.35.14 only; bootstrap awscli-2 is unpinned; and the runtime caller must prove exclusive child reaping.

Installed CLI reports aws-cli/2.35.14 Python/3.14.6 Darwin/25.6.0 arm64. Local help confirms this version exposes expected-owner and manual marker options for list-object-versions; body, if-none-match and expected-owner for put-object; file body/expected owner for upload-part; conditional completion and expected owner for complete-multipart-upload; and version-id/expected owner/local outfile for get-object. This is local syntax evidence only. The production AL2023 ARM CLI version and checksum remain unknown.

## Minimal adapter design

Add inert AWSCLIPublisherStorage that reuses the existing executable validation, environment, subprocess runner, timeout/output bounds, region/owner settings, and process-ownership preconditions. Limit it to one general-purpose versioned bucket in the existing region. Reuse strict UTF-8/duplicate-key parsing. Construction makes no request or child. No ambient profile, endpoint discovery, shell, or blind retries.

### Inventory

For each exact snapshots/YYYYMMDDTHHMMSSZ/ prefix, call s3api list-object-versions with expected owner, encoding-type url, max-keys 1000, no-paginate, and both continuation markers on later pages. No delimiter. Count every version and delete marker toward max_objects; fail closed on overflow. Require IsTruncated bool and advancing paired markers when true. Reject malformed pages, repeated/no-progress markers, common prefixes, prefix escapes, and service errors. Return every version and marker; never collapse by key.

Strictly URL-decode returned keys and both key markers once before comparing or reuse. Reject malformed escapes and invalid UTF-8. Require IsLatest=true for a sole ordinary version and reject null version IDs. The retention adapter validates marker shapes and IsLatest types, but does not request URL encoding; add explicit translation tests.

### Writes

Spool source bytes to a new private mode-0600 file, bounded by declared length, chunk size, and hash, before any service write. The existing runner stdin is a small in-memory request path, not a 500 GB object channel. Use file-backed s3api operations.

For objects up to 5 GB use PutObject with If-None-Match: * and expected owner. Larger objects require CreateMultipartUpload, file-backed UploadPart calls, then CompleteMultipartUpload with If-None-Match: *. S3 allows at most 10,000 parts of 5 MiB to 5 GiB (the last may be smaller), so 500 GB fits. Choose part size so each command should fit the existing 120-second deadline; actual throughput is unqualified. Preserve returned ETags exactly in bounded completion metadata; ETag is not the publisher SHA-256. Abort only the upload ID created by this call on error. If cleanup cannot be proved, fail without a receipt. Add an incomplete-multipart lifecycle rule because object-version inventory cannot see orphan MPUs.

Treat 412 as ObjectExists even for identical bytes. A 409, timeout, malformed response, or uncertain artifact write fails; do not blindly retry. Keep final COMPLETE ambiguity reconciliation in the publisher: accept only its exact byte readback and full verification.

AWS defines If-None-Match against the current version. In a versioned bucket it succeeds when there is no current object or when the current version is a delete marker. Therefore it is not an all-history condition alone. Deployment must verify Versioning=Enabled, reject directory buckets, deny runtime object/version deletes and version suspension, and enforce conditional writes for PutObject and CompleteMultipartUpload. AWS bucket-policy guidance requires s3:ObjectCreationOperation handling to exempt multipart initiation/part operations that do not accept conditional headers. Constrain lifecycle/admin mutations during publication. A preflight list cannot close a concurrent-delete race.

### Reads and seam limitation

Implement read_bytes over the same bounded path. For read_to use get-object with exact --version-id and --expected-bucket-owner to a unique private file; validate response identity, length, bounds, regular-file type, and stable file metadata; copy to the supplied destination in chunks no larger than chunk_bytes; detect short writes and clean up on every exit. Publisher remains responsible for SHA-256 checks.

The seam exposes version IDs in inventory but does not pass one to either read method; download_snapshot reads COMPLETE and manifest before inventory. A hidden per-prefix cache depends on call order and serialized adapter reuse. Recommended minimum contract correction: inventory first, pass exact version_id to read_bytes/read_to, and carry those selected versions through final verification. Do not qualify key-only GET as version-pinned.

Scratch headroom must be measured by the caller. The local snapshot, verification stages, and CLI file-backed upload/download can each add snapshot-sized allocations. Tests on an 8 GiB fixture prove private ownership/mode, not 500 GB capacity.

## Fake-process tests

1. Inventory argv/owner; paired-marker pagination and 1000-boundary across versions/markers; valid optional collection omission; malformed/missing IsTruncated; absent/repeated markers; duplicate keys/history; delete markers; IsLatest=false; null IDs; URL-encoding round-trip; malformed escapes/UTF-8; prefix escape; CommonPrefixes; output/max_objects overflow.
2. Small upload exact length/hash before request; private mode; conditional and expected-owner argv; valid VersionId; 412 mapping; 409/timeout/malformed JSON/null ID/source mutation/short or extra bytes/no space/cleanup failure leave no COMPLETE.
3. Multipart 5 GB boundary; bounded parts <=10,000; ordered parts and exact ETags; expected owner on all calls; conditional completion; abort only owned upload; failures never complete; no blind 409 retry; orphan lifecycle; two-writer 412 and current-delete-marker controls.
4. Reads pin version and owner; reject wrong ID/size, over-limit/truncated body, mutation, short destination, oversized chunk, stderr/output overflow; verify secure scratch/cleanup and bounded-memory copy.
5. Import/construction and invalid scope start zero children; no credential argv, shell, endpoint, profile or ambient inheritance; reuse T23.52 timeout/retained-pipe/signal/reap cases; exercise big fake files without buffering in memory.
6. Keep existing 20 completion/18 publisher tests green; fake-CLI rich 1000-brain round trip proves 1003 keys, bounds and staging-only download result.

## Open live gates

- Pin/checksum and qualify the exact AL2023 ARM CLI; local 2.35.14 does not qualify unpinned bootstrap package.
- Verify general-purpose bucket, region, expected owner, enabled versioning; exclude directory buckets.
- Review role/bucket/access-point policy, KMS, lifecycle and break-glass: required reads/list versions, conditional writes, no runtime delete/version suspension, incomplete MPU cleanup, and no expiration of active/retained objects.
- Establish credential/executable provenance, process/reaper ownership, private mount and physical quota in the real runner.
- Qualify timeout, part size, scratch headroom, requests/cost and full 500 GB readback.
- Only after separate authorization, use isolated nonproduction S3 for versioning, owner mismatch, conditional races, delete markers, lost responses, full inventory, pinned GET and cleanup. No AWS service request was made here.

## Primary references

- [Conditional writes and versioned-bucket behavior](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)
- [Enforce conditional writes by bucket policy](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes-enforce.html)
- [ListObjectVersions API](https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectVersions.html)
- [CLI list-object-versions](https://docs.aws.amazon.com/cli/latest/reference/s3api/list-object-versions.html)
- [CLI put-object](https://docs.aws.amazon.com/cli/latest/reference/s3api/put-object.html)
- [CLI complete-multipart-upload](https://docs.aws.amazon.com/cli/latest/reference/s3api/complete-multipart-upload.html)
- [CLI get-object](https://docs.aws.amazon.com/cli/latest/reference/s3api/get-object.html)
- [Multipart limits](https://docs.aws.amazon.com/AmazonS3/latest/userguide/qfacts.html)
- [Versioning-enabled writes](https://docs.aws.amazon.com/AmazonS3/latest/userguide/AddingObjectstoVersioningEnabledBuckets.html)
- [Versioning-suspended writes/null IDs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/AddingObjectstoVersionSuspendedBuckets.html)
- [S3 consistency](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html)

No source, deployment, credentials, provider request, or resource claim was changed or made.

