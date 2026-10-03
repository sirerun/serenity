"""Bounded, explicitly configured AWS CLI transport for hosted backups.

This adapter is intentionally not wired into a runner. It implements only the
publisher Storage protocol and owns the subprocesses it starts.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import selectors
import shutil
import stat
import subprocess
import sys
import tempfile
import time
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from types import MappingProxyType
from typing import BinaryIO
from urllib.parse import unquote

try:
    from . import backup_publish as publish
except ImportError:  # direct-script import compatibility
    import backup_publish as publish


MAX_OBJECTS = 1_003
MAX_OBJECT_BYTES = 500_000_000_000
MAX_TOTAL_BYTES = 500_000_000_000
MAX_PAGES = 2
PAGE_SIZE = 1_000
MAX_PAGE_JSON = 16 * 1024 * 1024
MAX_STDOUT = 64 * 1024
MAX_STDERR = 256 * 1024
MAX_TOTAL_STDOUT = 2 * 1024 * 1024 * 1024
MAX_TOTAL_STDERR = 64 * 1024 * 1024
MAX_WRITE_RESERVE_BYTES = 1_000_268_435_456
MAX_REMOTE_PROOF_BYTES = 1_500_050_343_936
MAX_INVOCATIONS = 24_576
OPERATION_SECONDS = 24 * 60 * 60
CHILD_SECONDS = 30 * 60
CLEANUP_SECONDS = 60
RANGE_BYTES = 128 * 1024 * 1024
STREAM_BYTES = 1024 * 1024
PART_BYTES = 256 * 1024 * 1024
SMALL_PUT_MAX = 5_000_000_000
_ACCOUNT = re.compile(r"^[0-9]{12}$")
_BUCKET = re.compile(r"^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$")
_REGION = re.compile(r"^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$")
_SHA256 = re.compile(r"^[0-9a-f]{64}$")
_CLI_VERSION = re.compile(r"^aws-cli/[0-9]+(?:\.[0-9]+){1,3}$")


class TransportError(publish.PublishError):
    """A bounded CLI operation failed without qualifying a receipt."""


class _OutputOverflow(Exception):
    def __init__(self, index: int, observed: int):
        self.index = index
        self.observed = observed


class DefiniteStorageFailure(publish.DefiniteStorageFailure):
    """A validated response proves a storage operation failed."""

    def __init__(self, message: str, *, error_code: str | None = None):
        super().__init__(message)
        self.error_code = error_code


class AmbiguousStorageFailure(publish.AmbiguousStorageFailure):
    """A write may have reached S3, but its response was lost or malformed."""

    def __init__(self, message: str, *, error_code: str | None = None):
        super().__init__(message)
        self.error_code = error_code


@dataclass(frozen=True)
class Config:
    cli_path: str
    cli_version: str
    cli_sha256: str
    bucket: str
    region: str
    expected_owner: str
    scratch_root: str
    credentials: Mapping[str, str]

    def __post_init__(self) -> None:
        object.__setattr__(
            self, "credentials", MappingProxyType(dict(self.credentials))
        )

    def validate(self) -> None:
        if not os.path.isabs(self.cli_path) or not self.cli_path:
            raise ValueError("AWS CLI path must be absolute")
        if not _CLI_VERSION.fullmatch(self.cli_version):
            raise ValueError("AWS CLI version pin is invalid")
        if not _SHA256.fullmatch(self.cli_sha256):
            raise ValueError("AWS CLI checksum pin is invalid")
        if not _BUCKET.fullmatch(self.bucket) or ".." in self.bucket:
            raise ValueError("bucket name is invalid")
        if not _REGION.fullmatch(self.region):
            raise ValueError("region is invalid")
        if not _ACCOUNT.fullmatch(self.expected_owner):
            raise ValueError("expected bucket owner is invalid")
        if not os.path.isabs(self.scratch_root):
            raise ValueError("scratch root must be absolute")
        allowed = {"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}
        if set(self.credentials) - allowed or not {
            "AWS_ACCESS_KEY_ID",
            "AWS_SECRET_ACCESS_KEY",
            "AWS_SESSION_TOKEN",
        } <= set(self.credentials):
            raise ValueError("only explicit short-lived AWS credentials are accepted")
        if any(
            not isinstance(value, str)
            or not value
            or any(char in value for char in "\x00\r\n")
            for value in self.credentials.values()
        ):
            raise ValueError("credential values must be non-empty strings")


@dataclass
class _Child:
    process: subprocess.Popen[bytes]
    started: float


class _Executor:
    """One-operation child owner with absolute deadline and bounded output."""

    def __init__(
        self, config: Config, *, process_factory=subprocess.Popen, clock=time.monotonic
    ):
        config.validate()
        self.config = config
        self._factory = process_factory
        self._clock = clock
        self.deadline = clock() + OPERATION_SECONDS
        self.children: dict[int, _Child] = {}
        self.calls = 0
        self.total_stdout_bytes = 0
        self.total_stderr_bytes = 0
        self._tmp = None
        self._config_path = None
        self._credentials_path = None
        self._cli_verified = False
        self._cli_verifying = False

    def _remaining(self) -> float:
        remaining = self.deadline - self._clock()
        if remaining <= 0:
            raise TimeoutError("backup operation deadline expired")
        return min(CHILD_SECONDS, remaining)

    def _ensure_files(self) -> tuple[str, str]:
        root = Path(self.config.scratch_root)
        info = root.lstat()
        if (
            not stat.S_ISDIR(info.st_mode)
            or stat.S_ISLNK(info.st_mode)
            or info.st_uid != os.geteuid()
            or info.st_mode & 0o077
        ):
            raise TransportError("scratch root must be caller-owned private directory")
        if self._tmp is None:
            self._tmp = tempfile.TemporaryDirectory(prefix="s3cli-", dir=root)
            os.chmod(self._tmp.name, 0o700)
            config_file = Path(self._tmp.name) / "config"
            cred_file = Path(self._tmp.name) / "credentials"
            self._exclusive_text(
                config_file, f"[default]\nregion = {self.config.region}\n"
            )
            credential_names = {
                "AWS_ACCESS_KEY_ID": "aws_access_key_id",
                "AWS_SECRET_ACCESS_KEY": "aws_secret_access_key",
                "AWS_SESSION_TOKEN": "aws_session_token",
            }
            lines = ["[default]"] + [
                f"{credential_names[key]} = {value}"
                for key, value in sorted(self.config.credentials.items())
            ]
            self._exclusive_text(cred_file, "\n".join(lines) + "\n")
            self._config_path, self._credentials_path = str(config_file), str(cred_file)
        return self._config_path, self._credentials_path

    @staticmethod
    def _exclusive_text(path: Path, content: str) -> None:
        fd = os.open(
            path,
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0),
            0o600,
        )
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())

    def _environment(self) -> dict[str, str]:
        config_file, credentials_file = self._ensure_files()
        return {
            "PATH": "/usr/bin:/bin",
            "AWS_CONFIG_FILE": config_file,
            "AWS_SHARED_CREDENTIALS_FILE": credentials_file,
            "AWS_PROFILE": "default",
            "AWS_DEFAULT_REGION": self.config.region,
            "AWS_EC2_METADATA_DISABLED": "true",
            "AWS_PAGER": "",
            "AWS_CLI_AUTO_PROMPT": "off",
            "AWS_MAX_ATTEMPTS": "1",
        }

    def run(
        self,
        args: list[str],
        *,
        output_limit: int = MAX_STDOUT,
        file_size_limit: int | None = None,
        write_may_transmit: bool = False,
    ) -> bytes:
        if self.calls >= MAX_INVOCATIONS:
            raise TransportError("AWS CLI invocation ceiling exceeded")
        self.calls += 1
        self._verify_cli()
        env = self._environment()
        timeout = self._remaining()
        target = [self.config.cli_path, "--no-cli-pager", "--no-cli-auto-prompt", *args]
        argv = (
            target
            if file_size_limit is None
            else [
                sys.executable,
                str(Path(__file__).with_name("backup_s3_child.py")),
                "--file-size-limit",
                str(file_size_limit),
                "--",
                *target,
            ]
        )
        process = self._factory(
            argv,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
            close_fds=True,
            bufsize=0,
        )
        self.children[process.pid] = _Child(process, self._clock())
        try:
            stdout, stderr, overflow = self._collect(
                process, timeout, output_limit, self._clock
            )
        except _OutputOverflow as error:
            self._terminate_owned(process)
            self.total_stdout_bytes += error.observed if error.index == 0 else 0
            self.total_stderr_bytes += error.observed if error.index == 1 else 0
            if error.index == 0 and write_may_transmit:
                raise AmbiguousStorageFailure(
                    "write child returned an oversized response"
                ) from None
            raise TransportError("AWS CLI child output exceeded its ceiling") from None
        except TimeoutError:
            self._terminate_owned(process)
            if write_may_transmit:
                raise AmbiguousStorageFailure(
                    "write child timed out after launch"
                ) from None
            raise TimeoutError("AWS CLI child exceeded operation deadline") from None
        except BaseException:
            self._terminate_owned(process)
            raise
        self.children.pop(process.pid, None)
        self.total_stdout_bytes += len(stdout)
        self.total_stderr_bytes += len(stderr)
        if (
            self.total_stdout_bytes > MAX_TOTAL_STDOUT
            or self.total_stderr_bytes > MAX_TOTAL_STDERR
        ):
            raise TransportError("aggregate AWS CLI output ceiling exceeded")
        if process.returncode != 0:
            detail = stderr[:1024].decode("utf-8", "replace")
            match = re.search(
                r"An error occurred \(([^)]+)\) when calling the ([A-Za-z0-9]+) operation",
                detail,
            )
            code = match.group(1) if match else None
            if (
                write_may_transmit
                and code is None
                and any(
                    token in detail
                    for token in (
                        "ReadTimeoutError",
                        "ConnectionClosedError",
                        "ConnectionResetError",
                        "IncompleteReadError",
                    )
                )
            ):
                raise AmbiguousStorageFailure(
                    "AWS CLI write connection ended without a response"
                )
            # A received service error, including 5xx and request-timeout error
            # codes, is definite under the frozen v1.2 adapter contract.
            raise DefiniteStorageFailure(
                f"AWS CLI command failed: {detail}", error_code=code
            )
        if overflow[1]:
            if write_may_transmit:
                raise AmbiguousStorageFailure(
                    "write child stderr exceeded its output ceiling"
                )
            raise TransportError("AWS CLI stderr exceeded its output ceiling")
        if overflow[0]:
            if write_may_transmit:
                raise AmbiguousStorageFailure(
                    "write child stdout exceeded its output ceiling"
                )
            raise TransportError("AWS CLI stdout exceeded its output ceiling")
        return stdout

    def _verify_cli(self) -> None:
        if self._cli_verified:
            return
        if self._cli_verifying:
            return
        path = Path(self.config.cli_path)
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode):
            raise TransportError("pinned AWS CLI must be a regular non-symlink file")
        digest = hashlib.sha256()
        fd = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
        try:
            opened = os.fstat(fd)
            if not stat.S_ISREG(opened.st_mode) or (opened.st_dev, opened.st_ino) != (
                info.st_dev,
                info.st_ino,
            ):
                raise TransportError("AWS CLI identity changed during open")
            while chunk := os.read(fd, STREAM_BYTES):
                self._remaining()
                digest.update(chunk)
            after = os.fstat(fd)
            if (opened.st_dev, opened.st_ino, opened.st_size, opened.st_mtime_ns) != (
                after.st_dev,
                after.st_ino,
                after.st_size,
                after.st_mtime_ns,
            ):
                raise TransportError("AWS CLI changed while hashing")
        finally:
            os.close(fd)
        if digest.hexdigest() != self.config.cli_sha256:
            raise TransportError("AWS CLI checksum does not match pinned value")
        self._cli_verifying = True
        try:
            raw_version = self.run(["--version"], output_limit=MAX_STDOUT)
            version_line = raw_version.decode("utf-8", "strict").strip()
            if not version_line.split(" ", 1)[0] == self.config.cli_version:
                raise TransportError("AWS CLI version does not match its explicit pin")
            self._cli_verified = True
        except UnicodeError as error:
            raise TransportError("AWS CLI version response is not UTF-8") from error
        finally:
            self._cli_verifying = False

    @staticmethod
    def _collect(
        process: subprocess.Popen[bytes], timeout: float, stdout_limit: int, clock
    ):
        selector = selectors.DefaultSelector()
        streams = (process.stdout, process.stderr)
        limits = (stdout_limit, MAX_STDERR)
        buffers = [bytearray(), bytearray()]
        observed = [0, 0]
        for index, stream in enumerate(streams):
            selector.register(stream, selectors.EVENT_READ, index)
        deadline = clock() + timeout
        try:
            while selector.get_map():
                remaining = deadline - clock()
                if remaining <= 0:
                    raise TimeoutError
                for key, _ in selector.select(min(remaining, 0.25)):
                    index = key.data
                    chunk = os.read(key.fd, STREAM_BYTES)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        key.fileobj.close()
                        continue
                    observed[index] += len(chunk)
                    available = limits[index] - len(buffers[index])
                    if available > 0:
                        buffers[index].extend(chunk[:available])
                    if len(chunk) > available:
                        raise _OutputOverflow(index, observed[index])
            try:
                process.wait(timeout=max(0.001, deadline - clock()))
            except subprocess.TimeoutExpired as error:
                raise TimeoutError from error
        finally:
            selector.close()
            for stream in streams:
                if stream is not None and not stream.closed:
                    stream.close()
        return bytes(buffers[0]), bytes(buffers[1]), [False, False]

    def _terminate_owned(self, process: subprocess.Popen[bytes]) -> None:
        child = self.children.get(process.pid)
        if child is None:
            raise TransportError("refusing to signal unowned child")
        deadline = self._clock() + CLEANUP_SECONDS
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=max(0.001, min(30.0, deadline - self._clock())))
        except subprocess.TimeoutExpired:
            if self.children.get(process.pid) is child and process.poll() is None:
                process.kill()
            remaining = deadline - self._clock()
            if remaining <= 0:
                raise TransportError("owned child exceeded cleanup deadline")
            process.wait(timeout=remaining)
        self.children.pop(process.pid, None)

    def close(self) -> None:
        failure = None
        for pid, child in list(self.children.items()):
            try:
                self._terminate_owned(child.process)
            except BaseException as error:  # noqa: BLE001 - cleanup failure is operation failure
                failure = error
        if self._tmp is not None:
            try:
                self._tmp.cleanup()
            except OSError as error:
                failure = error
            self._tmp = None
        if failure:
            raise TransportError(
                "owned child or credential-file cleanup failed"
            ) from failure


class S3Storage:
    """AWS CLI S3 storage protocol; construction is side-effect-free."""

    def __init__(
        self, config: Config, *, process_factory=subprocess.Popen, clock=time.monotonic
    ):
        config.validate()
        self.config = config
        self._factory = process_factory
        self._clock = clock
        self._op: _Executor | None = None
        self._closed = False
        self._entered = False
        self._failed = False
        self._reconciliation_only = False
        self._preflight_done = False
        self._versioning_checked = False
        self._remote_proof_bytes = 0
        self._upload_bytes = 0

    def _new_operation(self) -> _Executor:
        if self._closed:
            raise TransportError("storage operation is already closed")
        if not self._entered:
            raise TransportError(
                "storage transport must be used in a caller-owned context"
            )
        if self._op is None:
            self._op = _Executor(
                self.config, process_factory=self._factory, clock=self._clock
            )
        if not self._preflight_done:
            self._op._remaining()
            self._ensure_upload_space()
            self._op._remaining()
            self._preflight_done = True
        return self._op

    def _assert_usable(self, *, write: bool) -> None:
        if self._closed or self._failed or (write and self._reconciliation_only):
            raise TransportError("storage transaction is invalidated")

    def __enter__(self):
        if self._closed:
            raise TransportError("storage operation is already closed")
        self._entered = True
        self._op = _Executor(
            self.config, process_factory=self._factory, clock=self._clock
        )
        return self

    def __exit__(self, exc_type, exc, traceback):
        self.close()
        return False

    def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        if self._op is not None:
            self._op.close()

    def _bucket_url(self, key: str) -> str:
        if (
            not isinstance(key, str)
            or not key
            or key.startswith("/")
            or ".." in key.split("/")
        ):
            raise ValueError("unsafe S3 key")
        return f"s3://{self.config.bucket}/{key}"

    def _base(self) -> list[str]:
        return ["--region", self.config.region]

    def _request(
        self,
        op: _Executor,
        service: str,
        *args: str,
        output_limit=MAX_STDOUT,
        file_size_limit: int | None = None,
        write_may_transmit: bool = False,
    ) -> bytes:
        try:
            return op.run(
                [
                    "s3api",
                    service,
                    *args,
                    "--expected-bucket-owner",
                    self.config.expected_owner,
                ],
                output_limit=output_limit,
                file_size_limit=file_size_limit,
                write_may_transmit=write_may_transmit,
            )
        except AmbiguousStorageFailure:
            arg_key = args[args.index("--key") + 1] if "--key" in args else ""
            if service in {
                "put-object",
                "complete-multipart-upload",
            } and arg_key.endswith("/COMPLETE"):
                self._reconciliation_only = True
            else:
                self._failed = True
            raise
        except BaseException:
            self._failed = True
            raise

    def _check_versioning(self, op: _Executor) -> None:
        raw = self._request(op, "get-bucket-versioning", "--bucket", self.config.bucket)
        value = _json_object(raw, MAX_STDOUT)
        if value.get("Status") != "Enabled":
            raise DefiniteStorageFailure("bucket versioning must be Enabled")

    def list_prefix(
        self, prefix: str, *, max_objects: int
    ) -> list[publish.RemoteObject]:
        self._assert_usable(write=False)
        if max_objects != MAX_OBJECTS or not prefix or ".." in prefix.split("/"):
            raise ValueError("inventory request exceeds fixed contract")
        publish._prefix(prefix)
        op = self._new_operation()
        result = []
        key_marker = version_marker = None
        seen = set()
        for page in range(MAX_PAGES):
            args = [
                "--bucket",
                self.config.bucket,
                "--prefix",
                prefix,
                "--encoding-type",
                "url",
                "--no-paginate",
                "--max-keys",
                str(PAGE_SIZE),
            ]
            if key_marker is not None:
                args.extend(
                    ["--key-marker", key_marker, "--version-id-marker", version_marker]
                )
            raw = self._request(
                op, "list-object-versions", *args, output_limit=MAX_PAGE_JSON
            )
            data = _json_object(raw, MAX_PAGE_JSON)
            truncated = data.get("IsTruncated")
            if type(truncated) is not bool:
                raise TransportError("versions response has invalid IsTruncated")
            for field, marker in (("Versions", False), ("DeleteMarkers", True)):
                entries = data.get(field, [])
                if not isinstance(entries, list):
                    raise TransportError("versions response has invalid entries")
                for entry in entries:
                    item = _inventory_item(entry, marker, prefix)
                    identity = (item.key, item.version_id, item.delete_marker)
                    if identity in seen:
                        raise TransportError("duplicate version identity")
                    seen.add(identity)
                    result.append(item)
            if len(result) > max_objects:
                raise TransportError("version inventory exceeds object ceiling")
            if not truncated:
                return result
            next_key = _decode_key(_required_string(data, "NextKeyMarker"))
            next_version = _required_string(data, "NextVersionIdMarker")
            if key_marker is not None and (next_key, next_version) == (
                key_marker,
                version_marker,
            ):
                raise TransportError("version pagination did not advance")
            if page + 1 == MAX_PAGES:
                raise TransportError("version inventory exceeds page ceiling")
            key_marker, version_marker = next_key, next_version
        raise TransportError("version inventory incomplete")

    def read_bytes(self, key: str, *, version_id: str, max_bytes: int) -> bytes:
        if not 0 < max_bytes <= MAX_PAGE_JSON:
            raise ValueError("read_bytes is restricted to bounded control records")
        output = bytearray()
        self.read_to(
            key,
            _ByteSink(output),
            version_id=version_id,
            max_bytes=max_bytes,
            chunk_bytes=STREAM_BYTES,
        )
        return bytes(output)

    def read_to(
        self,
        key: str,
        destination: BinaryIO,
        *,
        version_id: str,
        max_bytes: int,
        chunk_bytes: int,
    ) -> int:
        self._assert_usable(write=False)
        self._bucket_url(key)
        if not isinstance(version_id, str) or not version_id or version_id == "null":
            raise ValueError("explicit non-null version ID is required")
        if not 0 < max_bytes <= MAX_OBJECT_BYTES or not 0 < chunk_bytes <= STREAM_BYTES:
            raise ValueError("read limits exceed fixed contract")
        op = self._new_operation()
        head = _json_object(
            self._request(
                op,
                "head-object",
                "--bucket",
                self.config.bucket,
                "--key",
                key,
                "--version-id",
                version_id,
            ),
            MAX_STDOUT,
        )
        size = head.get("ContentLength")
        if (
            type(size) is not int
            or size <= 0
            or size > max_bytes
            or head.get("VersionId") != version_id
        ):
            raise TransportError("version-bound HEAD failed length or identity checks")
        total = 0
        for start in range(0, size, RANGE_BYTES):
            op._remaining()
            end = min(size, start + RANGE_BYTES) - 1
            chunk_path = self._private_path(op)
            try:
                raw_meta = self._request(
                    op,
                    "get-object",
                    "--bucket",
                    self.config.bucket,
                    "--key",
                    key,
                    "--version-id",
                    version_id,
                    "--range",
                    f"bytes={start}-{end}",
                    chunk_path,
                    output_limit=MAX_STDOUT,
                    file_size_limit=RANGE_BYTES,
                )
                meta = _json_object(raw_meta, MAX_STDOUT)
                expected_length = end - start + 1
                expected_range = f"bytes {start}-{end}/{size}"
                if (
                    type(meta.get("ContentLength")) is not int
                    or meta["ContentLength"] != expected_length
                    or meta.get("ContentRange") != expected_range
                    or meta.get("VersionId") != version_id
                ):
                    raise TransportError(
                        "range response metadata failed identity checks"
                    )
                info = os.stat(chunk_path, follow_symlinks=False)
                if not stat.S_ISREG(info.st_mode) or info.st_size != expected_length:
                    raise TransportError(
                        "range body size did not match bounded response"
                    )
                with open(chunk_path, "rb") as source:
                    while chunk := source.read(min(chunk_bytes, STREAM_BYTES)):
                        op._remaining()
                        if (
                            self._remote_proof_bytes + len(chunk)
                            > MAX_REMOTE_PROOF_BYTES
                        ):
                            raise TransportError("remote proof-byte ceiling exceeded")
                        if destination.write(chunk) != len(chunk):
                            raise TransportError(
                                "download destination accepted a partial range chunk"
                            )
                        total += len(chunk)
                        self._remote_proof_bytes += len(chunk)
            finally:
                self._unlink_if_present_confirmed(chunk_path)
        if total != size:
            raise TransportError("assembled range body length mismatch")
        return total

    def put_if_absent(
        self,
        key: str,
        source: BinaryIO,
        *,
        length_bytes: int,
        sha256: str,
        chunk_bytes: int,
    ) -> None:
        self._assert_usable(write=True)
        if not 0 < length_bytes <= MAX_OBJECT_BYTES or not _SHA256.fullmatch(sha256):
            raise ValueError("upload length or checksum exceeds fixed contract")
        if not 0 < chunk_bytes <= STREAM_BYTES:
            raise ValueError("upload chunk size exceeds fixed contract")
        self._bucket_url(key)
        if self._upload_bytes + length_bytes > MAX_TOTAL_BYTES + 16_777_216 + 4_096:
            raise TransportError("snapshot upload-byte ceiling exceeded")
        op = self._new_operation()
        full_path = None
        try:
            self._ensure_upload_space()
            if not self._versioning_checked:
                self._check_versioning(op)
                self._versioning_checked = True
            full_path, full_identity = self._stage_upload(
                op, source, length_bytes, sha256, chunk_bytes
            )
            if length_bytes <= SMALL_PUT_MAX:
                self._single_put(op, key, full_path, length_bytes, sha256)
            else:
                self._multipart_put(op, key, full_path, full_identity, length_bytes)
            self._assert_file_identity(full_path, full_identity)
            self._upload_bytes += length_bytes
        except AmbiguousStorageFailure:
            if key.endswith("/COMPLETE"):
                self._reconciliation_only = True
            else:
                self._failed = True
            raise
        except BaseException:
            self._failed = True
            raise
        finally:
            if full_path is not None:
                try:
                    self._unlink_confirmed(full_path)
                except BaseException:
                    self._failed = True
                    raise

    def _ensure_upload_space(self) -> None:
        root = Path(self.config.scratch_root)
        info = root.lstat()
        if (
            not stat.S_ISDIR(info.st_mode)
            or stat.S_ISLNK(info.st_mode)
            or info.st_uid != os.geteuid()
            or info.st_mode & 0o077
        ):
            raise TransportError("scratch root must be caller-owned private directory")
        if shutil.disk_usage(root).free < MAX_WRITE_RESERVE_BYTES:
            raise TransportError(
                "private scratch volume lacks the required upload reserve"
            )

    @staticmethod
    def _stage_upload(
        op: _Executor,
        source: BinaryIO,
        length: int,
        expected_sha256: str,
        chunk_bytes: int,
    ) -> tuple[str, tuple[int, int, int, int, int]]:
        if op._tmp is None:
            op._ensure_files()
        fd, path = tempfile.mkstemp(prefix="upload-", dir=op._tmp.name)
        os.fchmod(fd, 0o600)
        digest = hashlib.sha256()
        count = 0
        try:
            with os.fdopen(fd, "wb") as target:
                while count < length:
                    op._remaining()
                    chunk = source.read(min(chunk_bytes, length - count))
                    if not isinstance(chunk, bytes) or not chunk:
                        raise TransportError(
                            "upload source ended before its declared length"
                        )
                    if len(chunk) > min(chunk_bytes, length - count):
                        raise TransportError(
                            "upload source exceeded its requested read bound"
                        )
                    target.write(chunk)
                    digest.update(chunk)
                    count += len(chunk)
                if source.read(1) != b"":
                    raise TransportError("upload source exceeds its declared length")
                target.flush()
                os.fsync(target.fileno())
                info = os.fstat(target.fileno())
                identity = (
                    info.st_dev,
                    info.st_ino,
                    info.st_size,
                    info.st_mtime_ns,
                    info.st_ctime_ns,
                )
            if count != length or digest.hexdigest() != expected_sha256:
                raise TransportError(
                    "upload source length or SHA-256 does not match its manifest"
                )
            S3Storage._assert_file_identity(path, identity)
            return path, identity
        except BaseException:
            S3Storage._unlink_confirmed(path)
            raise

    @staticmethod
    def _assert_file_identity(
        path: str, identity: tuple[int, int, int, int, int]
    ) -> None:
        info = os.lstat(path)
        actual = (
            info.st_dev,
            info.st_ino,
            info.st_size,
            info.st_mtime_ns,
            info.st_ctime_ns,
        )
        if (
            not stat.S_ISREG(info.st_mode)
            or info.st_uid != os.geteuid()
            or info.st_mode & 0o077
            or actual != identity
        ):
            raise TransportError("private upload spool changed identity or permissions")

    def _single_put(
        self, op: _Executor, key: str, path: str, length: int, digest_hex: str
    ) -> None:
        digest_b64 = base64.b64encode(bytes.fromhex(digest_hex)).decode("ascii")
        try:
            raw = self._request(
                op,
                "put-object",
                "--bucket",
                self.config.bucket,
                "--key",
                key,
                "--body",
                path,
                "--content-length",
                str(length),
                "--if-none-match",
                "*",
                "--checksum-sha256",
                digest_b64,
                write_may_transmit=True,
            )
        except DefiniteStorageFailure as error:
            if error.error_code == "PreconditionFailed":
                raise publish.ObjectExists(key) from error
            raise
        self._validate_write_response(raw)

    def _multipart_put(
        self,
        op: _Executor,
        key: str,
        full_path: str,
        full_identity: tuple[int, int, int, int, int],
        length: int,
    ) -> None:
        part_count = _multipart_part_count(length)
        created = _json_object(
            self._request(
                op,
                "create-multipart-upload",
                "--bucket",
                self.config.bucket,
                "--key",
                key,
                "--checksum-algorithm",
                "SHA256",
                output_limit=MAX_STDOUT,
            ),
            MAX_STDOUT,
        )
        upload_id = _required_string(created, "UploadId")
        parts = []
        try:
            with open(full_path, "rb") as full_source:
                for part_number in range(1, part_count + 1):
                    self._assert_file_identity(full_path, full_identity)
                    offset = (part_number - 1) * PART_BYTES
                    part_length = min(PART_BYTES, length - offset)
                    part_path, part_checksum = self._stage_part(
                        op, full_source, offset, part_length
                    )
                    try:
                        self._assert_file_identity(full_path, full_identity)
                        raw = self._request(
                            op,
                            "upload-part",
                            "--bucket",
                            self.config.bucket,
                            "--key",
                            key,
                            "--upload-id",
                            upload_id,
                            "--part-number",
                            str(part_number),
                            "--body",
                            part_path,
                            "--content-length",
                            str(part_length),
                            "--checksum-sha256",
                            part_checksum,
                            output_limit=MAX_STDOUT,
                        )
                        response = _json_object(raw, MAX_STDOUT)
                        etag = _required_string(response, "ETag")
                        returned_checksum = response.get("ChecksumSHA256")
                        if returned_checksum != part_checksum:
                            raise TransportError(
                                "uploaded part checksum response did not match"
                            )
                    finally:
                        # This file must be absent after every attempted part
                        # request, before any later request or completion.
                        self._unlink_confirmed(part_path)
                    parts.append(
                        {
                            "ChecksumSHA256": part_checksum,
                            "ETag": etag,
                            "PartNumber": part_number,
                        }
                    )
                    self._assert_file_identity(full_path, full_identity)
            if len(parts) != part_count:
                raise TransportError(
                    "multipart upload did not produce its exact part count"
                )
            payload = json.dumps(
                {"Parts": parts}, separators=(",", ":"), ensure_ascii=True
            )
            try:
                raw = self._request(
                    op,
                    "complete-multipart-upload",
                    "--bucket",
                    self.config.bucket,
                    "--key",
                    key,
                    "--upload-id",
                    upload_id,
                    "--multipart-upload",
                    payload,
                    "--if-none-match",
                    "*",
                    write_may_transmit=True,
                )
            except DefiniteStorageFailure as error:
                if error.error_code == "PreconditionFailed":
                    raise publish.ObjectExists(key) from error
                raise
            self._validate_write_response(raw)
        except BaseException:
            try:
                self._request(
                    op,
                    "abort-multipart-upload",
                    "--bucket",
                    self.config.bucket,
                    "--key",
                    key,
                    "--upload-id",
                    upload_id,
                    output_limit=MAX_STDOUT,
                )
            except BaseException as abort_error:
                raise TransportError(
                    "owned multipart upload cleanup failed"
                ) from abort_error
            raise

    @staticmethod
    def _stage_part(
        op: _Executor, full_source: BinaryIO, offset: int, length: int
    ) -> tuple[str, str]:
        if not 0 < length <= PART_BYTES:
            raise TransportError("multipart part size is outside its fixed bound")
        full_source.seek(offset)
        path = str(Path(op._tmp.name) / f"part-{os.urandom(12).hex()}")
        fd = os.open(
            path,
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0),
            0o600,
        )
        digest = hashlib.sha256()
        remaining = length
        try:
            with os.fdopen(fd, "wb") as target:
                while remaining:
                    op._remaining()
                    chunk = full_source.read(min(STREAM_BYTES, remaining))
                    if not chunk:
                        raise TransportError("full upload spool ended during part copy")
                    if len(chunk) > min(STREAM_BYTES, remaining):
                        raise TransportError("part source returned an oversized chunk")
                    target.write(chunk)
                    digest.update(chunk)
                    remaining -= len(chunk)
                target.flush()
                os.fsync(target.fileno())
            info = os.lstat(path)
            if (
                not stat.S_ISREG(info.st_mode)
                or info.st_uid != os.geteuid()
                or info.st_mode & 0o077
                or info.st_size != length
            ):
                raise TransportError(
                    "private multipart part failed its bounded-file check"
                )
            return path, base64.b64encode(digest.digest()).decode("ascii")
        except BaseException:
            S3Storage._unlink_confirmed(path)
            raise

    @staticmethod
    def _validate_write_response(raw: bytes) -> None:
        try:
            response = _json_object(raw, MAX_STDOUT)
            _required_string(response, "VersionId")
            _required_string(response, "ETag")
        except (TransportError, KeyError) as error:
            raise AmbiguousStorageFailure(
                "successful write lacked a valid versioned response"
            ) from error

    @staticmethod
    def _private_path(op: _Executor) -> str:
        if shutil.disk_usage(op.config.scratch_root).free < MAX_WRITE_RESERVE_BYTES:
            raise TransportError("private scratch volume lost its required reserve")
        if op._tmp is None:
            op._ensure_files()
        path = str(Path(op._tmp.name) / f"range-{os.urandom(12).hex()}")
        try:
            os.lstat(path)
        except FileNotFoundError:
            return path
        raise TransportError("random range output path already exists")

    @staticmethod
    def _unlink_owned(path: str) -> None:
        info = os.lstat(path)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid():
            raise TransportError("refusing to remove unowned range file")
        os.unlink(path)

    @staticmethod
    def _unlink_confirmed(path: str) -> None:
        S3Storage._unlink_owned(path)
        S3Storage._confirm_absent(path)

    @staticmethod
    def _unlink_if_present_confirmed(path: str) -> None:
        try:
            os.lstat(path)
        except FileNotFoundError:
            return
        S3Storage._unlink_confirmed(path)

    @staticmethod
    def _confirm_absent(path: str) -> None:
        try:
            os.lstat(path)
        except FileNotFoundError:
            return
        raise TransportError("owned private spool remains after unlink")


def _multipart_part_count(length: int) -> int:
    if not 0 < length <= MAX_OBJECT_BYTES:
        raise ValueError("multipart length exceeds fixed contract")
    count = (length + PART_BYTES - 1) // PART_BYTES
    if count > 10_000:
        raise ValueError("multipart part count exceeds S3's ceiling")
    return count


class _ByteSink:
    def __init__(self, target: bytearray):
        self.target = target

    def write(self, chunk: bytes) -> int:
        self.target.extend(chunk)
        return len(chunk)


def _json_object(raw: bytes, limit: int) -> dict:
    if len(raw) > limit:
        raise TransportError("JSON response exceeded its bound")
    _check_json_shape(raw)
    try:
        value = json.loads(
            raw.decode("utf-8", "strict"),
            object_pairs_hook=_unique_pairs,
            parse_constant=lambda value: (_ for _ in ()).throw(ValueError(value)),
        )
    except (UnicodeError, ValueError, json.JSONDecodeError, RecursionError) as error:
        raise TransportError("AWS CLI returned invalid bounded JSON") from error
    if not isinstance(value, dict):
        raise TransportError("AWS CLI JSON response must be an object")
    return value


def _unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON field")
        result[key] = value
    return result


def _check_json_shape(raw: bytes) -> None:
    depth = 0
    nodes = 0
    quoted = escaped = False
    for byte in raw:
        if quoted:
            if escaped:
                escaped = False
            elif byte == 0x5C:
                escaped = True
            elif byte == 0x22:
                quoted = False
            continue
        if byte == 0x22:
            quoted = True
        elif byte in (0x7B, 0x5B):
            depth += 1
            nodes += 1
            if depth > 64 or nodes > 1_000_000:
                raise TransportError("AWS CLI JSON structure exceeded its bound")
        elif byte in (0x7D, 0x5D):
            depth -= 1
            if depth < 0:
                raise TransportError("AWS CLI returned malformed JSON nesting")
        elif byte in (0x2C, 0x3A):
            nodes += 1
            if nodes > 1_000_000:
                raise TransportError("AWS CLI JSON structure exceeded its bound")


def _required_string(value: dict, key: str) -> str:
    item = value.get(key)
    try:
        encoded_length = len(item.encode("utf-8", "strict"))
    except (AttributeError, UnicodeError):
        encoded_length = 0
    if not isinstance(item, str) or not item or encoded_length > 1024:
        raise TransportError(f"missing or invalid {key}")
    return item


def _decode_key(value: str) -> str:
    if re.search(r"%(?![0-9a-fA-F]{2})", value):
        raise TransportError("inventory key has malformed URL encoding")
    try:
        return unquote(value, encoding="utf-8", errors="strict")
    except UnicodeError as error:
        raise TransportError("inventory key is not UTF-8") from error


def _inventory_item(entry: object, marker: bool, prefix: str) -> publish.RemoteObject:
    if not isinstance(entry, dict):
        raise TransportError("inventory entry must be an object")
    key = _decode_key(_required_string(entry, "Key"))
    version = _required_string(entry, "VersionId")
    latest = entry.get("IsLatest")
    if type(latest) is not bool or version == "null" or not key.startswith(prefix):
        raise TransportError("inventory entry identity is invalid")
    return publish.RemoteObject(key, version, marker, latest)
