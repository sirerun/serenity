"""Fake-adapter contract tests for the unused all-version retention library."""

import importlib.util
import json
import sys
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "backup_retention", Path(__file__).parents[1] / "deploy/hosted/backup_retention.py"
)
retention = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = retention
spec.loader.exec_module(retention)


NOW = datetime(2026, 10, 1, 12, tzinfo=timezone.utc)
OWNER = "owner-123"
BUCKET = "backup-test"


def key_at(stamp, name="brain.bundle"):
    return f"snapshots/{stamp:%Y%m%dT%H%M%SZ}/{name}"


class FakeStorage:
    def __init__(self):
        self.versions = []
        self.uploads = []
        self.deleted = []
        self.delete_batches = []
        self.aborted = []
        self.parts = {}
        self.calls = []
        self.delete_errors = []
        self.noop_delete = False
        self.repeat_version_marker = False
        self.missing_upload_pages = False

    def _page(self, records, marker, size, field):
        start = int(marker or 0)
        body = records[start:start + size]
        end = start + len(body)
        return body, end < len(records), str(end) if end < len(records) else None

    def list_object_versions(self, *, prefix, key_marker, version_id_marker, expected_owner):
        self.calls.append(("versions", prefix, expected_owner))
        merged = sorted(self.versions, key=lambda x: (x["Key"], x["VersionId"], x["DeleteMarker"]))
        body, more, marker = self._page(merged, key_marker, 2, "Versions")
        if more:
            return {"Versions": [x for x in body if not x["DeleteMarker"]],
                    "DeleteMarkers": [x for x in body if x["DeleteMarker"]],
                    "IsTruncated": True, "NextKeyMarker": marker, "NextVersionIdMarker": marker}
        return {"Versions": [x for x in body if not x["DeleteMarker"]],
                "DeleteMarkers": [x for x in body if x["DeleteMarker"]], "IsTruncated": False}

    def list_multipart_uploads(self, *, prefix, key_marker, upload_id_marker, expected_owner):
        self.calls.append(("uploads", prefix, expected_owner))
        if self.missing_upload_pages:
            return {"IsTruncated": False}
        body, more, marker = self._page(sorted(self.uploads, key=lambda x: (x["Key"], x["UploadId"])), key_marker, 2, "Uploads")
        result = {"Uploads": body, "IsTruncated": more}
        if more:
            result.update(NextKeyMarker=marker, NextUploadIdMarker=marker)
        return result

    def list_parts(self, *, key, upload_id, part_number_marker, expected_owner):
        parts = self.parts.get(upload_id, [])
        start = int(part_number_marker or 0)
        return {"Parts": parts[start:start + 1000], "IsTruncated": False}

    def delete_versions(self, *, objects, expected_owner):
        self.delete_batches.append(len(objects))
        if self.delete_errors:
            return {"Errors": list(self.delete_errors), "Deleted": []}
        if not self.noop_delete:
            for item in objects:
                self.deleted.append((item["Key"], item["VersionId"]))
                self.versions = [row for row in self.versions if (row["Key"], row["VersionId"]) != (item["Key"], item["VersionId"])]
        return {"Deleted": list(objects)}

    def abort_multipart(self, *, key, upload_id, expected_owner):
        self.aborted.append((key, upload_id))
        self.uploads = [x for x in self.uploads if x["UploadId"] != upload_id]

    def version(self, stamp, version_id, *, marker=False, name="brain.bundle", last_modified=None):
        key = key_at(stamp, name)
        self.versions.append({"Key": key, "VersionId": version_id, "DeleteMarker": marker,
                              "LastModified": (last_modified or stamp).isoformat().replace("+00:00", "Z")})
        return key

    def upload(self, stamp, upload_id):
        self.uploads.append({"Key": key_at(stamp, "partial"), "UploadId": upload_id,
                              "Initiated": stamp.isoformat().replace("+00:00", "Z")})


class RetentionTests(unittest.TestCase):
    def setUp(self):
        self.storage = FakeStorage()
        self.old = NOW - timedelta(days=29)
        self.new = NOW - timedelta(days=28)
        self.storage.version(self.old, "v-old")
        self.storage.version(self.old, "d-old", marker=True)
        self.storage.version(self.new, "v-new")
        self.storage.upload(NOW - timedelta(days=2), "u-old")
        self.storage.upload(NOW - timedelta(hours=2), "u-new")

    def plan(self):
        return retention.inventory(self.storage, bucket=BUCKET, expected_owner=OWNER, now=NOW)

    def apply(self, plan, now=NOW):
        return retention.apply(self.storage, plan, approved_sha256=plan.sha256,
                               bucket=BUCKET, expected_owner=OWNER, now=now)

    def test_plan_is_dry_run_exact_and_purge_verifies_all_versions_markers_and_old_uploads(self):
        plan = self.plan()
        self.assertEqual(len(plan.versions), 3)
        self.assertEqual(len(plan.uploads), 2)
        self.assertEqual(self.storage.deleted, [])
        self.assertEqual(self.storage.aborted, [])
        result = self.apply(plan)
        self.assertEqual(result.deleted_versions, 2)
        self.assertEqual(result.aborted_uploads, 1)
        self.assertEqual(self.storage.deleted, [(key_at(self.old), "d-old"), (key_at(self.old), "v-old")])
        self.assertEqual(self.storage.aborted, [(key_at(NOW - timedelta(days=2), "partial"), "u-old")])
        self.assertEqual({v["VersionId"] for v in self.storage.versions}, {"v-new"})
        self.assertEqual({u["UploadId"] for u in self.storage.uploads}, {"u-new"})
        self.assertEqual(result.oldest_surviving_snapshot, retention._iso(self.new))
        self.assertEqual(result.oldest_surviving_snapshot_age_seconds, 28 * 24 * 60 * 60)
        self.assertEqual(result.oldest_surviving_last_modified_age_seconds, 28 * 24 * 60 * 60)
        self.assertTrue(all(call[1:] == ("snapshots/", OWNER) for call in self.storage.calls))

    def test_plan_digest_and_scope_are_manual_apply_authorization(self):
        plan = self.plan()
        with self.assertRaises(retention.RetentionError):
            retention.apply(self.storage, plan, approved_sha256="0" * 64, bucket=BUCKET,
                            expected_owner=OWNER, now=NOW)
        with self.assertRaises(retention.RetentionError):
            retention.apply(self.storage, plan, approved_sha256=plan.sha256, bucket=BUCKET,
                            expected_owner="other-owner", now=NOW)
        with self.assertRaises(retention.RetentionError):
            retention.apply(self.storage, plan, approved_sha256=plan.sha256, bucket=BUCKET,
                            expected_owner=OWNER, now=NOW - timedelta(hours=1))
        self.assertEqual(self.storage.deleted, [])

    def test_canonical_plan_roundtrip_rejects_mutation_and_duplicate_fields(self):
        plan = self.plan()
        body = retention.serialize_plan(plan)
        self.assertEqual(retention.load_plan(body), plan)
        changed = json.loads(body)
        changed["prefix"] = "deletion-journal/"
        with self.assertRaises(retention.RetentionError):
            retention.load_plan(json.dumps(changed, sort_keys=True, separators=(",", ":")).encode() + b"\n")
        with self.assertRaisesRegex(retention.RetentionError, "duplicate"):
            retention.load_plan(body.replace(b'"bucket":"backup-test"', b'"bucket":"backup-test","bucket":"backup-test"'))

    def test_changed_inventory_refuses_without_deleting(self):
        plan = self.plan()
        self.storage.version(self.new, "concurrent")
        with self.assertRaisesRegex(retention.RetentionError, "inventory differs"):
            self.apply(plan)
        self.assertEqual(self.storage.deleted, [])

    def test_foreign_or_malformed_snapshot_key_fails_closed(self):
        for bad in ("deletion-journal/g1/entry", "snapshots/nope/file", "snapshots/20260230T120000Z/file"):
            with self.subTest(key=bad):
                store = FakeStorage()
                store.versions = [{"Key": bad, "VersionId": "v", "DeleteMarker": False,
                                   "LastModified": "2026-01-01T00:00:00Z"}]
                with self.assertRaises(retention.RetentionError):
                    retention.inventory(store, bucket=BUCKET, expected_owner=OWNER, now=NOW)

    def test_malformed_time_or_naive_clock_fails_closed(self):
        store = FakeStorage()
        store.version(self.old, "v", last_modified=datetime(2020, 1, 1))  # noqa: DTZ001 - malformed input fixture
        with self.assertRaises(retention.RetentionError):
            retention.inventory(store, bucket=BUCKET, expected_owner=OWNER, now=NOW)
        with self.assertRaises(retention.RetentionError):
            retention.inventory(FakeStorage(), bucket=BUCKET, expected_owner=OWNER, now=datetime(2026, 1, 1))  # noqa: DTZ001 - naive clock fixture

    def test_missing_multipart_inventory_is_not_empty_success(self):
        self.storage.missing_upload_pages = True
        with self.assertRaises(retention.RetentionError):
            self.plan()

    def test_per_item_delete_error_never_reports_success(self):
        plan = self.plan()
        self.storage.delete_errors = [{"Key": key_at(self.old), "Code": "AccessDenied"}]
        with self.assertRaisesRegex(retention.RetentionError, "deletion failed"):
            self.apply(plan)

    def test_post_delete_inventory_must_prove_absence(self):
        plan = self.plan()
        self.storage.noop_delete = True
        with self.assertRaisesRegex(retention.RetentionError, "remains"):
            self.apply(plan)

    def test_abort_must_prove_parts_are_gone(self):
        plan = self.plan()
        self.storage.parts["u-old"] = [{"PartNumber": 1}]
        with self.assertRaisesRegex(retention.RetentionError, "parts remain"):
            self.apply(plan)

    def test_repeated_and_missing_continuation_tokens_fail(self):
        class BadPages(FakeStorage):
            def list_object_versions(self, **kwargs):
                return {"Versions": [], "DeleteMarkers": [], "IsTruncated": True,
                        "NextKeyMarker": "same", "NextVersionIdMarker": "same"}
        with self.assertRaisesRegex(retention.RetentionError, "repeated"):
            retention.inventory(BadPages(), bucket=BUCKET, expected_owner=OWNER,
                                now=NOW, limits=retention.Limits(pages=3))

        class MissingMarker(FakeStorage):
            def list_object_versions(self, **kwargs):
                return {"Versions": [], "DeleteMarkers": [], "IsTruncated": True}
        with self.assertRaisesRegex(retention.RetentionError, "lacks continuation"):
            retention.inventory(MissingMarker(), bucket=BUCKET, expected_owner=OWNER, now=NOW)

    def test_inventory_limit_refuses_instead_of_truncating(self):
        with self.assertRaisesRegex(retention.RetentionError, "version inventory limit"):
            retention.inventory(self.storage, bucket=BUCKET, expected_owner=OWNER, now=NOW,
                                limits=retention.Limits(versions=2))

    def test_delete_requests_are_bounded_to_one_thousand_exact_versions(self):
        store = FakeStorage()
        for number in range(1001):
            store.version(self.old, f"v-{number:04d}")
        plan = retention.inventory(store, bucket=BUCKET, expected_owner=OWNER, now=NOW)
        result = retention.apply(store, plan, approved_sha256=plan.sha256, bucket=BUCKET,
                                 expected_owner=OWNER, now=NOW)
        self.assertEqual(store.delete_batches, [1000, 1])
        self.assertEqual(result.deleted_versions, 1001)

    def test_changed_cutoff_requires_new_manual_plan(self):
        plan = self.plan()
        with self.assertRaisesRegex(retention.RetentionError, "cutoff changed"):
            self.apply(plan, NOW + timedelta(seconds=1))
        self.assertEqual(self.storage.deleted, [])

    def test_duplicate_key_version_pair_with_conflicting_kind_is_rejected(self):
        store = FakeStorage()
        store.version(self.old, "same")
        store.version(self.old, "same", marker=True)
        with self.assertRaisesRegex(retention.RetentionError, "duplicate key/version"):
            retention.inventory(store, bucket=BUCKET, expected_owner=OWNER, now=NOW)

    def test_plan_age_policy_cannot_be_rewritten_even_with_recomputed_hash(self):
        plan = self.plan()
        forged = retention.Plan(plan.bucket, plan.expected_owner, plan.prefix, plan.generated_at,
                                retention._iso(NOW - timedelta(days=2)), plan.multipart_cutoff,
                                plan.versions, plan.uploads, "")
        forged = retention.Plan(**{**forged.__dict__, "sha256": retention._plan_hash(forged)})
        with self.assertRaisesRegex(retention.RetentionError, "age policy"):
            retention.apply(self.storage, forged, approved_sha256=forged.sha256,
                            bucket=BUCKET, expected_owner=OWNER, now=NOW)


if __name__ == "__main__":
    unittest.main()
