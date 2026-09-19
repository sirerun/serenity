"""Local synthetic embedders for --fixtures mode. Neither one calls a
network or a real model -- evidence.md is explicit that fixture mode "tests
harness mechanics, not semantic quality or actual costs."

Two distinct stubs, used for two distinct purposes (see
scripts/hosted/eval_embeddings.py):

  HashBagEmbedder    exercises the full corpus/scoring/reporting pipeline
                      end-to-end without crashing (the "harness mechanics"
                      pass). It is a real, if weak, bag-of-words-style
                      vector -- shared vocabulary between a query and a
                      fact nudges their cosine similarity up -- but it has
                      no notion of synonymy, so a genuine zero-content-word
                      paraphrase scores no better than an unrelated fact.

  FixedVectorEmbedder is the required negative control (T23.43.md
                      acceptance bullet 2: "Fixed-vector or lexical-only
                      stub fails the quality predicate"): every input maps
                      to the identical vector, so cosine similarity is
                      1.0 for every pair and ranking collapses to
                      insertion order. Hit@5 computed against it must come
                      out far below the frozen thresholds -- a scoring
                      pipeline that could not tell this apart from a real
                      pass would be worthless.
"""

from __future__ import annotations

import hashlib

from .textutil import tokenize

_HASH_DIM = 64


class HashBagEmbedder:
    """Deterministic bag-of-tokens hashing embedder, dimension _HASH_DIM."""

    def model_version(self) -> str:
        return f"fixture-hashbag@v1-dim{_HASH_DIM}"

    def embed(self, text: str) -> list[float]:
        vec = [0.0] * _HASH_DIM
        tokens = tokenize(text)
        if not tokens:
            # Never return an all-zero vector (cosine is undefined for it,
            # matching T23.42's own "reject empty vector" contract) --
            # empty input maps to a single fixed nonzero slot instead.
            vec[0] = 1.0
            return vec
        for tok in tokens:
            digest = hashlib.sha256(tok.encode("utf-8")).digest()
            idx = int.from_bytes(digest[:4], "big") % _HASH_DIM
            sign = 1.0 if digest[4] % 2 == 0 else -1.0
            vec[idx] += sign
        return vec


class FixedVectorEmbedder:
    """Always returns the same vector, regardless of input -- the "lexical-
    only stub" / "fixed-vector" negative control the acceptance criteria
    require to demonstrably fail the quality predicate.
    """

    def __init__(self, dim: int = _HASH_DIM):
        self._vec = [1.0] + [0.0] * (dim - 1)

    def model_version(self) -> str:
        return "fixture-fixed-vector@v0"

    def embed(self, text: str) -> list[float]:
        return list(self._vec)
