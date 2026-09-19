"""Hit@5 arithmetic and the frozen per-category floors."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from lib import scoring  # noqa: E402


class TestHitAtK(unittest.TestCase):
    def test_hit_when_expected_in_top_5(self):
        r = scoring.score_positive_case("c1", "paraphrase", "fact-a", ["fact-x", "fact-a", "fact-y"])
        self.assertTrue(r.hit)
        self.assertEqual(r.rank, 2)

    def test_miss_when_expected_outside_top_5(self):
        ranked = [f"fact-{i}" for i in range(10)]
        ranked[7] = "fact-a"  # rank 8, outside top 5
        r = scoring.score_positive_case("c1", "paraphrase", "fact-a", ranked)
        self.assertFalse(r.hit)
        self.assertEqual(r.rank, 8)

    def test_miss_when_expected_absent(self):
        r = scoring.score_positive_case("c1", "paraphrase", "fact-a", ["fact-x", "fact-y"])
        self.assertFalse(r.hit)
        self.assertIsNone(r.rank)

    def test_k_is_5_not_more(self):
        ranked = ["fact-x"] * 5 + ["fact-a"]  # rank 6
        r = scoring.score_positive_case("c1", "paraphrase", "fact-a", ranked)
        self.assertFalse(r.hit)


class TestEmptyCaseLeakage(unittest.TestCase):
    # T23.43.md: every expected-empty case must return NO current fact. An
    # earlier revision let an account's own unrelated filler pass; that made
    # the criterion vacuous, so these tests pin the strict reading.
    def test_an_unrelated_filler_result_is_a_violation_not_allowed_content(self):
        r = scoring.score_empty_case("e1", ["filler-01"])
        self.assertFalse(r.hit)
        self.assertEqual(r.forbidden_ids, ["filler-01"])

    def test_leakage_when_foreign_id_present(self):
        r = scoring.score_empty_case("e1", ["filler-01", "para-02a-fact"])
        self.assertFalse(r.hit)
        self.assertEqual(r.forbidden_ids, ["filler-01", "para-02a-fact"])

    def test_a_fact_from_the_facts_arm_is_a_violation_even_with_no_search_result(self):
        r = scoring.score_empty_case("e1", [], ["forgotten-empty-01"])
        self.assertFalse(r.hit)
        self.assertEqual(r.forbidden_ids, ["forgotten-empty-01"])

    def test_empty_results_pass(self):
        r = scoring.score_empty_case("e1", [])
        self.assertTrue(r.hit)
        self.assertEqual(r.forbidden_ids, [])

    def test_the_scorer_accepts_no_allowed_set(self):
        with self.assertRaises(TypeError):
            scoring.score_empty_case("e1", ["filler-01"], allowed_ids={"filler-01"})


class TestSummary(unittest.TestCase):
    def _positive(self, category, n_hit, n_total):
        results = []
        for i in range(n_total):
            hit = i < n_hit
            ranked = [f"{category}-{i}-fact"] if hit else []
            results.append(scoring.CaseResult(f"{category}-{i}", category, f"{category}-{i}-fact", ranked, hit, 1 if hit else None))
        return results

    def test_overall_floor_pass_requires_every_category_floor(self):
        results = (
            self._positive("paraphrase", 36, 40)
            + self._positive("name_entity", 19, 20)
            + self._positive("preference", 19, 20)
            + self._positive("multilingual", 9, 10)
            + self._positive("temporal", 5, 5)
        )
        summary = scoring.summarize(results, empty_results=[])
        self.assertTrue(summary.overall_floor_pass)
        self.assertAlmostEqual(summary.overall_hit_rate, 88 / 95)

    def test_one_category_below_floor_fails_overall(self):
        results = (
            self._positive("paraphrase", 36, 40)
            + self._positive("name_entity", 19, 20)
            + self._positive("preference", 18, 20)  # below the 19/20 floor
            + self._positive("multilingual", 9, 10)
            + self._positive("temporal", 5, 5)
        )
        summary = scoring.summarize(results, empty_results=[])
        self.assertFalse(summary.category_floor_pass["preference"])
        self.assertFalse(summary.overall_floor_pass)

    def test_high_overall_rate_with_one_zeroed_category_still_fails(self):
        # A model that aces everything except temporal (0/5) can still
        # clear 0.90 overall (90/95=0.947) -- category floors must catch
        # this, proving the per-category gate is load-bearing, not
        # redundant with the overall rate.
        results = (
            self._positive("paraphrase", 40, 40)
            + self._positive("name_entity", 20, 20)
            + self._positive("preference", 20, 20)
            + self._positive("multilingual", 10, 10)
            + self._positive("temporal", 0, 5)
        )
        summary = scoring.summarize(results, empty_results=[])
        self.assertGreaterEqual(summary.overall_hit_rate, scoring.OVERALL_HIT_AT_5_MIN)
        self.assertFalse(summary.category_floor_pass["temporal"])
        self.assertFalse(summary.overall_floor_pass)

    def test_empty_all_pass_requires_all_5_and_zero_leakage(self):
        empties = [scoring.score_empty_case(f"e{i}", []) for i in range(5)]
        summary = scoring.summarize([], empties)
        self.assertTrue(summary.empty_all_pass)
        self.assertEqual(summary.empty_leakage_total, 0)

        empties_with_leak = empties[:4] + [scoring.score_empty_case("e4", ["foreign"])]
        summary2 = scoring.summarize([], empties_with_leak)
        self.assertFalse(summary2.empty_all_pass)
        self.assertEqual(summary2.empty_leakage_total, 1)

    def test_lexical_negative_pass_threshold(self):
        ln = [scoring.CaseResult(f"p{i}", "paraphrase", f"fact-{i}", [], hit=(i < 18), rank=None) for i in range(20)]
        summary = scoring.summarize([], [], lexical_negative_results=ln)
        self.assertTrue(summary.lexical_negative_pass)

        ln_fail = [scoring.CaseResult(f"p{i}", "paraphrase", f"fact-{i}", [], hit=(i < 17), rank=None) for i in range(20)]
        summary2 = scoring.summarize([], [], lexical_negative_results=ln_fail)
        self.assertFalse(summary2.lexical_negative_pass)

    def test_lexical_negative_draft_is_an_unapproved_raw_count_not_a_ratio(self):
        """Pins the draft's behaviour so a silent change fails: a fixed
        numerator of 18 over the 21 flagged cases is 85.7%, weaker than the
        contract's 18/20 = 90%. The reviewer must choose a subset or a ratio
        before acceptance; this test records what the draft does today and
        does not endorse it."""
        def run(hits, total):
            ln = [scoring.CaseResult(f"p{i}", "paraphrase", f"fact-{i}", [], hit=(i < hits), rank=None) for i in range(total)]
            return scoring.summarize([], [], lexical_negative_results=ln)

        self.assertEqual((scoring.LEXICAL_NEGATIVE_MIN_HITS, scoring.LEXICAL_NEGATIVE_MIN_DENOM), (18, 20))
        self.assertTrue(run(18, 21).lexical_negative_pass, "draft raw count: 18 of 21 passes")
        self.assertLess(18 / 21, 18 / 20, "the raw count over 21 is weaker than the contract ratio")
        self.assertFalse(run(17, 21).lexical_negative_pass)
        self.assertFalse(run(18, 19).lexical_negative_pass, "a denominator under 20 never passes")


if __name__ == "__main__":
    unittest.main()
