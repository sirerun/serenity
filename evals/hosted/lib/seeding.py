"""Controlled seeding of the frozen corpus into a hosted qualification
account, with an empty-account proof before the first write and an
inventory proof after the last one.

This module exists because the live scoring path cannot work without it.
The hosted `recall` tool names a matched fact `source-<sha256>` (the id the
`remember` call returned), never the corpus's own fact id ("para-01-fact").
A live run therefore needs a receipt mapping each server-assigned id back to
the corpus fact it was seeded from; without one, every result is
unattributable and Hit@5 is meaninglessly zero. The receipt is that mapping,
bound into the live manifest by its sha256 (`corpus_seeded_confirmation`).

Two roles, two accounts (see manifest.py):

- `positive` holds every fact the 95 positive cases reference, plus one
  cross-account sentinel fact that exists nowhere else.
- `empty_case` tests the 5 expected-empty cases, which T23.43.md defines as
  forgotten/expired: each must return NO current fact. For each frozen empty
  query the seed run remembers a synthetic target fact (lib/forgotten_targets)
  and proves it present (inventory and search retrieval). Each target is then
  removed one of two ways, fixed by the reviewed plan: "expire" targets are
  remembered with one absolute `ttl` timestamp and are shown absent only after
  the service's own clock has passed it, with nothing called to remove them;
  "forget" targets are forgotten through the ordinary `forget` API. While the
  forget targets are still present they serve as a control (an expiry that
  emptied the index for the wrong reason would not leave them behind). The
  account ends holding zero current facts, so a live run scores every
  expected-empty query strictly: any returned result or fact is a violation.
  The sentinel is then queried against this account to show another
  account's content is unreachable.

The expiry wait counts against the manifest's total elapsed cap. A run whose
cap cannot cover TTL + margin + tail is blocked before its first call, and a
wait that no longer fits blocks before it starts. A service that ignores the
TTL, does not report the expiry, keeps an expired fact searchable, or runs a
clock so far behind that the fact is still visible after the margin blocks
the seed; none of these is ever read as a pass.

Seeding never overwrites, never resumes, and never reuses a non-empty
account: a partial seed leaves a receipt marked incomplete and the account
must be reset out-of-band before another attempt, because an account that
already holds data cannot honestly prove it was empty. If the hosted service
cannot do any step (a target not retrievable before forgetting, a forget that
does not remove, a search index that keeps a forgotten fact), seeding stops
BLOCKED; nothing is reinterpreted to pass.

Nothing here stores a credential value; receipts carry ids, hashes, counts,
the exact endpoint URL and the credential's environment-variable NAME only.
"""

from __future__ import annotations

import datetime
import hashlib
import json
import math
import re
import time
from dataclasses import dataclass
from pathlib import Path

from . import forgotten_targets as ft
from .mcp_client import BudgetExceeded, MCPClient, MCPError
from .scoring import K as RECALL_LIMIT

ROLE_POSITIVE = "positive"
ROLE_EMPTY_CASE = "empty_case"
ROLES = (ROLE_POSITIVE, ROLE_EMPTY_CASE)
RECEIPT_SCHEMA_VERSION = 3  # 2: expire-mode targets, expiry and expiry_check proofs; 3: ledger-backed spend provenance
WAIT_CHUNK_SECONDS = 5.0  # the expiry wait sleeps in chunks so the elapsed cap is observed during it
SOURCE_SLUG_PREFIX = "source-"
INVENTORY_LIMIT = 1000
EMPTY_PROBE_QUERY = "qualification emptiness probe"
_OPERATION_KEY = re.compile(r"^[A-Za-z0-9_.:-]{1,128}$")
_SHA256_HEX = re.compile(r"^[0-9a-f]{64}$")


# Server-controlled strings are never copied into a reason, a receipt or a
# result. A hostile or buggy server can reflect the Authorization header into
# any field it sends, so only members of these fixed sets, hashes, booleans
# and validated integers leave this module.
_REMEMBER_STATUSES = frozenset({"inserted", "duplicate", "superseded"})
_SEARCH_STATES = frozenset({"semantic", "lexical", "unavailable", "not_eligible"})
_USAGE_FIELDS = ("calls_made", "request_bytes", "response_bytes")


def fixed(value: object, allowed: frozenset) -> str:
    return value if isinstance(value, str) and value in allowed else "unexpected"


def _is_int(v: object) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def _nonneg_int(v: object) -> bool:
    return _is_int(v) and v >= 0


def _safe_int(v: object) -> int | None:
    return v if _is_int(v) and -10**9 < v < 10**9 else None


def iso_utc(epoch: float) -> str:
    """RFC 3339 UTC to whole seconds: the shape the hosted `ttl` parameter and
    its `valid_until` echo both use."""
    return datetime.datetime.fromtimestamp(epoch, tz=datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_iso_utc(value: object) -> float | None:
    """Epoch seconds for a timezone-aware ISO 8601 string, else None. The
    input is server-controlled text: it is parsed, never echoed."""
    if not isinstance(value, str) or len(value) > 64:
        return None
    try:
        t = datetime.datetime.fromisoformat(value)
    except ValueError:
        return None
    if t.tzinfo is None:
        return None
    try:
        return t.timestamp()
    except (OverflowError, OSError, ValueError):
        return None


def target_mode(fact_id: str) -> str | None:
    """How a supplemental target is removed ("forget" or "expire"), or None
    for any other fact."""
    if fact_id.startswith("forgotten-"):
        return ft.TARGET_MODE.get(fact_id[len("forgotten-"):])
    return None


@dataclass(frozen=True)
class Expiry:
    epoch: int  # the one absolute instant every expire-mode target is remembered with
    iso: str


def plan_expiry(clock) -> Expiry:
    """The fixed absolute expiry for this seed run: the local wall clock
    rounded up to a whole second, plus the plan's TTL. Computed once, so all
    expire-mode targets share one instant and a retried keyed remember carries
    the identical payload."""
    epoch = math.ceil(clock()) + ft.EXPIRY_TTL_SECONDS
    return Expiry(epoch=epoch, iso=iso_utc(epoch))


class SeedBlocked(RuntimeError):
    """Seeding stopped before completing; the message is the BLOCKED reason."""

    kind = "other"


class BudgetBlocked(SeedBlocked):
    """A cumulative cap (or the ledger that enforces it) stopped the run."""

    kind = "budget"


class ProviderUnavailable(SeedBlocked):
    """The service reported it had no working embedder (search degraded, or a
    write stored no vector)."""

    kind = "provider_unavailable"


def blocked_kind(exc: BaseException) -> str:
    """Why a run stopped, as one of "budget", "provider_unavailable", "other",
    "unexpected". Only the first two are the criterion the budget-boundary
    acceptance row is about."""
    if isinstance(exc, BudgetExceeded):
        return "budget"
    if isinstance(exc, SeedBlocked):
        return exc.kind
    if isinstance(exc, MCPError):
        return "other"
    return "unexpected"


def empty_case_ids(corpus: dict) -> list[str]:
    return [c["id"] for c in corpus["cases"] if c["category"] == "empty"]


def plan_fact_ids(corpus: dict, role: str) -> list[str]:
    """Deterministic (sorted) fact ids the role's account is seeded with.
    `positive` mirrors the lexical control's brain exactly (the sentinel is
    the one addition, semantically unrelated to every case), so the semantic
    and lexical arms rank the same candidates. `empty_case` is one forgotten
    target per expected-empty case and nothing else."""
    cases = corpus["cases"]
    if role == ROLE_POSITIVE:
        pos = [c for c in cases if c["category"] != "empty"]
        ids = {c["expected_fact_id"] for c in pos} | {d for c in pos for d in c["distractor_fact_ids"]}
        return sorted(ids | {ft.SENTINEL_FACT_ID})
    if role == ROLE_EMPTY_CASE:
        return sorted(ft.target_id(cid) for cid in empty_case_ids(corpus))
    raise ValueError(f"unknown role {role!r}")


def fact_text(fid: str, fact_by_id: dict) -> str:
    if fid == ft.SENTINEL_FACT_ID:
        return ft.SENTINEL_FACT_TEXT
    if fid.startswith("forgotten-"):
        return ft.FORGOTTEN_TARGET_TEXT[fid[len("forgotten-"):]]
    return fact_by_id[fid]["text"]


def plan_facts(corpus: dict, fact_by_id: dict, role: str) -> list[tuple[str, str]]:
    return [(fid, fact_text(fid, fact_by_id)) for fid in plan_fact_ids(corpus, role)]


def operation_key(corpus_sha256: str, fact_id: str) -> str:
    key = f"t2343-{corpus_sha256[:12]}-{fact_id}"
    if not _OPERATION_KEY.match(key):
        raise ValueError(f"fact id {fact_id!r} cannot form a valid operation_key")
    return key


def provenance(corpus_sha256: str, fact_id: str) -> str:
    return f"T23.43 synthetic qualification corpus {corpus_sha256[:12]} fact {fact_id}"


def slug_fact_key(slug: object) -> str | None:
    """The server-assigned fact id inside a recall result slug, or None if
    the slug does not have the entity-less `source-<sha256>` shape. Facts are
    seeded without an entity on purpose: an entity would replace this slug
    with the entity's own, which several facts could share."""
    if isinstance(slug, str) and slug.startswith(SOURCE_SLUG_PREFIX):
        key = slug[len(SOURCE_SLUG_PREFIX):]
        if _SHA256_HEX.match(key):
            return key
    return None


def resolve_result(result: dict, id_map: dict[str, str]) -> str:
    """Corpus fact id for one recall result, or `unresolved:<digest>` when it
    is not in the receipt. An unresolved value is deliberately left distinguishable
    from every corpus id so it can never score as a hit, and is always
    forbidden content for an empty case."""
    slug = result.get("slug")
    key = slug_fact_key(slug)
    if key is not None and key in id_map:
        return id_map[key]
    # A digest, never the slug: the slug is server-controlled text.
    return "unresolved:" + _sha256_bytes(str(slug).encode("utf-8"))[:12]


def resolve_fact(fact: dict, id_map: dict[str, str]) -> str:
    """Same rule as resolve_result, for an entry of recall's facts arm."""
    fid = fact.get("fact_id")
    if isinstance(fid, str) and fid in id_map:
        return id_map[fid]
    return "unresolved:" + _sha256_bytes(str(fid).encode("utf-8"))[:12]


def recall_query(client: MCPClient, guard_check, query: str, id_map: dict[str, str]) -> tuple[list[str], list[str]]:
    """One search recall. Returns (result ids, fact ids) as corpus ids or
    `unresolved:<digest>`. Raises SeedBlocked on a degraded search (the
    results would be keyword-only, not an embedding measurement) or a
    malformed envelope."""
    guard_check()
    resp = _args_ok(client.call_tool("recall", {"query": query, "limit": RECALL_LIMIT}), "recall")
    if resp.get("search_degraded"):
        raise ProviderUnavailable("provider unavailable: recall reported search_degraded; results would be keyword-only")
    results, facts = resp.get("results"), resp.get("facts")
    if not isinstance(results, list) or not isinstance(facts, list):
        raise SeedBlocked("recall response lacks results/facts lists")
    return (
        [resolve_result(r, id_map) for r in results if isinstance(r, dict)],
        [resolve_fact(f, id_map) for f in facts if isinstance(f, dict)],
    )


def _sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def receipt_bytes(receipt: dict) -> bytes:
    return (json.dumps(receipt, indent=2, sort_keys=True) + "\n").encode("utf-8")


def receipt_confirmation(receipt: dict) -> str:
    """The exact string a manifest's corpus_seeded_confirmation must carry."""
    return "sha256:" + _sha256_bytes(receipt_bytes(receipt))


def write_receipt(path: Path, receipt: dict) -> str:
    """Creates the file exclusively (never overwrites a prior receipt) and
    returns its confirmation string."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "xb") as fh:
        fh.write(receipt_bytes(receipt))
    return receipt_confirmation(receipt)


def load_receipt(path: Path, confirmation: str) -> tuple[dict | None, list[str]]:
    """Loads a receipt and checks its bytes hash to `confirmation`."""
    try:
        raw = Path(path).read_bytes()
    except OSError as e:
        return None, [f"seed receipt {str(path)!r} is unreadable: {e.strerror or e}"]
    actual = "sha256:" + _sha256_bytes(raw)
    if confirmation != actual:
        return None, [f"seed receipt hash {actual} does not match corpus_seeded_confirmation {confirmation!r}"]
    try:
        receipt = json.loads(raw)
    except json.JSONDecodeError:
        return None, ["seed receipt is not valid JSON"]
    if not isinstance(receipt, dict):
        return None, ["seed receipt is not a JSON object"]
    return receipt, []


def verify_receipt(
    receipt: dict,
    *,
    role: str,
    corpus: dict,
    corpus_sha256: str,
    facts_sha256: str,
    endpoint_url: str,
    credential_secret_ref: str,
) -> list[str]:
    """Problems that make a receipt unusable as live-run authority. Empty
    means the receipt is structurally sound and claims a complete,
    semantically indexed, exactly inventoried seed of this corpus into this
    exact endpoint for this role.

    Scope: the receipt is a hash-bound record, not proof. Its hash shows it
    was not edited after the manifest recorded it; it does not show anyone
    approved it, and it cannot show the harness produced it. The live run
    therefore re-inventories each account before scoring instead of trusting
    the receipt's claim about account state. Every field is type-checked
    before use: a malformed receipt yields problems, never an exception."""
    problems: list[str] = []
    try:
        _verify_receipt(receipt, role, corpus, corpus_sha256, facts_sha256, endpoint_url, credential_secret_ref, problems)
    except (TypeError, KeyError, AttributeError, ValueError):
        problems.append("seed receipt has a malformed nested structure")
    return problems


def _verify_receipt(receipt, role, corpus, corpus_sha256, facts_sha256, endpoint_url, credential_secret_ref, problems) -> None:
    def need(cond: bool, msg: str) -> bool:
        if not cond:
            problems.append(msg)
        return cond

    need(_is_int(receipt.get("schema_version")) and receipt.get("schema_version") == RECEIPT_SCHEMA_VERSION, f"seed receipt schema_version is not the integer {RECEIPT_SCHEMA_VERSION}")
    need(receipt.get("role") == role, f"seed receipt role is not {role!r}")
    need(receipt.get("corpus_sha256") == corpus_sha256, "seed receipt corpus_sha256 does not match the frozen corpus")
    need(receipt.get("facts_sha256") == facts_sha256, "seed receipt facts_sha256 does not match the frozen facts")
    need(receipt.get("endpoint_url") == endpoint_url, "seed receipt endpoint_url is not this target's exact endpoint_url")
    need(
        receipt.get("credential_secret_ref") == credential_secret_ref,
        "seed receipt credential_secret_ref is not this target's credential reference",
    )
    need(receipt.get("complete") is True, "seed receipt is not marked complete (a partial seed cannot authorize a live run)")

    usage = receipt.get("usage")
    if not need(
        isinstance(usage, dict) and all(_nonneg_int(usage.get(k)) for k in _USAGE_FIELDS),
        "seed receipt usage counters must be nonnegative integers (a bad count would forge budget credit)",
    ):
        return

    prov = receipt.get("ledger")
    need(
        isinstance(prov, dict)
        and isinstance(prov.get("ledger_id"), str) and prov.get("ledger_id")
        and isinstance(prov.get("identity_sha256"), str) and _SHA256_HEX.match(prov["identity_sha256"])
        and isinstance(prov.get("invocation"), str) and prov.get("invocation")
        and prov.get("role") == role
        and _nonneg_int(prov.get("calls")) and _nonneg_int(prov.get("request_bytes"))
        and (prov.get("calls"), prov.get("request_bytes")) == (usage["calls_made"], usage["request_bytes"]),
        "seed receipt has no ledger-backed spend provenance that matches its usage counters; a receipt "
        "cannot establish what was spent (re-seed under a ledger)",
    )

    proof = receipt.get("empty_account_proof")
    need(isinstance(proof, dict) and proof.get("passed") is True, "seed receipt has no passing empty-account proof taken before the first write")

    seeded, id_map, inv = receipt.get("seeded"), receipt.get("id_map"), receipt.get("post_seed_inventory")
    if not need(
        isinstance(seeded, list) and all(isinstance(x, dict) for x in seeded)
        and isinstance(id_map, dict) and isinstance(inv, dict)
        and isinstance(inv.get("fact_ids"), list),
        "seed receipt seeded/id_map/post_seed_inventory have the wrong shape",
    ):
        return
    need(
        all(isinstance(k, str) and _SHA256_HEX.match(k) and isinstance(v, str) for k, v in id_map.items())
        and all(isinstance(x, str) and _SHA256_HEX.match(x) for x in inv["fact_ids"]),
        "seed receipt ids must be 64-hex strings",
    )
    expected_ids = plan_fact_ids(corpus, role)
    need(
        sorted(str(x.get("corpus_fact_id")) for x in seeded) == expected_ids,
        f"seed receipt facts do not equal the {len(expected_ids)} corpus facts the {role} account must hold",
    )
    need(
        all(x.get("status") == "inserted" and x.get("search_state") == "semantic" for x in seeded),
        "every seeded fact must be status=inserted with search_state=semantic (a vector was stored)",
    )
    need(
        len(id_map) == len(seeded) and all(id_map.get(x.get("remember_id")) == x.get("corpus_fact_id") for x in seeded),
        "seed receipt id_map is inconsistent with its seeded list",
    )
    need(
        inv.get("matches_seeded") is True and sorted(inv["fact_ids"]) == sorted(id_map),
        "seed receipt has no passing post-seed inventory proof",
    )
    need(
        receipt.get("forgotten_targets_sha256") == ft.targets_sha256(),
        "seed receipt was not produced from the current forgotten-target/sentinel texts",
    )
    if role == ROLE_EMPTY_CASE:
        # Distinct credentials do not prove distinct accounts. Requiring that
        # this account was proven empty AFTER the positive account was fully
        # seeded rules out two credentials that resolve to one account
        # (the second emptiness proof would have failed). It is behavioural
        # evidence inside a hash-bound record, not an account identifier:
        # the recall/remember contract exposes none.
        need(
            receipt.get("preceded_by_seeded_facts") == len(plan_fact_ids(corpus, ROLE_POSITIVE)),
            "empty-case receipt was not taken after the positive account was fully seeded",
        )
        cases = empty_case_ids(corpus)
        expire_cases, forget_cases = ft.cases_with_mode(ft.MODE_EXPIRE), ft.cases_with_mode(ft.MODE_FORGET)
        presence, forgot = receipt.get("presence"), receipt.get("forget")
        post, cross = receipt.get("post_forget"), receipt.get("cross_account_probe")
        expiry, echeck = receipt.get("expiry"), receipt.get("expiry_check")
        if not need(
            isinstance(presence, list) and isinstance(forgot, list) and isinstance(post, dict) and isinstance(cross, dict)
            and isinstance(post.get("cases"), list) and isinstance(expiry, dict) and isinstance(echeck, dict)
            and isinstance(echeck.get("cases"), list),
            "empty-case receipt lacks its presence/forget/expiry/expiry_check/post_forget/cross_account_probe proofs",
        ):
            return
        need(
            [p.get("case_id") for p in presence if isinstance(p, dict)] == cases
            and all(isinstance(p, dict) and _is_int(p.get("rank")) and 1 <= p["rank"] <= RECALL_LIMIT for p in presence),
            "every expected-empty target must be retrievable (top-5) before it is removed",
        )
        need(
            [f.get("case_id") for f in forgot if isinstance(f, dict)] == forget_cases
            and all(isinstance(f, dict) and f.get("expired") is True and f.get("remember_id") in id_map for f in forgot),
            "every forget-mode target must have been forgotten (forget expired=true)",
        )
        _verify_expiry(receipt, expiry, echeck, expire_cases, forget_cases, need)
        need(
            post.get("inventory_total") == 0
            and [c.get("case_id") for c in post["cases"] if isinstance(c, dict)] == cases
            and all(isinstance(c, dict) and c.get("results_returned") == 0 and c.get("facts_returned") == 0 for c in post["cases"]),
            "post-removal proof must show an empty inventory and zero results and facts for every expected-empty query",
        )
        need(
            cross.get("passed") is True and cross.get("results_returned") == 0 and cross.get("facts_returned") == 0,
            "the cross-account sentinel must be unreachable from the empty-case account",
        )


def _is_number(v: object) -> bool:
    return isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v)


def _verify_expiry(receipt: dict, expiry: dict, echeck: dict, expire_cases: list, forget_cases: list, need) -> None:
    """The TTL half of the forgotten/expired proof. Every field is checked
    for type as well as value: the wait's evidence is the local clock reading
    taken after it, so a receipt cannot claim an expiry it did not wait for."""
    valid_until, epoch = expiry.get("valid_until"), expiry.get("valid_until_epoch")
    need(
        expiry.get("cases") == expire_cases
        and expiry.get("ttl_seconds") == ft.EXPIRY_TTL_SECONDS and _is_int(expiry.get("ttl_seconds"))
        and expiry.get("margin_seconds") == ft.EXPIRY_MARGIN_SECONDS and _is_int(expiry.get("margin_seconds")),
        "receipt expiry plan does not match the reviewed expiry cases, TTL and margin",
    )
    need(
        isinstance(valid_until, str) and _is_int(epoch) and parse_iso_utc(valid_until) == epoch and iso_utc(epoch) == valid_until,
        "receipt valid_until is not one consistent absolute RFC 3339 instant",
    )
    seeded = {x.get("corpus_fact_id"): x for x in receipt["seeded"]}
    need(
        all(
            seeded.get(ft.target_id(c), {}).get("valid_until") == valid_until
            and seeded.get(ft.target_id(c), {}).get("valid_until_returned") == valid_until
            for c in expire_cases
        ),
        "every expire-mode target must have been remembered with the fixed expiry and the service must have reported it back",
    )
    need(
        all("valid_until" not in seeded.get(ft.target_id(c), {"valid_until": 1}) for c in forget_cases),
        "a forget-mode target must not carry an expiry",
    )
    waited = expiry.get("probed_after_valid_until_seconds")
    need(
        _is_number(waited) and waited >= ft.EXPIRY_MARGIN_SECONDS,
        "the post-expiry probes were not taken a full margin after the expiry instant",
    )
    need(
        echeck.get("expired_absent_from_inventory") is True
        and echeck.get("forget_targets_still_present") is True
        and echeck.get("unexpected_facts") == 0 and _is_int(echeck.get("unexpected_facts"))
        and echeck.get("inventory_total") == len(forget_cases) and _is_int(echeck.get("inventory_total")),
        "the expiry check must show every expire-mode target gone while every forget-mode target was still present",
    )
    need(
        [c.get("case_id") for c in echeck["cases"] if isinstance(c, dict)] == expire_cases
        and all(
            isinstance(c, dict) and c.get("target_in_results") is False and c.get("target_in_facts") is False
            for c in echeck["cases"]
        ),
        "every expire-mode target must be absent from search results and facts after its expiry",
    )


def _args_ok(resp: object, what: str) -> dict:
    if not isinstance(resp, dict):
        raise SeedBlocked(f"{what}: response is not a JSON object")
    return resp


def prove_empty(client: MCPClient, guard_check) -> dict:
    """Empty-account proof: no world-visible unexpired fact and an empty
    search arm, with search not degraded. `recall` with no query returns the
    facts arm only; with a query it also searches pages and claims, so both
    arms are probed. Raises SeedBlocked with a fixed reason on any deviation.

    Scope: this proves the account behind THIS credential held nothing
    visible to recall at this moment. It does not prove the account is
    otherwise unused (private or expired facts are invisible to recall and
    also cannot be retrieved, so they cannot affect scoring)."""
    guard_check()
    facts_resp = _args_ok(client.call_tool("recall", {"limit": 1}), "empty-account fact probe")
    guard_check()
    probe = _args_ok(client.call_tool("recall", {"query": EMPTY_PROBE_QUERY, "limit": 5}), "empty-account search probe")
    results = probe.get("results")
    facts = facts_resp.get("facts")
    proof = {
        "facts_total": _safe_int(facts_resp.get("total")),
        "facts_returned": len(facts) if isinstance(facts, list) else None,
        "search_results_returned": len(results) if isinstance(results, list) else None,
        "search_degraded": bool(probe.get("search_degraded")),
    }
    if proof["search_degraded"]:
        raise ProviderUnavailable("provider unavailable: recall reported search_degraded; results would be keyword-only")
    proof["passed"] = (
        proof["facts_total"] == 0 and proof["facts_returned"] == 0 and proof["search_results_returned"] == 0
    )
    if not proof["passed"]:
        raise SeedBlocked(
            f"account is not empty (facts_total={proof['facts_total']}, "
            f"search_results={proof['search_results_returned']}); refusing to seed"
        )
    return proof


def fetch_inventory(client: MCPClient, guard_check) -> dict[str, str]:
    """Every world-visible unexpired fact currently in the account, as
    {fact_id: fact text}. Ids that are not 64-hex are rejected outright."""
    guard_check()
    resp = _args_ok(client.call_tool("recall", {"limit": INVENTORY_LIMIT}), "inventory")
    facts = resp.get("facts")
    if not isinstance(facts, list) or resp.get("total") != len(facts) or not all(isinstance(f, dict) for f in facts):
        raise SeedBlocked("inventory: malformed facts arm")
    if len(facts) >= INVENTORY_LIMIT:
        raise SeedBlocked(f"inventory: {len(facts)} facts reached the probe limit {INVENTORY_LIMIT}; account is not the seeded corpus")
    inventory: dict[str, str] = {}
    for f in facts:
        fid = f.get("fact_id")
        if not (isinstance(fid, str) and _SHA256_HEX.match(fid)):
            raise SeedBlocked("inventory: a fact_id is not a 64-hex id")
        inventory[fid] = f.get("fact") if isinstance(f.get("fact"), str) else ""
    return inventory


@dataclass
class SeedOutcome:
    receipt: dict
    blocked_reason: str | None
    blocked_kind: str | None = None


def seed_target(
    client: MCPClient,
    guard_check,
    *,
    role: str,
    corpus: dict,
    fact_by_id: dict,
    corpus_sha256: str,
    facts_sha256: str,
    source_sha: str,
    recorded_at: str,
    credential_secret_ref: str,
    preceded_by_seeded_facts: int,
    clock=time.time,
    sleep=time.sleep,
    remaining_seconds=None,
) -> SeedOutcome:
    """Seeds one account. `guard_check` raises SeedBlocked when a cap is
    reached between steps; the client itself precharges every request. Always
    returns a receipt, marked complete only when every proof held.
    `preceded_by_seeded_facts` is how many facts were already seeded into the
    other account before this account's emptiness proof (0 for the first).

    `clock` is the wall clock the expiry instant is computed from and waited
    on, `sleep` how the wait passes, and `remaining_seconds` (a callable) how
    much of the total elapsed cap is left; tests inject a shared fake clock,
    production uses the real ones and the budget guard's."""
    plan = plan_facts(corpus, fact_by_id, role)
    receipt: dict = {
        "schema_version": RECEIPT_SCHEMA_VERSION,
        "role": role,
        "corpus_sha256": corpus_sha256,
        "facts_sha256": facts_sha256,
        "forgotten_targets_sha256": ft.targets_sha256(),
        "endpoint_url": client.endpoint_url,
        "credential_secret_ref": credential_secret_ref,
        "source_sha": source_sha,
        "recorded_at": recorded_at,
        "planned_facts": len(plan),
        "preceded_by_seeded_facts": preceded_by_seeded_facts,
        "complete": False,
        "empty_account_proof": None,
        "seeded": [],
        "id_map": {},
        "post_seed_inventory": None,
        "usage": None,
    }
    reason: str | None = None
    kind: str | None = None
    expiry: Expiry | None = None
    try:
        guard_check()
        client.initialize()
        receipt["empty_account_proof"] = prove_empty(client, guard_check)
        if role == ROLE_EMPTY_CASE:
            expiry = plan_expiry(clock)
            receipt["expiry"] = {
                "cases": ft.cases_with_mode(ft.MODE_EXPIRE),
                "ttl_seconds": ft.EXPIRY_TTL_SECONDS,
                "margin_seconds": ft.EXPIRY_MARGIN_SECONDS,
                "valid_until": expiry.iso,
                "valid_until_epoch": expiry.epoch,
            }
        for fid, text in plan:
            guard_check()
            args = {
                "fact": text,
                "provenance": provenance(corpus_sha256, fid),
                "operation_key": operation_key(corpus_sha256, fid),
            }
            mode = target_mode(fid)
            if mode == ft.MODE_EXPIRE:
                args["ttl"] = expiry.iso
            resp = _args_ok(client.call_tool("remember", args), f"remember {fid}")
            rid = resp.get("id")
            status = fixed(resp.get("status"), _REMEMBER_STATUSES)
            state = fixed(resp.get("search_state"), _SEARCH_STATES)
            if status != "inserted" or not isinstance(rid, str) or not _SHA256_HEX.match(rid):
                raise SeedBlocked(f"remember {fid}: status={status}; an empty account must insert every fact")
            if state != "semantic" or resp.get("expired"):
                raise ProviderUnavailable(
                    f"remember {fid}: search_state={state}; no vector was stored, "
                    "so qualification would measure keyword search, not embeddings"
                )
            entry = {
                "corpus_fact_id": fid,
                "remember_id": rid,
                "status": status,
                "search_state": state,
                "operation_key": operation_key(corpus_sha256, fid),
            }
            if mode == ft.MODE_EXPIRE:
                returned = parse_iso_utc(resp.get("valid_until"))
                if returned is None or abs(returned - expiry.epoch) > 0.5:
                    raise SeedBlocked(
                        f"remember {fid}: the service did not report the requested expiry; "
                        "a fact that may never expire cannot be shown expired"
                    )
                entry["valid_until"] = expiry.iso
                entry["valid_until_returned"] = iso_utc(returned)
            receipt["seeded"].append(entry)
            receipt["id_map"][rid] = fid
        found = fetch_inventory(client, guard_check)
        expected = {x["remember_id"]: dict(plan)[x["corpus_fact_id"]] for x in receipt["seeded"]}
        receipt["post_seed_inventory"] = {
            "fact_ids": sorted(fid for fid in found if fid in expected) if found != expected else sorted(found),
            "matches_seeded": found == expected,
        }
        if found != expected:
            if expiry is not None and clock() >= expiry.epoch:
                raise SeedBlocked(
                    "post-seed inventory differs from what was seeded, and the expiry instant has already passed: "
                    "the TTL is too short for this endpoint's latency"
                )
            raise SeedBlocked("post-seed inventory differs from what was seeded (extra, missing, or altered facts)")
        if role == ROLE_EMPTY_CASE:
            _remove_and_prove_absent(
                client, guard_check, corpus, receipt, expiry, clock=clock, sleep=sleep, remaining_seconds=remaining_seconds
            )
        receipt["complete"] = True
    except SeedBlocked as e:
        reason, kind = str(e), blocked_kind(e)
    except BudgetExceeded as e:
        reason, kind = str(e), "budget"  # guard-authored text
    except MCPError as e:
        reason, kind = f"live call failed: {e}", "other"  # MCPError text is fixed classes only
    except Exception as e:  # noqa: BLE001 -- the partial receipt and its usage must survive any setup or parse error
        reason, kind = f"unexpected error: {type(e).__name__}", "unexpected"  # class name only, never its text
    receipt["usage"] = {
        "calls_made": client.calls_made,
        "request_bytes": client.total_request_bytes,
        "response_bytes": client.total_response_bytes,
    }
    return SeedOutcome(receipt, reason, kind)


def _await_expiry(guard_check, expiry: Expiry, clock, sleep, remaining_seconds) -> None:
    """Waits until the local wall clock is EXPIRY_MARGIN_SECONDS past the
    expiry instant. The wait is refused up front when the elapsed cap cannot
    hold it plus the probe tail, and it sleeps in short chunks so a cap that
    is reached mid-wait stops it. Nothing here shortens or skips the wait."""
    target = expiry.epoch + ft.EXPIRY_MARGIN_SECONDS
    needed = target - clock()
    if remaining_seconds is not None and needed + ft.EXPIRY_TAIL_SECONDS > remaining_seconds():
        raise SeedBlocked(
            f"max_elapsed_seconds leaves too little time: the expiry wait needs {math.ceil(max(needed, 0))}s "
            f"plus a {ft.EXPIRY_TAIL_SECONDS}s tail for the post-expiry probes"
        )
    for _ in range(1000):
        now = clock()
        if now >= target:
            return
        guard_check()
        sleep(min(target - now, WAIT_CHUNK_SECONDS))
    raise SeedBlocked("the wall clock did not advance during the expiry wait")


def _remove_and_prove_absent(
    client: MCPClient, guard_check, corpus: dict, receipt: dict, expiry: Expiry, *, clock, sleep, remaining_seconds
) -> None:
    """The forgotten/expired protocol for the empty-case account. Presence is
    shown first, by search, because absence afterwards proves nothing about a
    fact the index could never have returned. Then, in order: the expire-mode
    targets are left to lapse and shown absent while the forget-mode targets
    are still present (a control); the forget-mode targets are forgotten; and
    only then is the account held to the strict zero-current-facts standard."""
    id_map = receipt["id_map"]
    cases = empty_case_ids(corpus)
    expire_cases = ft.cases_with_mode(ft.MODE_EXPIRE)
    forget_cases = ft.cases_with_mode(ft.MODE_FORGET)
    if sorted(expire_cases + forget_cases) != sorted(cases) or not expire_cases or not forget_cases:
        raise SeedBlocked("the supplemental target plan does not cover the corpus's expected-empty cases with both removal modes")
    by_case = {cid: next(k for k, v in id_map.items() if v == ft.target_id(cid)) for cid in cases}
    queries = {c["id"]: c["query"] for c in corpus["cases"] if c["category"] == "empty"}

    receipt["presence"] = []
    for cid in cases:
        ids, _facts = recall_query(client, guard_check, queries[cid], id_map)
        if ft.target_id(cid) not in ids:
            late = ft.TARGET_MODE[cid] == ft.MODE_EXPIRE and clock() >= expiry.epoch
            raise SeedBlocked(
                f"presence not demonstrated for {cid}: the seeded target was not retrieved before it was removed, "
                "so its absence afterwards would prove nothing"
                + ("; its expiry passed first, so the TTL is too short for this endpoint's latency" if late else "")
            )
        receipt["presence"].append({"case_id": cid, "target_id": ft.target_id(cid), "rank": ids.index(ft.target_id(cid)) + 1})

    _await_expiry(guard_check, expiry, clock, sleep, remaining_seconds)
    probed_at = clock()
    receipt["expiry"]["probed_after"] = iso_utc(probed_at)
    receipt["expiry"]["probed_after_valid_until_seconds"] = round(probed_at - expiry.epoch, 3)

    expire_ids = {by_case[c] for c in expire_cases}
    forget_ids = {by_case[c] for c in forget_cases}
    found = fetch_inventory(client, guard_check)
    expiry_cases = []
    for cid in expire_cases:
        ids, facts = recall_query(client, guard_check, queries[cid], id_map)
        expiry_cases.append(
            {"case_id": cid, "target_in_results": ft.target_id(cid) in ids, "target_in_facts": ft.target_id(cid) in facts}
        )
    receipt["expiry_check"] = {
        "inventory_total": len(found),
        "expired_absent_from_inventory": not (expire_ids & set(found)),
        "forget_targets_still_present": forget_ids <= set(found),
        "unexpected_facts": len(set(found) - expire_ids - forget_ids),
        "cases": expiry_cases,
    }
    check = receipt["expiry_check"]
    if not check["expired_absent_from_inventory"] or any(c["target_in_results"] or c["target_in_facts"] for c in expiry_cases):
        raise SeedBlocked(
            "an expire-mode target is still visible after its expiry plus margin; the service did not expire it "
            "(or its clock is more than the margin behind), and no expiry can be claimed"
        )
    if not check["forget_targets_still_present"] or check["unexpected_facts"]:
        raise SeedBlocked(
            "the account changed while the expiry was pending (a forget-mode target vanished or an unknown fact "
            "appeared), so the expiry proves nothing"
        )

    receipt["forget"] = []
    for cid in forget_cases:
        guard_check()
        resp = _args_ok(
            client.call_tool("forget", {"id": by_case[cid], "reason": "T23.43 expected-empty qualification: forgotten target"}),
            f"forget {cid}",
        )
        if resp.get("expired") is not True:
            raise SeedBlocked(f"forget {cid}: the hosted service did not report the fact expired")
        receipt["forget"].append({"case_id": cid, "remember_id": by_case[cid], "expired": True})

    remaining = fetch_inventory(client, guard_check)
    post_cases = []
    for cid in cases:
        ids, facts = recall_query(client, guard_check, queries[cid], id_map)
        post_cases.append({"case_id": cid, "results_returned": len(ids), "facts_returned": len(facts)})
    receipt["post_forget"] = {"inventory_total": len(remaining), "cases": post_cases}
    if remaining or any(c["results_returned"] or c["facts_returned"] for c in post_cases):
        raise SeedBlocked(
            f"a removed fact is still visible (inventory={len(remaining)}, "
            f"query hits={sum(c['results_returned'] + c['facts_returned'] for c in post_cases)}); "
            "forget or expiry did not remove it"
        )

    ids, facts = recall_query(client, guard_check, ft.SENTINEL_FACT_TEXT, id_map)
    receipt["cross_account_probe"] = {
        "results_returned": len(ids), "facts_returned": len(facts), "passed": not ids and not facts,
    }
    if ids or facts:
        raise SeedBlocked("the cross-account sentinel is reachable from the empty-case account")


def _origin(url: str) -> str:
    from urllib.parse import urlsplit

    p = urlsplit(url)
    return f"{p.scheme}://{p.netloc}"
