"""Fail-closed planner/executor for snapshot-prefix S3 retention.

This library is intentionally unwired. Storage is supplied by a caller-owned
adapter; no AWS client or credentials are constructed here.
"""

from __future__ import annotations

import hashlib
import json
import re
from collections.abc import Mapping, Sequence
from dataclasses import asdict, dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Protocol

SNAPSHOTS_PREFIX = "snapshots/"
RETENTION_AGE = timedelta(days=29)
MULTIPART_AGE = timedelta(days=1)


class RetentionError(ValueError):
    """Inventory, authorization, or execution was incomplete or unsafe."""


class Storage(Protocol):
    """Minimal storage seam; implementations must return one raw API page."""

    def list_object_versions(self, *, prefix: str, key_marker: str | None,
                             version_id_marker: str | None, expected_owner: str) -> Mapping[str, Any]: ...

    def list_multipart_uploads(self, *, prefix: str, key_marker: str | None,
                               upload_id_marker: str | None, expected_owner: str) -> Mapping[str, Any]: ...

    def list_parts(self, *, key: str, upload_id: str, part_number_marker: int | None,
                   expected_owner: str) -> Mapping[str, Any]: ...

    def delete_versions(self, *, objects: Sequence[Mapping[str, str]],
                        expected_owner: str) -> Mapping[str, Any]: ...

    def abort_multipart(self, *, key: str, upload_id: str, expected_owner: str) -> None: ...


@dataclass(frozen=True)
class Limits:
    pages: int = 10_000
    versions: int = 1_000_000
    uploads: int = 100_000
    parts: int = 1_000_000
    plan_bytes: int = 64 * 1024 * 1024

    def __post_init__(self) -> None:
        if any(type(v) is not int or v <= 0 for v in asdict(self).values()):
            raise RetentionError("limits must be positive integers")


DEFAULT_LIMITS = Limits()


@dataclass(frozen=True, order=True)
class Version:
    key: str
    version_id: str
    delete_marker: bool
    last_modified: str
    snapshot_prefix: str


@dataclass(frozen=True, order=True)
class Upload:
    key: str
    upload_id: str
    initiated: str
    snapshot_prefix: str


@dataclass(frozen=True)
class Plan:
    bucket: str
    expected_owner: str
    prefix: str
    generated_at: str
    cutoff: str
    multipart_cutoff: str
    versions: tuple[Version, ...]
    uploads: tuple[Upload, ...]
    sha256: str


@dataclass(frozen=True)
class Result:
    plan_sha256: str
    deleted_versions: int
    aborted_uploads: int
    oldest_surviving_snapshot: str | None
    oldest_surviving_last_modified: str | None
    oldest_surviving_snapshot_age_seconds: int | None
    oldest_surviving_last_modified_age_seconds: int | None


def _utc(value: datetime, label: str) -> datetime:
    if not isinstance(value, datetime) or value.tzinfo is None or value.utcoffset() is None:
        raise RetentionError(f"{label} must be timezone-aware")
    return value.astimezone(timezone.utc).replace(microsecond=0)


def _iso(value: datetime) -> str:
    return _utc(value, "timestamp").isoformat().replace("+00:00", "Z")


def _parse_time(value: Any, label: str) -> datetime:
    if not isinstance(value, str):
        raise RetentionError(f"malformed {label}")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise RetentionError(f"malformed {label}") from error
    return _utc(parsed, label)


def _snapshot_prefix(key: Any) -> tuple[str, datetime]:
    if not isinstance(key, str) or len(key.encode("utf-8", errors="strict")) > 1024 or not key.startswith(SNAPSHOTS_PREFIX):
        raise RetentionError("foreign snapshot key")
    parts = key.split("/", 2)
    if len(parts) != 3 or parts[0] != "snapshots" or not re.fullmatch(r"[0-9]{8}T[0-9]{6}Z", parts[1]):
        raise RetentionError("malformed snapshot key")
    try:
        stamp = datetime.strptime(parts[1], "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc)
    except ValueError as error:
        raise RetentionError("malformed snapshot timestamp") from error
    return f"snapshots/{parts[1]}/", stamp


def _mapping(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise RetentionError(f"malformed {label} page")
    return value


def _token(page: Mapping[str, Any], truncated_key: str, first: str, second: str) -> tuple[str | None, str | None]:
    truncated = page.get(truncated_key)
    if type(truncated) is not bool:
        raise RetentionError("malformed pagination state")
    if not truncated:
        return None, None
    a, b = page.get(first), page.get(second)
    if not isinstance(a, str) or not a or not isinstance(b, str) or not b:
        raise RetentionError("truncated page lacks continuation markers")
    return a, b


def _json_plan(plan: Plan, include_hash: bool) -> bytes:
    body = {
        "bucket": plan.bucket, "expected_owner": plan.expected_owner,
        "prefix": plan.prefix, "generated_at": plan.generated_at,
        "cutoff": plan.cutoff, "multipart_cutoff": plan.multipart_cutoff,
        "versions": [asdict(item) for item in plan.versions],
        "uploads": [asdict(item) for item in plan.uploads],
    }
    if include_hash:
        body["sha256"] = plan.sha256
    return (json.dumps(body, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def _plan_hash(plan: Plan) -> str:
    return hashlib.sha256(_json_plan(plan, False)).hexdigest()


def serialize_plan(plan: Plan, *, limits: Limits = DEFAULT_LIMITS) -> bytes:
    """Return the canonical reviewable plan bytes after verifying its digest."""
    if not isinstance(plan, Plan) or _plan_hash(plan) != plan.sha256:
        raise RetentionError("plan digest mismatch")
    body = _json_plan(plan, True)
    if len(body) > limits.plan_bytes:
        raise RetentionError("serialized plan size limit exceeded")
    return body


def load_plan(body: bytes, *, limits: Limits = DEFAULT_LIMITS) -> Plan:
    """Load only the canonical serialization emitted by :func:`serialize_plan`."""
    if not isinstance(body, bytes) or len(body) > limits.plan_bytes:
        raise RetentionError("plan document exceeds size limit")

    def unique_object(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise RetentionError("duplicate plan field")
            result[key] = value
        return result

    try:
        raw = json.loads(body, object_pairs_hook=unique_object)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise RetentionError("malformed plan document") from error
    if not isinstance(raw, dict) or set(raw) != {
        "bucket", "expected_owner", "prefix", "generated_at", "cutoff", "multipart_cutoff",
        "versions", "uploads", "sha256",
    }:
        raise RetentionError("plan fields are incomplete or unknown")
    version_fields = {"key", "version_id", "delete_marker", "last_modified", "snapshot_prefix"}
    upload_fields = {"key", "upload_id", "initiated", "snapshot_prefix"}
    if not isinstance(raw["versions"], list) or not isinstance(raw["uploads"], list):
        raise RetentionError("plan inventories are malformed")
    if any(not isinstance(item, dict) or set(item) != version_fields for item in raw["versions"]):
        raise RetentionError("plan version entry is malformed")
    if any(not isinstance(item, dict) or set(item) != upload_fields for item in raw["uploads"]):
        raise RetentionError("plan upload entry is malformed")
    try:
        plan = Plan(raw["bucket"], raw["expected_owner"], raw["prefix"], raw["generated_at"],
                    raw["cutoff"], raw["multipart_cutoff"],
                    tuple(Version(**item) for item in raw["versions"]),
                    tuple(Upload(**item) for item in raw["uploads"]), raw["sha256"])
    except (TypeError, KeyError) as error:
        raise RetentionError("plan entries are malformed") from error
    if _plan_hash(plan) != plan.sha256 or _json_plan(plan, True) != body:
        raise RetentionError("plan digest or canonical encoding mismatch")
    if len(plan.versions) > limits.versions or len(plan.uploads) > limits.uploads:
        raise RetentionError("plan item limit exceeded")
    return plan


def _checked_page_count(current: int, limits: Limits) -> None:
    if current > limits.pages:
        raise RetentionError("inventory page limit exceeded")


def inventory(storage: Storage, *, bucket: str, expected_owner: str,
              now: datetime, limits: Limits = DEFAULT_LIMITS) -> Plan:
    """Take a complete, bounded inventory. It never truncates at a limit."""
    if not isinstance(bucket, str) or not bucket or not isinstance(expected_owner, str) or not expected_owner:
        raise RetentionError("bucket and expected owner are required")
    captured = _utc(now, "now")
    cutoff, mpu_cutoff = captured - RETENTION_AGE, captured - MULTIPART_AGE
    versions: list[Version] = []
    uploads: list[Upload] = []
    key_marker = version_marker = None
    seen_version_tokens: set[tuple[str, str]] = set()
    page_no = 0
    while True:
        page_no += 1
        _checked_page_count(page_no, limits)
        page = _mapping(storage.list_object_versions(prefix=SNAPSHOTS_PREFIX, key_marker=key_marker,
                                                      version_id_marker=version_marker,
                                                      expected_owner=expected_owner), "version")
        for field, marker in (("Versions", False), ("DeleteMarkers", True)):
            records = page.get(field)
            if not isinstance(records, list):
                raise RetentionError("missing or malformed version inventory")
            for record in records:
                record = _mapping(record, "version")
                key, version_id = record.get("Key"), record.get("VersionId")
                if not isinstance(version_id, str) or not version_id or len(version_id) > 1024:
                    raise RetentionError("malformed version identifier")
                prefix, _ = _snapshot_prefix(key)
                modified = _iso(_parse_time(record.get("LastModified"), "LastModified"))
                versions.append(Version(key, version_id, marker, modified, prefix))
                if len(versions) > limits.versions:
                    raise RetentionError("version inventory limit exceeded")
        key_marker, version_marker = _token(page, "IsTruncated", "NextKeyMarker", "NextVersionIdMarker")
        if key_marker is None:
            break
        token = (key_marker, version_marker or "")
        if token in seen_version_tokens:
            raise RetentionError("repeated version continuation marker")
        seen_version_tokens.add(token)

    key_marker = upload_marker = None
    seen_upload_tokens: set[tuple[str, str]] = set()
    while True:
        page_no += 1
        _checked_page_count(page_no, limits)
        page = _mapping(storage.list_multipart_uploads(prefix=SNAPSHOTS_PREFIX, key_marker=key_marker,
                                                        upload_id_marker=upload_marker,
                                                        expected_owner=expected_owner), "multipart")
        records = page.get("Uploads")
        if not isinstance(records, list):
            raise RetentionError("missing or malformed multipart inventory")
        for record in records:
            record = _mapping(record, "upload")
            key, upload_id = record.get("Key"), record.get("UploadId")
            if not isinstance(upload_id, str) or not upload_id or len(upload_id) > 1024:
                raise RetentionError("malformed upload identifier")
            prefix, _ = _snapshot_prefix(key)
            initiated = _iso(_parse_time(record.get("Initiated"), "multipart Initiated"))
            uploads.append(Upload(key, upload_id, initiated, prefix))
            if len(uploads) > limits.uploads:
                raise RetentionError("multipart inventory limit exceeded")
        key_marker, upload_marker = _token(page, "IsTruncated", "NextKeyMarker", "NextUploadIdMarker")
        if key_marker is None:
            break
        token = (key_marker, upload_marker or "")
        if token in seen_upload_tokens:
            raise RetentionError("repeated multipart continuation marker")
        seen_upload_tokens.add(token)

    ordered_versions = tuple(sorted(versions))
    ordered_uploads = tuple(sorted(uploads))
    version_ids = [(item.key, item.version_id) for item in ordered_versions]
    if len(version_ids) != len(set(version_ids)):
        raise RetentionError("duplicate key/version identifier")
    plan = Plan(bucket, expected_owner, SNAPSHOTS_PREFIX, _iso(captured), _iso(cutoff),
                _iso(mpu_cutoff), ordered_versions, ordered_uploads, "")
    plan = Plan(**{**asdict(plan), "versions": ordered_versions, "uploads": ordered_uploads, "sha256": _plan_hash(plan)})
    if len(_json_plan(plan, True)) > limits.plan_bytes:
        raise RetentionError("serialized plan size limit exceeded")
    return plan


def _eligible(plan: Plan, item: Version | Upload) -> bool:
    if isinstance(item, Version):
        _, captured = _snapshot_prefix(item.key)
        return captured <= _parse_time(plan.cutoff, "cutoff")
    return _parse_time(item.initiated, "multipart Initiated") <= _parse_time(plan.multipart_cutoff, "multipart cutoff")


def _parts_empty(storage: Storage, upload: Upload, expected_owner: str, limits: Limits) -> None:
    marker: int | None = None
    seen: set[int] = set()
    page_no = total = 0
    while True:
        page_no += 1
        _checked_page_count(page_no, limits)
        page = _mapping(storage.list_parts(key=upload.key, upload_id=upload.upload_id,
                                           part_number_marker=marker, expected_owner=expected_owner), "parts")
        parts = page.get("Parts")
        if not isinstance(parts, list):
            raise RetentionError("missing or malformed parts inventory")
        total += len(parts)
        if total > limits.parts or total:
            raise RetentionError("multipart parts remain after abort")
        truncated = page.get("IsTruncated")
        if type(truncated) is not bool:
            raise RetentionError("malformed parts pagination state")
        if not truncated:
            return
        next_marker = page.get("NextPartNumberMarker")
        if type(next_marker) is not int or next_marker <= 0 or next_marker in seen:
            raise RetentionError("malformed parts continuation marker")
        seen.add(next_marker)
        marker = next_marker


def _check_plan(plan: Plan, approved_sha256: str, bucket: str, expected_owner: str,
                now: datetime, limits: Limits) -> None:
    if not isinstance(plan, Plan) or plan.prefix != SNAPSHOTS_PREFIX or plan.bucket != bucket or plan.expected_owner != expected_owner:
        raise RetentionError("plan scope does not match fixed policy")
    if not isinstance(approved_sha256, str) or plan.sha256 != approved_sha256 or _plan_hash(plan) != plan.sha256:
        raise RetentionError("plan approval hash mismatch")
    if len(_json_plan(plan, True)) > limits.plan_bytes:
        raise RetentionError("serialized plan size limit exceeded")
    if _utc(now, "now") < _parse_time(plan.generated_at, "plan generation"):
        raise RetentionError("plan is from the future")
    generated = _parse_time(plan.generated_at, "plan generation")
    if plan.cutoff != _iso(generated - RETENTION_AGE) or plan.multipart_cutoff != _iso(generated - MULTIPART_AGE):
        raise RetentionError("plan age policy is invalid")
    if len(plan.versions) > limits.versions or len(plan.uploads) > limits.uploads:
        raise RetentionError("plan item limit exceeded")
    if tuple(sorted(plan.versions)) != plan.versions or len(set(plan.versions)) != len(plan.versions):
        raise RetentionError("plan versions are not canonical")
    if tuple(sorted(plan.uploads)) != plan.uploads or len(set(plan.uploads)) != len(plan.uploads):
        raise RetentionError("plan uploads are not canonical")
    for item in plan.versions:
        prefix, _ = _snapshot_prefix(item.key)
        _parse_time(item.last_modified, "LastModified")
        if (prefix != item.snapshot_prefix or not isinstance(item.version_id, str) or not item.version_id or len(item.version_id) > 1024
                or type(item.delete_marker) is not bool):
            raise RetentionError("malformed version in plan")
    for item in plan.uploads:
        prefix, _ = _snapshot_prefix(item.key)
        _parse_time(item.initiated, "multipart Initiated")
        if (prefix != item.snapshot_prefix or not isinstance(item.upload_id, str)
                or not item.upload_id or len(item.upload_id) > 1024):
            raise RetentionError("malformed upload in plan")


def apply(storage: Storage, plan: Plan, *, approved_sha256: str, bucket: str,
          expected_owner: str, now: datetime, limits: Limits = DEFAULT_LIMITS) -> Result:
    """Apply only an explicitly hash-approved plan after fresh inventory match."""
    current = _utc(now, "now")
    _check_plan(plan, approved_sha256, bucket, expected_owner, current, limits)
    if (_iso(current - RETENTION_AGE) != plan.cutoff
            or _iso(current - MULTIPART_AGE) != plan.multipart_cutoff):
        raise RetentionError("approved cutoff changed; create a new plan")
    fresh = inventory(storage, bucket=bucket, expected_owner=expected_owner, now=current, limits=limits)
    if fresh.versions != plan.versions or fresh.uploads != plan.uploads:
        raise RetentionError("current inventory differs from approved plan")

    targets = [item for item in plan.versions if _eligible(plan, item)]
    deleted = 0
    for offset in range(0, len(targets), 1000):
        batch = targets[offset:offset + 1000]
        if any(not _eligible(plan, item) or _snapshot_prefix(item.key)[1] > current - RETENTION_AGE for item in batch):
            raise RetentionError("candidate is no longer eligible")
        expected = {(item.key, item.version_id) for item in batch}
        response = _mapping(storage.delete_versions(objects=[{"Key": item.key, "VersionId": item.version_id} for item in batch],
                                                    expected_owner=expected_owner), "delete")
        errors = response.get("Errors", [])
        removed = response.get("Deleted")
        if not isinstance(errors, list) or errors or not isinstance(removed, list):
            raise RetentionError("version deletion failed")
        got: set[tuple[str, str]] = set()
        for record in removed:
            record = _mapping(record, "delete result")
            pair = (record.get("Key"), record.get("VersionId"))
            if pair not in expected or pair in got:
                raise RetentionError("unexpected or duplicate deletion result")
            got.add(pair)
        if got != expected:
            raise RetentionError("deletion response omitted requested versions")
        deleted += len(got)

    mpu_targets = [item for item in plan.uploads if _eligible(plan, item)]
    for upload in mpu_targets:
        if _parse_time(upload.initiated, "multipart Initiated") > current - MULTIPART_AGE:
            raise RetentionError("multipart upload is no longer eligible")
        storage.abort_multipart(key=upload.key, upload_id=upload.upload_id, expected_owner=expected_owner)
        _parts_empty(storage, upload, expected_owner, limits)

    after = inventory(storage, bucket=bucket, expected_owner=expected_owner, now=current, limits=limits)
    remaining = [item for item in after.versions if _snapshot_prefix(item.key)[1] <= current - RETENTION_AGE]
    remaining_uploads = [item for item in after.uploads if _parse_time(item.initiated, "multipart Initiated") <= current - MULTIPART_AGE]
    if remaining or remaining_uploads:
        raise RetentionError("eligible retention data remains after purge")
    surviving = [_snapshot_prefix(item.key)[1] for item in after.versions]
    modified = [_parse_time(item.last_modified, "LastModified") for item in after.versions]
    oldest_snapshot = min(surviving) if surviving else None
    oldest_modified = min(modified) if modified else None
    return Result(
        plan.sha256, deleted, len(mpu_targets),
        _iso(oldest_snapshot) if oldest_snapshot else None,
        _iso(oldest_modified) if oldest_modified else None,
        max(0, int((current - oldest_snapshot).total_seconds())) if oldest_snapshot else None,
        max(0, int((current - oldest_modified).total_seconds())) if oldest_modified else None,
    )
