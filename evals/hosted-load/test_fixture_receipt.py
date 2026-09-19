"""Tests for fixture_receipt.py (T23.60). Synthetic reports only: no fixture, service or provider is touched."""
from __future__ import annotations

import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import fixture_receipt as fr  # noqa: E402

BIN = {"prepare": "b" * 64, "verify": "c" * 64}
PLANS = {"scale": (1, [5000] * 10, 50000, 5_000_000_000), "builder": (3, [3334, 3333, 3333], 10000, 1_000_000_000), "free": (10, [1000], 1000, 100_000_000)}


def report(profile: str, per_brain: int | None, commits: int, git_bytes: int, *, reduced: bool | None = None, passed: bool = True, dim: int = 64, index_bytes: int = 10_000_000, commit_every: int = 500) -> dict:
    """A minimal verification report. per_brain None means the full plan split."""
    reduced = (profile == "smoke") if reduced is None else reduced
    per_block = per_brain
    accounts, brains = [], []
    facts = 0
    for plan, (count, split, cap, quota) in PLANS.items():
        for i in range(count):
            mem = 0
            for idx, n in enumerate(split):
                n = per_brain if per_brain is not None else n
                brains.append({"label": f"{plan}-{i}", "index": idx, "canonical_facts": n})
                mem += n
            facts += mem
            accounts.append({"label": f"{plan}-{i}", "plan_id": plan, "memories": mem, "memory_cap": cap, "at_or_over_memory_cap": mem >= cap, "storage_bytes": 1_000_000, "storage_quota_bytes": quota, "storage_share_of_quota": 1_000_000 / quota, "at_or_over_storage_quota": False,
                             "gateway_inventory_memories": mem, "gateway_inventory_storage_bytes": 1_000_000, "gateway_entitlement_plan": plan, "gateway_limit_inputs_refuse_a_nonreplay_remember": mem >= cap})
    total = 10_000_000 + index_bytes + git_bytes
    return {
        "embedder_pin": f"fixture-hash-embedder-d{dim}@infrastructure-only-v1", "embedder_dim": dim, "commit_every": commit_every,
        "fixture_content_sha256": (profile + str(per_block)).encode().hex()[:64].ljust(64, "0"), "control_db_sha256": "1" * 64, "marker_sha256": "2" * 64, "workload_sha256": "3" * 64,
        "schema": fr.REPORT_SCHEMA, "expect": profile, "marker_profile": profile, "reduced": reduced, "pass": passed,
        "satisfies_full_cardinality": profile == "full" and passed and not reduced,
        "totals": {"accounts": 14, "brains": 29, "canonical_memory_facts": facts, "index_fact_chunks": facts, "index_vectors_under_pin": facts, "git_commits_all_brains": commits},
        "accounts": accounts, "brains": brains,
        "checks": [{"id": "x", "pass": passed}],
        "storage_and_history_gap": {"storage_bytes_all_brains": total, "allocated_bytes_all_brains": total * 3, "files_all_brains": 10, "storage_bytes_by_component": {".git": git_bytes, ".serenity": index_bytes, "brain": 8_000_000, "other": 2_000_000 - git_bytes if git_bytes < 2_000_000 else 0}, "accounts_at_or_over_storage_quota": 0},
        "_sha256": "0" * 64,
    }


def inputs():
    return report("full", None, 300, 40_000_000), report("smoke", 10, 58, 1_000_000), report("smoke", 100, 58, 5_000_000), report("smoke", 100, 2929, 9_900_000, commit_every=1)


class BuildTests(unittest.TestCase):
    def test_full_and_smoke_are_labeled_apart_and_the_smoke_is_never_full(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        self.assertEqual(r["full"]["totals"]["canonical_memory_facts"], 90000)
        self.assertTrue(r["full"]["satisfies_full_cardinality"])
        self.assertTrue(r["smoke"]["reduced"])
        self.assertFalse(r["smoke"]["satisfies_full_cardinality"])
        self.assertEqual(r["smoke"]["totals"]["canonical_memory_facts"], 290)
        text = json.dumps(r["labels"])
        self.assertIn("unqualified", text)
        self.assertIn("never satisfies", text)

    def test_client_reach_counts_default_brains_only(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        self.assertEqual(r["client_reach"]["facts_on_default_brains"], 5000 + 3 * 3334 + 10 * 1000)
        self.assertEqual(r["client_reach"]["facts_on_default_brains"], 25002)
        self.assertEqual(r["client_reach"]["facts_on_other_brains"], 64998)

    def test_every_full_account_is_at_the_cap_and_none_at_the_storage_quota(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        self.assertEqual(r["at_the_memory_cap"]["accounts_at_cap"], 14)
        self.assertIn("limit_exceeded", r["at_the_memory_cap"]["consequence"])
        self.assertEqual(r["full"]["storage_and_history_gap"]["accounts_at_or_over_storage_quota"], 0)
        for plan in r["full"]["by_plan"].values():
            self.assertEqual(plan["accounts_at_memory_cap"], plan["accounts"])
            self.assertLess(plan["largest_share_of_storage_quota"], 1)

    def test_gateway_cross_check_is_reported_and_the_full_fixture_is_refused_at_the_cap(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        g = r["full"]["gateway_cross_check"]
        self.assertEqual(g, {"accounts": 14, "inventory_memories_equal_verifier": 14, "inventory_storage_bytes_equal_verifier": 14, "entitlement_equals_planned_plan": 14, "limit_inputs_refuse_a_nonreplay_remember": 14})
        self.assertEqual(r["smoke"]["gateway_cross_check"]["limit_inputs_refuse_a_nonreplay_remember"], 0)
        self.assertEqual(r["at_the_memory_cap"]["accounts_whose_gateway_limit_inputs_refuse_a_remember"], 14)

    def test_fixture_identity_is_recorded_per_profile(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        for name in ("full", "smoke"):
            ident = r[name]["fixture_identity"]
            self.assertEqual(len(ident["content_sha256"]), 64)
            self.assertEqual(ident["workload_sha256"], "3" * 64)
        self.assertNotEqual(r["full"]["fixture_identity"]["content_sha256"], r["smoke"]["fixture_identity"]["content_sha256"])

    def test_history_sample_arithmetic(self):
        r = fr.build(*inputs(), {}, "a" * 40, BIN)
        h = r["storage_and_history_gap"]["history_sample"]
        self.assertEqual(h["extra_git_bytes"], 4_900_000)
        self.assertEqual(h["commits_per_write"] - h["commits_batched"], 2871)
        self.assertEqual(h["extra_git_bytes_per_extra_commit"], round(4_900_000 / 2871, 1))
        self.assertIn("not an estimate for 5,000-fact brains", h["scope"])
        self.assertIn("unmeasured", r["storage_and_history_gap"]["history_is_batched"])

    def test_refuses_inputs_that_would_overstate_the_fixture(self):
        full, smoke, b, p = inputs()
        bad_full = copy.deepcopy(full)
        bad_full["totals"]["canonical_memory_facts"] = 89999
        with self.assertRaises(fr.ReceiptError):
            fr.build(bad_full, smoke, b, p, {}, "a" * 40, BIN)
        bad_smoke = copy.deepcopy(smoke)
        bad_smoke["satisfies_full_cardinality"] = True
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, bad_smoke, b, p, {}, "a" * 40, BIN)
        reduced_full = copy.deepcopy(full)
        reduced_full["reduced"] = True
        with self.assertRaises(fr.ReceiptError):
            fr.build(reduced_full, smoke, b, p, {}, "a" * 40, BIN)
        mismatch = copy.deepcopy(p)
        mismatch["totals"]["canonical_memory_facts"] = 2899
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, mismatch, {}, "a" * 40, BIN)
        same_commits = copy.deepcopy(p)
        same_commits["totals"]["git_commits_all_brains"] = b["totals"]["git_commits_all_brains"]
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, same_commits, {}, "a" * 40, BIN)


class HistoryAtScaleTests(unittest.TestCase):
    def test_the_per_write_full_fixture_measures_history_at_scale(self):
        full, smoke, b, p = inputs()
        per_write = report("full", None, 90_029, 400_000_000, commit_every=1)
        r = fr.build(full, smoke, b, p, {}, "a" * 40, BIN, per_write)
        h = r["storage_and_history_gap"]["history_at_scale"]
        self.assertEqual(h["facts"], 90000)
        self.assertEqual(h["commits_per_write"] - h["commits_batched"], 90_029 - 300)
        self.assertEqual(h["extra_git_bytes"], 360_000_000)
        self.assertEqual(h["extra_git_bytes_per_extra_commit"], round(360_000_000 / (90_029 - 300), 1))
        self.assertEqual(h["accounts_at_or_over_storage_quota_per_write"], 0)
        self.assertIn("not a production measurement", h["scope"])

    def test_it_refuses_two_full_fixtures_with_different_content(self):
        full, smoke, b, p = inputs()
        other = report("full", None, 90_029, 400_000_000, commit_every=1)
        other["fixture_content_sha256"] = "f" * 64
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, other)

    def test_it_refuses_a_per_write_fixture_that_batches_or_a_different_vector_width(self):
        full, smoke, b, p = inputs()
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, report("full", None, 90_029, 400_000_000, commit_every=500))
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, report("full", None, 90_029, 400_000_000, commit_every=1, dim=128))

    def test_it_is_absent_when_no_per_write_full_report_is_given(self):
        self.assertIsNone(fr.build(*inputs(), {}, "a" * 40, BIN)["storage_and_history_gap"]["history_at_scale"])

    def test_it_refuses_a_reduced_or_not_longer_history(self):
        full, smoke, b, p = inputs()
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, report("full", None, 300, 40_000_000))
        reduced = report("full", None, 90_029, 400_000_000, commit_every=1)
        reduced["reduced"] = True
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, reduced)


class VectorWidthTests(unittest.TestCase):
    def test_wide_vectors_measure_the_index_growth_per_float(self):
        full, smoke, b, p = inputs()
        wide = report("full", None, 300, 40_000_000, dim=1536, index_bytes=10_000_000 + 90_000 * 1472 * 4)
        r = fr.build(full, smoke, b, p, {}, "a" * 40, BIN, None, wide)
        w = r["storage_and_history_gap"]["vector_width_at_scale"]
        self.assertEqual((w["narrow_dim"], w["wide_dim"]), (64, 1536))
        self.assertEqual(w["extra_index_bytes"], 90_000 * 1472 * 4)
        self.assertEqual(w["extra_index_bytes_per_fact_per_extra_float"], 4.0)
        self.assertIn("not the pinned provider's width", w["scope"])
        self.assertIn("d64", r["labels"]["provider_quality"])

    def test_it_refuses_a_wide_fixture_with_a_different_commit_density(self):
        full, smoke, b, p = inputs()
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, None, report("full", None, 90_029, 400_000_000, dim=1536, commit_every=1))

    def test_it_refuses_a_narrower_or_different_content_fixture(self):
        full, smoke, b, p = inputs()
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, None, report("full", None, 300, 40_000_000, dim=64))
        other = report("full", None, 300, 40_000_000, dim=1536)
        other["fixture_content_sha256"] = "e" * 64
        with self.assertRaises(fr.ReceiptError):
            fr.build(full, smoke, b, p, {}, "a" * 40, BIN, None, other)


class CLITests(unittest.TestCase):
    def write(self, d: Path, name: str, data: dict) -> Path:
        path = d / name
        data = {k: v for k, v in data.items() if k != "_sha256"}
        path.write_text(json.dumps(data))
        return path

    def run_cli(self, d: Path, full, smoke, b, p, out: Path):
        args = [sys.executable, str(HERE / "fixture_receipt.py"), "--full", str(self.write(d, "f.json", full)), "--smoke", str(self.write(d, "s.json", smoke)),
                "--history-batched", str(self.write(d, "b.json", b)), "--history-per-write", str(self.write(d, "p.json", p)), "--source-sha", "a" * 40, "--prepare-binary-sha256", "b" * 64, "--verify-binary-sha256", "c" * 64, "--output", str(out)]
        return subprocess.run(args, capture_output=True, text=True)

    def test_writes_a_deterministic_receipt(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            first, second = d / "one.json", d / "two.json"
            self.assertEqual(self.run_cli(d, *inputs(), first).returncode, 0)
            self.assertEqual(self.run_cli(d, *inputs(), second).returncode, 0)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            self.assertEqual(json.loads(first.read_text())["task"], "T23.60")

    def test_a_failed_verification_blocks_and_writes_nothing(self):
        full, smoke, b, p = inputs()
        full = report("full", None, 300, 40_000_000, passed=False)
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            out = d / "receipt.json"
            proc = self.run_cli(d, full, smoke, b, p, out)
            self.assertEqual(proc.returncode, 2)
            self.assertIn("BLOCKED", proc.stderr)
            self.assertFalse(out.exists())


if __name__ == "__main__":
    unittest.main()
