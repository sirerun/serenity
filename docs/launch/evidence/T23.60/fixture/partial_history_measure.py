#!/usr/bin/env python3
"""Measure per-write Git history on the brains that finished in an interrupted full preparation (T23.60).

The full 90,000-memory fixture prepared with one commit per fact stopped at its 15th brain, so `fixtureprep verify`
refuses it (marker status `preparing`) and no whole-fixture history number exists. Fifteen brains did finish. This
script pairs each finished brain with the same brain (same account label and brain index, so the same facts) in a
verified full fixture that used batched commits, and compares their `.git` bytes and commit counts. Read only: it
opens the control databases read-only and immutable, sums regular-file sizes the way the gateway's inventory does,
and runs `git rev-list` and `git cat-file` with `--no-optional-locks`. It prints and writes labels and counts only.

The result is a partial, measured comparison on this host's Git. It is not a production history bound and it does
not cover the 14 brains that did not finish.
"""
from __future__ import annotations

import argparse
import json
import os
import sqlite3
import subprocess
import sys
from pathlib import Path


def brains_by_position(fixture: Path) -> dict[tuple[str, int], str]:
    """{(account label, brain index): brain id}, default brain first then creation order (the client's order)."""
    db = sqlite3.connect(f"file:{fixture / 'data' / 'control.db'}?mode=ro&immutable=1", uri=True)
    try:
        label = {i: e.split("@", 1)[0] for i, e in db.execute("SELECT id, email FROM accounts")}
        rows = db.execute("SELECT id, account_id, is_default FROM brains WHERE deleted_at IS NULL ORDER BY created_at, id").fetchall()
    finally:
        db.close()
    per: dict[str, list[tuple[str, bool]]] = {}
    for bid, aid, d in rows:
        per.setdefault(label[aid], []).append((bid, bool(d)))
    out = {}
    for lab, items in per.items():
        ordered = [b for b, d in items if d] + [b for b, d in items if not d]
        for idx, bid in enumerate(ordered):
            out[(lab, idx)] = bid
    return out


def tree_bytes(root: Path) -> int:
    total = 0
    for dirpath, _dirs, files in os.walk(root):
        for name in files:
            p = os.path.join(dirpath, name)
            if os.path.isfile(p) and not os.path.islink(p):
                total += os.path.getsize(p)
    return total


def git(root: Path, *args: str) -> tuple[int, str]:
    r = subprocess.run(["git", "--no-optional-locks", "-C", str(root), *args], capture_output=True, text=True)
    return r.returncode, r.stdout.strip()


def facts_in(brain: Path) -> int:
    return sum(1 for _ in (brain / "brain" / "sources").glob("*/*/bytes")) if (brain / "brain" / "sources").is_dir() else 0


def measure(batched: Path, per_write: Path) -> dict:
    b_ids, p_ids = brains_by_position(batched), brains_by_position(per_write)
    rows, unfinished, missing = [], [], []
    for (lab, idx), pid in sorted(p_ids.items(), key=lambda kv: list(b_ids).index(kv[0]) if kv[0] in b_ids else 10**6):
        bid = b_ids.get((lab, idx))
        pdir = per_write / "data" / "brains" / pid
        if bid is None:
            missing.append(f"{lab}/{idx}")
            continue
        bdir = batched / "data" / "brains" / bid
        facts = facts_in(bdir)
        ok_head = git(pdir, "cat-file", "-e", "HEAD^{commit}")[0] == 0 if (pdir / ".git").is_dir() else False
        commits = int(git(pdir, "rev-list", "--count", "HEAD")[1] or 0) if ok_head else 0
        if not ok_head or facts_in(pdir) != facts or commits != facts + 1:
            unfinished.append(f"{lab}/{idx}")
            continue
        bcommits = int(git(bdir, "rev-list", "--count", "HEAD")[1])
        rows.append({
            "label": lab, "index": idx, "facts": facts,
            "commits_batched": bcommits, "commits_per_write": commits,
            "git_bytes_batched": tree_bytes(bdir / ".git"), "git_bytes_per_write": tree_bytes(pdir / ".git"),
        })
    facts = sum(r["facts"] for r in rows)
    cb, cp = sum(r["commits_batched"] for r in rows), sum(r["commits_per_write"] for r in rows)
    gb, gp = sum(r["git_bytes_batched"] for r in rows), sum(r["git_bytes_per_write"] for r in rows)
    return {
        "schema": "serenity-hosted-load-partial-history-measurement",
        "version": 1,
        "scope": "Per-write Git history on the brains that finished in an interrupted full preparation, paired with the same brains (same account label, brain index and facts) in a verified full fixture that batched its commits. Partial: not a whole-fixture number and not a production bound. Measured on this host's Git.",
        "brains_measured": len(rows),
        "brains_not_finished": unfinished,
        "brains_not_in_the_batched_fixture": missing,
        "facts": facts,
        "batched": {"commits": cb, "git_bytes": gb},
        "per_write": {"commits": cp, "git_bytes": gp},
        "extra_commits": cp - cb,
        "extra_git_bytes": gp - gb,
        "extra_git_bytes_per_extra_commit": round((gp - gb) / (cp - cb), 1) if cp > cb else None,
        "per_brain": rows,
    }


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    p.add_argument("--batched", required=True, type=Path, help="a verified full fixture with batched commits")
    p.add_argument("--per-write", required=True, type=Path, help="the interrupted full fixture with one commit per fact")
    p.add_argument("--output", required=True, type=Path)
    a = p.parse_args()
    try:
        result = measure(a.batched, a.per_write)
    except (OSError, ValueError, KeyError, sqlite3.Error) as e:
        print(f"BLOCKED: {e}", file=sys.stderr)
        return 2
    a.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print(json.dumps({k: v for k, v in result.items() if k != "per_brain"}, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
