"""Synthetic facts that the 5 expected-empty cases are about, authored ONLY
for the live forgotten/expired qualification (never part of corpus.json).

T23.43.md defines the expected-empty cases as forgotten/expired cases and
requires each to return no current fact. A query for content that was never
authored proves only that an empty index returns nothing. To test forgetting
and expiry, each frozen empty query needs a fact that really existed and was
really removed. The seed run remembers one of these per case, shows it
retrievable (inventory and search), and then removes it one of two ways
(TARGET_MODE):

- "forget": through the ordinary `forget` API.
- "expire": it is remembered with a fixed absolute `ttl` timestamp, shown
  visible before that instant, and then shown absent after the service's own
  clock has passed it. Nothing is called to remove it. This is the TTL
  half of "forgotten/expired".

Either way the account ends holding zero current facts, and every expected-empty
query is scored strictly against it.

The frozen queries, the 95 positive cases and every threshold are untouched.
These texts, the mode assignment and the expiry timing live outside
corpus.json on purpose so the reviewed corpus hash does not move; they carry
their own hash (targets_sha256), recorded in every seed receipt, and are a
proposed addition for the task41 reviewer to confirm. Every subject is
invented; no customer data.
"""

from __future__ import annotations

import hashlib
import json

# empty case id -> the synthetic fact that is remembered, then removed.
FORGOTTEN_TARGET_TEXT = {
    "empty-01": "David gave his dog Biscuit up for adoption last year.",
    "empty-02": "Maya lived in apartment 4B before her 2023 move.",
    "empty-03": "The old vendor contract number was VC-2291 before it was superseded.",
    "empty-04": "The temporary badge code issued during the office renovation was 7714.",
    "empty-05": "The original launch date was March 3 before it was rescheduled.",
}

MODE_FORGET = "forget"
MODE_EXPIRE = "expire"

# How each target is removed. A superseded contract number and a temporary
# badge code are the two facts that lapse by their nature; the other three
# are corrections a person would make, so they are forgotten explicitly.
TARGET_MODE = {
    "empty-01": MODE_FORGET,
    "empty-02": MODE_FORGET,
    "empty-03": MODE_EXPIRE,
    "empty-04": MODE_EXPIRE,
    "empty-05": MODE_FORGET,
}

# Expiry timing, part of the reviewed plan (and of targets_sha256). The
# absolute expiry instant is computed once per seed run as
# ceil(local wall clock) + EXPIRY_TTL_SECONDS and sent as an RFC 3339 UTC
# timestamp, because the service refuses a relative TTL on a keyed remember.
# TTL must outlast the remembers and presence probes that precede the wait;
# MARGIN is how long past that instant the harness waits before probing, so a
# service clock a few seconds behind the harness's is not misread as a fact
# that failed to expire; TAIL is the budgeted time that must remain after
# the wait for the post-expiry probes.
EXPIRY_TTL_SECONDS = 60
EXPIRY_MARGIN_SECONDS = 5
EXPIRY_TAIL_SECONDS = 30

# The fact that exists only in the positive account; the empty account is
# queried for it to show one account cannot reach another's content.
SENTINEL_FACT_ID = "sentinel-cross-account"
SENTINEL_FACT_TEXT = "Sentinel record zx-QQ-7731 is stored only in the positive qualification account."


def target_id(case_id: str) -> str:
    return f"forgotten-{case_id}"


def cases_with_mode(mode: str) -> list[str]:
    """Empty case ids removed the given way, in case-id order."""
    return sorted(cid for cid, m in TARGET_MODE.items() if m == mode)


def expiry_wait_floor_seconds() -> int:
    """The least max_elapsed_seconds a seed run can have and still cross the
    expiry: the TTL, the margin past it, and the tail reserved for probes."""
    return EXPIRY_TTL_SECONDS + EXPIRY_MARGIN_SECONDS + EXPIRY_TAIL_SECONDS


def targets_sha256() -> str:
    """Hash of the whole supplemental plan a reviewer confirms: target texts,
    how each is removed, the expiry timing, and the sentinel."""
    canonical = json.dumps(
        {
            "targets": FORGOTTEN_TARGET_TEXT,
            "modes": TARGET_MODE,
            "expiry": {
                "ttl_seconds": EXPIRY_TTL_SECONDS,
                "margin_seconds": EXPIRY_MARGIN_SECONDS,
                "tail_seconds": EXPIRY_TAIL_SECONDS,
            },
            "sentinel": [SENTINEL_FACT_ID, SENTINEL_FACT_TEXT],
        },
        sort_keys=True, ensure_ascii=False, separators=(",", ":"),
    )
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()
