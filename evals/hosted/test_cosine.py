"""Vector scale invariance (T23.43.md step 3: "Query vectors and stored
vectors use cosine semantics; test vector scale invariance").
"""

from __future__ import annotations

import math
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from lib.cosine import VectorError, cosine_similarity, rank_by_cosine  # noqa: E402


class TestCosineScaleInvariance(unittest.TestCase):
    VECTORS = [
        ([1.0, 0.0, 0.0], [1.0, 0.0, 0.0]),
        ([1.0, 2.0, 3.0], [4.0, -1.0, 2.0]),
        ([0.001, 0.002, -0.003], [0.5, 0.5, 0.5]),
        ([-1.0, -2.0, 5.0, 0.25], [3.0, 1.0, -2.0, 7.0]),
    ]
    SCALES = [0.0001, 0.5, 1.0, 2.0, 1000.0, 1e6]

    def test_scaling_either_vector_preserves_similarity(self):
        for a, b in self.VECTORS:
            baseline = cosine_similarity(a, b)
            for scale in self.SCALES:
                scaled_a = [x * scale for x in a]
                got = cosine_similarity(scaled_a, b)
                self.assertAlmostEqual(got, baseline, places=9, msg=f"scale={scale} a={a} b={b}")

                scaled_b = [x * scale for x in b]
                got2 = cosine_similarity(a, scaled_b)
                self.assertAlmostEqual(got2, baseline, places=9, msg=f"scale={scale} a={a} b={b}")

    def test_scaling_both_vectors_preserves_similarity(self):
        for a, b in self.VECTORS:
            baseline = cosine_similarity(a, b)
            for scale in self.SCALES:
                got = cosine_similarity([x * scale for x in a], [y * scale for y in b])
                self.assertAlmostEqual(got, baseline, places=9)

    def test_ranking_is_invariant_to_uniform_rescaling_of_candidates(self):
        query = [1.0, 0.0, 0.0]
        candidates = [("a", [1.0, 0.0, 0.0]), ("b", [0.0, 1.0, 0.0]), ("c", [0.7, 0.7, 0.0])]
        baseline_order = [cid for cid, _ in rank_by_cosine(query, candidates)]
        for scale in self.SCALES:
            scaled = [(cid, [x * scale for x in vec]) for cid, vec in candidates]
            order = [cid for cid, _ in rank_by_cosine(query, scaled)]
            self.assertEqual(order, baseline_order, f"scale={scale}")

    def test_identical_direction_scaled_differently_scores_1(self):
        self.assertAlmostEqual(cosine_similarity([2.0, 0.0], [1000.0, 0.0]), 1.0, places=9)
        self.assertAlmostEqual(cosine_similarity([2.0, 0.0], [-1000.0, 0.0]), -1.0, places=9)

    def test_rejects_empty_vector(self):
        with self.assertRaises(VectorError):
            cosine_similarity([], [1.0])

    def test_rejects_nonfinite_vector(self):
        with self.assertRaises(VectorError):
            cosine_similarity([1.0, math.inf], [1.0, 2.0])
        with self.assertRaises(VectorError):
            cosine_similarity([1.0, math.nan], [1.0, 2.0])

    def test_rejects_dimension_mismatch(self):
        with self.assertRaises(VectorError):
            cosine_similarity([1.0, 2.0], [1.0, 2.0, 3.0])

    def test_rejects_zero_vector(self):
        with self.assertRaises(VectorError):
            cosine_similarity([0.0, 0.0], [1.0, 2.0])


if __name__ == "__main__":
    unittest.main()
