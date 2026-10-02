"""Filesystem and fake-transport tests for the bounded T23.52 byte pipeline."""

import hashlib
import importlib.util
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "backup_publish", Path(__file__).parents[1] / "backup_publish.py"
)
publish = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = publish
spec.loader.exec_module(publish)

PREFIX = "snapshots/20261001T083000Z/"
FIXTURE_ROOT = os.environ.get("SERENITY_RECOVERY_TEST_TMPDIR")
GiB = 1024**3


def supported_limits():
    return publish.Limits(
        max_snapshot_bytes=30 * GiB,
        max_manifest_bytes=16 * 1024 * 1024,
        max_completion_bytes=4096,
        max_brains=1000,
        max_artifacts=1001,
        max_objects=1003,
        chunk_bytes=4096,
        max_json_depth=16,
        max_json_nodes=50_000,
    )


class FakeStore:
    """Fully enumerating fake versioned store with bounded byte transfers."""

    def __init__(self):
        self.objects = {}
        self.extra_versions = []
        self.put_order = []
        self.fail_put_at = None
        self.fail_put_after_commit_at = None
        self.rewrite_after_commit_at = None
        self.rewrite_after_commit_body = b""
        self.fail_reads = set()
        self.mutate_source = None
        self.puts = 0

    def list_prefix(self, prefix, *, max_objects):
        found = [publish.RemoteObject(key, "v1", False)
                 for key in self.objects if key.startswith(prefix)]
        found.extend(item for item in self.extra_versions if item.key.startswith(prefix))
        if len(found) > max_objects:
            raise publish.PublishError("fake complete version inventory exceeds bound")
        return found

    def put_if_absent(self, key, source, *, length_bytes, sha256, chunk_bytes):
        if key in self.objects or any(item.key == key for item in self.extra_versions):
            raise publish.ObjectExists(key)
        self.puts += 1
        if self.fail_put_at == self.puts:
            raise OSError("injected failure before put")
        body = bytearray()
        while chunk := source.read(chunk_bytes):
            if len(chunk) > chunk_bytes:
                raise AssertionError("uploader yielded an oversized chunk")
            body.extend(chunk)
        if len(body) != length_bytes or hashlib.sha256(body).hexdigest() != sha256:
            raise AssertionError("put did not receive exact advertised bytes")
        self.objects[key] = bytes(body)
        self.put_order.append(key)
        if self.rewrite_after_commit_at == self.puts:
            self.objects[key] = self.rewrite_after_commit_body
        if self.mutate_source is not None:
            self.mutate_source()
            self.mutate_source = None
        if self.fail_put_after_commit_at == self.puts:
            raise OSError("injected lost response after remote commit")

    def read_bytes(self, key, *, max_bytes):
        if key in self.fail_reads:
            raise OSError("injected read failure")
        body = self.objects[key]
        if len(body) > max_bytes:
            raise publish.PublishError("fake object exceeds read bound")
        return body

    def read_to(self, key, destination, *, max_bytes, chunk_bytes):
        if key in self.fail_reads:
            raise OSError("injected stream failure")
        body = self.objects[key]
        if len(body) > max_bytes:
            for start in range(0, max_bytes, chunk_bytes):
                destination.write(body[start:min(max_bytes, start + chunk_bytes)])
            destination.write(body[max_bytes:max_bytes + chunk_bytes])
            return len(body)
        for start in range(0, len(body), chunk_bytes):
            destination.write(body[start:start + chunk_bytes])
        return len(body)


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(dir=FIXTURE_ROOT or None)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        info = self.root.stat()
        self.assertTrue(self.root.is_dir())
        self.assertEqual(info.st_uid, os.geteuid())
        self.assertEqual(info.st_mode & 0o077, 0)
        self.snapshot = self.root / "snapshot"
        self.snapshot.mkdir(mode=0o700)
        self.limits = supported_limits()
        self.store = FakeStore()
        self._make_snapshot(1)

    def _make_snapshot(self, brain_count):
        brains = []
        # The large acceptance fixture is laid out as 100 accounts x 10 brains.
        for index in range(brain_count):
            brain_id = f"{index // 10:04x}{index % 10:012x}"
            name = f"brain-{brain_id}.bundle"
            body = f"bundle-{index}".encode()
            (self.snapshot / name).write_bytes(body)
            brains.append({
                "id": brain_id,
                "relative_path": name,
                "length_bytes": len(body),
                "sha256": hashlib.sha256(body).hexdigest(),
                "empty": False,
                "heads": [{"ref": "HEAD", "object_id": "a" * 40}],
            })
        control = b"sqlite snapshot bytes"
        (self.snapshot / "control.db").write_bytes(control)
        manifest = {
            "version": 2,
            "source": {"build_sha": "build-test", "schema_version": 9},
            "created_at": "2026-10-01T08:30:00Z",
            "control_db": {
                "relative_path": "control.db",
                "length_bytes": len(control),
                "sha256": hashlib.sha256(control).hexdigest(),
            },
            "brains": brains,
            "journal_watermark": {
                "generation": 1,
                "sequence_id": 1,
                "entry_hash": "b" * 64,
            },
        }
        (self.snapshot / "manifest.json").write_text(
            json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n"
        )

    def test_publish_and_download_verified_snapshot(self):
        receipt = publish.publish_snapshot(
            self.snapshot, PREFIX, staging_root=self.root,
            storage=self.store, limits=self.limits,
        )
        self.assertEqual(receipt.snapshot_prefix, PREFIX)
        self.assertEqual(receipt.object_count, 4)
        self.assertEqual(self.store.put_order[-1], PREFIX + "COMPLETE")
        self.assertEqual(self.store.put_order[-2], PREFIX + "manifest.json")
        self.assertTrue(self.store.put_order.index(PREFIX + "manifest.json") > self.store.put_order.index(PREFIX + "control.db"))
        restored_bytes = publish.download_snapshot(
            PREFIX, staging_parent=self.root, storage=self.store, limits=self.limits,
        )
        self.assertEqual((restored_bytes / "control.db").read_bytes(), b"sqlite snapshot bytes")
        self.assertTrue((restored_bytes / "COMPLETE").is_file())
        publish.backup_completion.verify_completed(restored_bytes, PREFIX)

    def test_supports_one_hundred_accounts_by_ten_rich_brains(self):
        for entry in self.snapshot.iterdir():
            entry.unlink()
        self._make_snapshot(1000)
        receipt = publish.publish_snapshot(
            self.snapshot, PREFIX, staging_root=self.root,
            storage=self.store, limits=self.limits,
        )
        self.assertEqual(receipt.object_count, 1003)
        self.assertEqual(len(self.store.objects), 1003)
        lower = publish.Limits(
            max_snapshot_bytes=30 * GiB,
            max_manifest_bytes=16 * 1024 * 1024,
            max_completion_bytes=4096,
            max_brains=999,
            max_artifacts=1000,
            max_objects=1002,
            chunk_bytes=4096,
            max_json_depth=16,
            max_json_nodes=50_000,
        )
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=FakeStore(), limits=lower,
            )

    def test_interrupted_each_publication_phase_never_commits_completion(self):
        for entry in self.snapshot.iterdir():
            entry.unlink()
        self._make_snapshot(3)
        for failed_put in (1, 2, 3, 4, 5):
            with self.subTest(failed_put=failed_put):
                store = FakeStore()
                if failed_put % 2:
                    store.fail_put_at = failed_put
                else:
                    store.fail_put_after_commit_at = failed_put
                with self.assertRaises(publish.PublishError):
                    publish.publish_snapshot(
                        self.snapshot, PREFIX, staging_root=self.root,
                        storage=store, limits=self.limits,
                    )
                self.assertNotIn(PREFIX + "COMPLETE", store.objects)

    def test_remote_readback_failure_never_publishes_completion(self):
        self.store.fail_reads.add(PREFIX + "control.db")
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )
        self.assertNotIn(PREFIX + "COMPLETE", self.store.objects)

    def test_ambiguous_final_success_requires_exact_readback(self):
        self.store.fail_put_after_commit_at = 4
        receipt = publish.publish_snapshot(
            self.snapshot, PREFIX, staging_root=self.root,
            storage=self.store, limits=self.limits,
        )
        self.assertEqual(receipt.snapshot_prefix, PREFIX)
        self.assertEqual(self.store.put_order[-1], PREFIX + "COMPLETE")

    def test_ambiguous_final_failure_without_readback_has_no_receipt(self):
        self.store.fail_put_after_commit_at = 4
        self.store.fail_reads.add(PREFIX + "COMPLETE")
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )

    def test_final_completion_put_failure_without_remote_record_has_no_receipt(self):
        self.store.fail_put_at = 4
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )
        self.assertNotIn(PREFIX + "COMPLETE", self.store.objects)

    def test_ambiguous_final_write_with_wrong_record_is_not_reconciled(self):
        self.store.fail_put_after_commit_at = 4
        self.store.rewrite_after_commit_at = 4
        self.store.rewrite_after_commit_body = b"{}"
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )

    def test_existing_prefix_object_or_delete_marker_is_refused(self):
        self.store.objects[PREFIX + "old"] = b"old"
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )
        self.assertEqual(self.store.put_order, [])
        other = FakeStore()
        other.extra_versions.append(publish.RemoteObject(PREFIX + "old", "v0", True))
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=other, limits=self.limits,
            )

    def test_changed_local_staging_never_publishes_completion(self):
        def mutate():
            (self.snapshot / "control.db").write_bytes(b"changed bytes")

        self.store.mutate_source = mutate
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(
                self.snapshot, PREFIX, staging_root=self.root,
                storage=self.store, limits=self.limits,
            )
        self.assertNotIn(PREFIX + "COMPLETE", self.store.objects)

    def test_extra_local_file_symlink_and_unsafe_manifest_path_fail_before_put(self):
        extra = self.snapshot / "unexpected"
        extra.write_bytes(b"extra")
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                     storage=self.store, limits=self.limits)
        extra.unlink()
        link = self.snapshot / "linked"
        link.symlink_to(self.snapshot / "control.db")
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                     storage=self.store, limits=self.limits)
        link.unlink()
        manifest = json.loads((self.snapshot / "manifest.json").read_text())
        manifest["control_db"]["relative_path"] = "../outside"
        (self.snapshot / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                     storage=self.store, limits=self.limits)
        self.assertEqual(self.store.put_order, [])

    def test_manifest_size_and_total_declared_bytes_are_bounded(self):
        manifest = json.loads((self.snapshot / "manifest.json").read_text())
        manifest["control_db"]["length_bytes"] = 30 * GiB + 1
        (self.snapshot / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                     storage=self.store, limits=self.limits)
        self.assertEqual(self.store.put_order, [])

    def test_excess_brain_inventory_and_bad_limits_are_rejected(self):
        extra = [
            {"id": f"{index:016x}", "empty": True}
            for index in range(1001)
        ]
        manifest = json.loads((self.snapshot / "manifest.json").read_text())
        manifest["brains"] = extra
        (self.snapshot / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaises(publish.PublishError):
            publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                     storage=self.store, limits=self.limits)
        with self.assertRaises(ValueError):
            publish.Limits(True, 16 * 1024 * 1024, 4096, 1000, 1001, 1003, 4096, 16, 50_000)
        with self.assertRaises(ValueError):
            publish.Limits(30 * GiB, 16 * 1024 * 1024, 4096, 1001, 1002, 1004, 4096, 16, 50_000)
        with self.assertRaises(ValueError):
            publish.Limits(30 * GiB, 16 * 1024 * 1024, 4096, 1000, 1000, 1002, 4096, 16, 50_000)
        with self.assertRaises(ValueError):
            publish.Limits(sys.maxsize + 1, 16 * 1024 * 1024, 4096, 1000, 1001, 1003, 4096, 16, 50_000)

    def test_utf16_deep_json_and_low_node_budget_are_rejected(self):
        valid = (self.snapshot / "manifest.json").read_bytes()
        with self.subTest("UTF-16"):
            (self.snapshot / "manifest.json").write_bytes(valid.decode("utf-8").encode("utf-16"))
            with self.assertRaises(publish.PublishError):
                publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                         storage=self.store, limits=self.limits)
        with self.subTest("deep nesting"), self.assertRaises(publish.PublishError):
            publish._json_bytes(b"[" * 17 + b"0" + b"]" * 17, "manifest", self.limits)
        with self.subTest("node budget"):
            low_nodes = publish.Limits(
                max_snapshot_bytes=30 * GiB,
                max_manifest_bytes=16 * 1024 * 1024,
                max_completion_bytes=4096,
                max_brains=1000,
                max_artifacts=1001,
                max_objects=1003,
                chunk_bytes=4096,
                max_json_depth=16,
                max_json_nodes=4,
            )
            with self.assertRaises(publish.PublishError):
                publish._parse_manifest(valid, low_nodes)

    def test_download_rejects_missing_complete_tamper_extras_and_truncation(self):
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        del self.store.objects[PREFIX + "COMPLETE"]
        before = set(self.root.iterdir())
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.assertEqual(set(self.root.iterdir()), before)
        # Restore a valid publisher output, then corrupt bytes and inventory.
        self.store = FakeStore()
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "control.db"] = b"tampered"
        before = set(self.root.iterdir())
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.assertEqual(set(self.root.iterdir()), before)

    def test_download_rejects_truncated_completion_and_manifest_hash_mismatch(self):
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        good_completion = self.store.objects[PREFIX + "COMPLETE"]
        self.store.objects[PREFIX + "COMPLETE"] = b"{\"version\":"
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "COMPLETE"] = b"{}"
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "COMPLETE"] = good_completion
        self.store.objects[PREFIX + "manifest.json"] += b" "
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)

    def test_download_rejects_over_limit_artifact_stream_and_cleans_stage(self):
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "control.db"] += b"x"
        before = set(self.root.iterdir())
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.assertEqual(set(self.root.iterdir()), before)

    def test_download_rejects_truncated_manifest_and_unexpected_remote_object(self):
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "manifest.json"] = b'{"version":2'
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)
        self.store = FakeStore()
        publish.publish_snapshot(self.snapshot, PREFIX, staging_root=self.root,
                                 storage=self.store, limits=self.limits)
        self.store.objects[PREFIX + "extra"] = b"extra"
        with self.assertRaises(publish.PublishError):
            publish.download_snapshot(PREFIX, staging_parent=self.root,
                                      storage=self.store, limits=self.limits)


if __name__ == "__main__":
    unittest.main()
