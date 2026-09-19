"""Hit@5 and the frozen per-category floors
(docs/tasks/hosted-completion/T23.43.md acceptance section, restated in
docs/launch/hosted-completion/embedding-eval.md). Pure functions over
already-ranked id lists -- this module never calls a network, an embedder,
or a subprocess; scripts/hosted/eval_embeddings.py supplies the ranked
lists from whichever arm (fixture hash-bag, fixed-vector stub, real
lexical control, or live MCP recall) it is currently scoring.
"""

from __future__ import annotations

from dataclasses import dataclass, field

K = 5

# T23.43.md acceptance section, verbatim thresholds.
CATEGORY_FLOORS = {
    "paraphrase": (36, 40),
    "name_entity": (19, 20),
    "preference": (19, 20),
    "multilingual": (9, 10),
    "temporal": (5, 5),
}
OVERALL_HIT_AT_5_MIN = 0.90
LEXICAL_NEGATIVE_MIN_HITS = 18
LEXICAL_NEGATIVE_MIN_DENOM = 20


@dataclass
class CaseResult:
    case_id: str
    category: str
    expected_fact_id: str | None
    ranked_ids: list[str]
    hit: bool  # expected_fact_id in ranked_ids[:K] (positive cases only)
    rank: int | None  # 1-based rank of expected_fact_id if present, else None
    forbidden_ids: list[str] = field(default_factory=list)  # ids present that must never appear


def score_positive_case(case_id: str, category: str, expected_fact_id: str, ranked_ids: list[str]) -> CaseResult:
    top_k = ranked_ids[:K]
    hit = expected_fact_id in top_k
    rank = (ranked_ids.index(expected_fact_id) + 1) if expected_fact_id in ranked_ids else None
    return CaseResult(case_id, category, expected_fact_id, ranked_ids, hit, rank)


def score_empty_case(case_id: str, result_ids: list[str], fact_ids: list[str] | tuple = ()) -> CaseResult:
    """An expected-empty case passes only when NOTHING current comes back:
    no search result and no fact from recall's facts arm. There is no allowed
    set. T23.43.md requires every expected-empty case to "return no current
    fact"; an earlier revision let an account's own unrelated filler fact
    pass, which made the criterion vacuous, so any returned id -- a forgotten
    fact that resurfaced, another account's content, or unrelated filler --
    is a violation."""
    forbidden = [*result_ids, *fact_ids]
    return CaseResult(
        case_id, "empty", None, list(result_ids), hit=(len(forbidden) == 0), rank=None, forbidden_ids=forbidden
    )


@dataclass
class Summary:
    overall_hit_rate: float
    overall_hits: int
    overall_total: int
    by_category: dict[str, tuple[int, int]]  # category -> (hits, total)
    category_floor_pass: dict[str, bool]
    overall_floor_pass: bool
    empty_leakage_total: int
    empty_all_pass: bool
    lexical_negative_hits: int
    lexical_negative_total: int
    lexical_negative_pass: bool


def summarize(
    positive_results: list[CaseResult],
    empty_results: list[CaseResult],
    lexical_negative_results: list[CaseResult] | None = None,
) -> Summary:
    by_category: dict[str, tuple[int, int]] = {}
    for cat in CATEGORY_FLOORS:
        cat_results = [r for r in positive_results if r.category == cat]
        hits = sum(1 for r in cat_results if r.hit)
        by_category[cat] = (hits, len(cat_results))

    overall_hits = sum(1 for r in positive_results if r.hit)
    overall_total = len(positive_results)
    overall_rate = (overall_hits / overall_total) if overall_total else 0.0

    category_floor_pass = {}
    for cat, (min_hits, expected_total) in CATEGORY_FLOORS.items():
        hits, total = by_category.get(cat, (0, 0))
        category_floor_pass[cat] = total == expected_total and hits >= min_hits

    overall_floor_pass = overall_rate >= OVERALL_HIT_AT_5_MIN and all(category_floor_pass.values())

    empty_leakage_total = sum(len(r.forbidden_ids) for r in empty_results)
    empty_all_pass = len(empty_results) == 5 and all(r.hit for r in empty_results)

    lexical_negative_results = lexical_negative_results or []
    ln_hits = sum(1 for r in lexical_negative_results if r.hit)
    ln_total = len(lexical_negative_results)
    ln_pass = ln_total >= LEXICAL_NEGATIVE_MIN_DENOM and ln_hits >= LEXICAL_NEGATIVE_MIN_HITS

    return Summary(
        overall_hit_rate=overall_rate,
        overall_hits=overall_hits,
        overall_total=overall_total,
        by_category=by_category,
        category_floor_pass=category_floor_pass,
        overall_floor_pass=overall_floor_pass,
        empty_leakage_total=empty_leakage_total,
        empty_all_pass=empty_all_pass,
        lexical_negative_hits=ln_hits,
        lexical_negative_total=ln_total,
        lexical_negative_pass=ln_pass,
    )
