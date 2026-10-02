"""Bounded manifest-v2 object publisher and downloader.

This module deliberately has no provider client, credential discovery, restore,
or journal-admission factory. Callers must supply an object store and a local
snapshot already created by the trusted hosted backup coordinator. A verified
byte tree is not proof of snapshot provenance or recovery authority.
"""

from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import os
import re
import shutil
import stat
import sys
import tempfile
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import BinaryIO, Protocol

_COMPLETION_PATH = Path(__file__).with_name("backup_completion.py")
_COMPLETION_SPEC = importlib.util.spec_from_file_location("backup_completion", _COMPLETION_PATH)
if _COMPLETION_SPEC is None or _COMPLETION_SPEC.loader is None:
    raise RuntimeError("cannot load backup completion verifier")
backup_completion = importlib.util.module_from_spec(_COMPLETION_SPEC)
_COMPLETION_SPEC.loader.exec_module(backup_completion)

_SNAPSHOT_PREFIX = re.compile(r"snapshots/(\d{8}T\d{6}Z)/\Z")
_ARTIFACT_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]*\Z")
_BRAIN_ID = re.compile(r"[A-Za-z0-9]{16,64}\Z")
_SHA256 = re.compile(r"[0-9a-f]{64}\Z")
_MAX_MANIFEST_BYTES = 16 * 1024 * 1024
_MAX_COMPLETION_BYTES = 4096
_MAX_BRAINS = 1000
_MAX_ARTIFACTS = _MAX_BRAINS + 1
_MAX_OBJECTS = _MAX_ARTIFACTS + 2  # manifest.json and COMPLETE
_MAX_CHUNK_BYTES = 1024 * 1024
_MAX_SNAPSHOT_BYTES = 500_000_000_000


class PublishError(RuntimeError):
    """The exact remote snapshot could not be proven complete."""


class ObjectExists(PublishError):
    """An object or prefix already exists and must not be overwritten."""


class Storage(Protocol):
    """Bounded, credential-free transfer seam supplied by the caller.

    Implementations must fully enumerate all versions and delete markers under
    the requested prefix or fail, enforce max_objects/max_bytes while
    streaming, and make put_if_absent atomic. An implementation must not
    silently truncate, paginate incompletely, or select ambient credentials.
    No implementation is supplied here. Atomic put-if-absent behavior on a
    versioned S3 bucket requires separate real-adapter qualification.
    """

    def list_prefix(self, prefix: str, *, max_objects: int) -> Sequence[RemoteObject]: ...

    def put_if_absent(self, key: str, source: BinaryIO, *, length_bytes: int,
                      sha256: str, chunk_bytes: int) -> None: ...

    def read_bytes(self, key: str, *, max_bytes: int) -> bytes: ...

    def read_to(self, key: str, destination: BinaryIO, *, max_bytes: int,
                chunk_bytes: int) -> int: ...


@dataclass(frozen=True)
class Limits:
    """Explicit supported envelope; this contract does not narrow hosted scale."""

    max_snapshot_bytes: int
    max_manifest_bytes: int
    max_completion_bytes: int
    max_brains: int
    max_artifacts: int
    max_objects: int
    chunk_bytes: int
    max_json_depth: int
    max_json_nodes: int

    def __post_init__(self) -> None:
        values = (self.max_snapshot_bytes, self.max_manifest_bytes,
                  self.max_completion_bytes, self.max_brains,
                  self.max_artifacts, self.max_objects, self.chunk_bytes,
                  self.max_json_depth, self.max_json_nodes)
        if any(type(value) is not int or value <= 0 or value > sys.maxsize for value in values):
            raise ValueError("all transfer limits must be positive integers")
        if self.max_snapshot_bytes > _MAX_SNAPSHOT_BYTES:
            raise ValueError("snapshot bound exceeds the 500 GB supported ceiling")
        if self.max_manifest_bytes > _MAX_MANIFEST_BYTES or self.max_completion_bytes > _MAX_COMPLETION_BYTES:
            raise ValueError("record bound exceeds its helper limit")
        if self.max_brains > _MAX_BRAINS or self.max_artifacts > _MAX_ARTIFACTS:
            raise ValueError("inventory bound exceeds the 1,000-brain supported ceiling")
        if self.max_artifacts < self.max_brains + 1 or self.max_objects != self.max_artifacts + 2:
            raise ValueError("artifact/object limits do not cover the configured brain inventory")
        if self.chunk_bytes > _MAX_CHUNK_BYTES:
            raise ValueError("transfer chunks may not exceed 1 MiB")
        if self.max_json_depth > 16 or self.max_json_nodes > 50_000:
            raise ValueError("JSON complexity limit exceeds the supported parser ceiling")


@dataclass(frozen=True)
class PublishReceipt:
    snapshot_prefix: str
    manifest_sha256: str
    object_count: int
    artifact_bytes: int


@dataclass(frozen=True)
class _Artifact:
    name: str
    length_bytes: int
    sha256: str


@dataclass(frozen=True)
class RemoteObject:
    key: str
    version_id: str
    delete_marker: bool


class _UploadReader:
    """Hash and length guard around one streamed upload source."""

    def __init__(self, stream: BinaryIO, *, length_bytes: int, sha256: str,
                 chunk_bytes: int) -> None:
        self._stream = stream
        self._length = length_bytes
        self._expected = sha256
        self._chunk = chunk_bytes
        self._digest = hashlib.sha256()
        self.read_bytes = 0

    def read(self, size: int = -1) -> bytes:
        if size == 0:
            return b""
        request = self._chunk if size is None or size < 0 else min(size, self._chunk)
        data = self._stream.read(request)
        if not isinstance(data, bytes):
            raise PublishError("upload source returned non-byte data")
        self.read_bytes += len(data)
        if self.read_bytes > self._length:
            raise PublishError("upload source exceeds manifest length")
        self._digest.update(data)
        return data

    def verify(self) -> None:
        if self.read_bytes != self._length or self._digest.hexdigest() != self._expected:
            raise PublishError("storage did not consume the exact manifest-bound upload bytes")


def _put_bytes(storage: Storage, key: str, body: bytes, limits: Limits) -> None:
    digest = hashlib.sha256(body).hexdigest()
    source = _UploadReader(io.BytesIO(body), length_bytes=len(body), sha256=digest,
                           chunk_bytes=limits.chunk_bytes)
    storage.put_if_absent(key, source, length_bytes=len(body), sha256=digest,
                          chunk_bytes=limits.chunk_bytes)
    source.verify()


def _put_file(storage: Storage, key: str, path: Path, item: _Artifact,
              limits: Limits) -> None:
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_size != item.length_bytes:
            raise PublishError("upload source changed before transfer")
        source = _UploadReader(stream, length_bytes=item.length_bytes,
                               sha256=item.sha256, chunk_bytes=limits.chunk_bytes)
        storage.put_if_absent(key, source, length_bytes=item.length_bytes,
                              sha256=item.sha256, chunk_bytes=limits.chunk_bytes)
        source.verify()
        after = os.fstat(stream.fileno())
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
            after.st_size, after.st_mtime_ns, after.st_ctime_ns
        ):
            raise PublishError("upload source changed during transfer")


def _prefix(value: str) -> str:
    if not isinstance(value, str):
        raise PublishError("snapshot prefix must be text")
    match = _SNAPSHOT_PREFIX.fullmatch(value)
    if not match:
        raise PublishError("snapshot prefix must be snapshots/YYYYMMDDTHHMMSSZ/")
    try:
        parsed = datetime.strptime(match.group(1), "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc)
    except ValueError as error:
        raise PublishError("snapshot prefix timestamp is invalid") from error
    if parsed.strftime("%Y%m%dT%H%M%SZ") != match.group(1):
        raise PublishError("snapshot prefix timestamp is not canonical")
    return value


def _duplicate_rejecting_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise PublishError("duplicate JSON field")
        result[key] = value
    return result


def _check_json_complexity(body: str, *, max_depth: int, max_nodes: int) -> None:
    depth = 0
    nodes = 0
    in_string = False
    escaped = False
    previous = ""
    for char in body:
        if in_string:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == '"':
                in_string = False
            previous = char
            continue
        if char == '"':
            in_string = True
            nodes += 1
        elif char in "[{":
            depth += 1
            nodes += 1
            if depth > max_depth:
                raise PublishError("JSON nesting exceeds its depth limit")
        elif char in "]}":
            depth -= 1
            if depth < 0:
                raise PublishError("JSON nesting is malformed")
        elif (char.isdigit() or char in "tfn-") and previous in " \t\r\n:,\\[{":
            nodes += 1
        if nodes > max_nodes:
            raise PublishError("JSON document exceeds its node limit")
        previous = char
    if in_string or depth != 0:
        raise PublishError("JSON document is truncated")


def _json_bytes(body: bytes, label: str, limits: Limits) -> dict:
    try:
        text = body.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise PublishError(f"{label} is not UTF-8") from error
    _check_json_complexity(text, max_depth=limits.max_json_depth,
                           max_nodes=limits.max_json_nodes)
    try:
        value = json.loads(text, object_pairs_hook=_duplicate_rejecting_object,
                           parse_constant=lambda _: (_ for _ in ()).throw(PublishError("invalid JSON number")))
    except (json.JSONDecodeError, RecursionError, ValueError) as error:
        raise PublishError(f"invalid {label} JSON") from error
    if not isinstance(value, dict):
        raise PublishError(f"{label} must be a JSON object")
    return value


def _read_regular(path: Path, max_bytes: int, chunk_bytes: int = _MAX_CHUNK_BYTES) -> bytes:
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_size > max_bytes:
            raise PublishError("record is not a bounded regular file")
        body = bytearray()
        while chunk := stream.read(min(chunk_bytes, max_bytes + 1 - len(body))):
            body.extend(chunk)
            if len(body) > max_bytes:
                raise PublishError("record exceeds its byte limit")
        after = os.fstat(stream.fileno())
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
            after.st_size, after.st_mtime_ns, after.st_ctime_ns
        ) or len(body) != after.st_size:
            raise PublishError("record changed during read")
        return bytes(body)


def _hash_regular(path: Path, max_bytes: int, chunk_bytes: int) -> tuple[int, str]:
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_size > max_bytes:
            raise PublishError("artifact is not a bounded regular file")
        digest = hashlib.sha256()
        length = 0
        while chunk := stream.read(chunk_bytes):
            length += len(chunk)
            if length > max_bytes:
                raise PublishError("artifact exceeds its byte limit")
            digest.update(chunk)
        after = os.fstat(stream.fileno())
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
            after.st_size, after.st_mtime_ns, after.st_ctime_ns
        ) or length != after.st_size:
            raise PublishError("artifact changed during read")
        return length, digest.hexdigest()


def _allowed_keys(value: object, allowed: set[str], label: str) -> dict:
    if not isinstance(value, dict) or not set(value).issubset(allowed):
        raise PublishError(f"{label} has unknown or invalid fields")
    return value


def _parse_manifest(body: bytes, limits: Limits) -> tuple[tuple[_Artifact, ...], int]:
    if len(body) > limits.max_manifest_bytes:
        raise PublishError("manifest exceeds its byte limit")
    manifest = _json_bytes(body, "manifest", limits)
    if set(manifest) != {"version", "source", "created_at", "control_db", "brains", "journal_watermark"}:
        raise PublishError("manifest-v2 fields are incomplete or unknown")
    if type(manifest["version"]) is not int or manifest["version"] != 2:
        raise PublishError("only manifest version 2 is supported")
    source = _allowed_keys(manifest["source"], {"build_sha", "schema_version"}, "source")
    if set(source) != {"build_sha", "schema_version"} or not isinstance(source["build_sha"], str) or not source["build_sha"].strip() or any(c.isspace() for c in source["build_sha"]):
        raise PublishError("manifest source identity is invalid")
    if type(source["schema_version"]) is not int or source["schema_version"] <= 0:
        raise PublishError("manifest schema version is invalid")
    created_at = manifest["created_at"]
    if not isinstance(created_at, str):
        raise PublishError("manifest creation time is invalid")
    try:
        created = datetime.fromisoformat(created_at.replace("Z", "+00:00"))
    except ValueError as error:
        raise PublishError("manifest creation time is invalid") from error
    if created.tzinfo is None or created.utcoffset() != timezone.utc.utcoffset(created):
        raise PublishError("manifest creation time must be UTC")
    watermark = _allowed_keys(manifest["journal_watermark"], {"generation", "sequence_id", "entry_hash"}, "journal watermark")
    if set(watermark) != {"generation", "sequence_id", "entry_hash"} or type(watermark["generation"]) is not int or type(watermark["sequence_id"]) is not int or not isinstance(watermark["entry_hash"], str):
        raise PublishError("manifest journal watermark shape is invalid")
    if (watermark["generation"] < 1 or watermark["sequence_id"] < 1
            or not _SHA256.fullmatch(watermark["entry_hash"])):
        raise PublishError("manifest must carry a populated journal watermark")

    artifacts: list[_Artifact] = []
    seen: set[str] = {"manifest.json", "complete"}

    def claim(reference: object, label: str) -> None:
        item = _allowed_keys(reference, {"relative_path", "length_bytes", "sha256"}, label)
        if set(item) != {"relative_path", "length_bytes", "sha256"}:
            raise PublishError(f"{label} artifact fields are incomplete")
        name, length, digest = item["relative_path"], item["length_bytes"], item["sha256"]
        if not isinstance(name, str) or not _ARTIFACT_NAME.fullmatch(name):
            raise PublishError(f"{label} artifact path is unsafe")
        folded = name.casefold()
        if folded in seen:
            raise PublishError("manifest contains duplicate or reserved artifact path")
        if type(length) is not int or length <= 0 or not isinstance(digest, str) or not _SHA256.fullmatch(digest):
            raise PublishError(f"{label} artifact length or checksum is invalid")
        artifacts.append(_Artifact(name, length, digest))
        seen.add(folded)

    control = manifest["control_db"]
    claim(control, "control database")
    if control["relative_path"] != "control.db":
        raise PublishError("manifest control database path is unexpected")
    brains = manifest["brains"]
    if not isinstance(brains, list) or len(brains) > limits.max_brains:
        raise PublishError("manifest brain inventory exceeds its limit")
    previous = ""
    for brain in brains:
        brain = _allowed_keys(brain, {"id", "empty", "relative_path", "length_bytes", "sha256", "heads"}, "brain")
        brain_id = brain.get("id")
        if not isinstance(brain_id, str) or not _BRAIN_ID.fullmatch(brain_id) or brain_id <= previous:
            raise PublishError("brain inventory must be sorted, unique, and valid")
        previous = brain_id
        if type(brain.get("empty")) is not bool:
            raise PublishError("brain empty flag is invalid")
        if brain["empty"]:
            if any(brain.get(name, default) != default for name, default in (
                ("relative_path", ""), ("length_bytes", 0), ("sha256", ""), ("heads", [])
            )):
                raise PublishError("empty brain must not name an artifact or bundle heads")
        else:
            claim({key: brain[key] for key in ("relative_path", "length_bytes", "sha256") if key in brain}, f"brain {brain_id}")
            heads = brain.get("heads")
            if not isinstance(heads, list) or not heads:
                raise PublishError("non-empty brain must declare bundle heads")
            previous_ref = ""
            for head in heads:
                head = _allowed_keys(head, {"ref", "object_id"}, "bundle head")
                if set(head) != {"ref", "object_id"} or not isinstance(head["ref"], str) or not isinstance(head["object_id"], str):
                    raise PublishError("bundle head shape is invalid")
                if (head["ref"] != "HEAD" and not head["ref"].startswith("refs/")) or any(
                    char.isspace() or ord(char) < 32 for char in head["ref"]
                ) or head["ref"] <= previous_ref or not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", head["object_id"]):
                    raise PublishError("bundle head identity is invalid")
                previous_ref = head["ref"]

    if len(artifacts) > limits.max_artifacts:
        raise PublishError("manifest artifact count exceeds its limit")
    total = sum(item.length_bytes for item in artifacts)
    if total > limits.max_snapshot_bytes:
        raise PublishError("snapshot artifact bytes exceed their limit")
    return tuple(artifacts), total


def _check_private_root(path: Path) -> os.stat_result:
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) & 0o077 or info.st_uid != os.geteuid():
        raise PublishError("staging root must be a caller-owned private directory")
    return info


def _check_snapshot(snapshot: Path, staging_root: Path, limits: Limits) -> tuple[bytes, tuple[_Artifact, ...], int]:
    root_info = _check_private_root(staging_root)
    snap_info = snapshot.lstat()
    if not stat.S_ISDIR(snap_info.st_mode) or stat.S_IMODE(snap_info.st_mode) & 0o077 or snap_info.st_uid != root_info.st_uid:
        raise PublishError("snapshot must be a caller-owned private directory")
    if snapshot.parent != staging_root or snapshot.is_symlink():
        raise PublishError("snapshot must be a direct child of its private staging root")
    manifest = _read_regular(snapshot / "manifest.json", limits.max_manifest_bytes, limits.chunk_bytes)
    artifacts, total = _parse_manifest(manifest, limits)
    names = {item.name.casefold() for item in artifacts} | {"manifest.json"}
    if (snapshot / "COMPLETE").exists() or (snapshot / "COMPLETE").is_symlink():
        names.add("complete")
    found: set[str] = set()
    with os.scandir(snapshot) as entries:
        for entry in entries:
            if entry.name.casefold() in found:
                raise PublishError("snapshot contains case-folded duplicate names")
            found.add(entry.name.casefold())
            info = entry.stat(follow_symlinks=False)
            if not stat.S_ISREG(info.st_mode):
                raise PublishError("snapshot contains a non-regular entry")
    if found != names:
        raise PublishError("snapshot contains missing or unexpected entries")
    for artifact in artifacts:
        length, digest = _hash_regular(snapshot / artifact.name, limits.max_snapshot_bytes, limits.chunk_bytes)
        if (length, digest) != (artifact.length_bytes, artifact.sha256):
            raise PublishError("local artifact differs from the manifest")
    return manifest, artifacts, total


def _read_exact_keys(storage: Storage, prefix: str, expected: set[str], limits: Limits) -> None:
    actual = storage.list_prefix(prefix, max_objects=limits.max_objects)
    if not isinstance(actual, Sequence) or isinstance(actual, (str, bytes)) or len(actual) > limits.max_objects:
        raise PublishError("remote prefix inventory is malformed or oversized")
    keys: list[str] = []
    for item in actual:
        if not isinstance(item, RemoteObject) or not isinstance(item.key, str) or not item.key.startswith(prefix):
            raise PublishError("remote inventory escaped the snapshot prefix")
        if not isinstance(item.version_id, str) or not item.version_id or type(item.delete_marker) is not bool:
            raise PublishError("remote version inventory is malformed")
        if item.delete_marker:
            raise PublishError("snapshot prefix already contains a delete marker")
        keys.append(item.key)
    if len(set(keys)) != len(keys) or set(keys) != expected:
        raise PublishError("remote prefix contains missing or unexpected objects")


class _BoundedWriter:
    def __init__(self, stream: BinaryIO, limit: int, chunk_bytes: int) -> None:
        self._stream = stream
        self._limit = limit
        self._chunk = chunk_bytes
        self.written = 0

    def write(self, data: bytes) -> int:
        if not isinstance(data, (bytes, bytearray, memoryview)) or len(data) > self._chunk:
            raise PublishError("storage transfer chunk is malformed or oversized")
        if self.written + len(data) > self._limit:
            raise PublishError("download exceeds its byte limit")
        count = self._stream.write(data)
        if count != len(data):
            raise PublishError("short local staging write")
        self.written += count
        return count

    def flush(self) -> None:
        self._stream.flush()


def _download_file(storage: Storage, key: str, destination: Path, limit: int, limits: Limits) -> int:
    descriptor = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            bounded = _BoundedWriter(stream, limit, limits.chunk_bytes)
            reported = storage.read_to(key, bounded, max_bytes=limit, chunk_bytes=limits.chunk_bytes)
            bounded.flush()
            os.fsync(stream.fileno())
            if type(reported) is not int or reported != bounded.written:
                raise PublishError("storage byte count does not match downloaded bytes")
            return reported
    except BaseException:
        try:
            destination.unlink()
        except FileNotFoundError:
            pass
        raise


def _write_new_file(path: Path, body: bytes) -> None:
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(body)
        stream.flush()
        os.fsync(stream.fileno())


def _read_remote_small(storage: Storage, key: str, limit: int) -> bytes:
    body = storage.read_bytes(key, max_bytes=limit)
    if not isinstance(body, bytes) or len(body) > limit:
        raise PublishError("remote record exceeds its byte limit")
    return body


def _fsync_dir(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def publish_snapshot(snapshot: str | os.PathLike[str], snapshot_prefix: str, *,
                     staging_root: str | os.PathLike[str], storage: Storage,
                     limits: Limits) -> PublishReceipt:
    """Publish an immutable local v2 snapshot and return only after remote proof."""
    prefix = _prefix(snapshot_prefix)
    root = Path(os.path.abspath(staging_root))
    source = Path(os.path.abspath(snapshot))
    if source.parent != root:
        raise PublishError("snapshot must be a direct child of caller staging root")
    try:
        manifest, artifacts, total = _check_snapshot(source, root, limits)
        manifest_sha = hashlib.sha256(manifest).hexdigest()
        _read_exact_keys(storage, prefix, set(), limits)
        local_complete = backup_completion.create_completion(source, prefix)
        backup_completion.verify_completed(source, prefix)
        complete_bytes = _read_regular(source / "COMPLETE", limits.max_completion_bytes, limits.chunk_bytes)
        if local_complete != {"version": 1, "snapshot_prefix": prefix, "manifest_sha256": manifest_sha}:
            raise PublishError("local completion record is unexpected")
        if complete_bytes != (json.dumps(local_complete, sort_keys=True) + "\n").encode():
            raise PublishError("local completion record is not canonical")
        for item in artifacts:
            _put_file(storage, prefix + item.name, source / item.name, item, limits)
        _put_bytes(storage, prefix + "manifest.json", manifest, limits)

        # Verify all remote bytes before committing the completion object.
        check_root = Path(tempfile.mkdtemp(prefix=".serenity-publish-verify-", dir=root))
        try:
            _verify_remote_payload(storage, prefix, manifest, artifacts, check_root, limits)
        finally:
            _remove_owned_private(check_root)
        # Do this before the remote commit point. After COMPLETE is accepted,
        # local staging is no longer part of the remote snapshot's validity.
        final_manifest, final_artifacts, final_total = _check_snapshot(source, root, limits)
        if final_manifest != manifest or final_artifacts != artifacts or final_total != total:
            raise PublishError("local snapshot changed during publication")
        try:
            _put_bytes(storage, prefix + "COMPLETE", complete_bytes, limits)
        except ObjectExists:
            # A definite occupied-key response is not an ambiguous write.
            raise
        except Exception as put_error:  # noqa: BLE001 - transfer errors are adapter-defined
            # The final put can succeed remotely while its response is lost.
            # Accept only an exact, fully verified readback of that commit.
            try:
                actual = _read_remote_small(storage, prefix + "COMPLETE", limits.max_completion_bytes)
                if actual != complete_bytes:
                    raise PublishError("ambiguous completion differs from intended record")
                _verify_remote_completed(storage, prefix, complete_bytes, root, limits)
            except Exception:  # noqa: BLE001 - readback errors are adapter-defined
                raise PublishError("completion write outcome could not be proven") from put_error
        _verify_remote_completed(storage, prefix, complete_bytes, root, limits)
        return PublishReceipt(prefix, manifest_sha, len(artifacts) + 2, total)
    except PublishError:
        raise
    except Exception as error:
        raise PublishError("snapshot publication failed") from error


def _verify_remote_payload(storage: Storage, prefix: str, manifest: bytes,
                           artifacts: tuple[_Artifact, ...], target: Path,
                           limits: Limits) -> None:
    expected = {prefix + item.name for item in artifacts} | {prefix + "manifest.json"}
    _read_exact_keys(storage, prefix, expected, limits)
    remote_manifest = _read_remote_small(storage, prefix + "manifest.json", limits.max_manifest_bytes)
    if remote_manifest != manifest:
        raise PublishError("remote manifest bytes differ from local manifest")
    _write_new_file(target / "manifest.json", remote_manifest)
    for item in artifacts:
        _download_file(storage, prefix + item.name, target / item.name,
                       item.length_bytes, limits)
    parsed, total = _parse_manifest(remote_manifest, limits)
    if parsed != artifacts or total > limits.max_snapshot_bytes:
        raise PublishError("remote manifest inventory changed")
    for item in artifacts:
        length, digest = _hash_regular(target / item.name, limits.max_snapshot_bytes, limits.chunk_bytes)
        if (length, digest) != (item.length_bytes, item.sha256):
            raise PublishError("remote artifact checksum or length mismatch")


def _remove_owned_private(path: Path) -> None:
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) & 0o077 or info.st_uid != os.geteuid():
        raise PublishError("refusing to clean staging not owned as a private directory")
    shutil.rmtree(path)


def _verify_remote_completed(storage: Storage, prefix: str, complete_bytes: bytes,
                             staging_root: Path, limits: Limits) -> None:
    # A fresh stage is used for reconciliation and post-commit proof; it is
    # always a direct child of the caller's private staging root.
    target = Path(tempfile.mkdtemp(prefix=".serenity-publish-final-", dir=staging_root))
    try:
        manifest_body = _read_remote_small(storage, prefix + "manifest.json", limits.max_manifest_bytes)
        artifacts, total = _parse_manifest(manifest_body, limits)
        expected = {prefix + item.name for item in artifacts} | {
            prefix + "manifest.json", prefix + "COMPLETE"
        }
        _read_exact_keys(storage, prefix, expected, limits)
        remote_complete = _read_remote_small(storage, prefix + "COMPLETE", limits.max_completion_bytes)
        if remote_complete != complete_bytes:
            raise PublishError("remote completion bytes differ from local completion")
        _write_new_file(target / "manifest.json", manifest_body)
        for item in artifacts:
            _download_file(storage, prefix + item.name, target / item.name,
                           item.length_bytes, limits)
        _write_new_file(target / "COMPLETE", remote_complete)
        if sum(item.length_bytes for item in artifacts) != total:
            raise PublishError("artifact byte count changed")
        backup_completion.verify_completed(target, prefix)
    finally:
        _remove_owned_private(target)


def download_snapshot(snapshot_prefix: str, *, staging_parent: str | os.PathLike[str],
                      storage: Storage, limits: Limits) -> Path:
    """Return a new private byte-verified snapshot directory; never restores it."""
    prefix = _prefix(snapshot_prefix)
    parent = Path(os.path.abspath(staging_parent))
    _check_private_root(parent)
    stage = Path(tempfile.mkdtemp(prefix=".serenity-download-", dir=parent))
    try:
        complete_bytes = _read_remote_small(storage, prefix + "COMPLETE", limits.max_completion_bytes)
        complete = _json_bytes(complete_bytes, "completion", limits)
        if set(complete) != {"version", "snapshot_prefix", "manifest_sha256"} or type(complete.get("version")) is not int or complete["version"] != 1 or complete.get("snapshot_prefix") != prefix or not isinstance(complete.get("manifest_sha256"), str) or not _SHA256.fullmatch(complete["manifest_sha256"]):
            raise PublishError("remote completion record is invalid")
        manifest = _read_remote_small(storage, prefix + "manifest.json", limits.max_manifest_bytes)
        if hashlib.sha256(manifest).hexdigest() != complete["manifest_sha256"]:
            raise PublishError("completion does not bind the remote manifest")
        artifacts, total = _parse_manifest(manifest, limits)
        expected = {prefix + item.name for item in artifacts} | {
            prefix + "manifest.json", prefix + "COMPLETE"
        }
        _read_exact_keys(storage, prefix, expected, limits)
        _write_new_file(stage / "manifest.json", manifest)
        for item in artifacts:
            _download_file(storage, prefix + item.name, stage / item.name,
                           item.length_bytes, limits)
        _write_new_file(stage / "COMPLETE", complete_bytes)
        if sum(item.length_bytes for item in artifacts) != total:
            raise PublishError("downloaded artifact inventory changed")
        backup_completion.verify_completed(stage, prefix)
        _fsync_dir(stage)
        return stage
    except Exception as error:
        _remove_owned_private(stage)
        if isinstance(error, PublishError):
            raise
        raise PublishError("remote snapshot did not pass complete byte verification") from error
    except BaseException:
        _remove_owned_private(stage)
        raise
