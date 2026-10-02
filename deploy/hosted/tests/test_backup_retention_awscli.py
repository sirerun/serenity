"""Fake-process controls for the bounded AWS CLI retention adapter."""

import importlib.util
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

_ADAPTER_PATH = Path(__file__).parents[1] / "backup_retention_awscli.py"
_LIBRARY_PATH = Path(__file__).parents[1] / "backup_retention.py"


def _load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


retention = _load("backup_retention", _LIBRARY_PATH)
adapter = _load("backup_retention_awscli", _ADAPTER_PATH)

OWNER = "123456789012"
BUCKET = "synthetic-retention-bucket"
KEY = "snapshots/20261001T000000Z/brain.bundle"
STAMP = "20261001T000000Z"
FIXTURE_ROOT = os.environ.get("SERENITY_RECOVERY_TEST_TMPDIR")


class AdapterTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not FIXTURE_ROOT:
            raise RuntimeError("SERENITY_RECOVERY_TEST_TMPDIR must name the owned private test mount")
        cls.fixture_root = Path(FIXTURE_ROOT).resolve(strict=True)
        root_info = cls.fixture_root.stat()
        if (not cls.fixture_root.is_dir() or root_info.st_uid != os.geteuid()
                or root_info.st_mode & (stat.S_IWGRP | stat.S_IWOTH)):
            raise RuntimeError("private test mount is not owned and private")

    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix="retention-cli-test-", dir=self.fixture_root))
        self.addCleanup(shutil.rmtree, self.root, True)

    def credential_values(self):
        return {
            "AWS_ACCESS_KEY_ID": "SYNTHETIC_ACCESS",
            "AWS_SECRET_ACCESS_KEY": "SYNTHETIC_SECRET",
            "AWS_SESSION_TOKEN": "SYNTHETIC_TEMPORARY_TOKEN",
        }

    def write_fake(self, *, stdout=b"{}", stderr=b"", code=0, behavior="normal"):
        executable = self.root / "aws-fake"
        trace = self.root / f"trace-{len(list(self.root.iterdir()))}.json"
        descendant_pid = self.root / f"descendant-{len(list(self.root.iterdir()))}.pid"
        marker_fifo = self.root / f"descendant-{len(list(self.root.iterdir()))}.fifo"
        source = f'''#!{sys.executable}
import json, os, signal, sys, time
from pathlib import Path
trace = Path({str(trace)!r})
body = sys.stdin.buffer.read()
trace.write_text(json.dumps({{"argv":sys.argv[1:], "environment":dict(os.environ), "stdin":body.decode("utf-8", "strict")}}, sort_keys=True))
if {behavior!r} == "descendant_pipes":
    fifo = {str(marker_fifo)!r}
    pidfile = Path({str(descendant_pid)!r})
    pid = os.fork()
    if pid == 0:
        pidfile.write_text(str(os.getpid()))
        with open(fifo, "rb") as gate:
            gate.read(1)
        Path(fifo + ".late").write_text("late")
        os._exit(0)
    os._exit(0)
if {behavior!r} == "pause":
    signal.pause()
if {behavior!r} == "self_terminate_after_timeout":
    signal.alarm(2)
    signal.pause()
sys.stdout.buffer.write(bytes.fromhex({stdout.hex()!r}))
sys.stderr.buffer.write(bytes.fromhex({stderr.hex()!r}))
sys.stdout.flush()
sys.stderr.flush()
os._exit({int(code)})
'''
        executable.write_text(source, encoding="utf-8")
        executable.chmod(0o700)
        return executable, trace, descendant_pid, marker_fifo

    def make_storage(self, executable, *, timeout=2, stdout_limit=1024, stderr_limit=1024, credentials=None):
        return adapter.AWSCLIStorage(
            executable=str(executable), region="us-west-2", bucket=BUCKET,
            expected_owner=OWNER, credential_env=credentials or self.credential_values(),
            command_timeout_seconds=timeout, stdout_limit_bytes=stdout_limit,
            stderr_limit_bytes=stderr_limit,
        )

    @staticmethod
    def completed_versions(*, versions=None, markers=None, truncated=False, **extra):
        body = {"IsTruncated": truncated}
        if versions is not None:
            body["Versions"] = versions
        if markers is not None:
            body["DeleteMarkers"] = markers
        body.update(extra)
        return json.dumps(body, separators=(",", ":")).encode()

    def test_constructor_and_scope_reject_invalid_values_without_spawning(self):
        executable, trace, *_ = self.write_fake(stdout=b"{}")
        credentials = self.credential_values()
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(executable, credentials={"AWS_ACCESS_KEY_ID": "x"})
        with self.assertRaises(adapter.AdapterError):
            adapter.AWSCLIStorage(executable=str(executable), region="us-east-1", bucket=BUCKET,
                                  expected_owner=OWNER, credential_env=credentials,
                                  command_timeout_seconds=2, stdout_limit_bytes=1, stderr_limit_bytes=1)
        for kwargs in ({"bucket": "other-bucket"}, {"expected_owner": "000000000000"}):
            request = {"bucket": BUCKET, "expected_owner": OWNER, "prefix": "snapshots/",
                       "key_marker": None, "version_id_marker": None}
            request.update(kwargs)
            with self.assertRaises(adapter.AdapterError):
                self.make_storage(executable).list_object_versions(**request)
        self.assertFalse(trace.exists())

    def test_constructor_copies_credentials_and_child_environment_is_allowlisted(self):
        executable, trace, *_ = self.write_fake(stdout=b'{"IsTruncated":false}')
        credentials = self.credential_values()
        storage = self.make_storage(executable, credentials=credentials)
        credentials["AWS_ACCESS_KEY_ID"] = "MUTATED"
        self.assertFalse(trace.exists())
        storage.list_object_versions(bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                                     key_marker=None, version_id_marker=None)
        recorded = json.loads(trace.read_text())
        env = recorded["environment"]
        expected_environment = {
            "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
            "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_EC2_METADATA_DISABLED",
            "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "AWS_CLI_AUTO_PROMPT", "AWS_PAGER",
            "AWS_MAX_ATTEMPTS", "LC_ALL",
        }
        # macOS injects this platform-local text encoding variable during exec;
        # it is not inherited by Popen's explicit environment mapping.
        if sys.platform == "darwin":
            expected_environment.add("__CF_USER_TEXT_ENCODING")
        self.assertEqual(set(env), expected_environment)
        self.assertEqual(env["AWS_ACCESS_KEY_ID"], "SYNTHETIC_ACCESS")
        self.assertNotIn("AWS_PROFILE", env)
        self.assertNotIn("AWS_ENDPOINT_URL", env)
        self.assertNotIn("PATH", env)
        self.assertNotIn("HOME", env)

    def test_exact_argv_manual_markers_and_optional_nontruncated_collections(self):
        body = self.completed_versions(truncated=False)
        executable, trace, *_ = self.write_fake(stdout=body)
        storage = self.make_storage(executable)
        page = storage.list_object_versions(bucket=BUCKET, expected_owner=OWNER,
                                            prefix="snapshots/", key_marker=None,
                                            version_id_marker=None)
        self.assertEqual(page["Versions"], [])
        self.assertEqual(page["DeleteMarkers"], [])
        argv = json.loads(trace.read_text())["argv"]
        self.assertEqual(argv[:2], ["s3api", "list-object-versions"])
        self.assertIn("--region", argv)
        self.assertIn("us-west-2", argv)
        self.assertIn("--expected-bucket-owner", argv)
        self.assertIn(OWNER, argv)
        self.assertIn("--no-paginate", argv)
        self.assertIn("--cli-error-format", argv)
        self.assertIn("json", argv)
        self.assertEqual(argv[argv.index("--prefix") + 1], "snapshots/")
        self.assertEqual(argv[argv.index("--max-keys") + 1], "1000")

        next_key = f"snapshots/{STAMP}/brain.bundle"
        next_version = "opaque-next-version"
        executable2, trace2, *_ = self.write_fake(stdout=self.completed_versions(
            versions=[{"Key": KEY, "VersionId": "v1", "IsLatest": True,
                       "LastModified": "2026-10-01T00:00:00Z"}],
            markers=[], truncated=True, NextKeyMarker=next_key,
            NextVersionIdMarker=next_version))
        storage2 = self.make_storage(executable2)
        page2 = storage2.list_object_versions(bucket=BUCKET, expected_owner=OWNER,
                                              prefix="snapshots/", key_marker=next_key,
                                              version_id_marker=next_version)
        self.assertEqual(len(page2["Versions"]), 1)
        argv2 = json.loads(trace2.read_text())["argv"]
        self.assertEqual(argv2[argv2.index("--key-marker") + 1], next_key)
        self.assertEqual(argv2[argv2.index("--version-id-marker") + 1], next_version)
        self.assertEqual(argv2.count("s3api"), 1)

    def test_success_drains_large_buffered_stdout_after_cli_exits(self):
        # The child exits immediately after writing. The adapter must still
        # drain the kernel's buffered pipe data instead of treating process
        # exit as proof that inherited output pipes are stuck open.
        versions = [
            {"Key": f"snapshots/{STAMP}/artifact-{index:04d}",
             "VersionId": f"version-{index:04d}", "IsLatest": False,
             "LastModified": "2026-10-01T00:00:00Z"}
            for index in range(900)
        ]
        body = self.completed_versions(versions=versions, markers=[], truncated=False)
        self.assertGreater(len(body), 64 * 1024)
        executable, trace, *_ = self.write_fake(stdout=body)
        page = self.make_storage(executable, timeout=5, stdout_limit=1024 * 1024).list_object_versions(
            bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
            key_marker=None, version_id_marker=None)
        self.assertTrue(trace.exists())
        self.assertEqual(len(page["Versions"]), len(versions))
        self.assertEqual(page["Versions"][-1]["VersionId"], "version-0899")

    def test_upload_and_part_continuation_markers_are_explicit_single_page_calls(self):
        next_key = f"snapshots/{STAMP}/multipart"
        executable, trace, *_ = self.write_fake(stdout=json.dumps({
            "IsTruncated": True,
            "Uploads": [{"Key": next_key, "UploadId": "upload-next", "Initiated": "2026-10-01T00:00:00Z"}],
            "NextKeyMarker": next_key,
            "NextUploadIdMarker": "upload-next",
        }).encode())
        self.make_storage(executable).list_multipart_uploads(
            bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
            key_marker=next_key, upload_id_marker="upload-next")
        argv = json.loads(trace.read_text())["argv"]
        self.assertEqual(argv[:2], ["s3api", "list-multipart-uploads"])
        self.assertEqual(argv[argv.index("--max-uploads") + 1], "1000")
        self.assertEqual(argv[argv.index("--key-marker") + 1], next_key)
        self.assertEqual(argv[argv.index("--upload-id-marker") + 1], "upload-next")
        self.assertIn("--no-paginate", argv)

        executable2, trace2, *_ = self.write_fake(stdout=json.dumps({
            "IsTruncated": True,
            "Parts": [{"PartNumber": 1}],
            "NextPartNumberMarker": 7,
        }).encode())
        self.make_storage(executable2).list_parts(
            bucket=BUCKET, expected_owner=OWNER, key=next_key,
            upload_id="opaque-upload", part_number_marker=3)
        argv2 = json.loads(trace2.read_text())["argv"]
        self.assertEqual(argv2[:2], ["s3api", "list-parts"])
        self.assertEqual(argv2[argv2.index("--max-parts") + 1], "1000")
        self.assertEqual(argv2[argv2.index("--part-number-marker") + 1], "3")
        self.assertEqual(argv2[argv2.index("--expected-bucket-owner") + 1], OWNER)

    def test_versions_truncated_optional_counterpart_only_normalizes_with_valid_progress(self):
        valid = [
            self.completed_versions(versions=[{"Key": KEY, "VersionId": "v1", "IsLatest": True,
                                                "LastModified": "2026-10-01T00:00:00Z"}], truncated=True,
                                    NextKeyMarker=KEY, NextVersionIdMarker="v1"),
            self.completed_versions(markers=[{"Key": KEY, "VersionId": "v1", "IsLatest": True,
                                               "LastModified": "2026-10-01T00:00:00Z"}], truncated=True,
                                    NextKeyMarker=KEY, NextVersionIdMarker="v1"),
        ]
        for body in valid:
            with self.subTest(body=body):
                executable, _, *_ = self.write_fake(stdout=body)
                page = self.make_storage(executable).list_object_versions(
                    bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                    key_marker=None, version_id_marker=None)
                self.assertIn("Versions", page)
                self.assertIn("DeleteMarkers", page)
        invalid = [
            b'{"IsTruncated":false,"Versions":null}',
            b'{"Versions":[],"DeleteMarkers":[]}',
            self.completed_versions(truncated=True, NextKeyMarker=KEY, NextVersionIdMarker="v"),
            self.completed_versions(versions=[], truncated=True, NextKeyMarker=KEY,
                                    NextVersionIdMarker="v"),
            b'{"IsTruncated":true,"Versions":[],"DeleteMarkers":[],"NextKeyMarker":"x","NextVersionIdMarker":"v"}',
        ]
        for body in invalid:
            with self.subTest(body=body):
                executable, trace, *_ = self.write_fake(stdout=body)
                with self.assertRaises(adapter.AdapterError):
                    self.make_storage(executable).list_object_versions(
                        bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                        key_marker=None, version_id_marker=None)
                self.assertTrue(trace.exists())

    def test_upload_and_parts_pages_validate_optional_and_truncated_arrays(self):
        cases = [
            ("list-multipart-uploads", b'{"IsTruncated":false}', "list_multipart_uploads"),
            ("list-parts", b'{"IsTruncated":false}', "list_parts"),
        ]
        for operation, body, method in cases:
            with self.subTest(operation=operation):
                executable, _, *_ = self.write_fake(stdout=body)
                storage = self.make_storage(executable)
                if method == "list_multipart_uploads":
                    result = storage.list_multipart_uploads(bucket=BUCKET, expected_owner=OWNER,
                        prefix="snapshots/", key_marker=None, upload_id_marker=None)
                    self.assertEqual(result["Uploads"], [])
                else:
                    result = storage.list_parts(bucket=BUCKET, expected_owner=OWNER, key=KEY,
                        upload_id="opaque-upload", part_number_marker=None)
                    self.assertEqual(result["Parts"], [])
        invalid = [
            ("list-multipart-uploads", b'{"IsTruncated":true,"Uploads":[],"NextKeyMarker":"' + KEY.encode() + b'","NextUploadIdMarker":"u"}'),
            ("list-parts", b'{"IsTruncated":true,"Parts":[],"NextPartNumberMarker":1}'),
            ("list-parts", b'{"IsTruncated":false,"Parts":null}'),
            ("list-parts", b'{"Parts":[]}'),
        ]
        for operation, body in invalid:
            with self.subTest(operation=operation, body=body):
                executable, *_ = self.write_fake(stdout=body)
                storage = self.make_storage(executable)
                with self.assertRaises(adapter.AdapterError):
                    if operation == "list-multipart-uploads":
                        storage.list_multipart_uploads(bucket=BUCKET, expected_owner=OWNER,
                            prefix="snapshots/", key_marker=None, upload_id_marker=None)
                    else:
                        storage.list_parts(bucket=BUCKET, expected_owner=OWNER, key=KEY,
                            upload_id="opaque-upload", part_number_marker=None)

    def test_list_parts_maps_only_exact_no_such_upload_code_status_and_operation(self):
        exact, *_ = self.write_fake(stderr=b'{"Code":"NoSuchUpload","Message":"synthetic"}', code=254)
        with self.assertRaises(retention.UploadNotFound):
            self.make_storage(exact).list_parts(bucket=BUCKET, expected_owner=OWNER, key=KEY,
                upload_id="opaque-upload", part_number_marker=None)
        controls = [
            (b'{"Code":"AccessDenied","Message":"synthetic"}', 254),
            (b'{"Code":"NoSuchUpload","Message":"synthetic"}', 255),
            (b'{"Code":"NoSuchUpload","Message":"synthetic","Code":"AccessDenied"}', 254),
            (b'not-json', 254),
        ]
        for stderr, code in controls:
            with self.subTest(stderr=stderr, code=code):
                executable, *_ = self.write_fake(stderr=stderr, code=code)
                with self.assertRaises(adapter.AdapterError):
                    self.make_storage(executable).list_parts(bucket=BUCKET, expected_owner=OWNER,
                        key=KEY, upload_id="opaque-upload", part_number_marker=None)
        wrong_operation, *_ = self.write_fake(stderr=b'{"Code":"NoSuchUpload"}', code=254)
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(wrong_operation).list_object_versions(bucket=BUCKET,
                expected_owner=OWNER, prefix="snapshots/", key_marker=None,
                version_id_marker=None)

    def test_delete_uses_bounded_stdin_quiet_false_and_requires_exact_confirmations(self):
        response = {"Deleted": [{"Key": KEY, "VersionId": "null"}]}
        executable, trace, *_ = self.write_fake(stdout=json.dumps(response).encode())
        storage = self.make_storage(executable)
        storage.delete_versions(bucket=BUCKET, expected_owner=OWNER,
                                objects=[{"Key": KEY, "VersionId": "null"}])
        recorded = json.loads(trace.read_text())
        argv = recorded["argv"]
        self.assertEqual(argv[0:2], ["s3api", "delete-objects"])
        self.assertEqual(argv[argv.index("--delete") + 1], "file:///dev/stdin")
        request = json.loads(recorded["stdin"])
        self.assertEqual(request, {"Objects": [{"Key": KEY, "VersionId": "null"}], "Quiet": False})
        self.assertEqual(argv[argv.index("--expected-bucket-owner") + 1], OWNER)

        bad_responses = [
            {"Deleted": []},
            {"Deleted": [{"Key": KEY, "VersionId": "null"}], "Errors": [{"Code": "Denied"}]},
            {"Deleted": [{"Key": KEY, "VersionId": "null"}, {"Key": KEY, "VersionId": "null"}]},
            {"Deleted": [{"Key": "snapshots/20261001T000000Z/foreign", "VersionId": "null"}]},
        ]
        for bad in bad_responses:
            with self.subTest(response=bad):
                executable2, *_ = self.write_fake(stdout=json.dumps(bad).encode())
                with self.assertRaises(adapter.AdapterError):
                    self.make_storage(executable2).delete_versions(bucket=BUCKET,
                        expected_owner=OWNER, objects=[{"Key": KEY, "VersionId": "null"}])
        for objects in ([], [{"Key": "outside/x", "VersionId": "v"}],
                        [{"Key": KEY, "VersionId": "v\n"}],
                        [{"Key": KEY, "VersionId": "v", "Extra": "x"}],
                        [{"Key": KEY, "VersionId": "v"}] * 1001):
            with self.subTest(objects_len=len(objects)):
                executable3, trace3, *_ = self.write_fake(stdout=b'{"Deleted":[]}')
                with self.assertRaises(adapter.AdapterError):
                    self.make_storage(executable3).delete_versions(bucket=BUCKET,
                        expected_owner=OWNER, objects=objects)
                self.assertFalse(trace3.exists())

    def test_delete_streams_request_larger_than_pipe_capacity(self):
        objects = [
            {"Key": f"snapshots/{STAMP}/artifact-{index:04d}", "VersionId": f"v-{index:04d}"}
            for index in range(1000)
        ]
        response = {"Deleted": list(objects)}
        executable, trace, *_ = self.write_fake(stdout=json.dumps(response).encode())
        storage = self.make_storage(executable, timeout=10, stdout_limit=1024 * 1024)
        storage.delete_versions(bucket=BUCKET, expected_owner=OWNER, objects=objects)
        recorded = json.loads(trace.read_text())
        body = json.loads(recorded["stdin"])
        self.assertEqual(len(recorded["stdin"].encode("utf-8")), len(json.dumps(
            {"Objects": objects, "Quiet": False}, sort_keys=True, separators=(",", ":"),
            ensure_ascii=False).encode("utf-8")))
        self.assertEqual(body["Objects"], objects)
        self.assertFalse(body["Quiet"])

    def test_abort_requires_empty_success_output_and_uses_fixed_scope(self):
        executable, trace, *_ = self.write_fake(stdout=b"")
        self.make_storage(executable).abort_multipart(bucket=BUCKET, expected_owner=OWNER,
                                                      key=KEY, upload_id="opaque-upload")
        argv = json.loads(trace.read_text())["argv"]
        self.assertEqual(argv[0:2], ["s3api", "abort-multipart-upload"])
        self.assertIn(KEY, argv)
        self.assertIn("opaque-upload", argv)
        self.assertIn(OWNER, argv)
        nonempty, *_ = self.write_fake(stdout=b"{}")
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(nonempty).abort_multipart(bucket=BUCKET, expected_owner=OWNER,
                                                        key=KEY, upload_id="opaque-upload")

    def test_strict_json_rejects_duplicates_depth_trailing_nonfinite_and_oversize(self):
        values = [
            b'{"IsTruncated":false,"IsTruncated":false}',
            b'{"IsTruncated":false} trailing',
            b'{"IsTruncated":false,"Unexpected":NaN}',
            (b'{"IsTruncated":false,"Nested":' + b"[" * 40 + b"0" + b"]" * 40 + b"}"),
            b"{" + b" " * 1024,
        ]
        for body in values:
            with self.subTest(body=body[:60]):
                executable, *_ = self.write_fake(stdout=body)
                storage = self.make_storage(executable, stdout_limit=512)
                with self.assertRaises(adapter.AdapterError):
                    storage.list_object_versions(bucket=BUCKET, expected_owner=OWNER,
                        prefix="snapshots/", key_marker=None, version_id_marker=None)
        executable, *_ = self.write_fake(stdout=b'{"IsTruncated":false}')
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(executable, stdout_limit=8).list_object_versions(
                bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                key_marker=None, version_id_marker=None)
        executable, *_ = self.write_fake(stderr=b"x" * 200)
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(executable, stderr_limit=8).list_object_versions(
                bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                key_marker=None, version_id_marker=None)

    def test_timeout_kills_blocked_cli_and_output_limit_fails_closed(self):
        executable, trace, *_ = self.write_fake(behavior="self_terminate_after_timeout")
        with self.assertRaisesRegex(adapter.AdapterError, "timed out"):
            self.make_storage(executable, timeout=1).list_object_versions(
                bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                key_marker=None, version_id_marker=None)
        self.assertTrue(trace.exists())
        noisy, *_ = self.write_fake(stdout=b"x" * 4096)
        with self.assertRaisesRegex(adapter.AdapterError, "output limit"):
            self.make_storage(noisy, stdout_limit=32).list_object_versions(
                bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                key_marker=None, version_id_marker=None)

    def test_retained_descendant_pipe_times_out_and_owned_group_is_killed(self):
        if not hasattr(os, "fork") or not hasattr(os, "killpg"):
            self.skipTest("POSIX fork and process groups are required")
        executable, trace, pidfile, fifo = self.write_fake(behavior="descendant_pipes")
        os.mkfifo(fifo, 0o600)
        with self.assertRaises(adapter.AdapterError):
            self.make_storage(executable, timeout=1).list_object_versions(
                bucket=BUCKET, expected_owner=OWNER, prefix="snapshots/",
                key_marker=None, version_id_marker=None)
        self.assertTrue(trace.exists())
        self.assertTrue(pidfile.exists())
        child_pid = int(pidfile.read_text())
        state = subprocess.run(["/bin/ps", "-o", "stat=", "-p", str(child_pid)],
                               check=False, capture_output=True, text=True).stdout.strip()
        self.assertTrue(not state or state.startswith("Z"), f"descendant still running: {state}")
        self.assertFalse(Path(str(fifo) + ".late").exists())


if __name__ == "__main__":
    unittest.main()
