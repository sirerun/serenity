import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location("backup_completion", Path(__file__).parents[1] / "backup_completion.py")
completion = importlib.util.module_from_spec(spec)
spec.loader.exec_module(completion)


class CompletionTests(unittest.TestCase):
    prefix = "snapshots/20261001T083000Z/"

    def setUp(self):
        # Test storage, including temporary files, stays on the build volume.
        self.temporary = tempfile.TemporaryDirectory(dir=os.environ.get("TMPDIR"))
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.brain_id = "b" * 16
        (self.root / "control.db").write_bytes(b"synthetic database bytes")
        (self.root / "brain.bundle").write_bytes(b"synthetic canonical bundle bytes")
        self.manifest = {
            "version": 2,
            "source": {"build_sha": "a" * 40, "schema_version": 9},
            "created_at": "2026-10-01T08:30:00Z",
            "control_db": self.artifact("control.db"),
            "brains": [{"id": self.brain_id, "empty": False, "heads": [{"ref": "HEAD", "object_id": "c" * 40}], **self.artifact("brain.bundle")}],
            "journal_watermark": {"generation": 1, "sequence_id": 1, "entry_hash": "d" * 64},
        }
        self.save_manifest()

    def artifact(self, name):
        body = (self.root / name).read_bytes()
        return {"relative_path": name, "length_bytes": len(body), "sha256": hashlib.sha256(body).hexdigest()}

    def save_manifest(self):
        (self.root / "manifest.json").write_text(json.dumps(self.manifest))

    def seal(self):
        return completion.create_completion(self.root, self.prefix)

    def verify(self):
        return completion.verify_completed(self.root, self.prefix)

    def test_completed_roundtrip_binds_manifest_prefix_and_permissions(self):
        record = self.seal()
        self.assertEqual(self.verify(), record)
        self.assertEqual(record["manifest_sha256"], hashlib.sha256((self.root / "manifest.json").read_bytes()).hexdigest())
        self.assertEqual((self.root / "COMPLETE").stat().st_mode & 0o777, 0o600)

    def test_interrupted_upload_without_completion_is_rejected(self):
        with self.assertRaises(completion.VerificationError):
            self.verify()

    def test_tampered_artifacts_or_manifest_are_rejected(self):
        self.seal()
        for name in ("control.db", "brain.bundle", "manifest.json"):
            with self.subTest(name=name):
                path = self.root / name
                body = path.read_bytes()
                try:
                    path.write_bytes(body + b" ")
                    with self.assertRaises(completion.VerificationError):
                        self.verify()
                finally:
                    path.write_bytes(body)

    def test_completion_cannot_be_reused_for_other_prefix(self):
        self.seal()
        with self.assertRaises(completion.VerificationError):
            completion.verify_completed(self.root, "snapshots/20261002T083000Z/")

    def test_duplicate_or_unsafe_artifact_paths_are_rejected(self):
        for name in ("../outside", "/outside", "control.db", "CONTROL.DB", "COMPLETE", "manifest.json"):
            with self.subTest(name=name):
                self.manifest["brains"][0]["relative_path"] = name
                self.save_manifest()
                with self.assertRaises(completion.VerificationError):
                    self.seal()

    def test_artifact_symlink_is_rejected_without_reading_target(self):
        path = self.root / "brain.bundle"
        path.unlink()
        path.symlink_to(self.root / "control.db")
        with self.assertRaises(OSError):
            self.seal()

    def test_fifo_is_rejected_without_blocking(self):
        path = self.root / "brain.bundle"
        path.unlink()
        os.mkfifo(path)
        with self.assertRaises(completion.VerificationError):
            self.seal()

    def test_unknown_artifact_prevents_completion(self):
        (self.root / "extra-secret").write_bytes(b"not in manifest")
        with self.assertRaises(completion.VerificationError):
            self.seal()
        self.assertFalse((self.root / "COMPLETE").exists())

    def test_legacy_or_noninteger_manifest_version_is_refused(self):
        for version in (1, True, 2.0):
            with self.subTest(version=version):
                self.manifest["version"] = version
                self.save_manifest()
                with self.assertRaises(completion.VerificationError):
                    self.seal()

    def test_duplicate_json_field_is_rejected(self):
        path = self.root / "manifest.json"
        path.write_text(path.read_text().replace('"version": 2', '"version": 2, "version": 2'))
        with self.assertRaises(completion.VerificationError):
            self.seal()

    def test_truncated_or_extended_completion_is_rejected(self):
        self.seal()
        path = self.root / "COMPLETE"
        record = json.loads(path.read_text())
        for body in ('{', json.dumps({**record, "untrusted": True}), json.dumps({**record, "version": True})):
            with self.subTest(body=body):
                path.write_text(body)
                with self.assertRaises(completion.VerificationError):
                    self.verify()

    def test_completion_is_never_overwritten(self):
        self.seal()
        body = (self.root / "COMPLETE").read_bytes()
        self.assertEqual(self.seal(), self.verify())
        self.assertEqual((self.root / "COMPLETE").read_bytes(), body)

    def test_existing_invalid_completion_is_not_overwritten(self):
        path = self.root / "COMPLETE"
        path.write_bytes(b"invalid existing record")
        with self.assertRaises(completion.VerificationError):
            self.seal()
        self.assertEqual(path.read_bytes(), b"invalid existing record")

    def test_crash_orphan_before_or_after_link_does_not_break_retry(self):
        # A killed process leaves its sibling temporary file. It belongs to
        # the caller's outer staging, not to snapshot artifact inventory.
        orphan = self.root.parent / ("." + self.root.name + "-completion-orphan")
        orphan.write_bytes(b"interrupted temporary record")
        self.addCleanup(orphan.unlink)
        record = self.seal()
        self.assertEqual(self.verify(), record)
        self.assertEqual(self.seal(), record)
        self.assertTrue(orphan.exists())  # Never delete another attempt's file.

    def test_link_failure_cleans_only_current_temporary_record(self):
        before = set(self.root.parent.glob("." + self.root.name + "-completion-*"))
        with mock.patch.object(completion.os, "link", side_effect=OSError("link denied")):
            with self.assertRaises(OSError):
                self.seal()
        self.assertFalse((self.root / "COMPLETE").exists())
        self.assertEqual(set(self.root.parent.glob("." + self.root.name + "-completion-*")), before)
        self.assertEqual(self.seal(), self.verify())

    def test_directory_sync_failure_keeps_valid_completion_for_retry(self):
        with mock.patch.object(completion, "_sync_directory", side_effect=OSError("sync failed")):
            with self.assertRaises(OSError):
                self.seal()
        self.assertEqual(self.seal(), self.verify())

    def test_nonprivate_staging_is_refused(self):
        self.root.chmod(0o755)
        with self.assertRaises(completion.VerificationError):
            self.seal()

    def test_snapshot_prefix_requires_valid_utc_time(self):
        for prefix in ("other/20261001T083000Z/", "snapshots/20261301T083000Z/", "snapshots/../../", self.prefix + "extra"):
            with self.subTest(prefix=prefix):
                with self.assertRaises(completion.VerificationError):
                    completion.create_completion(self.root, prefix)

    def test_duplicate_brain_inventory_is_rejected(self):
        self.manifest["brains"].append(dict(self.manifest["brains"][0]))
        self.save_manifest()
        with self.assertRaises(completion.VerificationError):
            self.seal()


if __name__ == "__main__":
    unittest.main()
