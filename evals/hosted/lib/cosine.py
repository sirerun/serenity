"""Cosine similarity over plain float vectors -- the semantics T23.43.md
step 3 pins ("Query vectors and stored vectors use cosine semantics; test
vector scale invariance"): scaling either operand by any positive constant
must never change the similarity value (up to floating-point rounding),
because cosine similarity is scale-invariant by construction (it divides
out both vectors' norms). test_cosine.py exercises this property directly.
"""

from __future__ import annotations

import math


class VectorError(ValueError):
    pass


def _validate(vec: list[float], name: str) -> None:
    if not vec:
        raise VectorError(f"{name}: empty vector")
    for x in vec:
        if not math.isfinite(x):
            raise VectorError(f"{name}: nonfinite value {x!r}")


def cosine_similarity(a: list[float], b: list[float]) -> float:
    """Cosine similarity of a and b. Raises VectorError on empty,
    nonfinite, dimension-mismatched, or all-zero vectors -- the same
    "reject empty, nonfinite, malformed or inconsistent-dimension vectors"
    contract T23.42 owns for the live provider adapter; this module is
    T23.43's own local scorer and enforces it independently rather than
    trusting an upstream caller.
    """
    _validate(a, "a")
    _validate(b, "b")
    if len(a) != len(b):
        raise VectorError(f"dimension mismatch: {len(a)} vs {len(b)}")
    dot = sum(x * y for x, y in zip(a, b))
    norm_a = math.sqrt(sum(x * x for x in a))
    norm_b = math.sqrt(sum(y * y for y in b))
    if norm_a == 0.0 or norm_b == 0.0:
        raise VectorError("zero-magnitude vector has no defined cosine similarity")
    return dot / (norm_a * norm_b)


def rank_by_cosine(query_vec: list[float], candidates: list[tuple[str, list[float]]]) -> list[tuple[str, float]]:
    """Returns [(id, score), ...] sorted by descending cosine similarity,
    ties broken by id for determinism.
    """
    scored = [(cid, cosine_similarity(query_vec, vec)) for cid, vec in candidates]
    scored.sort(key=lambda item: (-item[1], item[0]))
    return scored
