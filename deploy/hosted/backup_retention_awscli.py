"""Bounded AWS CLI v2 adapter for backup_retention.Storage.

This module is deliberately inert at import and construction.  A caller must
supply the executable, scope, temporary credentials, and all resource bounds.
It does not create credentials or execute a retention plan.
"""

from __future__ import annotations

import json
import os
import re
import selectors
import signal
import stat
import subprocess
import time
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any


class AdapterError(RuntimeError):
    """The CLI failed or returned an invalid result; details are intentionally hidden."""


_CREDENTIAL_KEYS = {
    "AWS_ACCESS_KEY_ID",
    "AWS_SECRET_ACCESS_KEY",
    "AWS_SESSION_TOKEN",
}
_MAX_INPUT_BYTES = 4 * 1024 * 1024
_MAX_JSON_DEPTH = 32
_MAX_JSON_NODES = 300_000
_MAX_JSON_OBJECT_FIELDS = 64
_MAX_FIELD_BYTES = 1024
_SNAPSHOT_PREFIX = "snapshots/"
_SNAPSHOT_STAMP = re.compile(r"[0-9]{8}T[0-9]{6}Z\Z")
_OWNER = re.compile(r"[0-9]{12}\Z")
_BUCKET = re.compile(r"[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])\Z")


def _text(value: Any, label: str, limit: int = _MAX_FIELD_BYTES) -> str:
    if not isinstance(value, str) or not value:
        raise AdapterError(f"invalid {label}")
    try:
        encoded = value.encode("utf-8", errors="strict")
    except UnicodeError:
        raise AdapterError(f"invalid {label}") from None
    if len(encoded) > limit or any(ord(char) < 32 or ord(char) == 127 for char in value):
        raise AdapterError(f"invalid {label}")
    return value


def _snapshot_key(value: Any) -> str:
    key = _text(value, "snapshot key")
    pieces = key.split("/", 2)
    if (len(pieces) != 3 or pieces[0] != "snapshots"
            or not _SNAPSHOT_STAMP.fullmatch(pieces[1])):
        raise AdapterError("key is outside snapshot scope")
    try:
        time.strptime(pieces[1], "%Y%m%dT%H%M%SZ")
    except ValueError:
        raise AdapterError("invalid snapshot timestamp") from None
    return key


def _key_marker(value: Any) -> str:
    return _snapshot_key(value)


def _identifier(value: Any, label: str) -> str:
    return _text(value, label)


def _pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    if len(pairs) > _MAX_JSON_OBJECT_FIELDS:
        raise ValueError("object field limit")
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def _bad_constant(_: str) -> None:
    raise ValueError("nonfinite number")


def _json_document(body: bytes, label: str) -> dict[str, Any]:
    try:
        decoded = body.decode("utf-8", errors="strict")
        document = json.loads(decoded, object_pairs_hook=_pairs, parse_constant=_bad_constant)
    except (UnicodeError, ValueError, RecursionError, json.JSONDecodeError):
        raise AdapterError(f"invalid {label} JSON") from None
    if not isinstance(document, dict):
        raise AdapterError(f"invalid {label} JSON root")
    stack: list[tuple[Any, int]] = [(document, 1)]
    nodes = 0
    while stack:
        item, depth = stack.pop()
        nodes += 1
        if depth > _MAX_JSON_DEPTH or nodes > _MAX_JSON_NODES:
            raise AdapterError(f"{label} JSON limits exceeded")
        if isinstance(item, dict):
            if len(item) > _MAX_JSON_OBJECT_FIELDS:
                raise AdapterError(f"{label} JSON field limit exceeded")
            stack.extend((child, depth + 1) for child in item.values())
        elif isinstance(item, list):
            stack.extend((child, depth + 1) for child in item)
        elif isinstance(item, float):
            raise AdapterError(f"invalid numeric value in {label} JSON")
    return document


def _trusted_executable(value: Any) -> str:
    if not isinstance(value, str) or not value or not os.path.isabs(value):
        raise AdapterError("CLI executable must be an absolute path")
    path = Path(value)
    try:
        if str(path) != value or os.path.realpath(value) != value:
            raise AdapterError("CLI executable path is not canonical")
        info = path.lstat()
    except OSError:
        raise AdapterError("CLI executable is unavailable") from None
    if not stat.S_ISREG(info.st_mode) or not (info.st_mode & (stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)):
        raise AdapterError("CLI executable is not a regular executable")
    if info.st_uid not in {0, os.geteuid()} or info.st_mode & (stat.S_IWGRP | stat.S_IWOTH):
        raise AdapterError("CLI executable is not trusted")
    current = path.parent
    while True:
        try:
            ancestor = current.lstat()
        except OSError:
            raise AdapterError("CLI executable ancestry is unavailable") from None
        if (not stat.S_ISDIR(ancestor.st_mode) or ancestor.st_uid not in {0, os.geteuid()}
                or ancestor.st_mode & (stat.S_IWGRP | stat.S_IWOTH)):
            raise AdapterError("CLI executable ancestry is not trusted")
        if current.parent == current:
            break
        current = current.parent
    return value


def _positive_int(value: Any, minimum: int, maximum: int, label: str) -> int:
    if type(value) is not int or not minimum <= value <= maximum:
        raise AdapterError(f"invalid {label}")
    return value


def _collection(page: dict[str, Any], field: str, *, required: bool) -> list[Any] | None:
    if field not in page:
        if required:
            raise AdapterError("incomplete S3 page")
        return None
    value = page[field]
    if not isinstance(value, list):
        raise AdapterError("malformed S3 page")
    return value


def _next_marker(page: dict[str, Any], field: str) -> str:
    return _text(page.get(field), "continuation marker")


def _validate_versions(page: dict[str, Any]) -> dict[str, Any]:
    truncated = page.get("IsTruncated")
    if type(truncated) is not bool:
        raise AdapterError("incomplete S3 page")
    versions = _collection(page, "Versions", required=False)
    markers = _collection(page, "DeleteMarkers", required=False)
    if truncated:
        if versions is None and markers is None:
            raise AdapterError("truncated S3 page lacks entries")
        if versions is not None and markers is not None and not versions and not markers:
            raise AdapterError("truncated S3 page lacks entries")
        if versions is None and (markers is None or not markers):
            raise AdapterError("truncated S3 page lacks entries")
        if markers is None and (versions is None or not versions):
            raise AdapterError("truncated S3 page lacks entries")
        _key_marker(page.get("NextKeyMarker"))
        _identifier(page.get("NextVersionIdMarker"), "version continuation marker")
    if versions is None:
        page["Versions"] = []
    if markers is None:
        page["DeleteMarkers"] = []
    for field in ("Versions", "DeleteMarkers"):
        for item in page[field]:
            if not isinstance(item, dict):
                raise AdapterError("malformed S3 page entry")
            _snapshot_key(item.get("Key"))
            _identifier(item.get("VersionId"), "version ID")
            if type(item.get("IsLatest")) is not bool:
                raise AdapterError("malformed S3 page entry")
            _text(item.get("LastModified"), "version timestamp")
    return page


def _validate_uploads(page: dict[str, Any]) -> dict[str, Any]:
    truncated = page.get("IsTruncated")
    if type(truncated) is not bool:
        raise AdapterError("incomplete S3 page")
    uploads = _collection(page, "Uploads", required=truncated)
    if truncated:
        if not uploads:
            raise AdapterError("truncated S3 page lacks entries")
        _key_marker(page.get("NextKeyMarker"))
        _identifier(page.get("NextUploadIdMarker"), "upload continuation marker")
    if uploads is None:
        page["Uploads"] = []
    for item in page["Uploads"]:
        if not isinstance(item, dict):
            raise AdapterError("malformed S3 page entry")
        _snapshot_key(item.get("Key"))
        _identifier(item.get("UploadId"), "upload ID")
        _text(item.get("Initiated"), "upload timestamp")
    return page


def _validate_parts(page: dict[str, Any]) -> dict[str, Any]:
    truncated = page.get("IsTruncated")
    if type(truncated) is not bool:
        raise AdapterError("incomplete S3 page")
    parts = _collection(page, "Parts", required=truncated)
    if truncated:
        if not parts:
            raise AdapterError("truncated S3 page lacks entries")
        marker = page.get("NextPartNumberMarker")
        _positive_int(marker, 1, 10_000, "part continuation marker")
    if parts is None:
        page["Parts"] = []
    if any(not isinstance(item, dict) for item in page["Parts"]):
        raise AdapterError("malformed S3 page entry")
    return page


class AWSCLIStorage:
    """Fixed-scope, bounded adapter; all constructor policy values are required."""

    def __init__(self, *, executable: str, region: str, bucket: str, expected_owner: str,
                 credential_env: Mapping[str, str], command_timeout_seconds: int,
                 stdout_limit_bytes: int, stderr_limit_bytes: int) -> None:
        self.executable = _trusted_executable(executable)
        if region != "us-west-2":
            raise AdapterError("unsupported region")
        if not isinstance(bucket, str) or not _BUCKET.fullmatch(bucket):
            raise AdapterError("invalid bucket")
        if not isinstance(expected_owner, str) or not _OWNER.fullmatch(expected_owner):
            raise AdapterError("invalid expected bucket owner")
        if not isinstance(credential_env, Mapping) or set(credential_env) != _CREDENTIAL_KEYS:
            raise AdapterError("explicit temporary credentials are required")
        self.credentials: dict[str, str] = {}
        for key in sorted(_CREDENTIAL_KEYS):
            value = credential_env[key]
            limit = 16 * 1024 if key == "AWS_SESSION_TOKEN" else 256
            value = _text(value, "temporary credential", limit)
            self.credentials[key] = value
        self.region = region
        self.bucket = bucket
        self.expected_owner = expected_owner
        self.timeout = _positive_int(command_timeout_seconds, 1, 120, "command timeout")
        self.stdout_limit = _positive_int(stdout_limit_bytes, 1, 8 * 1024 * 1024, "stdout limit")
        self.stderr_limit = _positive_int(stderr_limit_bytes, 1, 64 * 1024, "stderr limit")

    def _scope(self, bucket: Any, expected_owner: Any) -> None:
        if bucket != self.bucket or expected_owner != self.expected_owner:
            raise AdapterError("request scope does not match configured scope")

    def _environment(self) -> dict[str, str]:
        return {
            **self.credentials,
            "AWS_CONFIG_FILE": "/dev/null",
            "AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
            "AWS_EC2_METADATA_DISABLED": "true",
            "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "true",
            "AWS_CLI_AUTO_PROMPT": "off",
            "AWS_PAGER": "",
            "AWS_MAX_ATTEMPTS": "1",
            "LC_ALL": "C",
        }

    def _base(self, operation: str) -> list[str]:
        return [self.executable, "s3api", operation,
                "--region", self.region,
                "--no-cli-pager", "--no-paginate", "--output", "json",
                "--cli-error-format", "json", "--no-cli-auto-prompt"]

    @staticmethod
    def _signal_group(process: subprocess.Popen[bytes]) -> None:
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        except OSError:
            pass
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        except OSError:
            pass
        try:
            process.wait(timeout=1)
        except (subprocess.TimeoutExpired, OSError):
            pass

    def _run(self, operation: str, args: Sequence[str], input_bytes: bytes | None = None) -> bytes:
        if os.name != "posix" or not hasattr(os, "killpg"):
            raise AdapterError("process-group isolation is unsupported")
        argv = self._base(operation) + list(args)
        try:
            process = subprocess.Popen(
                argv, stdin=subprocess.PIPE if input_bytes is not None else subprocess.DEVNULL,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=False,
                close_fds=True, start_new_session=True, env=self._environment(),
            )
        except (OSError, ValueError):
            raise AdapterError("AWS CLI process could not be started") from None

        stdout = bytearray()
        stderr = bytearray()
        written = 0
        deadline = time.monotonic() + self.timeout
        selector = selectors.DefaultSelector()
        try:
            assert process.stdout is not None and process.stderr is not None
            for stream in (process.stdout, process.stderr, process.stdin):
                if stream is not None:
                    os.set_blocking(stream.fileno(), False)
            selector.register(process.stdout, selectors.EVENT_READ, "stdout")
            selector.register(process.stderr, selectors.EVENT_READ, "stderr")
            if process.stdin is not None:
                selector.register(process.stdin, selectors.EVENT_WRITE, "stdin")
            pending = memoryview(input_bytes or b"")
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise AdapterError("AWS CLI command timed out")
                events = selector.select(remaining)
                if not events:
                    if process.poll() is not None:
                        raise AdapterError("AWS CLI left inherited output pipes open")
                    continue
                for key, _ in events:
                    stream = key.fileobj
                    kind = key.data
                    if kind == "stdin":
                        if written >= len(pending):
                            selector.unregister(stream)
                            stream.close()
                            continue
                        try:
                            count = os.write(stream.fileno(), pending[written:written + 64 * 1024])
                        except BlockingIOError:
                            continue
                        except BrokenPipeError:
                            selector.unregister(stream)
                            stream.close()
                            if written < len(pending):
                                raise AdapterError("AWS CLI closed request input early") from None
                            continue
                        written += count
                        if written >= len(pending):
                            selector.unregister(stream)
                            stream.close()
                        continue
                    target, limit = (stdout, self.stdout_limit) if kind == "stdout" else (stderr, self.stderr_limit)
                    try:
                        chunk = os.read(stream.fileno(), min(64 * 1024, limit - len(target) + 1))
                    except BlockingIOError:
                        continue
                    if not chunk:
                        selector.unregister(stream)
                        stream.close()
                        continue
                    target.extend(chunk)
                    if len(target) > limit:
                        raise AdapterError("AWS CLI output limit exceeded")
            if process.stdin is not None and not process.stdin.closed:
                process.stdin.close()
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise AdapterError("AWS CLI command timed out")
            try:
                code = process.wait(timeout=remaining)
            except subprocess.TimeoutExpired:
                raise AdapterError("AWS CLI command timed out") from None
            if code != 0:
                if operation == "list-parts" and code == 254:
                    try:
                        error_document = _json_document(bytes(stderr), "CLI error")
                    except AdapterError:
                        raise AdapterError("AWS CLI service request failed") from None
                    if error_document.get("Code") == "NoSuchUpload":
                        raise _UploadNotFoundSignal
                raise AdapterError("AWS CLI service request failed")
            if stderr:
                raise AdapterError("AWS CLI wrote unexpected error output")
            if input_bytes is not None and written != len(input_bytes):
                raise AdapterError("AWS CLI request input was incomplete")
            return bytes(stdout)
        except _UploadNotFoundSignal:
            self._signal_group(process)
            raise
        except AdapterError:
            self._signal_group(process)
            raise
        except (OSError, ValueError, selectors.SelectorError):
            self._signal_group(process)
            raise AdapterError("AWS CLI process failed") from None
        finally:
            selector.close()
            for stream in (process.stdin, process.stdout, process.stderr):
                if stream is not None and not stream.closed:
                    try:
                        stream.close()
                    except OSError:
                        pass
            # A CLI helper is never allowed to leave background work behind,
            # even if it closed its inherited output descriptors before exit.
            self._signal_group(process)

    def _request(self, operation: str, args: Sequence[str], *, body: bytes | None = None) -> dict[str, Any]:
        result = self._run(operation, args, body)
        return _json_document(result, "CLI response")

    def list_object_versions(self, *, bucket: str, prefix: str, key_marker: str | None,
                             version_id_marker: str | None, expected_owner: str) -> Mapping[str, Any]:
        self._scope(bucket, expected_owner)
        if prefix != _SNAPSHOT_PREFIX or (key_marker is None) != (version_id_marker is None):
            raise AdapterError("invalid version inventory scope")
        args = ["--bucket", self.bucket, "--expected-bucket-owner", self.expected_owner,
                "--prefix", _SNAPSHOT_PREFIX, "--max-keys", "1000"]
        if key_marker is not None and version_id_marker is not None:
            args.extend(["--key-marker", _key_marker(key_marker), "--version-id-marker",
                         _identifier(version_id_marker, "version continuation marker")])
        page = self._request("list-object-versions", args)
        return _validate_versions(page)

    def list_multipart_uploads(self, *, bucket: str, prefix: str, key_marker: str | None,
                               upload_id_marker: str | None, expected_owner: str) -> Mapping[str, Any]:
        self._scope(bucket, expected_owner)
        if prefix != _SNAPSHOT_PREFIX or (key_marker is None) != (upload_id_marker is None):
            raise AdapterError("invalid multipart inventory scope")
        args = ["--bucket", self.bucket, "--expected-bucket-owner", self.expected_owner,
                "--prefix", _SNAPSHOT_PREFIX, "--max-uploads", "1000"]
        if key_marker is not None and upload_id_marker is not None:
            args.extend(["--key-marker", _key_marker(key_marker), "--upload-id-marker",
                         _identifier(upload_id_marker, "upload continuation marker")])
        page = self._request("list-multipart-uploads", args)
        return _validate_uploads(page)

    def list_parts(self, *, bucket: str, key: str, upload_id: str,
                   part_number_marker: int | None, expected_owner: str) -> Mapping[str, Any]:
        self._scope(bucket, expected_owner)
        key = _snapshot_key(key)
        upload_id = _identifier(upload_id, "upload ID")
        args = ["--bucket", self.bucket, "--expected-bucket-owner", self.expected_owner,
                "--key", key, "--upload-id", upload_id, "--max-parts", "1000"]
        if part_number_marker is not None:
            marker = _positive_int(part_number_marker, 1, 10_000, "part number marker")
            args.extend(["--part-number-marker", str(marker)])
        try:
            page = self._request("list-parts", args)
        except _UploadNotFoundSignal:
            from backup_retention import UploadNotFound
            raise UploadNotFound() from None
        return _validate_parts(page)

    def delete_versions(self, *, bucket: str, objects: Sequence[Mapping[str, str]],
                        expected_owner: str) -> Mapping[str, Any]:
        self._scope(bucket, expected_owner)
        if not isinstance(objects, Sequence) or isinstance(objects, (str, bytes)) or not 1 <= len(objects) <= 1000:
            raise AdapterError("invalid version delete batch")
        normalized: list[dict[str, str]] = []
        seen: set[tuple[str, str]] = set()
        for item in objects:
            if not isinstance(item, Mapping) or set(item) != {"Key", "VersionId"}:
                raise AdapterError("invalid version delete entry")
            key = _snapshot_key(item["Key"])
            version_id = _identifier(item["VersionId"], "version ID")
            pair = key, version_id
            if pair in seen:
                raise AdapterError("duplicate version delete entry")
            seen.add(pair)
            normalized.append({"Key": key, "VersionId": version_id})
        body = json.dumps({"Objects": normalized, "Quiet": False}, sort_keys=True,
                          separators=(",", ":"), ensure_ascii=False, allow_nan=False).encode("utf-8")
        if len(body) > _MAX_INPUT_BYTES:
            raise AdapterError("version delete input limit exceeded")
        args = ["--bucket", self.bucket, "--expected-bucket-owner", self.expected_owner,
                "--delete", "file:///dev/stdin"]
        response = self._request("delete-objects", args, body=body)
        deleted = response.get("Deleted")
        errors = response.get("Errors", [])
        if not isinstance(deleted, list) or not isinstance(errors, list) or errors:
            raise AdapterError("invalid version delete response")
        expected = set(seen)
        got: set[tuple[str, str]] = set()
        for item in deleted:
            if not isinstance(item, dict):
                raise AdapterError("invalid version delete confirmation")
            key = item.get("Key")
            version_id = item.get("VersionId")
            if not isinstance(key, str) or not isinstance(version_id, str):
                raise AdapterError("invalid version delete confirmation")
            pair = key, version_id
            if pair not in expected or pair in got:
                raise AdapterError("unexpected version delete confirmation")
            got.add(pair)
        if got != expected:
            raise AdapterError("incomplete version delete confirmation")
        return response

    def abort_multipart(self, *, bucket: str, key: str, upload_id: str,
                        expected_owner: str) -> None:
        self._scope(bucket, expected_owner)
        args = ["--bucket", self.bucket, "--expected-bucket-owner", self.expected_owner,
                "--key", _snapshot_key(key), "--upload-id", _identifier(upload_id, "upload ID")]
        output = self._run("abort-multipart-upload", args)
        if output.strip():
            raise AdapterError("unexpected abort response")


class _UploadNotFoundSignal(Exception):
    """Private bridge from process classification to the existing planner API."""
