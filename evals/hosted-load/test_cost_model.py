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
            self.assertIn(entry["kind"], ("published", "constant"), name)

    def test_every_published_rate_carries_a_source_citation(self):
        for name, entry in cost_model.RATE_TABLE.items():
            if entry["kind"] == "published":
                self.assertIn("source", entry, name)
                self.assertTrue(entry["source"], name)

    def test_unknown_rates_have_explicit_reasons_not_omitted(self):
        for name, reason in cost_model.UNKNOWN_RATES.items():
            self.assertTrue(reason)

    def test_t4g_burst_credit_matches_official_rate_not_the_old_wrong_recollection(self):
        """Regression: an earlier PARTIAL receipt used an unverified $0.05/vCPU-hour
        recollection; the coordinator's review flagged the official rate as $0.04."""
        self.assertEqual(cost_model.rate("ec2_t4g_burst_credit_usd_per_vcpu_hour"), 0.04)

    def test_s3_rate_is_the_us_west_2_figure_not_us_east_1(self):
        self.assertEqual(cost_model.rate("s3_standard_usd_per_gb_month"), 0.023)


class AccountTotalsTests(unittest.TestCase):
    def test_idle_scenario_has_zero_accounts_and_zero_usage(self):
        totals, per_plan = cost_model.account_totals({}, 0.0)
        self.assertEqual(totals["accounts"], 0)
        self.assertEqual(totals["storage_bytes"], 0.0)
        self.assertEqual(per_plan, {})

    def test_full_limit_mix_totals_nine_gb_storage(self):
        totals, per_plan = cost_model.account_totals({"free": 10, "builder": 3, "scale": 1}, 1.0)
        self.assertEqual(totals["accounts"], 14)
        # 10*100MB + 3*1GB + 1*5GB = 9 GB customer storage at full limit.
        self.assertAlmostEqual(totals["storage_bytes"], 9_000_000_000, delta=1)
        self.assertEqual(set(per_plan), {"free", "builder", "scale"})
        self.assertEqual(per_plan["scale"]["accounts"], 1)

    def test_usage_fraction_scales_linearly(self):
        full, _ = cost_model.account_totals({"builder": 1}, 1.0)
        half, _ = cost_model.account_totals({"builder": 1}, 0.5)
        self.assertAlmostEqual(half["recalls"], full["recalls"] / 2)


class BackupRetentionMathTests(unittest.TestCase):
    """Regression coverage for the coordinator's correction: deploy/hosted/backup.sh
    writes a unique timestamped key prefix every run (never the same key twice), so
    S3's per-key noncurrent-version amplification does not apply. The real driver is
    Expiration(30d) then NoncurrentVersionExpiration(30d) in series -- roughly a
    60-day object lifetime, ~1440 retained hourly backup-sets at steady state, not
    ~720 same-key versions."""

    def test_retained_backup_sets_is_about_sixty_days_not_thirty(self):
        self.assertEqual(cost_model.S3_OBJECT_LIFETIME_DAYS, 60)
        self.assertEqual(cost_model.RETAINED_BACKUP_SETS, 60 * 24 + 1)
        self.assertGreater(cost_model.RETAINED_BACKUP_SETS, 1400)
        self.assertLess(cost_model.RETAINED_BACKUP_SETS, 1500)

    def test_objects_per_backup_counts_manifest_control_db_complete_and_bundles(self):
        scenario = cost_model.price_scenario("t", {"mix": {"free": 2}, "usage_fraction": 1.0, "assumption": "t"}, None)
        # 2 accounts -> manifest.json + control.db + COMPLETE + 2 bundles = 5.
        self.assertEqual(scenario["known_categories_usd"]["s3_storage_backups"]["objects_per_backup"], 5)


class PriceScenarioTests(unittest.TestCase):
    def test_storage_amplification_scales_with_customer_data_not_flat(self):
        """Real behavior check, not a canned number: doubling the customer data
        size must roughly double the S3 backup storage cost, proving the model
        actually reads scenario size rather than returning a constant."""
        small = cost_model.price_scenario("s", {"mix": {"free": 1}, "usage_fraction": 1.0, "assumption": "t"}, None)
        big = cost_model.price_scenario("b", {"mix": {"free": 10}, "usage_fraction": 1.0, "assumption": "t"}, None)
        small_s3 = small["known_categories_usd"]["s3_storage_backups"]["value"]
        big_s3 = big["known_categories_usd"]["s3_storage_backups"]["value"]
        self.assertGreater(big_s3, small_s3 * 5)

    def test_idle_scenario_has_no_storage_risk(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertFalse(idle["storage_risk"]["exceeds_usable_data_volume"])

    def test_idle_cheaper_than_full_limit_mix(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        full = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        self.assertLess(idle["known_subtotal_usd"], full["known_subtotal_usd"])

    def test_embeddings_reported_as_unknown_not_zero_and_includes_query_and_readiness_components(self):
        full = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        embed = full["unknown_categories"]["embeddings"]
        self.assertIsNone(embed["monthly_usd"])
        self.assertGreater(embed["write_tokens"]["value"], 0)
        self.assertGreater(embed["query_tokens"]["value"], 0)
        self.assertGreater(embed["readiness_tokens"]["value"], 0)
        self.assertEqual(embed["total_tokens"], embed["write_tokens"]["value"] + embed["query_tokens"]["value"] + embed["rebuild_reembed_tokens"]["value"] + embed["readiness_tokens"]["value"])

    def test_idle_scenario_has_zero_readiness_tokens_is_false_readiness_runs_regardless_of_accounts(self):
        """Readiness probing is a fixed operational cadence, not proportional to
        account count -- it must be nonzero even at 0 accounts."""
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertGreater(idle["unknown_categories"]["embeddings"]["readiness_tokens"]["value"], 0)

    def test_kms_requests_free_tier_absorbs_light_scenarios(self):
        ten = cost_model.price_scenario("t", cost_model.SCENARIOS["10_accounts_light"], None)
        self.assertEqual(ten["known_categories_usd"]["kms_requests"]["value"], 0.0)

    def test_data_transfer_free_tier_absorbs_all_scenarios_at_these_volumes(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS["full_limit_mix"], None)
        self.assertEqual(full["known_categories_usd"]["data_transfer_out"]["value"], 0.0)

    def test_email_within_resend_free_tier_at_full_limit_mix(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS["full_limit_mix"], None)
        self.assertTrue(full["known_categories_usd"]["email"]["within_free_tier"])
        self.assertEqual(full["known_categories_usd"]["email"]["value"], 0.0)

    def test_per_plan_contribution_apportions_variable_costs_by_storage_share(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS["full_limit_mix"], None)
        by_plan = full["per_plan_contribution"]["by_plan"]
        self.assertEqual(set(by_plan), {"free", "builder", "scale"})
        total = sum(p["variable_cost_usd"] for p in by_plan.values())
        self.assertAlmostEqual(total, full["per_plan_contribution"]["variable_total_usd"], places=2)
        # Scale has the largest single-account storage share (5 GB of 9 GB total),
        # so it must carry more variable cost than the 3-Builder or 10-Free groups.
        self.assertGreater(by_plan["scale"]["variable_cost_usd"], by_plan["free"]["variable_cost_usd"])

    def test_per_plan_contribution_empty_for_idle(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertEqual(idle["per_plan_contribution"]["by_plan"], {})

    def test_measurements_override_assumption_and_are_tagged_measured(self):
        without = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], None)
        with_m = cost_model.price_scenario("full", cost_model.SCENARIOS["full_limit_mix"], {"snapshot_size_bytes": 1_000_000})
        self.assertEqual(without["known_categories_usd"]["s3_storage_backups"]["kind"], "assumption")
        self.assertEqual(with_m["known_categories_usd"]["s3_storage_backups"]["kind"], "measured")
        self.assertNotEqual(without["known_categories_usd"]["s3_storage_backups"]["value"], with_m["known_categories_usd"]["s3_storage_backups"]["value"])


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

    def test_blocked_not_crashed_on_invalid_json_manifest(self):
        import sys
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            bad = Path(tmp) / "manifest.json"
            bad.write_text("{not valid json")
            argv = sys.argv
            sys.argv = ["cost_model.py", "--manifest", str(bad), "--output", str(Path(tmp) / "out.json")]
            try:
                code = cost_model.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 2)

    def test_blocked_not_crashed_on_invalid_json_measurements(self):
        import sys
        import tempfile
        manifest_path = Path(__file__).resolve().parents[2] / "docs" / "launch" / "evidence" / "T23.60" / "manifest.json"
        with tempfile.TemporaryDirectory() as tmp:
            bad = Path(tmp) / "measurements.json"
            bad.write_text("[not json")
            argv = sys.argv
            sys.argv = ["cost_model.py", "--manifest", str(manifest_path), "--measurements", str(bad), "--output", str(Path(tmp) / "out.json")]
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
            self.assertIn("citation_note", data)
            self.assertTrue(data["scenarios"]["full_limit_mix"]["known_subtotal_usd"] > 60.0)


if __name__ == "__main__":
    unittest.main()
