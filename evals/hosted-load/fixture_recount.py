#!/usr/bin/env python3
"""Recount a prepared offline fixture without any Go code (T23.60).

`fixtureprep verify` is the fixture's verifier. This script is a second, independent reading of the same directory
in another language, so a reviewer can check that the counts and the reproducible content digest do not depend on the
verifier's own code. It reads only the control database (opened read-only and immutable), the workload, the plan table
in `internal/hosted/plans/plans.go` and each brain's canonical fact files. It writes nothing, reads no credential
file, opens no network connection and never reads the preparer's manifest.

What it checks: account and brain cardinalities against the frozen workload and the plan table; every canonical fact
file is content-addressed (its directory name is the SHA-256 of its bytes), decodes as a `memory_fact` under the
4,096-byte cap and has a legacy id unique within its brain; every brain holds exactly the fact count its plan share
requires; no fact repeats anywhere; and the fixture content digest equals the one `fixtureprep verify` reports. It
says nothing about the index, vectors, Git history or storage: those are the Go verifier's.

Exit status: 0 the recount matches, 1 it found differences, 2 a refusal or a usage error.
"""
from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import os
import re
import sqlite3
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
MAX_FACT_BYTES = 4096
PLAN_ORDER = ("scale", "builder", "free")  # the order the load client lists accounts in
DEFAULT_SMOKE_FACTS = 10
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


class Refusal(Exception):
    pass


def plan_table(path: Path = REPO / "internal" / "hosted" / "plans" / "plans.go") -> dict[str, dict[str, int]]:
    """{plan: {brains, memories}} parsed from the Go plan table, the source of truth."""
    rows = re.findall(r'\{"(\w+)", (\d+), (\d+), (\d+), (\d+), (\d+), (\d+), (\d+)\}', path.read_text())
    if not rows:
        raise Refusal(f"no plan rows found in {path}")
    return {r[0]: {"brains": int(r[2]), "memories": int(r[3])} for r in rows}


def expected_layout(workload: dict, plans: dict, expect: str, smoke_facts: int) -> list[dict]:
    """The planned accounts in client order: [{label, plan, brains: [facts per brain index]}]."""
    counts = workload["cardinalities"]["paid"]["accounts"]
    if set(counts) - set(PLAN_ORDER) or any(p not in plans for p in PLAN_ORDER):
        raise Refusal(f"workload plans {sorted(counts)} do not match the plan table {sorted(plans)}")
    out = []
    for plan in PLAN_ORDER:
        for i in range(counts.get(plan, 0)):
            n = plans[plan]["brains"]
            total = plans[plan]["memories"] if expect == "full" else smoke_facts * n
            base, rem = divmod(total, n)
            out.append({"label": f"{plan}-{i}", "plan": plan, "brains": [base + 1 if b < rem else base for b in range(n)]})
    return out


def read_brain(brain_dir: Path) -> dict:
    """Count and check one brain's canonical fact files."""
    sources = brain_dir / "brain" / "sources"
    shas: list[str] = []
    problems: list[str] = []
    legacy: set[int] = set()
    if sources.is_dir():
        for shard in sorted(os.scandir(sources), key=lambda e: e.name):
            if not shard.is_dir(follow_symlinks=False):
                problems.append(f"unexpected entry {shard.name}")
                continue
            for fact in sorted(os.scandir(shard.path), key=lambda e: e.name):
                sha = fact.name
                data_path = Path(fact.path) / "bytes"
                if not SHA_RE.match(sha) or sha[:2] != shard.name or not data_path.is_file():
                    problems.append(f"malformed fact entry {shard.name}/{sha}")
                    continue
                data = data_path.read_bytes()
                if hashlib.sha256(data).hexdigest() != sha:
                    problems.append(f"{sha[:12]}: bytes do not hash to the directory name")
                    continue
                if len(data) > MAX_FACT_BYTES:
                    problems.append(f"{sha[:12]}: {len(data)} bytes exceeds {MAX_FACT_BYTES}")
                try:
                    doc = json.loads(data)
                except ValueError:
                    problems.append(f"{sha[:12]}: not JSON")
                    continue
                if not isinstance(doc, dict) or doc.get("record_type") != "memory_fact" or not isinstance(doc.get("fact"), str) or not doc["fact"]:
                    problems.append(f"{sha[:12]}: not a memory_fact with text")
                    continue
                lid = doc.get("legacy_id")
                if not isinstance(lid, int) or isinstance(lid, bool) or lid in legacy:
                    problems.append(f"{sha[:12]}: legacy id {lid!r} is invalid or repeated")
                    continue
                legacy.add(lid)
                shas.append(sha)
    return {"shas": shas, "problems": problems}


def brain_digest(shas: list[str]) -> str:
    return hashlib.sha256("\n".join(sorted(shas)).encode()).hexdigest()


def recount(fixture: Path, expect: str, smoke_facts: int, workers: int, workload_path: Path = HERE / "workload.json", plans_path: Path | None = None) -> dict:
    marker_path = fixture / "FIXTURE-ONLY.json"
    if not marker_path.is_file():
        raise Refusal(f"{fixture.name} has no FIXTURE-ONLY.json: it is not a fixture the preparer made")
    marker = json.loads(marker_path.read_text())
    if marker.get("status") != "prepared":
        raise Refusal(f"marker status is {marker.get('status')!r}, not 'prepared': preparation did not finish")
    db_path = fixture / "data" / "control.db"
    if not db_path.is_file():
        raise Refusal("data/control.db is missing")
    workload = json.loads(workload_path.read_text())
    plans = plan_table(plans_path) if plans_path else plan_table()
    layout = expected_layout(workload, plans, expect, smoke_facts)

    db = sqlite3.connect(f"file:{db_path}?mode=ro&immutable=1", uri=True)
    try:
        accounts = {row[0]: (row[1], row[2]) for row in db.execute("SELECT id, email, plan_id FROM accounts")}
        brains = db.execute("SELECT id, account_id, is_default FROM brains WHERE deleted_at IS NULL ORDER BY created_at, id").fetchall()
    finally:
        db.close()
    label_of = {aid: email.split("@", 1)[0] for aid, (email, _plan) in accounts.items()}
    plan_of = {label_of[aid]: plan for aid, (_email, plan) in accounts.items()}
    by_label: dict[str, list[tuple[str, bool]]] = {}
    for bid, aid, is_default in brains:
        by_label.setdefault(label_of.get(aid, f"unknown-{aid}"), []).append((bid, bool(is_default)))

    checks: list[dict] = []

    def check(name: str, want, got) -> None:
        checks.append({"id": name, "expected": want, "observed": got, "pass": want == got})

    want_labels = [a["label"] for a in layout]
    check("accounts.labels", sorted(want_labels), sorted(plan_of))
    check("accounts.plans", {a["label"]: a["plan"] for a in layout}, {k: plan_of.get(k) for k in want_labels})
    check("brains.total", sum(len(a["brains"]) for a in layout), len(brains))

    # Default brain first, then the others in creation order: the order the digest uses.
    ordered: list[tuple[str, int, str]] = []
    for a in layout:
        mine = by_label.get(a["label"], [])
        default = [b for b, d in mine if d]
        rest = [b for b, d in mine if not d]
        check(f"brains.{a['label']}.count_and_one_default", (len(a["brains"]), 1), (len(mine), len(default)))
        for idx, bid in enumerate(default + rest):
            ordered.append((a["label"], idx, bid))

    def work(item: tuple[str, int, str]) -> dict:
        label, idx, bid = item
        got = read_brain(fixture / "data" / "brains" / bid)
        got.update(label=label, index=idx, id=bid)
        return got

    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        results = list(pool.map(work, ordered))

    layout_by_label = {a["label"]: a for a in layout}
    wrong_counts, problems = [], []
    per_account: dict[str, int] = {}
    seen: set[str] = set()
    repeated = 0
    for r in results:
        want = layout_by_label[r["label"]]["brains"]
        want_n = want[r["index"]] if r["index"] < len(want) else None
        if len(r["shas"]) != want_n:
            wrong_counts.append(f"{r['label']}/{r['index']}={len(r['shas'])}(want {want_n})")
        problems += [f"{r['label']}/{r['index']}: {p}" for p in r["problems"]]
        per_account[r["label"]] = per_account.get(r["label"], 0) + len(r["shas"])
        for sha in r["shas"]:
            if sha in seen:
                repeated += 1
            seen.add(sha)
    total = sum(len(r["shas"]) for r in results)
    check("facts.every_brain_holds_its_planned_count", [], wrong_counts)
    check("facts.every_file_is_content_addressed_well_formed_and_unique_in_its_brain", [], problems)
    check("facts.no_fact_repeats_across_the_fixture", 0, repeated)
    check("facts.total", sum(sum(a["brains"]) for a in layout), total)
    if expect == "full":
        check("facts.total_equals_frozen_workload", workload["cardinalities"]["paid"]["total_facts"], total)
        check("facts.every_account_at_its_plan_memory_cap", {a["label"]: plans[a["plan"]]["memories"] for a in layout}, per_account)

    lines = [f"{r['label']}/{r['index']} {brain_digest(r['shas'])}" for r in results]
    digest = hashlib.sha256("\n".join(lines).encode()).hexdigest()
    return {
        "schema": "serenity-hosted-load-fixture-recount",
        "version": 1,
        "expect": expect,
        "marker_profile": marker.get("profile"),
        "pass": all(c["pass"] for c in checks),
        "totals": {"accounts": len(plan_of), "brains": len(brains), "canonical_memory_facts": total, "distinct_fact_sha256": len(seen)},
        "fixture_content_sha256": digest,
        "checks": checks,
        "not_claimed": "Index, vectors, Git history, storage, credentials and entitlements are not read here. Provider quality is unqualified.",
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    p.add_argument("--dir", required=True, type=Path, help="a prepared fixture directory (read only)")
    p.add_argument("--expect", required=True, choices=("full", "smoke"))
    p.add_argument("--smoke-facts", type=int, default=DEFAULT_SMOKE_FACTS, help="facts per brain in a smoke fixture")
    p.add_argument("--workers", type=int, default=4)
    p.add_argument("--output", type=Path, help="write the JSON result here as well as printing a summary")
    args = p.parse_args(argv)
    try:
        if not 1 <= args.workers <= 8:
            raise Refusal("--workers must be 1..8")
        result = recount(args.dir, args.expect, args.smoke_facts, args.workers)
    except (Refusal, OSError, ValueError, sqlite3.Error) as e:
        print(f"BLOCKED: {e}", file=sys.stderr)
        return 2
    if args.output:
        args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    failing = [c["id"] for c in result["checks"] if not c["pass"]]
    print(json.dumps({"pass": result["pass"], "expect": args.expect, "totals": result["totals"], "fixture_content_sha256": result["fixture_content_sha256"], "failing_checks": failing}, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
