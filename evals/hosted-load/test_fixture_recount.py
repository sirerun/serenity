#!/usr/bin/env python3
"""Tests for the independent fixture recount (T23.60). They build tiny synthetic fixtures in a temporary directory."""
from __future__ import annotations

import contextlib
import hashlib
import io
import json
import os
import re
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import fixture_recount as rc  # noqa: E402

WORKLOAD = json.loads((rc.HERE / "workload.json").read_text())
PLANS = rc.plan_table()


def fact_bytes(label: str, brain: int, i: int, text: str | None = None) -> bytes:
    doc = {"format_version": 1, "record_type": "memory_fact", "legacy_id": i + 1, "fact": text or f"f{i:04d} {label} brain{brain} alarm budget", "provenance": "fixture:T23.60 synthetic infrastructure-only"}
    return json.dumps(doc, separators=(",", ":")).encode()


def put_fact(brain_dir: Path, data: bytes) -> str:
    sha = hashlib.sha256(data).hexdigest()
    d = brain_dir / "brain" / "sources" / sha[:2] / sha
    d.mkdir(parents=True, exist_ok=True)
    (d / "bytes").write_bytes(data)
    return sha


def make_fixture(root: Path, per_brain: int = 1, id_seed: str = "a") -> Path:
    """A smoke-shaped fixture: every planned account and brain, per_brain facts each."""
    layout = rc.expected_layout(WORKLOAD, PLANS, "smoke", per_brain)
    (root / "data" / "brains").mkdir(parents=True)
    (root / "FIXTURE-ONLY.json").write_text(json.dumps({"status": "prepared", "profile": "smoke"}))
    db = sqlite3.connect(root / "data" / "control.db")
    db.execute("CREATE TABLE accounts(id TEXT PRIMARY KEY, email TEXT, plan_id TEXT)")
    db.execute("CREATE TABLE brains(id TEXT PRIMARY KEY, account_id TEXT, is_default INTEGER, created_at TEXT, deleted_at TEXT)")
    n = 0
    for a in layout:
        aid = f"acct-{id_seed}-{a['label']}"
        db.execute("INSERT INTO accounts VALUES(?,?,?)", (aid, f"{a['label']}@fixture.invalid", a["plan"]))
        for idx, count in enumerate(a["brains"]):
            n += 1
            bid = f"BR{id_seed}{n:04d}"
            db.execute("INSERT INTO brains VALUES(?,?,?,?,NULL)", (bid, aid, 1 if idx == 0 else 0, f"2026-01-01T00:00:{n:02d}Z"))
            brain_dir = root / "data" / "brains" / bid
            for i in range(count):
                put_fact(brain_dir, fact_bytes(a["label"], idx, i))
            (brain_dir / "brain" / "sources").mkdir(parents=True, exist_ok=True)
    db.commit()
    db.close()
    return root


def run(fixture: Path, expect: str = "smoke", smoke_facts: int = 1) -> dict:
    return rc.recount(fixture, expect, smoke_facts, 2)


def failing(result: dict) -> set[str]:
    return {c["id"] for c in result["checks"] if not c["pass"]}


def some_fact_file(fixture: Path) -> Path:
    return next((fixture / "data" / "brains").glob("*/brain/sources/*/*/bytes"))


class LayoutTests(unittest.TestCase):
    def test_the_full_layout_is_14_accounts_29_brains_and_90000_memories(self):
        layout = rc.expected_layout(WORKLOAD, PLANS, "full", 0)
        self.assertEqual(len(layout), 14)
        self.assertEqual(sum(len(a["brains"]) for a in layout), 29)
        self.assertEqual(sum(sum(a["brains"]) for a in layout), 90000)
        self.assertEqual([a["label"] for a in layout][:5], ["scale-0", "builder-0", "builder-1", "builder-2", "free-0"])
        self.assertEqual(layout[1]["brains"], [3334, 3333, 3333])
        self.assertEqual(layout[0]["brains"], [5000] * 10)

    def test_the_plan_table_is_read_from_the_go_source(self):
        self.assertEqual(PLANS, {"free": {"brains": 1, "memories": 1000}, "builder": {"brains": 3, "memories": 10000}, "scale": {"brains": 10, "memories": 50000}})

    def test_an_unknown_plan_in_the_workload_is_refused(self):
        bad = json.loads(json.dumps(WORKLOAD))
        bad["cardinalities"]["paid"]["accounts"]["enterprise"] = 1
        with self.assertRaises(rc.Refusal):
            rc.expected_layout(bad, PLANS, "full", 0)


class RecountTests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.tmp = Path(self._tmp.name)

    def test_a_fixture_that_matches_passes_and_counts_every_fact(self):
        r = run(make_fixture(self.tmp / "f", 2), smoke_facts=2)
        self.assertTrue(r["pass"], failing(r))
        self.assertEqual(r["totals"], {"accounts": 14, "brains": 29, "canonical_memory_facts": 58, "distinct_fact_sha256": 58})

    def test_the_content_digest_does_not_depend_on_brain_or_account_ids(self):
        a = run(make_fixture(self.tmp / "a", 2, "a"), smoke_facts=2)
        b = run(make_fixture(self.tmp / "b", 2, "b"), smoke_facts=2)
        self.assertEqual(a["fixture_content_sha256"], b["fixture_content_sha256"])

    def test_the_digest_is_the_definition_the_go_verifier_uses(self):
        """Per brain: sha256 of the sorted fact shas joined by newline; then sha256 of 'label/index digest' lines."""
        fx = make_fixture(self.tmp / "f", 1)
        layout = rc.expected_layout(WORKLOAD, PLANS, "smoke", 1)
        lines = []
        for a in layout:
            for idx, _ in enumerate(a["brains"]):
                sha = hashlib.sha256(fact_bytes(a["label"], idx, 0)).hexdigest()
                lines.append(f"{a['label']}/{idx} {hashlib.sha256(sha.encode()).hexdigest()}")
        want = hashlib.sha256("\n".join(lines).encode()).hexdigest()
        self.assertEqual(run(fx)["fixture_content_sha256"], want)

    def test_a_flipped_bit_fails_content_addressing(self):
        fx = make_fixture(self.tmp / "f")
        p = some_fact_file(fx)
        data = bytearray(p.read_bytes())
        data[len(data) // 2] ^= 1
        p.write_bytes(bytes(data))
        r = run(fx)
        self.assertFalse(r["pass"])
        self.assertIn("facts.every_file_is_content_addressed_well_formed_and_unique_in_its_brain", failing(r))

    def test_a_removed_fact_fails_the_count(self):
        fx = make_fixture(self.tmp / "f")
        p = some_fact_file(fx)
        p.unlink()
        p.parent.rmdir()
        r = run(fx)
        self.assertFalse(r["pass"])
        self.assertIn("facts.every_brain_holds_its_planned_count", failing(r))

    def test_an_added_fact_fails_the_count(self):
        fx = make_fixture(self.tmp / "f")
        brain = some_fact_file(fx).parents[4]
        put_fact(brain, fact_bytes("extra", 0, 7))
        r = run(fx)
        self.assertFalse(r["pass"])
        self.assertIn("facts.every_brain_holds_its_planned_count", failing(r))

    def test_a_swapped_fact_keeps_the_count_but_changes_the_digest(self):
        good = run(make_fixture(self.tmp / "f", 1, "a"))
        fx = make_fixture(self.tmp / "g", 1, "a")
        p = some_fact_file(fx)
        brain = p.parents[4]
        p.unlink()
        p.parent.rmdir()
        put_fact(brain, fact_bytes("other", 0, 0, "swapped in fact text"))
        r = run(fx)
        self.assertTrue(r["pass"], "the count and the well-formedness are unchanged")
        self.assertNotEqual(r["fixture_content_sha256"], good["fixture_content_sha256"])

    def test_a_repeated_legacy_id_in_one_brain_fails(self):
        fx = make_fixture(self.tmp / "f", 2)
        brain = some_fact_file(fx).parents[4]
        for shard in list((brain / "brain" / "sources").iterdir()):
            for d in list(shard.iterdir()):
                (d / "bytes").unlink()
                d.rmdir()
            shard.rmdir()
        put_fact(brain, fact_bytes("x", 0, 0, "first"))
        put_fact(brain, fact_bytes("x", 0, 0, "second with the same legacy id"))
        r = run(fx, smoke_facts=2)
        self.assertFalse(r["pass"])
        self.assertIn("facts.every_file_is_content_addressed_well_formed_and_unique_in_its_brain", failing(r))

    def test_an_oversized_fact_fails(self):
        fx = make_fixture(self.tmp / "f")
        p = some_fact_file(fx)
        brain = p.parents[4]
        p.unlink()
        p.parent.rmdir()
        put_fact(brain, fact_bytes("big", 0, 0, "w " * 2100))
        self.assertIn("facts.every_file_is_content_addressed_well_formed_and_unique_in_its_brain", failing(run(fx)))

    def test_a_smoke_fixture_verified_as_full_fails(self):
        r = run(make_fixture(self.tmp / "f"), "full")
        self.assertFalse(r["pass"])
        self.assertIn("facts.total_equals_frozen_workload", failing(r))
        self.assertIn("facts.every_account_at_its_plan_memory_cap", failing(r))

    def test_a_missing_brain_directory_fails(self):
        fx = make_fixture(self.tmp / "f")
        brain = some_fact_file(fx).parents[4]
        for p in sorted(brain.rglob("*"), reverse=True):
            p.unlink() if p.is_file() else p.rmdir()
        brain.rmdir()
        r = run(fx)
        self.assertFalse(r["pass"])

    def test_an_account_on_the_wrong_plan_fails(self):
        fx = make_fixture(self.tmp / "f")
        db = sqlite3.connect(fx / "data" / "control.db")
        db.execute("UPDATE accounts SET plan_id='free' WHERE email LIKE 'scale-0@%'")
        db.commit()
        db.close()
        self.assertIn("accounts.plans", failing(run(fx)))

    def test_a_missing_or_unfinished_fixture_is_refused(self):
        fx = make_fixture(self.tmp / "f")
        (fx / "FIXTURE-ONLY.json").write_text(json.dumps({"status": "preparing", "profile": "smoke"}))
        with self.assertRaises(rc.Refusal):
            run(fx)
        (fx / "FIXTURE-ONLY.json").unlink()
        with self.assertRaises(rc.Refusal):
            run(fx)
        with self.assertRaises(rc.Refusal):
            run(self.tmp / "does-not-exist")

    def test_a_missing_control_database_is_refused(self):
        fx = make_fixture(self.tmp / "f")
        (fx / "data" / "control.db").unlink()
        with self.assertRaises(rc.Refusal):
            run(fx)

    def test_it_writes_nothing_to_the_fixture(self):
        fx = make_fixture(self.tmp / "f", 2)

        def snapshot():
            return sorted((str(p.relative_to(fx)), p.stat().st_size, p.stat().st_mtime_ns) for p in fx.rglob("*"))

        before = snapshot()
        run(fx, smoke_facts=2)
        self.assertEqual(before, snapshot())


class CLITests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.tmp = Path(self._tmp.name)

    def call(self, *args: str) -> tuple[int, str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = rc.main(list(args))
        return code, out.getvalue(), err.getvalue()

    def test_exit_codes_are_zero_one_and_two(self):
        fx = make_fixture(self.tmp / "f")
        code, out, _ = self.call("--dir", str(fx), "--expect", "smoke", "--smoke-facts", "1")
        self.assertEqual(code, 0, out)
        code, out, _ = self.call("--dir", str(fx), "--expect", "full")
        self.assertEqual(code, 1)
        self.assertIn("facts.total_equals_frozen_workload", json.loads(out)["failing_checks"])
        code, _, err = self.call("--dir", str(self.tmp / "nope"), "--expect", "smoke")
        self.assertEqual(code, 2)
        self.assertTrue(err.startswith("BLOCKED:"))

    def test_the_output_file_carries_every_check(self):
        fx = make_fixture(self.tmp / "f")
        target = self.tmp / "out.json"
        self.assertEqual(self.call("--dir", str(fx), "--expect", "smoke", "--smoke-facts", "1", "--output", str(target))[0], 0)
        doc = json.loads(target.read_text())
        self.assertEqual(doc["schema"], "serenity-hosted-load-fixture-recount")
        self.assertTrue(all(c["pass"] for c in doc["checks"]))
        self.assertIn("unqualified", doc["not_claimed"])


class SourceTests(unittest.TestCase):
    def test_the_script_imports_no_network_module_and_reads_no_environment(self):
        text = (rc.HERE / "fixture_recount.py").read_text()
        imports = set(re.findall(r"^(?:import|from)\s+([\w.]+)", text, re.M))
        self.assertFalse(imports & {"socket", "http", "urllib", "ssl", "requests", "subprocess", "ftplib", "smtplib"}, imports)
        self.assertNotIn("os.environ", text)
        self.assertNotIn("getenv", text)
        self.assertNotIn("credentials/", text, "it never opens the credentials directory")

    def test_it_never_opens_a_fixture_file_for_writing(self):
        text = (rc.HERE / "fixture_recount.py").read_text()
        self.assertNotRegex(text, r"open\([^)]*['\"][wa]b?['\"]")
        self.assertEqual(len(re.findall(r"write_text|write_bytes", text)), 1, "the only write is the optional --output file")


if __name__ == "__main__":
    unittest.main()
