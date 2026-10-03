"""Network-free tests for the sealed, bounded AWS CLI transport."""

from __future__ import annotations

import hashlib
import io
import json
import os
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from deploy.hosted import backup_s3_transport as transport

FIXTURE_ROOT = os.environ.get("SERENITY_RECOVERY_TEST_TMPDIR")
PREFIX = "snapshots/20261002T000000Z/"
KEY = PREFIX + "control.db"
BODY = b"control data"


FAKE_CLI = r"""#!@PYTHON@
import hashlib as _hashlib, json, os, pathlib, sys, time
args = sys.argv[1:]
if "--version" in args:
    if os.environ.get("FAKE_AWS_LOG"):
        with open(os.environ["FAKE_AWS_LOG"], "a", encoding="utf-8") as log:
            log.write(json.dumps({"args": args, "env": dict(os.environ), "service": "version"}) + "\n")
    print("aws-cli/2.0.0 Python/test test-platform")
    raise SystemExit(0)
if os.environ.get("FAKE_AWS_MODE") == "large-output":
    print("x" * 10000)
    raise SystemExit(0)
if os.environ.get("FAKE_AWS_MODE") == "flood-output":
    while True:
        os.write(1, b"x" * 65536)
service = args[args.index("s3api") + 1]
record = {"args": args, "env": dict(os.environ), "service": service}
if "--body" in args:
    body_path = args[args.index("--body") + 1]
    info = os.stat(body_path)
    record.update({"body_path": body_path, "body_mode": info.st_mode & 0o777,
                   "body_size": info.st_size,
                   "body_sha256": _hashlib.sha256(pathlib.Path(body_path).read_bytes()).hexdigest()})
if service == "complete-multipart-upload" and os.environ.get("FAKE_AWS_LOG"):
    previous = [json.loads(line) for line in pathlib.Path(os.environ["FAKE_AWS_LOG"]).read_text().splitlines()]
    part_paths = [call["body_path"] for call in previous if call.get("service") == "upload-part"]
    record["parts_absent_before_complete"] = all(not pathlib.Path(path).exists() for path in part_paths)
if os.environ.get("FAKE_AWS_LOG"):
    with open(os.environ["FAKE_AWS_LOG"], "a", encoding="utf-8") as log:
        log.write(json.dumps(record) + "\n")
if service == "get-bucket-versioning":
    print(json.dumps({"Status": os.environ.get("FAKE_VERSIONING", "Enabled")}))
elif service == "list-object-versions":
    if os.environ.get("FAKE_INVENTORY_MODE") == "malformed":
        print("{")
    elif os.environ.get("FAKE_PAGINATE"):
        if "--key-marker" in args:
            page = {"IsTruncated": False, "Versions": [{
                "Key": "snapshots/20261002T000000Z/control.db", "VersionId": "v2", "IsLatest": True,
                "ETag": '"etag2"', "Size": @BODY_LEN@, "LastModified": "2026-10-02T00:00:00Z"
            }], "DeleteMarkers": []}
        else:
            page = {"IsTruncated": True, "Versions": [{
                "Key": "snapshots/20261002T000000Z/control.db", "VersionId": "v1", "IsLatest": False,
                "ETag": '"etag1"', "Size": @BODY_LEN@, "LastModified": "2026-10-01T00:00:00Z"
            }], "DeleteMarkers": [{
                "Key": "snapshots/20261002T000000Z/old.db", "VersionId": "d1", "IsLatest": False,
                "LastModified": "2026-10-01T00:00:00Z"
            }], "NextKeyMarker": "snapshots/20261002T000000Z/control.db", "NextVersionIdMarker": "v1"}
        print(json.dumps(page))
    elif os.environ.get("FAKE_TOO_MANY"):
        versions = [{"Key": f"{os.environ['FAKE_PREFIX']}k{i}", "VersionId": f"v{i}", "IsLatest": True}
                    for i in range(1004)]
        print(json.dumps({"IsTruncated": False, "Versions": versions, "DeleteMarkers": []}))
    else:
        print(json.dumps({"IsTruncated": False, "Versions": [{
            "Key": "snapshots/20261002T000000Z/control.db", "VersionId": "v1", "IsLatest": True,
            "ETag": '"etag"', "Size": @BODY_LEN@, "LastModified": "2026-10-02T00:00:00Z"
        }], "DeleteMarkers": []}))
elif service == "head-object":
    version = "wrong-version" if os.environ.get("FAKE_HEAD_VERSION_MISMATCH") else "v1"
    print(json.dumps({"ContentLength": @BODY_LEN@, "VersionId": version, "ETag": '"etag"'}))
elif service == "get-object":
    output = args[args.index("--expected-bucket-owner") - 1]
    range_value = args[args.index("--range") + 1]
    first, last = map(int, range_value.removeprefix("bytes=").split("-"))
    chunk = @BODY@[first:last + 1]
    pathlib.Path(output).write_bytes(chunk)
    version = "wrong-version" if os.environ.get("FAKE_RANGE_VERSION_MISMATCH") else "v1"
    print(json.dumps({"ContentLength": len(chunk), "ContentRange": f"bytes {first}-{last}/@BODY_LEN@", "VersionId": version}))
elif service == "put-object":
    if os.environ.get("FAKE_PUT_ERROR"):
        print(f"An error occurred ({os.environ['FAKE_PUT_ERROR']}) when calling the PutObject operation: injected", file=sys.stderr)
        raise SystemExit(255)
    if os.environ.get("FAKE_PUT_SLEEP"):
        time.sleep(float(os.environ["FAKE_PUT_SLEEP"]))
    print(json.dumps({"VersionId": "v2", "ETag": '"put-etag"'}))
elif service == "create-multipart-upload":
    print(json.dumps({"UploadId": "upload-test"}))
elif service == "upload-part":
    if os.environ.get("FAKE_FAIL_PART") == args[args.index("--part-number") + 1]:
        print("An error occurred (AccessDenied) when calling the UploadPart operation: injected", file=sys.stderr)
        raise SystemExit(255)
    number = args[args.index("--part-number") + 1]
    checksum = args[args.index("--checksum-sha256") + 1]
    print(json.dumps({"ETag": f'"part-{number}"', "ChecksumSHA256": checksum}))
elif service == "complete-multipart-upload":
    if os.environ.get("FAKE_COMPLETE_ERROR"):
        print(f"An error occurred ({os.environ['FAKE_COMPLETE_ERROR']}) when calling the CompleteMultipartUpload operation: injected", file=sys.stderr)
        raise SystemExit(255)
    print(json.dumps({"VersionId": "v3", "ETag": '"complete-etag"'}))
elif service == "abort-multipart-upload":
    pass
else:
    print("unknown service", file=sys.stderr)
    raise SystemExit(2)
"""


class TransportTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(dir=FIXTURE_ROOT or None)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        os.chmod(self.root, 0o700)
        self.scratch = self.root / "scratch"
        self.scratch.mkdir(mode=0o700)
        self.cli = self.root / "fake-aws"
        script = (
            FAKE_CLI.replace("@PYTHON@", os.sys.executable)
            .replace("@BODY@", repr(BODY))
            .replace("@BODY_LEN@", str(len(BODY)))
        )
        self.cli.write_text(script, encoding="utf-8")
        self.cli.chmod(0o700)
        self.log = self.root / "calls.jsonl"
        self.config = transport.Config(
            cli_path=str(self.cli),
            cli_version="aws-cli/2.0.0",
            cli_sha256=hashlib.sha256(self.cli.read_bytes()).hexdigest(),
            bucket="serenity-backup-test",
            region="us-west-2",
            expected_owner="123456789012",
            scratch_root=str(self.scratch),
            credentials={
                "AWS_ACCESS_KEY_ID": "test-access",
                "AWS_SECRET_ACCESS_KEY": "test-secret",
                "AWS_SESSION_TOKEN": "test-session",
            },
        )
        self.factory = self._factory
        self._real_popen = transport.subprocess.Popen

    def _factory(self, argv, **kwargs):
        kwargs["env"] = dict(kwargs["env"], FAKE_AWS_LOG=str(self.log))
        return self._real_popen(argv, **kwargs)

    def _factory_with(self, **overrides):
        def factory(argv, **kwargs):
            kwargs["env"] = dict(kwargs["env"], **overrides, FAKE_AWS_LOG=str(self.log))
            return self._real_popen(argv, **kwargs)

        return factory

    def _ambiguous_put_factory(self, **overrides):
        clock_state = {"active": False, "poll": 0, "put_children": 0}

        def factory(argv, **kwargs):
            if "put-object" in argv:
                clock_state["active"] = True
                clock_state["put_children"] += 1
            kwargs["env"] = dict(
                kwargs["env"],
                **overrides,
                FAKE_PUT_SLEEP="1",
                FAKE_AWS_LOG=str(self.log),
            )
            return self._real_popen(argv, **kwargs)

        def clock():
            if not clock_state["active"]:
                return time.monotonic()
            clock_state["poll"] += 1
            return 0 if clock_state["poll"] <= 2 else 1_000_000

        return factory, clock, clock_state

    def _assert_failed_read_invalidates_put(self, storage):
        with self.assertRaises(transport.TransportError):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=hashlib.sha256(BODY).hexdigest(),
                chunk_bytes=1024,
            )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertFalse(
            any(call.get("service") == "put-object" for call in calls), calls
        )

    def test_malformed_inventory_invalidates_same_context_before_public_put(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config,
                process_factory=self._factory_with(FAKE_INVENTORY_MODE="malformed"),
            ) as storage,
        ):
            with self.assertRaises(transport.TransportError):
                storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
            self._assert_failed_read_invalidates_put(storage)

    def test_wrong_head_version_invalidates_same_context_before_public_put(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config,
                process_factory=self._factory_with(FAKE_HEAD_VERSION_MISMATCH="1"),
            ) as storage,
        ):
            with self.assertRaises(transport.TransportError):
                storage.read_to(
                    KEY,
                    io.BytesIO(),
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                )
            self._assert_failed_read_invalidates_put(storage)

    def test_wrong_range_version_invalidates_same_context_before_public_put(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config,
                process_factory=self._factory_with(FAKE_RANGE_VERSION_MISMATCH="1"),
            ) as storage,
        ):
            with self.assertRaises(transport.TransportError):
                storage.read_to(
                    KEY,
                    io.BytesIO(),
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                )
            self._assert_failed_read_invalidates_put(storage)

    def test_destination_write_failure_invalidates_same_context_before_public_put(self):
        class BrokenDestination:
            def write(self, _chunk):
                raise OSError("injected destination write failure")

        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            with self.assertRaisesRegex(OSError, "destination write"):
                storage.read_to(
                    KEY,
                    BrokenDestination(),
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                )
            self._assert_failed_read_invalidates_put(storage)

    def test_range_cleanup_failure_invalidates_same_context_before_public_put(self):
        real_unlink = transport.os.unlink
        injected = False

        def fail_range_unlink(path, *args, **kwargs):
            nonlocal injected
            if not injected and Path(path).name.startswith("range-"):
                injected = True
                raise OSError("injected range cleanup failure")
            return real_unlink(path, *args, **kwargs)

        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            with (
                mock.patch.object(
                    transport.os, "unlink", side_effect=fail_range_unlink
                ),
                self.assertRaisesRegex(OSError, "range cleanup"),
            ):
                storage.read_to(
                    KEY,
                    io.BytesIO(),
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                )
            self.assertTrue(injected)
            self._assert_failed_read_invalidates_put(storage)

    def _disk_reserve(self):
        return mock.patch.object(
            transport.shutil,
            "disk_usage",
            return_value=SimpleNamespace(
                total=2 * transport.MAX_WRITE_RESERVE_BYTES,
                used=transport.MAX_WRITE_RESERVE_BYTES - 1,
                free=transport.MAX_WRITE_RESERVE_BYTES + 1,
            ),
        )

    def test_config_is_immutable_and_rejects_ambient_provider_controls(self):
        with self.assertRaises(TypeError):
            self.config.credentials["AWS_PROFILE"] = "other"
        with self.assertRaises(ValueError):
            transport.Config(**{**self.config.__dict__, "cli_path": "aws"}).validate()
        with self.assertRaises(ValueError):
            transport.Config(
                **{
                    **self.config.__dict__,
                    "credentials": {
                        **self.config.credentials,
                        "AWS_PROFILE": "ambient",
                    },
                }
            ).validate()

    def test_inventory_and_range_get_are_explicitly_version_pinned(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            inventory = storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
            self.assertEqual(
                [(item.key, item.version_id, item.is_latest) for item in inventory],
                [(KEY, "v1", True)],
            )
            output = bytearray()
            count = storage.read_to(
                KEY,
                transport._ByteSink(output),
                version_id="v1",
                max_bytes=len(BODY),
                chunk_bytes=3,
            )
            self.assertEqual((count, bytes(output)), (len(BODY), BODY))
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        head = next(call for call in calls if "head-object" in call["args"])
        get = next(call for call in calls if "get-object" in call["args"])
        self.assertEqual(head["args"][head["args"].index("--version-id") + 1], "v1")
        self.assertEqual(get["args"][get["args"].index("--version-id") + 1], "v1")
        self.assertEqual(
            get["args"][get["args"].index("--range") + 1], f"bytes=0-{len(BODY) - 1}"
        )
        observed = calls[0]["env"]
        self.assertNotIn("AWS_ENDPOINT_URL", observed)
        self.assertEqual(observed["AWS_PROFILE"], "default")
        self.assertNotIn("HOME", observed)
        self.assertEqual(observed["AWS_MAX_ATTEMPTS"], "1")

    def test_inventory_paginates_versions_and_delete_markers_before_returning(self):
        def paginated(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"], FAKE_PAGINATE="1", FAKE_AWS_LOG=str(self.log)
            )
            return self._real_popen(argv, **kwargs)

        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=paginated) as storage,
        ):
            inventory = storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
        self.assertEqual(
            {
                (item.key, item.version_id, item.delete_marker, item.is_latest)
                for item in inventory
            },
            {
                (KEY, "v1", False, False),
                (KEY, "v2", False, True),
                (PREFIX + "old.db", "d1", True, False),
            },
        )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        pages = [call for call in calls if "list-object-versions" in call["args"]]
        self.assertEqual(len(pages), 2)
        self.assertEqual(
            pages[1]["args"][pages[1]["args"].index("--key-marker") + 1], KEY
        )

    def test_oversized_inventory_fails_without_returning_partial_versions(self):
        def too_many(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"],
                FAKE_TOO_MANY="1",
                FAKE_PREFIX=PREFIX,
                FAKE_AWS_LOG=str(self.log),
            )
            return self._real_popen(argv, **kwargs)

        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=too_many) as storage,
            self.assertRaises(transport.TransportError),
        ):
            storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
        self.assertEqual(len(self.log.read_text().splitlines()), 2)

    def test_child_output_ceiling_rejects_without_retaining_unbounded_output(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            op = storage._new_operation()
            op._verify_cli()
            env = op._environment()
            env["FAKE_AWS_LOG"] = str(self.log)
            env["FAKE_AWS_MODE"] = "large-output"
            original = op._environment
            op._environment = lambda: env
            with self.assertRaises(transport.TransportError):
                op.run(["test"], output_limit=128)
            self.assertFalse(op.children)
            op._environment = original

    def test_output_overflow_stops_and_reaps_flooding_child(self):
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            op = storage._new_operation()
            op._verify_cli()
            env = op._environment()
            env["FAKE_AWS_MODE"] = "flood-output"
            op._environment = lambda: env
            started = time.monotonic()
            with self.assertRaises(transport.AmbiguousStorageFailure):
                op.run(
                    ["s3api", "put-object"],
                    output_limit=128,
                    write_may_transmit=True,
                )
            self.assertLess(time.monotonic() - started, 5)
            self.assertFalse(op.children)

    def test_small_upload_rejects_suspended_versioning_before_any_write(self):
        def suspended(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"], FAKE_VERSIONING="Suspended", FAKE_AWS_LOG=str(self.log)
            )
            return self._real_popen(argv, **kwargs)

        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=suspended) as storage,
            self.assertRaises(transport.DefiniteStorageFailure),
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=1024,
            )
        calls = [json.loads(line)["args"] for line in self.log.read_text().splitlines()]
        self.assertEqual(len(calls), 2)
        self.assertIn("get-bucket-versioning", calls[1])
        self.assertNotIn("put-object", calls[1])

    def test_single_put_is_private_conditional_and_cleans_before_return(self):
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=1024,
            )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        request = next(call for call in calls if call.get("service") == "put-object")
        args = request["args"]
        self.assertEqual(request["body_mode"], 0o600)
        self.assertEqual(request["body_size"], len(BODY))
        self.assertEqual(request["body_sha256"], digest)
        self.assertFalse(Path(request["body_path"]).exists())
        self.assertEqual(args[args.index("--if-none-match") + 1], "*")
        self.assertEqual(args[args.index("--content-length") + 1], str(len(BODY)))

    def test_conditional_precondition_maps_only_to_object_exists(self):
        def collision(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"],
                FAKE_PUT_ERROR="PreconditionFailed",
                FAKE_AWS_LOG=str(self.log),
            )
            return self._real_popen(argv, **kwargs)

        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=collision) as storage,
            self.assertRaises(transport.publish.ObjectExists),
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=1024,
            )
        self.assertFalse(
            any(
                call.get("service") == "complete-multipart-upload"
                for call in map(json.loads, self.log.read_text().splitlines())
            )
        )

    def test_received_service_errors_including_5xx_remain_definite(self):
        def denied(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"],
                FAKE_PUT_ERROR="RequestTimeout",
                FAKE_AWS_LOG=str(self.log),
            )
            return self._real_popen(argv, **kwargs)

        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=denied) as storage,
            self.assertRaises(transport.DefiniteStorageFailure) as caught,
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=1024,
            )
        self.assertEqual(caught.exception.error_code, "RequestTimeout")

    def test_ambiguous_non_complete_write_invalidates_context(self):
        timeout, clock, clock_state = self._ambiguous_put_factory()
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config, process_factory=timeout, clock=clock
            ) as storage,
        ):
            with self.assertRaises(transport.AmbiguousStorageFailure):
                storage.put_if_absent(
                    KEY,
                    io.BytesIO(BODY),
                    length_bytes=len(BODY),
                    sha256=digest,
                    chunk_bytes=1024,
                )
            clock_state["active"] = False
            calls_after_failure = self.log.read_text().splitlines()
            with self.assertRaises(transport.TransportError):
                storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
            self.assertEqual(self.log.read_text().splitlines(), calls_after_failure)
        self.assertEqual(clock_state["put_children"], 1)

    def test_ambiguous_complete_write_allows_read_only_reconciliation(self):
        timeout, clock, clock_state = self._ambiguous_put_factory()
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config, process_factory=timeout, clock=clock
            ) as storage,
        ):
            with self.assertRaises(transport.AmbiguousStorageFailure):
                storage.put_if_absent(
                    PREFIX + "COMPLETE",
                    io.BytesIO(BODY),
                    length_bytes=len(BODY),
                    sha256=digest,
                    chunk_bytes=1024,
                )
            clock_state["active"] = False
            destination = io.BytesIO()
            self.assertEqual(
                storage.read_to(
                    PREFIX + "COMPLETE",
                    destination,
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                ),
                len(BODY),
            )
            self.assertEqual(destination.getvalue(), BODY)
            with self.assertRaises(transport.TransportError):
                storage.put_if_absent(
                    KEY,
                    io.BytesIO(BODY),
                    length_bytes=len(BODY),
                    sha256=digest,
                    chunk_bytes=1024,
                )
        self.assertEqual(clock_state["put_children"], 1)

    def test_failed_reconciliation_proof_poisons_read_only_context(self):
        timeout, clock, clock_state = self._ambiguous_put_factory(
            FAKE_INVENTORY_MODE="malformed"
        )
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config, process_factory=timeout, clock=clock
            ) as storage,
        ):
            with self.assertRaises(transport.AmbiguousStorageFailure):
                storage.put_if_absent(
                    PREFIX + "COMPLETE",
                    io.BytesIO(BODY),
                    length_bytes=len(BODY),
                    sha256=digest,
                    chunk_bytes=1024,
                )
            clock_state["active"] = False
            with self.assertRaises(transport.TransportError):
                storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
            calls_after_failure = self.log.read_text().splitlines()
            with self.assertRaises(transport.TransportError):
                storage.list_prefix(PREFIX, max_objects=transport.MAX_OBJECTS)
            self.assertEqual(self.log.read_text().splitlines(), calls_after_failure)

    def test_final_complete_cleanup_failure_blocks_read_only_reconciliation(self):
        timeout, clock, clock_state = self._ambiguous_put_factory()
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            self._disk_reserve(),
            transport.S3Storage(
                self.config, process_factory=timeout, clock=clock
            ) as storage,
        ):
            with (
                mock.patch.object(
                    storage,
                    "_unlink_confirmed",
                    side_effect=OSError("injected final spool cleanup failure"),
                ),
                self.assertRaisesRegex(OSError, "final spool cleanup"),
            ):
                storage.put_if_absent(
                    PREFIX + "COMPLETE",
                    io.BytesIO(BODY),
                    length_bytes=len(BODY),
                    sha256=digest,
                    chunk_bytes=1024,
                )
            clock_state["active"] = False
            with self.assertRaises(transport.TransportError):
                storage.read_to(
                    PREFIX + "COMPLETE",
                    io.BytesIO(),
                    version_id="v1",
                    max_bytes=1024,
                    chunk_bytes=1024,
                )
            calls = [json.loads(line) for line in self.log.read_text().splitlines()]
            self.assertFalse(
                any(call.get("service") == "head-object" for call in calls)
            )

    def test_multipart_part_spools_are_gone_before_complete_and_full_spool_before_return(
        self,
    ):
        digest = hashlib.sha256(BODY).hexdigest()
        with (
            mock.patch.object(transport, "PART_BYTES", 4),
            mock.patch.object(transport, "SMALL_PUT_MAX", 3),
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=self.factory) as storage,
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=3,
            )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        parts = [call for call in calls if call.get("service") == "upload-part"]
        complete = next(
            call for call in calls if call.get("service") == "complete-multipart-upload"
        )
        self.assertEqual(len(parts), 3)
        self.assertTrue(
            all(call["body_mode"] == 0o600 and call["body_size"] == 4 for call in parts)
        )
        self.assertTrue(all(not Path(call["body_path"]).exists() for call in parts))
        self.assertTrue(complete["parts_absent_before_complete"])
        manifest = json.loads(
            complete["args"][complete["args"].index("--multipart-upload") + 1]
        )
        self.assertEqual([part["PartNumber"] for part in manifest["Parts"]], [1, 2, 3])
        self.assertEqual(
            complete["args"][complete["args"].index("--if-none-match") + 1], "*"
        )

    def test_failed_part_is_unlinked_then_owned_upload_is_aborted_without_completion(
        self,
    ):
        def part_failure(argv, **kwargs):
            kwargs["env"] = dict(
                kwargs["env"], FAKE_FAIL_PART="2", FAKE_AWS_LOG=str(self.log)
            )
            return self._real_popen(argv, **kwargs)

        digest = hashlib.sha256(BODY).hexdigest()
        with (
            mock.patch.object(transport, "PART_BYTES", 4),
            mock.patch.object(transport, "SMALL_PUT_MAX", 3),
            self._disk_reserve(),
            transport.S3Storage(self.config, process_factory=part_failure) as storage,
            self.assertRaises(transport.DefiniteStorageFailure),
        ):
            storage.put_if_absent(
                KEY,
                io.BytesIO(BODY),
                length_bytes=len(BODY),
                sha256=digest,
                chunk_bytes=3,
            )
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        parts = [call for call in calls if call.get("service") == "upload-part"]
        self.assertEqual(len(parts), 2)
        self.assertFalse(Path(parts[-1]["body_path"]).exists())
        self.assertTrue(
            any(call.get("service") == "abort-multipart-upload" for call in calls)
        )
        self.assertFalse(
            any(call.get("service") == "complete-multipart-upload" for call in calls)
        )

    def test_multipart_budget_boundaries_are_calculated_without_large_files(self):
        self.assertEqual(transport._multipart_part_count(5_000_000_001), 19)
        self.assertEqual(transport._multipart_part_count(500_000_000_000), 1_863)

    def test_cli_json_nesting_and_subprocess_metadata_are_bounded(self):
        with self.assertRaises(transport.TransportError):
            transport._json_object(b'{"x":' + b"[" * 65 + b"0" + b"]" * 65 + b"}", 4096)
        with self.assertRaises(transport.TransportError):
            transport._required_string({"UploadId": "x" * 1025}, "UploadId")

    def test_invalidated_context_refuses_further_write(self):
        with transport.S3Storage(self.config) as storage:
            storage._reconciliation_only = True
            with self.assertRaises(transport.TransportError):
                storage.put_if_absent(
                    "snapshots/a",
                    io.BytesIO(b"x"),
                    length_bytes=1,
                    sha256=hashlib.sha256(b"x").hexdigest(),
                    chunk_bytes=1,
                )

    def test_child_cleanup_shares_one_sixty_second_deadline(self):
        clock = [0.0]

        class HungChild:
            pid = 8123
            returncode = None

            def __init__(self):
                self.waits = []

            def poll(self):
                return self.returncode

            def terminate(self):
                pass

            def kill(self):
                self.returncode = -9

            def wait(self, timeout):
                self.waits.append(timeout)
                clock[0] += timeout
                if self.returncode is None:
                    raise subprocess.TimeoutExpired("fake", timeout)
                return self.returncode

        executor = transport._Executor(self.config, clock=lambda: clock[0])
        child = HungChild()
        executor.children[child.pid] = transport._Child(child, 0.0)
        executor._terminate_owned(child)
        self.assertEqual(clock[0], 60.0)
        self.assertEqual(child.waits, [30.0, 30.0])
        self.assertNotIn(child.pid, executor.children)
        self.assertEqual(transport.PART_BYTES, 268_435_456)
        self.assertEqual(transport.MAX_WRITE_RESERVE_BYTES, 1_000_268_435_456)


if __name__ == "__main__":
    unittest.main()
