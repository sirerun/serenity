"""Synthetic facts that the 5 expected-empty cases are about, authored ONLY
for the live forgotten-fact qualification (never part of corpus.json).

T23.43.md defines the expected-empty cases as forgotten/expired cases and
requires each to return no current fact. A query for content that was never
authored proves only that an empty index returns nothing. To test forgetting,
each frozen empty query needs a fact that really existed and was really
forgotten: the seed run remembers one of these per case, shows it retrievable
(inventory and search), forgets it through the ordinary `forget` API, then
shows it gone from both arms.

The frozen queries, the 95 positive cases and every threshold are untouched.
These texts live outside corpus.json on purpose so the reviewed corpus hash
does not move; they carry their own hash (FORGOTTEN_TARGETS_SHA256), recorded
in every seed receipt, and are a proposed addition for the task41 reviewer to
confirm. Every subject is invented; no customer data.
"""

from __future__ import annotations

import hashlib
import json

# empty case id -> the synthetic fact that is remembered, then forgotten.
FORGOTTEN_TARGET_TEXT = {
    "empty-01": "David gave his dog Biscuit up for adoption last year.",
    "empty-02": "Maya lived in apartment 4B before her 2023 move.",
    "empty-03": "The old vendor contract number was VC-2291 before it was superseded.",
    "empty-04": "The temporary badge code issued during the office renovation was 7714.",
    "empty-05": "The original launch date was March 3 before it was rescheduled.",
}

# The fact that exists only in the positive account; the empty account is
# queried for it to show one account cannot reach another's content.
SENTINEL_FACT_ID = "sentinel-cross-account"
SENTINEL_FACT_TEXT = "Sentinel record zx-QQ-7731 is stored only in the positive qualification account."


def target_id(case_id: str) -> str:
    return f"forgotten-{case_id}"


def targets_sha256() -> str:
    canonical = json.dumps(
        {"targets": FORGOTTEN_TARGET_TEXT, "sentinel": [SENTINEL_FACT_ID, SENTINEL_FACT_TEXT]},
        sort_keys=True, ensure_ascii=False, separators=(",", ":"),
    )
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()
