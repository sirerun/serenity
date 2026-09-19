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

    def test_a_window_starts_at_the_first_request_not_on_a_wall_clock_minute(self):
        """internal/hosted/gateway/admission.go: `if now.Sub(b.start) >= time.Minute { b = bucket{start: now} }`.
        The wall-clock-minute model let a key that started at 45 s reset at 60 s."""
        limiter = harness.RateLimiter(limit=1)
        self.assertTrue(limiter.allow("k", now_s=45.0))
        self.assertFalse(limiter.allow("k", now_s=61.0))  # 16 s into the window: refused (the old model reset at 60)
        self.assertFalse(limiter.allow("k", now_s=104.999))
        self.assertTrue(limiter.allow("k", now_s=105.0))  # exactly 60 s after the window started

    def test_a_refused_request_neither_starts_nor_extends_a_window(self):
        limiter = harness.RateLimiter(limit=1)
        self.assertTrue(limiter.allow("k", now_s=0.0))
        for t in (10.0, 30.0, 59.0):
            self.assertFalse(limiter.allow("k", now_s=t))
        self.assertTrue(limiter.allow("k", now_s=60.0))  # not pushed out by the refusals at 10, 30 and 59

    def test_keys_have_independent_windows(self):
        limiter = harness.RateLimiter(limit=1)
        self.assertTrue(limiter.allow("a", now_s=0.0))
        self.assertTrue(limiter.allow("b", now_s=30.0))
        self.assertFalse(limiter.allow("a", now_s=59.0))
        self.assertTrue(limiter.allow("a", now_s=60.0))
        self.assertFalse(limiter.allow("b", now_s=60.0))
        self.assertTrue(limiter.allow("b", now_s=90.0))

    def test_the_production_limits_are_the_gateways(self):
        self.assertEqual((harness.ACCOUNT_RATE_LIMIT_PER_MIN, harness.IP_RATE_LIMIT_PER_MIN), (120, 600))  # gateway.go: allow("account:..", 120), allow("ip:..", 600)


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


class FactSizeBoundTests(unittest.TestCase):
    """F1. The gateway refuses a fact over 4096 bytes (gateway.go, len(input.Fact) > 4096)."""

    def test_the_vocabulary_is_short_enough_for_the_largest_frozen_fact(self):
        workload = harness.load_workload(WORKLOAD_PATH)
        top = workload["fact_tokens"]["max"]
        self.assertLessEqual(max(len(w) for w in harness._TEXT_VOCAB), 7)
        self.assertLessEqual(harness.max_rendered_bytes(top), harness.FACT_MAX_BYTES)
        self.assertEqual(harness.FACT_MAX_BYTES, 4096)

    def test_the_text_is_exactly_the_requested_number_of_words_never_truncated(self):
        for tokens in (1, 32, 200, 512):
            for seed in range(50):
                text = harness.render_text(f"s{seed}", tokens)
                self.assertEqual(len(text.split()), tokens)
                self.assertLessEqual(len(text.encode("utf-8")), harness.max_rendered_bytes(tokens))

    def test_the_vocabulary_has_no_duplicate_or_non_ascii_words(self):
        self.assertEqual(len(set(harness._TEXT_VOCAB)), len(harness._TEXT_VOCAB))
        self.assertTrue(all(w.isascii() and w.isalpha() and w == w.lower() for w in harness._TEXT_VOCAB))

    def test_word_counts_are_documented_as_nominal(self):
        self.assertIn("not a token count", harness.render_text.__doc__)
        self.assertIn("no tokenizer is assumed", harness.render_text.__doc__)


class RenderTextBoundTests(unittest.TestCase):
    def test_max_rendered_bytes_bounds_every_seed_and_is_tight_for_the_longest_word(self):
        for tokens in (1, 2, 20, 128, 512):
            bound = harness.max_rendered_bytes(tokens)
            for seed in range(200):
                self.assertLessEqual(len(harness.render_text(f"seed-{seed}", tokens).encode("utf-8")), bound)
        longest = max(len(w) for w in harness._TEXT_VOCAB)
        self.assertEqual(harness.max_rendered_bytes(3), 3 * longest + 2)

    def test_a_non_positive_token_count_renders_and_bounds_one_word(self):
        self.assertEqual(harness.max_rendered_bytes(0), harness.max_rendered_bytes(1))
        self.assertLessEqual(len(harness.render_text("x", 0)), harness.max_rendered_bytes(0))


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

    def test_outcomes_are_broken_down_by_phase_and_the_phases_sum_to_the_whole_run(self):
        r = harness.run_repetition(self.workload, self.accounts, seed=123)
        self.assertEqual(sum(p["offered"] for p in r["outcomes_by_phase"].values()), r["total_offered"])
        for outcome, count in r["outcomes"].items():
            self.assertEqual(sum(p.get(outcome, 0) for p in r["outcomes_by_phase"].values()), count)
        self.assertEqual(set(r["outcomes_by_phase"]), {p["name"] for p in self.workload["phases"]})

    def test_the_frozen_hot_tenant_exceeds_the_account_limit_in_the_steady_phase(self):
        """W1, reproduced with the limiter's real window. The reviewer computed 180 of 7379
        steady requests (2.44 percent) independently; the numbers must match. If the reviewer
        changes the workload, update docs/launch/evidence/T23.60/decision-request-hot-tenant-rate-limit.md."""
        r = harness.run_repetition(self.workload, self.accounts, seed=20260918)
        steady, burst = r["outcomes_by_phase"]["steady"], r["outcomes_by_phase"]["burst"]
        self.assertEqual((steady["rejected_rate_limit"], steady["offered"]), (180, 7379))
        self.assertEqual((burst["rejected_rate_limit"], burst["offered"]), (544, 2430))
        self.assertEqual(r["rate_limited_by_account_and_phase"], {"scale-0": {"warmup": 28, "steady": 180, "burst": 544}})  # only the hot tenant
        self.assertAlmostEqual(steady["rejected_rate_limit"] / steady["offered"] * 100, 2.4393, places=3)
        self.assertGreater(steady["rejected_rate_limit"] / steady["offered"] * 100, self.workload["thresholds"]["unexpected_admission_rejection_max_pct"])

    def test_the_simulator_matches_an_independent_fixed_window_count(self):
        """A second implementation of admission.go's window, written from the Go source, agrees with the harness for any seed."""
        for seed in (1, 20260918):
            arrivals = harness.generate_arrivals(self.workload, self.accounts, __import__("random").Random(seed))
            starts, counts, refused = {}, {}, 0
            for a in arrivals:
                key, t = a["account"], a["t"]
                if key not in starts or t - starts[key] >= 60:
                    starts[key], counts[key] = t, 0
                if counts[key] >= 120:
                    refused += 1
                else:
                    counts[key] += 1
            r = harness.run_repetition(self.workload, self.accounts, seed=seed)
            self.assertEqual(r["outcomes"].get("rejected_rate_limit", 0), refused)

    def test_the_address_limit_is_checked_first_and_can_refuse_on_its_own(self):
        """gateway.go checks allow("ip:..", 600) before the account limit. A workload of 20 requests a second
        from one client address, spread over every account so no account limit binds, must hit the 600 a minute address limit."""
        workload = dict(self.workload, concurrency={"clients": 8, "baseline_request_rate_per_s": 20}, hot_tenant_traffic_fraction=0.0, phases=[{"name": "steady", "minutes": 2}])
        r = harness.run_repetition(workload, self.accounts, seed=3)
        self.assertGreater(r["outcomes"].get("rejected_ip_rate_limit", 0), 0)
        self.assertGreaterEqual(r["admission_rejection_pct"], r["outcomes"]["rejected_ip_rate_limit"] / r["total_offered"] * 100)

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
