import json
import unittest
from pathlib import Path

import harness

WORKLOAD_PATH = Path(__file__).with_name("workload.json")


class WorkloadLoadingTests(unittest.TestCase):
    def test_loads_frozen_workload(self):
        workload = harness.load_workload(WORKLOAD_PATH)
        self.assertEqual(workload["repetitions"], 3)
        self.assertEqual(workload["concurrency"]["clients"], 8)

    def test_rejects_missing_field(self):
        bad = {"phases": [], "concurrency": {}, "traffic_mix": {"recall": 1.0}, "cardinalities": {}, "thresholds": {}}
        tmp = Path(__file__).with_name("_tmp_bad_workload.json")
        tmp.write_text(json.dumps(bad))
        try:
            with self.assertRaises(ValueError):
                harness.load_workload(tmp)
        finally:
            tmp.unlink()

    def test_rejects_traffic_mix_not_summing_to_one(self):
        bad = json.loads(WORKLOAD_PATH.read_text())
        bad["traffic_mix"] = {"recall": 0.5, "remember": 0.1, "forget": 0.1}
        tmp = Path(__file__).with_name("_tmp_bad_mix.json")
        tmp.write_text(json.dumps(bad))
        try:
            with self.assertRaises(ValueError):
                harness.load_workload(tmp)
        finally:
            tmp.unlink()


class BuildAccountsTests(unittest.TestCase):
    def test_full_limit_mix_has_14_accounts(self):
        workload = harness.load_workload(WORKLOAD_PATH)
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        self.assertEqual(len(accounts), 14)
        self.assertEqual(sum(1 for a in accounts if a["plan"] == "free"), 10)
        self.assertEqual(sum(1 for a in accounts if a["plan"] == "builder"), 3)
        self.assertEqual(sum(1 for a in accounts if a["plan"] == "scale"), 1)

    def test_unknown_profile_raises(self):
        workload = harness.load_workload(WORKLOAD_PATH)
        with self.assertRaises(ValueError):
            harness.build_accounts(workload["cardinalities"], "nonexistent_profile")

    def test_unknown_plan_id_raises(self):
        with self.assertRaises(ValueError):
            harness.build_accounts({"paid": {"accounts": {"enterprise": 1}}}, "paid")


class RateLimiterTests(unittest.TestCase):
    def test_allows_up_to_limit_then_rejects_within_same_minute(self):
        limiter = harness.RateLimiter(limit=3)
        results = [limiter.allow("acct-1", now_s=t) for t in (0.0, 1.0, 2.0, 3.0)]
        self.assertEqual(results, [True, True, True, False])

    def test_resets_in_next_minute_window(self):
        limiter = harness.RateLimiter(limit=1)
        self.assertTrue(limiter.allow("acct-1", now_s=0.0))
        self.assertFalse(limiter.allow("acct-1", now_s=1.0))
        self.assertTrue(limiter.allow("acct-1", now_s=61.0))


class PoolAdmissionTests(unittest.TestCase):
    def test_admits_first_open_as_cold(self):
        pool = harness.Pool(max_open=8, max_in_flight=16, idle_timeout_s=600)
        admitted, cold = pool.acquire("brain-a", now_s=0.0)
        self.assertTrue(admitted)
        self.assertTrue(cold)

    def test_reacquiring_same_open_brain_is_warm(self):
        pool = harness.Pool(max_open=8, max_in_flight=16, idle_timeout_s=600)
        pool.acquire("brain-a", now_s=0.0)
        pool.release("brain-a", now_s=0.5)
        admitted, cold = pool.acquire("brain-a", now_s=1.0)
        self.assertTrue(admitted)
        self.assertFalse(cold)

    def test_in_flight_cap_rejects_immediately_real_sensitivity(self):
        """Real behavior change under a tighter cap, not a canned pass: proves the
        admission model actually responds to MaxInFlight rather than always admitting."""
        loose = harness.Pool(max_open=8, max_in_flight=16, idle_timeout_s=600)
        tight = harness.Pool(max_open=8, max_in_flight=1, idle_timeout_s=600)
        loose.acquire("brain-a", now_s=0.0)
        tight.acquire("brain-a", now_s=0.0)
        loose_admitted, _ = loose.acquire("brain-b", now_s=0.0)
        tight_admitted, _ = tight.acquire("brain-b", now_s=0.0)
        self.assertTrue(loose_admitted)
        self.assertFalse(tight_admitted)

    def test_max_open_evicts_oldest_idle_brain(self):
        pool = harness.Pool(max_open=1, max_in_flight=16, idle_timeout_s=600)
        pool.acquire("brain-a", now_s=0.0)
        pool.release("brain-a", now_s=0.5)
        admitted, cold = pool.acquire("brain-b", now_s=1.0)
        self.assertTrue(admitted)
        self.assertTrue(cold)
        self.assertNotIn("brain-a", pool.open_brains)

    def test_max_open_rejects_when_no_idle_brain_to_evict(self):
        pool = harness.Pool(max_open=1, max_in_flight=16, idle_timeout_s=600)
        pool.acquire("brain-a", now_s=0.0)  # still in flight, users=1 equivalent (no release)
        admitted, _ = pool.acquire("brain-b", now_s=1.0)
        self.assertFalse(admitted)


class PercentileTests(unittest.TestCase):
    def test_known_values(self):
        values = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
        self.assertAlmostEqual(harness.percentile(values, 50), 5.5, places=6)
        self.assertEqual(harness.percentile(values, 0), 1)
        self.assertEqual(harness.percentile(values, 100), 10)

    def test_empty_list_is_zero(self):
        self.assertEqual(harness.percentile([], 95), 0.0)


class ArrivalDeterminismTests(unittest.TestCase):
    def test_same_seed_same_schedule(self):
        import random
        workload = harness.load_workload(WORKLOAD_PATH)
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        a = harness.generate_arrivals(workload, accounts, random.Random(42))
        b = harness.generate_arrivals(workload, accounts, random.Random(42))
        self.assertEqual(a, b)

    def test_different_seed_differs(self):
        import random
        workload = harness.load_workload(WORKLOAD_PATH)
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        a = harness.generate_arrivals(workload, accounts, random.Random(1))
        b = harness.generate_arrivals(workload, accounts, random.Random(2))
        self.assertNotEqual(a, b)

    def test_arrivals_are_nonempty_and_within_phase_window(self):
        import random
        workload = harness.load_workload(WORKLOAD_PATH)
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(7))
        total_s = sum(p["minutes"] for p in workload["phases"]) * 60
        self.assertGreater(len(arrivals), 1000)
        self.assertTrue(all(0 <= a["t"] <= total_s for a in arrivals))


class RunRepetitionTests(unittest.TestCase):
    def setUp(self):
        self.workload = harness.load_workload(WORKLOAD_PATH)
        self.accounts = harness.build_accounts(self.workload["cardinalities"], "paid")

    def test_replay_determinism_same_seed_identical_outcomes(self):
        r1 = harness.run_repetition(self.workload, self.accounts, seed=123)
        r2 = harness.run_repetition(self.workload, self.accounts, seed=123)
        self.assertEqual(r1["total_offered"], r2["total_offered"])
        self.assertEqual(r1["outcomes"], r2["outcomes"])

    def test_rejections_counted_in_total_outcomes_not_only_admitted_latencies(self):
        r = harness.run_repetition(self.workload, self.accounts, seed=5, saturation=True)
        total_from_outcomes = sum(r["outcomes"].values())
        self.assertEqual(total_from_outcomes, r["total_offered"])
        self.assertGreater(r["outcomes"].get("rejected_capacity", 0), 0)

    def test_saturation_run_has_materially_higher_rejection_than_main_run(self):
        """Red/green sensitivity: a vacuous model would report similar rejection
        regardless of capacity. The saturation pool (max_open=1, max_in_flight=2)
        must reject far more than the production-shaped main pool (8/16)."""
        main = harness.run_repetition(self.workload, self.accounts, seed=99, saturation=False)
        saturation = harness.run_repetition(self.workload, self.accounts, seed=99, saturation=True)
        self.assertGreater(saturation["admission_rejection_pct"], main["admission_rejection_pct"])

    def test_quota_boundary_saturation_kept_separate_from_main_completion_stat(self):
        main = harness.run_repetition(self.workload, self.accounts, seed=11, saturation=False)
        self.assertIn("saturation", main)
        self.assertFalse(main["saturation"])


class ThresholdEvaluationTests(unittest.TestCase):
    def setUp(self):
        self.workload = harness.load_workload(WORKLOAD_PATH)

    def _run(self, recall_p95, remember_p95, completion_pct, unexpected_5xx_pct, rejection_pct, cold_p95):
        return {
            "latency_by_verb_s": {"recall": {"p95": recall_p95}, "remember": {"p95": remember_p95}},
            "completion_pct": completion_pct,
            "unexpected_5xx_pct": unexpected_5xx_pct,
            "admission_rejection_pct": rejection_pct,
            "cold_open_p95_s": cold_p95,
        }

    def test_within_thresholds_passes(self):
        good = self._run(recall_p95=1.0, remember_p95=1.5, completion_pct=99.5, unexpected_5xx_pct=0.0, rejection_pct=0.2, cold_p95=10.0)
        checks = harness.evaluate_thresholds(self.workload, good)
        self.assertTrue(checks["recall_p95_max_s"]["pass"])
        self.assertTrue(checks["remember_p95_max_s"]["pass"])
        self.assertTrue(checks["min_offered_completion_pct"]["pass"])

    def test_exceeding_thresholds_fails_real_sensitivity(self):
        """Same evaluator, worse inputs: proves pass/fail actually tracks the
        observed numbers rather than always returning True."""
        bad = self._run(recall_p95=5.0, remember_p95=9.0, completion_pct=50.0, unexpected_5xx_pct=5.0, rejection_pct=20.0, cold_p95=120.0)
        checks = harness.evaluate_thresholds(self.workload, bad)
        self.assertFalse(checks["recall_p95_max_s"]["pass"])
        self.assertFalse(checks["remember_p95_max_s"]["pass"])
        self.assertFalse(checks["min_offered_completion_pct"]["pass"])
        self.assertFalse(checks["unexpected_5xx_max_pct"]["pass"])
        self.assertFalse(checks["unexpected_admission_rejection_max_pct"]["pass"])
        self.assertFalse(checks["cold_ready_max_s"]["pass"])

    def test_resource_thresholds_marked_unmeasured_in_fixtures(self):
        checks = harness.evaluate_thresholds(self.workload, self._run(1, 1, 100, 0, 0, 1))
        self.assertIsNone(checks["cpu_max_pct"]["pass"])
        self.assertIsNone(checks["rss_max_pct_of_ram"]["pass"])
        self.assertIsNone(checks["disk_max_pct"]["pass"])


if __name__ == "__main__":
    unittest.main()
