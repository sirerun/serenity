#!/usr/bin/env python3
"""Generate evals/hosted/corpus.json and evals/hosted/facts.json from the
literal, reviewable case/fact data in lib/corpus_data.py, and record the
frozen corpus_hash (docs/launch/hosted-completion/evidence.md "Qualification
inputs are frozen"). No network access; deterministic output.

Usage: python3 evals/hosted/corpus_gen.py [--check]
  --check: verify the on-disk corpus.json/facts.json exactly match what
           corpus_data.py would produce right now (used by test_corpus.py
           and by CI to catch an unreviewed drift between the two).
"""

from __future__ import annotations

import hashlib
import json
import sys
from dataclasses import asdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from lib import corpus_data  # noqa: E402
from lib import textutil  # noqa: E402

CORPUS_PATH = HERE / "corpus.json"
FACTS_PATH = HERE / "facts.json"

# T23.43.md step 1: "At least 20 paraphrases lack useful content-word
# overlap." Computed from the actual query/fact text, never trusted from a
# hand-set flag.
MIN_ZERO_OVERLAP_PARAPHRASES = 20


def canonical_json(obj) -> str:
    return json.dumps(obj, sort_keys=True, ensure_ascii=False, separators=(",", ":"))


def sha256_of(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def build() -> tuple[list[dict], list[dict], dict]:
    cases, facts = corpus_data.build_corpus()
    fact_by_id = {f.id: f for f in facts}

    case_dicts = []
    zero_overlap_paraphrases = 0
    for c in cases:
        d = asdict(c)
        if c.category == "paraphrase" and c.expected_fact_id is not None:
            fact_text = fact_by_id[c.expected_fact_id].text
            overlap = textutil.content_overlap(c.query, fact_text, frozenset(c.subject_tokens))
            d["computed_content_overlap"] = sorted(overlap)
            d["lacks_content_word_overlap"] = len(overlap) == 0
            if len(overlap) == 0:
                zero_overlap_paraphrases += 1
        else:
            d["computed_content_overlap"] = None
            d["lacks_content_word_overlap"] = None
        case_dicts.append(d)

    fact_dicts = [asdict(f) for f in facts]

    if zero_overlap_paraphrases < MIN_ZERO_OVERLAP_PARAPHRASES:
        raise SystemExit(
            f"corpus_gen: only {zero_overlap_paraphrases} paraphrase cases have zero content-word "
            f"overlap; need >= {MIN_ZERO_OVERLAP_PARAPHRASES}. Fix lib/corpus_data.py before freezing."
        )

    meta = {
        "total_cases": len(case_dicts),
        "positive_cases": sum(1 for c in case_dicts if c["category"] != "empty"),
        "empty_cases": sum(1 for c in case_dicts if c["category"] == "empty"),
        "by_category": {
            cat: sum(1 for c in case_dicts if c["category"] == cat)
            for cat in sorted({c["category"] for c in case_dicts})
        },
        "zero_overlap_paraphrases": zero_overlap_paraphrases,
        "min_zero_overlap_paraphrases_required": MIN_ZERO_OVERLAP_PARAPHRASES,
        "total_facts": len(fact_dicts),
    }
    return case_dicts, fact_dicts, meta


def main(argv: list[str]) -> int:
    check_only = "--check" in argv
    case_dicts, fact_dicts, meta = build()

    corpus_hash = sha256_of(canonical_json(case_dicts))
    facts_hash = sha256_of(canonical_json(fact_dicts))
    meta["corpus_hash_sha256"] = corpus_hash
    meta["facts_hash_sha256"] = facts_hash

    corpus_out = {"meta": meta, "cases": case_dicts}
    facts_out = {"facts": fact_dicts}

    # Normalize through a JSON round-trip before comparing: dataclass
    # fields like distractor_fact_ids are tuples in memory but always
    # lists once read back from disk, and Python's (1, 2) != [1, 2] would
    # otherwise make --check report a false drift on every run.
    normalized_corpus_out = json.loads(canonical_json(corpus_out))
    normalized_facts_out = json.loads(canonical_json(facts_out))

    if check_only:
        ok = True
        if not CORPUS_PATH.exists() or json.loads(CORPUS_PATH.read_text()) != normalized_corpus_out:
            print("corpus_gen --check: evals/hosted/corpus.json is stale or missing", file=sys.stderr)
            ok = False
        if not FACTS_PATH.exists() or json.loads(FACTS_PATH.read_text()) != normalized_facts_out:
            print("corpus_gen --check: evals/hosted/facts.json is stale or missing", file=sys.stderr)
            ok = False
        return 0 if ok else 1

    CORPUS_PATH.write_text(json.dumps(corpus_out, indent=2, sort_keys=True, ensure_ascii=False) + "\n")
    FACTS_PATH.write_text(json.dumps(facts_out, indent=2, sort_keys=True, ensure_ascii=False) + "\n")
    print(f"wrote {CORPUS_PATH} and {FACTS_PATH}")
    print(json.dumps(meta, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
