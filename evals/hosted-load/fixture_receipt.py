#!/usr/bin/env python3
"""Summarize independent fixture verification reports into one labeled receipt (T23.60).

Inputs are the JSON reports written by `fixtureprep verify -report` (numbers read from the fixture directory by the
verifier, never from the preparer's manifest) and, optionally, timing files copied from the preparer's own clock. The
receipt states, in every section, which profile it describes: a reduced smoke never satisfies the full 90,000-memory,
29-brain cardinalities, the storage and history gap is reported apart from the memory count, and provider quality is
unqualified because the vectors come from an infrastructure-only hash embedder.

It makes no API call, contacts no service and reads no credential.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

REPORT_SCHEMA = "serenity-hosted-load-fixture-verification"
FULL_ACCOUNTS, FULL_BRAINS, FULL_FACTS = 14, 29, 90000


class ReceiptError(ValueError):
    pass


def _load(path: Path, expect: str) -> dict:
    report = json.loads(path.read_text())
    if report.get("schema") != REPORT_SCHEMA:
        raise ReceiptError(f"{path.name}: not a fixture verification report")
    if report.get("pass") is not True:
        failed = [c["id"] for c in report.get("checks", []) if not c.get("pass")]
        raise ReceiptError(f"{path.name}: verification did not pass ({failed[:5]})")
    if report.get("expect") != expect or report.get("marker_profile") != expect:
        raise ReceiptError(f"{path.name}: expected a {expect} report, got expect={report.get('expect')} marker={report.get('marker_profile')}")
    report["_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
    return report


def _totals(report: dict) -> dict:
    t = report["totals"]
    return {
        "accounts": t["accounts"],
        "brains": t["brains"],
        "canonical_memory_facts": t["canonical_memory_facts"],
        "index_fact_chunks": t["index_fact_chunks"],
        "index_vectors_under_pin": t["index_vectors_under_pin"],
        "git_commits_all_brains": t["git_commits_all_brains"],
    }


def _accounts(report: dict) -> dict:
    by_plan: dict[str, dict] = {}
    for a in report["accounts"]:
        p = by_plan.setdefault(a["plan_id"], {"accounts": 0, "memories": 0, "memory_cap_each": a["memory_cap"], "accounts_at_memory_cap": 0, "storage_bytes": 0, "storage_quota_each": a["storage_quota_bytes"], "largest_share_of_storage_quota": 0.0})
        p["accounts"] += 1
        p["memories"] += a["memories"]
        p["accounts_at_memory_cap"] += 1 if a["at_or_over_memory_cap"] else 0
        p["storage_bytes"] += a["storage_bytes"]
        p["largest_share_of_storage_quota"] = max(p["largest_share_of_storage_quota"], round(a["storage_share_of_quota"], 6))
    return by_plan


def _gateway(report: dict) -> dict:
    """What the gateway's own Inventory and Entitlement code returned for the fixture, compared with the verifier's counts."""
    accts = report["accounts"]
    return {
        "accounts": len(accts),
        "inventory_memories_equal_verifier": sum(1 for a in accts if a["gateway_inventory_memories"] == a["memories"]),
        "inventory_storage_bytes_equal_verifier": sum(1 for a in accts if a["gateway_inventory_storage_bytes"] == a["storage_bytes"]),
        "entitlement_equals_planned_plan": sum(1 for a in accts if a["gateway_entitlement_plan"] == a["plan_id"]),
        "limit_inputs_refuse_a_nonreplay_remember": sum(1 for a in accts if a["gateway_limit_inputs_refuse_a_nonreplay_remember"]),
    }


def _profile(report: dict) -> dict:
    gap = report["storage_and_history_gap"]
    comp = gap["storage_bytes_by_component"]
    total = gap["storage_bytes_all_brains"]
    default = sum(b["canonical_facts"] for b in report["brains"] if b["index"] == 0)
    everything = sum(b["canonical_facts"] for b in report["brains"])
    return {
        "profile": report["marker_profile"],
        "reduced": report["reduced"],
        "satisfies_full_cardinality": report["satisfies_full_cardinality"],
        "verification_report_sha256": report["_sha256"],
        "embedder_pin": report["embedder_pin"],
        "embedder_dim": report["embedder_dim"],
        "commit_every": report["commit_every"],
        "fixture_identity": {
            "content_sha256": report["fixture_content_sha256"],
            "content_sha256_meaning": "sha256 over each brain's sorted fact SHA-256 list, in plan order. Two preparations of the same profile give the same value; brain ids, credentials, timestamps and Git history do not enter it.",
            "control_db_sha256": report["control_db_sha256"],
            "marker_sha256": report["marker_sha256"],
            "workload_sha256": report["workload_sha256"],
        },
        "totals": _totals(report),
        "by_plan": _accounts(report),
        "gateway_cross_check": _gateway(report),
        "facts_on_default_brains": default,
        "facts_on_other_brains": everything - default,
        "storage_and_history_gap": {
            "storage_bytes_all_brains": total,
            "allocated_bytes_all_brains": gap["allocated_bytes_all_brains"],
            "files_all_brains": gap["files_all_brains"],
            "storage_bytes_by_component": comp,
            "component_share_of_storage": {k: round(v / total, 4) for k, v in sorted(comp.items())},
            "accounts_at_or_over_storage_quota": gap["accounts_at_or_over_storage_quota"],
        },
    }


def build(full: dict, smoke: dict, sample_batched: dict, sample_per_write: dict, timings: dict[str, dict], source_sha: str, binaries: dict[str, str], full_per_write: dict | None = None, full_wide: dict | None = None) -> dict:
    t = _totals(full)
    if (t["accounts"], t["brains"], t["canonical_memory_facts"]) != (FULL_ACCOUNTS, FULL_BRAINS, FULL_FACTS) or not full["satisfies_full_cardinality"] or full["reduced"]:
        raise ReceiptError("the full report does not hold 14 accounts, 29 brains and 90,000 memories")
    if not smoke["reduced"] or smoke["satisfies_full_cardinality"] or smoke["totals"]["canonical_memory_facts"] >= FULL_FACTS:
        raise ReceiptError("the smoke report must be reduced and must not satisfy the full cardinalities")
    for name, sample in (("batched", sample_batched), ("per_write", sample_per_write)):
        if not sample["reduced"] or sample["satisfies_full_cardinality"]:
            raise ReceiptError(f"the {name} history sample must be a reduced smoke")
    if sample_per_write["commit_every"] != 1 or sample_batched["commit_every"] <= 1:
        raise ReceiptError("the per-write history sample must commit once per fact and the batched one must batch")
    if sample_batched["embedder_dim"] != sample_per_write["embedder_dim"]:
        raise ReceiptError("the two history samples must use the same vector width")
    if sample_batched["totals"]["canonical_memory_facts"] != sample_per_write["totals"]["canonical_memory_facts"]:
        raise ReceiptError("the two history samples must hold the same number of facts")
    gb, gp = sample_batched["storage_and_history_gap"], sample_per_write["storage_and_history_gap"]
    extra_commits = sample_per_write["totals"]["git_commits_all_brains"] - sample_batched["totals"]["git_commits_all_brains"]
    if extra_commits <= 0:
        raise ReceiptError("the per-write sample must have more commits than the batched sample")
    git_delta = gp["storage_bytes_by_component"][".git"] - gb["storage_bytes_by_component"][".git"]
    at_scale = None
    if full_per_write is not None:
        tp = _totals(full_per_write)
        if (tp["accounts"], tp["brains"], tp["canonical_memory_facts"]) != (FULL_ACCOUNTS, FULL_BRAINS, FULL_FACTS) or not full_per_write["satisfies_full_cardinality"] or full_per_write["reduced"]:
            raise ReceiptError("the per-write full report does not hold 14 accounts, 29 brains and 90,000 memories")
        if full_per_write["fixture_content_sha256"] != full["fixture_content_sha256"]:
            raise ReceiptError("the two full fixtures must hold identical facts: their content digests differ")
        if full_per_write["commit_every"] != 1 or full["commit_every"] <= 1:
            raise ReceiptError("the per-write full fixture must commit once per fact and the batched one must batch")
        if full_per_write["embedder_dim"] != full["embedder_dim"]:
            raise ReceiptError("the two full fixtures must use the same vector width, or the Git delta is confounded with the index")
        extra_full = tp["git_commits_all_brains"] - t["git_commits_all_brains"]
        if extra_full <= 0:
            raise ReceiptError("the per-write full fixture must have more commits than the batched full fixture")
        fb, fp = full["storage_and_history_gap"], full_per_write["storage_and_history_gap"]
        delta = fp["storage_bytes_by_component"][".git"] - fb["storage_bytes_by_component"][".git"]
        at_scale = {
            "scope": "The full 90,000-memory fixture prepared twice with the same facts: one commit per 500 facts and one commit per fact, as production commits after each acknowledged remember. The bytes are this fixture's own Git objects. They are not a production measurement: production history also carries real timestamps, other operations and repacking.",
            "facts": t["canonical_memory_facts"],
            "commits_batched": t["git_commits_all_brains"],
            "commits_per_write": tp["git_commits_all_brains"],
            "git_bytes_batched": fb["storage_bytes_by_component"][".git"],
            "git_bytes_per_write": fp["storage_bytes_by_component"][".git"],
            "extra_git_bytes": delta,
            "extra_git_bytes_per_extra_commit": round(delta / extra_full, 1),
            "storage_bytes_batched": fb["storage_bytes_all_brains"],
            "storage_bytes_per_write": fp["storage_bytes_all_brains"],
            "allocated_bytes_batched": fb["allocated_bytes_all_brains"],
            "allocated_bytes_per_write": fp["allocated_bytes_all_brains"],
            "largest_account_share_of_storage_quota_per_write": max(a["storage_share_of_quota"] for a in full_per_write["accounts"]),
            "accounts_at_or_over_storage_quota_per_write": fp["accounts_at_or_over_storage_quota"],
            "verification_report_sha256": {"batched": full["_sha256"], "per_write": full_per_write["_sha256"]},
        }
    wide = None
    if full_wide is not None:
        tw = _totals(full_wide)
        if (tw["accounts"], tw["brains"], tw["canonical_memory_facts"]) != (FULL_ACCOUNTS, FULL_BRAINS, FULL_FACTS) or not full_wide["satisfies_full_cardinality"] or full_wide["reduced"]:
            raise ReceiptError("the wide-vector full report does not hold 14 accounts, 29 brains and 90,000 memories")
        if full_wide["fixture_content_sha256"] != full["fixture_content_sha256"]:
            raise ReceiptError("the wide-vector fixture must hold the same facts: its content digest differs")
        if full_wide["commit_every"] != full["commit_every"]:
            raise ReceiptError("the two full fixtures must use the same commit density, or the index delta is confounded with history")
        if full_wide["embedder_dim"] <= full["embedder_dim"]:
            raise ReceiptError("the wide-vector fixture must use a wider vector than the first full fixture")
        gn, gw = full["storage_and_history_gap"], full_wide["storage_and_history_gap"]
        d_dim = full_wide["embedder_dim"] - full["embedder_dim"]
        d_index = gw["storage_bytes_by_component"][".serenity"] - gn["storage_bytes_by_component"][".serenity"]
        wide = {
            "scope": "The full 90,000-memory fixture prepared twice with the same facts and the same commit density (one commit per %d facts), with vectors of two widths. The width is a fixture parameter. It is not the pinned provider's width, which is unknown, and the vectors carry no semantic quality." % full["commit_every"],
            "narrow_dim": full["embedder_dim"],
            "wide_dim": full_wide["embedder_dim"],
            "wide_pin": full_wide["embedder_pin"],
            "facts": t["canonical_memory_facts"],
            "index_bytes_narrow": gn["storage_bytes_by_component"][".serenity"],
            "index_bytes_wide": gw["storage_bytes_by_component"][".serenity"],
            "extra_index_bytes": d_index,
            "extra_index_bytes_per_fact_per_extra_float": round(d_index / (t["canonical_memory_facts"] * d_dim), 3),
            "storage_bytes_narrow": gn["storage_bytes_all_brains"],
            "storage_bytes_wide": gw["storage_bytes_all_brains"],
            "allocated_bytes_narrow": gn["allocated_bytes_all_brains"],
            "allocated_bytes_wide": gw["allocated_bytes_all_brains"],
            "largest_account_share_of_storage_quota_narrow": max(a["storage_share_of_quota"] for a in full["accounts"]),
            "largest_account_share_of_storage_quota_wide": max(a["storage_share_of_quota"] for a in full_wide["accounts"]),
            "accounts_at_or_over_storage_quota_wide": gw["accounts_at_or_over_storage_quota"],
            "verification_report_sha256": {"narrow": full["_sha256"], "wide": full_wide["_sha256"]},
        }
    return {
        "schema": "serenity-hosted-load-fixture-receipt",
        "version": 1,
        "task": "T23.60",
        "source_sha": source_sha,
        "fixtureprep_binary_sha256": binaries,
        "labels": {
            "kind": "infrastructure-only offline fixture preparation and inventory",
            "not_a_qualification": "Not a cold-open, load, latency, steady-state or storage-saturation result. No service ran.",
            "reduced_smoke_vs_full": "'smoke' is a labeled reduced preparation (29 brains, few facts each) and never satisfies the 90,000-memory, 29-brain cardinalities. 'full' holds exactly 14 accounts, 29 brains and 90,000 memories.",
            "provider_quality": "unqualified: vectors come from a hash embedder (%s). They say nothing about retrieval or provider quality, and their width is a fixture parameter, not the pinned provider's." % full["embedder_pin"],
        },
        "full": _profile(full),
        "smoke": _profile(smoke),
        "at_the_memory_cap": {
            "accounts_at_cap": sum(1 for a in full["accounts"] if a["at_or_over_memory_cap"]),
            "accounts_whose_gateway_limit_inputs_refuse_a_remember": sum(1 for a in full["accounts"] if a["gateway_limit_inputs_refuse_a_nonreplay_remember"]),
            "consequence": "The gateway refuses every further nonreplay remember on an account at its plan memory cap with limit_exceeded (class quota, outcome tool_error). The refusal is computed from the gateway's own Inventory and Entitlement on this fixture and the rule in internal/hosted/gateway/gateway.go; no request was sent and no service ran. See decision-request-full-cardinality-eligible-traffic.md.",
        },
        "storage_and_history_gap": {
            "memory_count_vs_storage": "The full fixture fills the memory count. It does not fill any storage quota: see full.storage_and_history_gap and full.by_plan[*].largest_share_of_storage_quota.",
            "history_is_batched": "The full fixture commits facts in batches (commit_every in the marker). Production commits after every acknowledged remember, so production history is longer and larger. The maximum production history size is unmeasured; history_at_scale, when present, measures this fixture's own per-write history.",
            "history_at_scale": at_scale,
            "vector_width_at_scale": wide,
            "history_sample": {
                "scope": "Reduced smoke at 100 facts per brain, the same 2,900 facts, prepared twice: one commit per 500 facts and one commit per fact. It measures the Git size of extra commits at that brain size only. Tree objects grow with brain size, so it is not an estimate for 5,000-fact brains.",
                "facts": sample_batched["totals"]["canonical_memory_facts"],
                "commits_batched": sample_batched["totals"]["git_commits_all_brains"],
                "commits_per_write": sample_per_write["totals"]["git_commits_all_brains"],
                "git_bytes_batched": gb["storage_bytes_by_component"][".git"],
                "git_bytes_per_write": gp["storage_bytes_by_component"][".git"],
                "extra_git_bytes": git_delta,
                "extra_git_bytes_per_extra_commit": round(git_delta / extra_commits, 1),
                "allocated_bytes_batched": gb["allocated_bytes_all_brains"],
                "allocated_bytes_per_write": gp["allocated_bytes_all_brains"],
                "verification_report_sha256": {"batched": sample_batched["_sha256"], "per_write": sample_per_write["_sha256"]},
            },
            "fact_text_floor": "At the 4,096-byte fact cap, fact text is at most 4.096% of each plan's storage quota. The envelope, Git objects and derived index add to it.",
        },
        "client_reach": {
            "note": "The current load client reads one credential per account, bound to the default brain. Facts on other brains count toward the account's memory total and are unreachable by that client.",
            "facts_on_default_brains": _profile(full)["facts_on_default_brains"],
            "facts_on_other_brains": _profile(full)["facts_on_other_brains"],
        },
        "preparer_timing": {
            "note": "Wall time from the preparer's own clock, copied from its manifest. The verifier does not read it, and it is not a service latency.",
            **timings,
        },
    }


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--full", required=True, type=Path)
    p.add_argument("--smoke", required=True, type=Path)
    p.add_argument("--history-batched", required=True, type=Path)
    p.add_argument("--history-per-write", required=True, type=Path)
    p.add_argument("--timing", action="append", default=[], metavar="NAME=FILE")
    p.add_argument("--source-sha", required=True)
    p.add_argument("--full-per-write", type=Path, help="optional: verification report of the full fixture prepared with one commit per fact")
    p.add_argument("--full-wide", type=Path, help="optional: verification report of the full fixture prepared with wider vectors")
    p.add_argument("--prepare-binary-sha256", required=True)
    p.add_argument("--verify-binary-sha256", required=True)
    p.add_argument("--output", required=True, type=Path)
    args = p.parse_args()
    try:
        timings = {}
        for item in args.timing:
            name, _, path = item.partition("=")
            timings[name] = json.loads(Path(path).read_text())
        per_write_full = _load(args.full_per_write, "full") if args.full_per_write else None
        receipt = build(_load(args.full, "full"), _load(args.smoke, "smoke"), _load(args.history_batched, "smoke"), _load(args.history_per_write, "smoke"), timings, args.source_sha,
                        {"prepare": args.prepare_binary_sha256, "verify": args.verify_binary_sha256}, per_write_full, _load(args.full_wide, "full") if args.full_wide else None)
    except (ReceiptError, KeyError, ValueError, OSError) as exc:
        print(f"BLOCKED: {exc}", file=sys.stderr)
        return 2
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"output": str(args.output), "full_facts": receipt["full"]["totals"]["canonical_memory_facts"], "smoke_facts": receipt["smoke"]["totals"]["canonical_memory_facts"]}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
