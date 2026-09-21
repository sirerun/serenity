"""Corpus composition tests (T23.43.md step 1's exact required counts).
Run via: python3 -m unittest discover -s evals/hosted -p "test_*.py"
"""

from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from lib import corpus_data, textutil  # noqa: E402
import corpus_gen  # noqa: E402


class TestCorpusComposition(unittest.TestCase):
    def setUp(self):
        self.cases, self.facts = corpus_data.build_corpus()
        self.fact_by_id = {f.id: f for f in self.facts}

    def test_total_case_count(self):
        self.assertEqual(len(self.cases), 100)

    def test_category_counts_match_spec(self):
        expected = {
            "paraphrase": 40,
            "name_entity": 20,
            "preference": 20,
            "multilingual": 10,
            "temporal": 5,
            "empty": 5,
        }
        counts = {}
        for c in self.cases:
            counts[c.category] = counts.get(c.category, 0) + 1
        self.assertEqual(counts, expected)

    def test_95_positive_cases_have_exactly_one_expected_fact(self):
        positive = [c for c in self.cases if c.category != "empty"]
        self.assertEqual(len(positive), 95)
        for c in positive:
            self.assertIsNotNone(c.expected_fact_id, c.id)
            self.assertIn(c.expected_fact_id, self.fact_by_id, c.id)

    def test_positive_cases_have_scored_distractors(self):
        positive = [c for c in self.cases if c.category != "empty"]
        for c in positive:
            self.assertGreaterEqual(len(c.distractor_fact_ids), 2, c.id)
            for d in c.distractor_fact_ids:
                self.assertIn(d, self.fact_by_id, f"{c.id}: distractor {d} missing")
                self.assertNotEqual(d, c.expected_fact_id, f"{c.id}: distractor equals its own expected fact")

    def test_empty_cases_have_no_expected_fact_and_an_isolated_filler(self):
        empty = [c for c in self.cases if c.category == "empty"]
        self.assertEqual(len(empty), 5)
        for c in empty:
            self.assertIsNone(c.expected_fact_id, c.id)
            self.assertIsNotNone(c.isolated_filler_fact_id, c.id)
            self.assertIn(c.isolated_filler_fact_id, self.fact_by_id, c.id)

    def test_at_least_20_paraphrases_lack_content_word_overlap(self):
        paraphrases = [c for c in self.cases if c.category == "paraphrase"]
        self.assertEqual(len(paraphrases), 40)
        zero_overlap = 0
        for c in paraphrases:
            fact_text = self.fact_by_id[c.expected_fact_id].text
            overlap = textutil.content_overlap(c.query, fact_text, frozenset(c.subject_tokens))
            if len(overlap) == 0:
                zero_overlap += 1
        self.assertGreaterEqual(zero_overlap, 20, f"only {zero_overlap} paraphrases lack content-word overlap")

    def test_multilingual_cases_use_10_distinct_languages(self):
        ml = [c for c in self.cases if c.category == "multilingual"]
        self.assertEqual(len(ml), 10)
        languages = {c.language for c in ml}
        self.assertEqual(len(languages), 10, f"expected 10 distinct languages, got {sorted(languages)}")
        for c in ml:
            self.assertNotEqual(c.language, "en", c.id)

    def test_temporal_cases_reference_explicit_dates(self):
        temporal = [c for c in self.cases if c.category == "temporal"]
        self.assertEqual(len(temporal), 5)
        for c in temporal:
            fact_text = self.fact_by_id[c.expected_fact_id].text
            tokens = [tok.strip(".,;:") for tok in fact_text.replace("-", " ").split()]
            has_year = any(tok.isdigit() and len(tok) == 4 for tok in tokens)
            self.assertTrue(has_year, f"{c.id}: fact lacks an explicit year: {fact_text!r}")

    def test_no_customer_data_markers(self):
        # A cheap guard against accidentally pasting anything real: every
        # subject/company name in this synthetic corpus is invented, and
        # none of them should collide with this repo's own real people
        # (David, Sire) or real companies.
        banned = {"sire", "serenity", "anthropic", "david ndungu"}
        for f in self.facts:
            lowered = f.text.lower()
            for term in banned:
                self.assertNotIn(term, lowered, f"{f.id}: unexpected real-world term {term!r}")

    def test_fact_ids_are_unique(self):
        ids = [f.id for f in self.facts]
        self.assertEqual(len(ids), len(set(ids)))

    def test_case_ids_are_unique(self):
        ids = [c.id for c in self.cases]
        self.assertEqual(len(ids), len(set(ids)))


class TestCorpusGenFreeze(unittest.TestCase):
    """corpus.json/facts.json are the frozen, committed artifacts a task41
    reviewer signs off on; this proves they are byte-identical to what
    corpus_data.py would produce today (evidence.md's "Qualification
    inputs are frozen" -- an unreviewed drift must fail loudly, not pass
    silently)."""

    def test_corpus_gen_check_matches_on_disk_output(self):
        result = subprocess.run(
            [sys.executable, str(HERE / "corpus_gen.py"), "--check"],
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_on_disk_corpus_has_recorded_hash(self):
        corpus = json.loads((HERE / "corpus.json").read_text())
        self.assertIn("corpus_hash_sha256", corpus["meta"])
        self.assertEqual(len(corpus["meta"]["corpus_hash_sha256"]), 64)


if __name__ == "__main__":
    unittest.main()
