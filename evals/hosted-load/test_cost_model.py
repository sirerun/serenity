import importlib.util
import json
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "hosted" / "cost_model.py"
spec = importlib.util.spec_from_file_location("cost_model", SCRIPT)
cost_model = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cost_model)


class RateTableTests(unittest.TestCase):
    def test_every_named_rate_has_a_kind_and_is_never_silently_measured(self):
        for name, entry in cost_model.RATE_TABLE.items():
            self.assertIn("kind", entry, name)
            self.assertIn(entry["kind"], ("assumption_recollection", "constant"), name)

    def test_unknown_rates_have_explicit_reasons_not_omitted(self):
        for name, reason in cost_model.UNKNOWN_RATES.items():
            self.assertTrue(reason)


class AccountTotalsTests(unittest.TestCase):
    def test_idle_scenario_has_zero_accounts_and_zero_usage(self):
        totals = cost_model.account_totals({}, 0.0)
        self.assertEqual(totals["accounts"], 0)
        self.assertEqual(totals["storage_bytes"], 0.0)

    def test_full_limit_mix_totals_ninety_thousand_facts_equivalent_recalls_scale_with_plan(self):
        totals = cost_model.account_totals({"free": 10, "builder": 3, "scale": 1}, 1.0)
        self.assertEqual(totals["accounts"], 14)
        # 10*100MB + 3*1GB + 1*5GB = 9 GB customer storage at full limit.
        self.assertAlmostEqual(totals["storage_bytes"], 9_000_000_000, delta=1)

    def test_usage_fraction_scales_linearly(self):
        full = cost_model.account_totals({"builder": 1}, 1.0)
        half = cost_model.account_totals({"builder": 1}, 0.5)
        self.assertAlmostEqual(half["recalls"], full["recalls"] / 2)


class PriceScenarioTests(unittest.TestCase):
    def test_full_snapshot_amplification_multiplies_storage_not_flat(self):
        """Real behavior check, not a canned number: doubling the customer data
        size must roughly double the S3 hourly-full-snapshot cost, proving the
        720x-versions-in-flight model actually reads scenario size."""
        small = cost_model.price_scenario("s", {"mix": {"free": 1}, "usage_fraction": 1.0, "assumption": "t"}, None)
        big = cost_model.price_scenario("b", {"mix": {"free": 10}, "usage_fraction": 1.0, "assumption": "t"}, None)
        small_s3 = small["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["value"]
        big_s3 = big["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["value"]
        self.assertGreater(big_s3, small_s3 * 5)

    def test_idle_scenario_has_no_storage_risk(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertFalse(idle["storage_risk"]["exceeds_usable_data_volume"])

    def test_idle_cheaper_than_full_limit_mix(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        full = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        self.assertLess(idle["known_subtotal_usd"], full["known_subtotal_usd"])

    def test_embeddings_reported_as_unknown_not_zero(self):
        full = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        self.assertIsNone(full["unknown_categories"]["embeddings"]["monthly_usd"])
        self.assertGreater(full["unknown_categories"]["embeddings"]["tokens_estimate"], 0)

    def test_measurements_override_assumption_and_are_tagged_measured(self):
        without = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        with_m = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], {"snapshot_size_bytes": 1_000_000})
        self.assertEqual(without["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["kind"], "assumption_recollection")
        self.assertEqual(with_m["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["kind"], "measured")
        self.assertNotEqual(without["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["value"], with_m["known_categories_usd"]["s3_storage_hourly_full_snapshot"]["value"])


class CLITests(unittest.TestCase):
    def test_blocked_on_missing_manifest(self):
        import sys
        argv = sys.argv
        sys.argv = ["cost_model.py", "--manifest", "/nonexistent/manifest.json", "--output", "/tmp/out.json"]
        try:
            code = cost_model.main()
        finally:
            sys.argv = argv
        self.assertEqual(code, 2)

    def test_full_run_writes_valid_json_and_flags_ceiling(self):
        import sys
        import tempfile
        manifest_path = Path(__file__).resolve().parents[2] / "docs" / "launch" / "evidence" / "T23.60" / "manifest.json"
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "cost.json"
            argv = sys.argv
            sys.argv = ["cost_model.py", "--manifest", str(manifest_path), "--output", str(out)]
            try:
                code = cost_model.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 0)
            data = json.loads(out.read_text())
            self.assertEqual(data["task_id"], "T23.60")
            self.assertIn("full_limit_mix", data["scenarios"])
            self.assertIn("citation_limitation", data)


if __name__ == "__main__":
    unittest.main()
