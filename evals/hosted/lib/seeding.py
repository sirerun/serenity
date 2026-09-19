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
  query the seed run remembers a synthetic target fact (lib/forgotten_targets),
  proves it present (inventory and search retrieval), forgets it through the
  ordinary `forget` API, then proves it absent from both recall arms. The
  account ends holding zero current facts, so a live run scores every
  expected-empty query strictly: any returned result or fact is a violation.
  The sentinel is then queried against this account to show another
  account's content is unreachable.

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

import hashlib
import json
import re
from dataclasses import dataclass
from pathlib import Path

from . import forgotten_targets as ft
from .mcp_client import BudgetExceeded, MCPClient, MCPError
from .scoring import K as RECALL_LIMIT

ROLE_POSITIVE = "positive"
ROLE_EMPTY_CASE = "empty_case"
ROLES = (ROLE_POSITIVE, ROLE_EMPTY_CASE)
RECEIPT_SCHEMA_VERSION = 1
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


class SeedBlocked(RuntimeError):
    """Seeding stopped before completing; the message is the BLOCKED reason."""


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
        raise SeedBlocked("provider unavailable: recall reported search_degraded; results would be keyword-only")
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

    need(_is_int(receipt.get("schema_version")) and receipt.get("schema_version") == RECEIPT_SCHEMA_VERSION, "seed receipt schema_version is not the integer 1")
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
        presence, forgot = receipt.get("presence"), receipt.get("forget")
        post, cross = receipt.get("post_forget"), receipt.get("cross_account_probe")
        if need(
            isinstance(presence, list) and isinstance(forgot, list) and isinstance(post, dict) and isinstance(cross, dict)
            and isinstance(post.get("cases"), list),
            "empty-case receipt lacks its presence/forget/post_forget/cross_account_probe proofs",
        ):
            need(
                [p.get("case_id") for p in presence if isinstance(p, dict)] == cases
                and all(isinstance(p, dict) and _is_int(p.get("rank")) and 1 <= p["rank"] <= RECALL_LIMIT for p in presence),
                "every expected-empty target must be retrievable (top-5) before it is forgotten",
            )
            need(
                [f.get("case_id") for f in forgot if isinstance(f, dict)] == cases
                and all(isinstance(f, dict) and f.get("expired") is True and f.get("remember_id") in id_map for f in forgot),
                "every expected-empty target must have been forgotten (forget expired=true)",
            )
            need(
                post.get("inventory_total") == 0
                and [c.get("case_id") for c in post["cases"] if isinstance(c, dict)] == cases
                and all(isinstance(c, dict) and c.get("results_returned") == 0 and c.get("facts_returned") == 0 for c in post["cases"]),
                "post-forget proof must show an empty inventory and zero results and facts for every expected-empty query",
            )
            need(
                cross.get("passed") is True and cross.get("results_returned") == 0 and cross.get("facts_returned") == 0,
                "the cross-account sentinel must be unreachable from the empty-case account",
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
        raise SeedBlocked("provider unavailable: recall reported search_degraded; results would be keyword-only")
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
) -> SeedOutcome:
    """Seeds one account. `guard_check` raises SeedBlocked when a cap is
    reached between steps; the client itself precharges every request. Always
    returns a receipt, marked complete only when every proof held.
    `preceded_by_seeded_facts` is how many facts were already seeded into the
    other account before this account's emptiness proof (0 for the first)."""
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
    try:
        guard_check()
        client.initialize()
        receipt["empty_account_proof"] = prove_empty(client, guard_check)
        for fid, text in plan:
            guard_check()
            resp = _args_ok(
                client.call_tool(
                    "remember",
                    {
                        "fact": text,
                        "provenance": provenance(corpus_sha256, fid),
                        "operation_key": operation_key(corpus_sha256, fid),
                    },
                ),
                f"remember {fid}",
            )
            rid = resp.get("id")
            status = fixed(resp.get("status"), _REMEMBER_STATUSES)
            state = fixed(resp.get("search_state"), _SEARCH_STATES)
            if status != "inserted" or not isinstance(rid, str) or not _SHA256_HEX.match(rid):
                raise SeedBlocked(f"remember {fid}: status={status}; an empty account must insert every fact")
            if state != "semantic" or resp.get("expired"):
                raise SeedBlocked(
                    f"remember {fid}: search_state={state}; no vector was stored, "
                    "so qualification would measure keyword search, not embeddings"
                )
            receipt["seeded"].append(
                {
                    "corpus_fact_id": fid,
                    "remember_id": rid,
                    "status": status,
                    "search_state": state,
                    "operation_key": operation_key(corpus_sha256, fid),
                }
            )
            receipt["id_map"][rid] = fid
        found = fetch_inventory(client, guard_check)
        expected = {x["remember_id"]: dict(plan)[x["corpus_fact_id"]] for x in receipt["seeded"]}
        receipt["post_seed_inventory"] = {
            "fact_ids": sorted(fid for fid in found if fid in expected) if found != expected else sorted(found),
            "matches_seeded": found == expected,
        }
        if found != expected:
            raise SeedBlocked("post-seed inventory differs from what was seeded (extra, missing, or altered facts)")
        if role == ROLE_EMPTY_CASE:
            _forget_and_prove_absent(client, guard_check, corpus, receipt)
        receipt["complete"] = True
    except SeedBlocked as e:
        reason = str(e)
    except BudgetExceeded as e:
        reason = str(e)  # guard-authored text
    except MCPError as e:
        reason = f"live call failed: {e}"  # MCPError text is fixed classes only
    receipt["usage"] = {
        "calls_made": client.calls_made,
        "request_bytes": client.total_request_bytes,
        "response_bytes": client.total_response_bytes,
    }
    return SeedOutcome(receipt, reason)


def _forget_and_prove_absent(client: MCPClient, guard_check, corpus: dict, receipt: dict) -> None:
    """The forgotten-fact protocol for the empty-case account. Presence is
    shown first, by search, because absence afterwards proves nothing about a
    fact the index could never have returned."""
    id_map = receipt["id_map"]
    by_case = {cid: next(k for k, v in id_map.items() if v == ft.target_id(cid)) for cid in empty_case_ids(corpus)}
    queries = {c["id"]: c["query"] for c in corpus["cases"] if c["category"] == "empty"}

    receipt["presence"] = []
    for cid in empty_case_ids(corpus):
        ids, _facts = recall_query(client, guard_check, queries[cid], id_map)
        if ft.target_id(cid) not in ids:
            raise SeedBlocked(
                f"presence not demonstrated for {cid}: the seeded target was not retrieved before forgetting, "
                "so its absence afterwards would prove nothing"
            )
        receipt["presence"].append({"case_id": cid, "target_id": ft.target_id(cid), "rank": ids.index(ft.target_id(cid)) + 1})

    receipt["forget"] = []
    for cid in empty_case_ids(corpus):
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
    for cid in empty_case_ids(corpus):
        ids, facts = recall_query(client, guard_check, queries[cid], id_map)
        post_cases.append({"case_id": cid, "results_returned": len(ids), "facts_returned": len(facts)})
    receipt["post_forget"] = {"inventory_total": len(remaining), "cases": post_cases}
    if remaining or any(c["results_returned"] or c["facts_returned"] for c in post_cases):
        raise SeedBlocked(
            f"a forgotten fact is still visible (inventory={len(remaining)}, "
            f"query hits={sum(c['results_returned'] + c['facts_returned'] for c in post_cases)}); forget did not remove it"
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
