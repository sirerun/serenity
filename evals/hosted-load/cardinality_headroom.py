#!/usr/bin/env python3
"""Inventory-only replay of the frozen hosted-load workload against the plan memory caps (T23.60).

Answers one question for the eligible-traffic versus saturation decision request: from what memory
count can the frozen mix (80% recall, 15% remember, 5% forget) run without the gateway's memory-count
rule refusing a remember, and what happens when every account starts exactly at its cap?

It makes no API call, reads no credential and needs no provider. It replays the same deterministic
arrivals the load client would offer (harness.generate_arrivals with load.BASE_SEED) against an
inventory counter per account. It models only the memory-count rule (a nonreplay remember is refused
when the account's live memories are at or above Plan.Memories). The limiter, the storage rule, the
monthly write cap, provider faults, cold opens, races and latency are not modeled, so the result is an
inventory count and never a measured runtime failure.

Two forget models, because the load client's behavior differs from a forget of any stored fact:

- forget_removes_any_fact: a forget removes one live fact whenever the account has one, as if the
  fixture seeded facts the client could forget. This is the more favorable model for the sample.
- current_client: load.py forgets only an id this run remembered and otherwise skips the offered
  forget (SKIPPED_FORGET), so an account at its cap can neither remember nor forget.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import random
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
WORKLOAD = Path(__file__).with_name("workload.json")
SCOPES = ("steady_only", "through_steady", "one_repetition", "three_repetitions_cumulative")


def _load_client():
    spec = importlib.util.spec_from_file_location("load_client", REPO / "scripts" / "hosted" / "load.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _by_phase(arrivals: list[dict]) -> dict[str, list[dict]]:
    out: dict[str, list[dict]] = {}
    for a in arrivals:
        out.setdefault(a["phase"], []).append(a)
    return out


def replay_at_cap(arrivals: list[dict], accounts: list[dict], phases: list[str], model: str, start: dict[str, int] | None = None) -> dict:
    """Replay every account and count what the memory-count rule refuses. Each account starts at its plan memory cap unless `start` says otherwise."""
    cap = {a["id"]: a["allowance"]["memories"] for a in accounts}
    count = dict(cap) if start is None else dict(start)
    remembered = {a["id"]: 0 for a in accounts}  # ids this run remembered and has not forgotten (current_client only).
    stats = {p: {"offered": 0, "remember_offered": 0, "remember_refused_at_cap": 0, "remember_accepted": 0, "forget_offered": 0, "forget_dispatched": 0, "forget_skipped_no_id": 0} for p in phases}
    for a in arrivals:
        s, k = stats[a["phase"]], a["account"]
        s["offered"] += 1
        if a["verb"] == "remember":
            s["remember_offered"] += 1
            if count[k] >= cap[k]:
                s["remember_refused_at_cap"] += 1
            else:
                count[k] += 1
                remembered[k] += 1
                s["remember_accepted"] += 1
        elif a["verb"] == "forget":
            s["forget_offered"] += 1
            can = count[k] > 0 if model == "forget_removes_any_fact" else remembered[k] > 0
            if can:
                count[k] -= 1
                remembered[k] = max(0, remembered[k] - 1)
                s["forget_dispatched"] += 1
            else:
                s["forget_skipped_no_id"] += 1
    for s in stats.values():
        s["refused_pct_of_all_offered"] = round(100 * s["remember_refused_at_cap"] / s["offered"], 4) if s["offered"] else 0.0
        # Refused and skipped requests stay in the denominator and are never OK, so this is the most a phase can complete
        # even if every other offered request succeeds. The frozen threshold is min_offered_completion_pct.
        s["completion_ceiling_pct"] = round(100 * (s["offered"] - s["remember_refused_at_cap"] - s["forget_skipped_no_id"]) / s["offered"], 4) if s["offered"] else 0.0
    return {"starting_memories": sum(cap.values()) if start is None else sum(start.values()), "ending_memories": sum(count.values()), "by_phase": stats}


def headroom_needed(arrivals: list[dict], accounts: list[dict], model: str) -> dict[str, int]:
    """Slots below its cap an account must start with so no remember in `arrivals` is refused: the peak net growth."""
    net = {a["id"]: 0 for a in accounts}
    peak = {a["id"]: 0 for a in accounts}
    remembered = {a["id"]: 0 for a in accounts}
    for a in arrivals:
        k = a["account"]
        if a["verb"] == "remember":
            net[k] += 1
            remembered[k] += 1
        elif a["verb"] == "forget":
            if model == "forget_removes_any_fact" or remembered[k] > 0:
                net[k] -= 1
                remembered[k] = max(0, remembered[k] - 1)
        peak[k] = max(peak[k], net[k])
    return peak


def scope_arrivals(arrivals: list[dict], scope: str) -> list[dict]:
    phases = _by_phase(arrivals)
    if scope == "steady_only":
        return phases["steady"]
    if scope == "through_steady":
        return phases["warmup"] + phases["steady"]
    if scope == "one_repetition":
        return arrivals
    if scope == "three_repetitions_cumulative":
        return arrivals * 3
    raise ValueError(scope)


def build(workload: dict, client) -> dict:
    accounts = client.harness.build_accounts(workload["cardinalities"], "paid")
    arrivals = client.harness.generate_arrivals(workload, accounts, random.Random(client.BASE_SEED))
    phase_names = [p["name"] for p in workload["phases"]]
    caps = {a["id"]: a["allowance"]["memories"] for a in accounts}
    result = {
        "scope": "Deterministic inventory-only replay of the frozen workload's first repetition against the plan memory caps. No API call, provider, credential or cloud resource. Models only the gateway's memory-count rule; the limiter, the storage rule, the monthly write cap, provider faults, cold opens, races and latency are not modeled. Not a measured runtime failure.",
        "workload_sha256": hashlib.sha256(WORKLOAD.read_bytes()).hexdigest(),
        "base_seed": client.BASE_SEED,
        "frozen_mix": workload["traffic_mix"],
        "cap_total": sum(caps.values()),
        "offered_by_phase": {p: len(v) for p, v in _by_phase(arrivals).items()},
        "starting_at_cap": {m: replay_at_cap(arrivals, accounts, phase_names, m) for m in ("forget_removes_any_fact", "current_client")},
        "headroom_needed_to_avoid_every_memory_count_refusal": {},
    }
    for model in ("forget_removes_any_fact", "current_client"):
        by_scope = {}
        for scope in SCOPES:
            scoped = scope_arrivals(arrivals, scope)
            peaks = headroom_needed(scoped, accounts, model)
            by_scope[scope] = {
                "total_headroom": sum(peaks.values()),
                "starting_memories_if_each_account_starts_at_cap_minus_headroom": sum(caps.values()) - sum(peaks.values()),
                "ending_memories_after_the_scope": replay_at_cap(scoped, accounts, phase_names, model, {k: caps[k] - peaks[k] for k in caps})["ending_memories"],
                "by_account": {a["id"]: {"plan": a["plan"], "cap": caps[a["id"]], "headroom": peaks[a["id"]]} for a in accounts},
            }
        result["headroom_needed_to_avoid_every_memory_count_refusal"][model] = by_scope
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    client = _load_client()
    workload = json.loads(WORKLOAD.read_text())
    data = build(workload, client)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(data, indent=2) + "\n")
    steady = data["starting_at_cap"]["forget_removes_any_fact"]["by_phase"]["steady"]
    print(json.dumps({"output": str(args.output), "steady_offered": steady["offered"], "steady_remember_refused_at_cap": steady["remember_refused_at_cap"], "steady_refused_pct_of_all_offered": steady["refused_pct_of_all_offered"]}, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
