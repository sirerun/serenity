"""Manifest-bound completion records for staged manifest-v2 snapshots.

This unused helper verifies bytes, not authenticity, SQLite/Git semantics,
journal authority, or upload success. Callers must separately establish those
properties and keep staging immutable through verification and transfer.
"""

import hashlib
import json
import os
import re
import stat
import tempfile
from datetime import datetime
from pathlib import Path


class VerificationError(ValueError):
    """Snapshot is incomplete or its manifest binding is invalid."""


def _object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise VerificationError("duplicate JSON field")
        result[key] = value
    return result


def _read_file(path, limit=None):
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode):
            raise VerificationError("artifact is not a regular file")
        if limit is not None and before.st_size > limit:
            raise VerificationError("record exceeds size limit")
        digest = hashlib.sha256()
        body = bytearray() if limit is not None else None
        length = 0
        while chunk := stream.read(1024 * 1024):
            length += len(chunk)
            if limit is not None and length > limit:
                raise VerificationError("record exceeds size limit")
            digest.update(chunk)
            if body is not None:
                body.extend(chunk)
        after = os.fstat(stream.fileno())
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
            after.st_size, after.st_mtime_ns, after.st_ctime_ns
        ) or length != after.st_size:
            raise VerificationError("artifact changed during verification")
        return digest.hexdigest(), length, bytes(body) if body is not None else None


def _json(body):
    try:
        value = json.loads(body, object_pairs_hook=_object)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise VerificationError("invalid JSON record") from error
    if not isinstance(value, dict):
        raise VerificationError("record is not an object")
    return value


def _prefix(prefix):
    if not isinstance(prefix, str) or not re.fullmatch(r"snapshots/[0-9]{8}T[0-9]{6}Z/", prefix):
        raise VerificationError("invalid snapshot prefix")
    try:
        datetime.strptime(prefix.split("/")[1], "%Y%m%dT%H%M%SZ")
    except ValueError as error:
        raise VerificationError("invalid snapshot timestamp") from error


def _artifact(snapshot, artifact, names):
    if not isinstance(artifact, dict):
        raise VerificationError("artifact is not an object")
    name = artifact.get("relative_path")
    length = artifact.get("length_bytes")
    expected = artifact.get("sha256")
    if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", name):
        raise VerificationError("unsafe artifact path")
    if name.casefold() in names or name.casefold() in {"manifest.json", "complete"}:
        raise VerificationError("duplicate or reserved artifact path")
    if type(length) is not int or length <= 0:
        raise VerificationError("invalid artifact length")
    if not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
        raise VerificationError("invalid artifact checksum")
    digest, actual_length, _ = _read_file(snapshot / name)
    if (digest, actual_length) != (expected, length):
        raise VerificationError("artifact checksum or length mismatch")
    names.add(name.casefold())


def _verify_artifacts(snapshot, allow_completion):
    snapshot = Path(snapshot)
    root_info = snapshot.lstat()
    if not stat.S_ISDIR(root_info.st_mode):
        raise VerificationError("snapshot root must be a real directory")
    if stat.S_IMODE(root_info.st_mode) & 0o077:
        raise VerificationError("snapshot staging must be private")
    digest, _, body = _read_file(snapshot / "manifest.json", 16 * 1024 * 1024)
    manifest = _json(body)
    if type(manifest.get("version")) is not int or manifest["version"] != 2:
        raise VerificationError("completion requires manifest version2")
    brains = manifest.get("brains")
    if not isinstance(brains, list):
        raise VerificationError("invalid brain inventory")
    names = set()
    _artifact(snapshot, manifest.get("control_db"), names)
    if manifest["control_db"]["relative_path"] != "control.db":
        raise VerificationError("unexpected control database path")
    previous = ""
    for brain in brains:
        if not isinstance(brain, dict) or not isinstance(brain.get("id"), str):
            raise VerificationError("invalid brain entry")
        if not re.fullmatch(r"[A-Za-z0-9]{16,64}", brain["id"]) or brain["id"] <= previous:
            raise VerificationError("brain inventory must be sorted and unique")
        previous = brain["id"]
        if type(brain.get("empty")) is not bool:
            raise VerificationError("invalid empty-brain flag")
        if brain["empty"]:
            if brain.get("relative_path", "") or brain.get("sha256", "") or type(brain.get("length_bytes", 0)) is not int or brain.get("length_bytes", 0) != 0 or brain.get("heads"):
                raise VerificationError("empty brain names an artifact")
        else:
            _artifact(snapshot, brain, names)
    expected = names | {"manifest.json"}
    if allow_completion:
        expected.add("complete")
    actual = [path.name.casefold() for path in snapshot.iterdir()]
    if len(actual) != len(set(actual)) or set(actual) != expected:
        raise VerificationError("snapshot contains missing or unexpected artifacts")
    return snapshot, digest


def create_completion(snapshot, prefix):
    """Seal already-validated private staging; never overwrite a completion."""
    _prefix(prefix)
    snapshot = Path(snapshot)
    if (snapshot / "COMPLETE").exists() or (snapshot / "COMPLETE").is_symlink():
        record = verify_completed(snapshot, prefix)
        _sync_directory(snapshot)
        return record
    snapshot, digest = _verify_artifacts(snapshot, False)
    record = {"version": 1, "snapshot_prefix": prefix, "manifest_sha256": digest}
    # Keep an interrupted temporary record outside the manifest inventory.
    # The caller must own this outer staging directory for crash cleanup.
    # Its sibling location also keeps the eventual hard link on one volume.
    with tempfile.NamedTemporaryFile(dir=snapshot.parent, prefix=f".{snapshot.name}-completion-", delete=False) as stream:
        temporary = Path(stream.name)
        try:
            stream.write((json.dumps(record, sort_keys=True) + "\n").encode())
            stream.flush()
            os.fsync(stream.fileno())
            os.link(temporary, snapshot / "COMPLETE")
        finally:
            temporary.unlink()
    _sync_directory(snapshot)
    return record


def _sync_directory(snapshot):
    descriptor = os.open(snapshot, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def verify_completed(snapshot, prefix):
    """Verify completion, exact manifest hash and every artifact byte."""
    _prefix(prefix)
    snapshot, digest = _verify_artifacts(snapshot, True)
    _, _, body = _read_file(snapshot / "COMPLETE", 4096)
    record = _json(body)
    expected = {"version": 1, "snapshot_prefix": prefix, "manifest_sha256": digest}
    if record != expected or type(record.get("version")) is not int:
        raise VerificationError("completion does not bind this manifest and prefix")
    return record
