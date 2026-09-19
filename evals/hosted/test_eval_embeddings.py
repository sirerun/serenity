"""Integration tests for scripts/hosted/eval_embeddings.py: the harness
command contract (docs/launch/hosted-completion/evidence.md) end to end.
Run via: python3 -m unittest discover -s evals/hosted -p "test_*.py"

The lexical-only-control arm additionally runs only when SERENITY_BIN and
SEEDBRAIN_BIN point at real pre-built binaries (never built by this test
itself -- see lib/lexical_control.py's docstring on why). Its absence is a
genuine, explicit skip, not a silent pass.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
SCRIPTS_HOSTED = REPO_ROOT / "scripts" / "hosted"
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(SCRIPTS_HOSTED))

import eval_embeddings  # noqa: E402
from fake_hosted_mcp import FakeClock, FakeHostedMCP, RealClock  # noqa: E402
from lib import mcp_client, scoring, seeding  # noqa: E402
from lib.fixture_embedder import HashBagEmbedder  # noqa: E402

CORPUS_PATH = HERE / "corpus.json"
FACTS_PATH = HERE / "facts.json"
TEMPLATE_PATH = REPO_ROOT / "docs" / "launch" / "hosted-completion" / "qualification.example.json"


def _corpus_hash() -> str:
    return json.loads(CORPUS_PATH.read_text())["meta"]["corpus_hash_sha256"]


def _args(tmp_output: Path, manifest_path: Path, *, fixtures: bool, live: bool = False) -> "eval_embeddings.argparse.Namespace":
    argv = ["--manifest", str(manifest_path), "--output", str(tmp_output)]
    argv.append("--fixtures" if fixtures else "--live")
    return eval_embeddings.parse_args(argv)


def _mode_args(mode: str, output: Path, manifest_path: Path, *extra: str) -> "eval_embeddings.argparse.Namespace":
    return eval_embeddings.parse_args([f"--{mode}", "--manifest", str(manifest_path), "--output", str(output), *extra])


class TestFixturesModeNeverClaimsQualityPass(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        manifest = json.loads(TEMPLATE_PATH.read_text())
        manifest["task_id"] = "T23.43"
        self.manifest_path = Path(self.tmpdir.name) / "manifest.json"
        self.manifest_path.write_text(json.dumps(manifest))
        self.output_path = Path(self.tmpdir.name) / "result.json"

    def test_fixtures_run_never_reports_status_pass(self):
        args = _args(self.output_path, self.manifest_path, fixtures=True)
        result, exit_code = eval_embeddings.run_fixtures(args)
        self.assertNotEqual(result["status"], "PASS")
        self.assertEqual(exit_code, eval_embeddings.EXIT_PASS)  # harness itself ran without a tested failure

    def test_fixtures_run_covers_all_100_cases(self):
        args = _args(self.output_path, self.manifest_path, fixtures=True)
        result, _ = eval_embeddings.run_fixtures(args)
        self.assertEqual(result["cases"]["planned"], 100)

    def test_fixed_vector_stub_acceptance_row_is_pass_not_the_quality_row(self):
        args = _args(self.output_path, self.manifest_path, fixtures=True)
        result, _ = eval_embeddings.run_fixtures(args)
        stub_row = next(r for r in result["acceptance"] if "Fixed-vector" in r["criterion"])
        self.assertEqual(stub_row["status"], "PASS")
        quality_row = next(r for r in result["acceptance"] if r["criterion"].startswith("Hit@5"))
        self.assertEqual(quality_row["status"], "BLOCKED")

    def test_result_matches_required_top_level_keys(self):
        args = _args(self.output_path, self.manifest_path, fixtures=True)
        result, _ = eval_embeddings.run_fixtures(args)
        required = {
            "task_id", "profile", "status", "source_sha", "recorded_at", "evidence_level",
            "dependencies", "commands", "cases", "acceptance", "artifacts", "cost",
            "limitations", "blockers",
        }
        self.assertTrue(required.issubset(result.keys()), required - result.keys())
        self.assertRegex(result["source_sha"], r"^[0-9a-f]{40}$")


@unittest.skipUnless(
    os.environ.get("SERENITY_BIN") and os.environ.get("SEEDBRAIN_BIN"),
    "SERENITY_BIN/SEEDBRAIN_BIN not set -- build ./cmd/serenity and "
    "./evals/hosted/fixtures/seedbrain under the R-build-lease protocol and export their paths",
)
class TestLexicalControlArm(unittest.TestCase):
    def test_lexical_arm_runs_and_never_exceeds_the_quality_floor_either(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = json.loads(TEMPLATE_PATH.read_text())
            manifest["task_id"] = "T23.43"
            manifest_path = Path(tmp) / "manifest.json"
            manifest_path.write_text(json.dumps(manifest))
            args = _args(Path(tmp) / "out.json", manifest_path, fixtures=True)
            result, _ = eval_embeddings.run_fixtures(args)
            self.assertEqual(result["cases"]["skipped"], 0)
            lexical_summary = result["_debug"]["lexical_summary"]
            self.assertIsNotNone(lexical_summary)
            # A local synthetic disposable brain queried without live
            # embeddings must not clear the real quality bar either.
            self.assertFalse(lexical_summary["overall_floor_pass"])


POS_TOKEN, EMPTY_TOKEN = "pos-token-SENTINEL-1", "empty-token-SENTINEL-2"
POS_ENV, EMPTY_ENV = "T2343_TEST_CREDENTIAL", "T2343_TEST_EMPTY_CREDENTIAL"
GENEROUS = dict(max_calls=10_000, max_input_tokens=10**9, approved_max_usd=1000.0)


def _no_lexical_hits():
    """Injected lexical control: every case misses, as the real control does
    on this corpus. Real binaries are exercised by TestLexicalControlArm."""
    corpus = json.loads(CORPUS_PATH.read_text())
    pos = [c for c in corpus["cases"] if c["category"] != "empty"]
    return (
        [scoring.score_positive_case(c["id"], c["category"], c["expected_fact_id"], []) for c in pos],
        [],
    )


def _all_lexical_hits():
    corpus = json.loads(CORPUS_PATH.read_text())
    pos = [c for c in corpus["cases"] if c["category"] != "empty"]
    return (
        [scoring.score_positive_case(c["id"], c["category"], c["expected_fact_id"], [c["expected_fact_id"]]) for c in pos],
        [],
    )


class LiveFixture(unittest.TestCase):
    """A FakeHostedMCP with two isolated accounts, a real --seed run against
    it (producing real receipts), and helpers to build live manifests on top.
    Nothing here touches a real network, credential or paid provider."""

    reflect: str | None = None

    def setUp(self):
        # One wall clock shared by the harness and the fake server. The seed run
        # sleeps on it, so the empty-case account's real TTL expiry is crossed
        # instantly while the server still evaluates now >= valid_until.
        self.clock = FakeClock()
        self.fake = FakeHostedMCP({POS_TOKEN: "acct-pos", EMPTY_TOKEN: "acct-empty"}, clock=self.clock)
        self.origin = self.fake.start()
        self.addCleanup(self.fake.stop)
        self.tmp = Path(tempfile.mkdtemp(prefix="t2343-live-"))
        self.addCleanup(lambda: __import__("shutil").rmtree(self.tmp, ignore_errors=True))
        for name, val in ((POS_ENV, POS_TOKEN), (EMPTY_ENV, EMPTY_TOKEN)):
            os.environ[name] = val
            self.addCleanup(os.environ.pop, name, None)
        patcher = mock.patch.object(eval_embeddings, "dirty_paths", return_value=[])
        patcher.start()
        self.addCleanup(patcher.stop)
        self.corpus = json.loads(CORPUS_PATH.read_text())
        self.fact_by_id = {f["id"]: f for f in json.loads(FACTS_PATH.read_text())["facts"]}

    # -- manifest builders -------------------------------------------------
    def manifest_dict(self, phase: str, *, receipts: dict | None = None, **budget) -> dict:
        m = json.loads(TEMPLATE_PATH.read_text())
        m["task_id"] = "T23.43"
        b = dict(GENEROUS, max_elapsed_seconds=600, max_cost_per_call_usd=0.001,
                 authorization_ref="test-authorization", automatic_reset=False, auto_top_up=False)
        b.update(budget)
        m["budget"] = b
        m["provider"].update(model="test-model", version_pin="test-pin-1", dimensions=8,
                             serving_provider="test-serving", privacy_review_ref="test-privacy", secret_ref="EMBEDDINGS_API_KEY")
        m["environment"] = {"kind": "disposable", "origin": None, "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False}
        for key, env in (("hosted_mcp", POS_ENV), ("empty_case_hosted_mcp", EMPTY_ENV)):
            m[key] = {"endpoint_url": f"{self.origin}/mcp", "credential_secret_ref": env, "allowed_origins": [self.origin]}
        m["corpus_sha256"] = _corpus_hash()
        if phase == "seed":
            m["seeding"] = {"authorized": True, "authorization_ref": "test-seeding-authorization"}
        else:
            for role, key in eval_embeddings.TARGETS:
                m[key]["seed_receipt_path"] = str(receipts[role]["path"])
                m[key]["corpus_seeded_confirmation"] = receipts[role]["confirmation"]
        return m

    def write_manifest(self, m: dict, name: str = "manifest.json") -> Path:
        path = self.tmp / name
        path.write_text(json.dumps(m))
        return path

    def seed(self, name: str = "receipts", *, clock=None, sleep=None, **budget) -> tuple[dict, int]:
        rdir = self.tmp / name
        mp = self.write_manifest(self.manifest_dict("seed", **budget), f"{name}-seed-manifest.json")
        args = _mode_args("seed", self.tmp / f"{name}-seed-result.json", mp, "--receipt-dir", str(rdir))
        result, code = eval_embeddings.run_seed(args, clock=clock or self.clock.now, sleep=sleep or self.clock.sleep)
        self.seed_result, self.receipt_dir = result, rdir
        return result, code

    def seeded_receipts(self) -> dict:
        assert self.seed_result["status"] == "PARTIAL", self.seed_result["blockers"]
        return {
            role: {"path": info["path"], "confirmation": info["confirmation"],
                   "usage": json.loads(Path(info["path"]).read_text())["usage"]}
            for role, info in self.seed_result["seed_receipts"].items()
        }

    def prior(self, receipts: dict) -> tuple[int, int]:
        return (sum(r["usage"]["calls_made"] for r in receipts.values()),
                sum(r["usage"]["request_bytes"] for r in receipts.values()))

    def live(self, receipts: dict, *, lexical=_no_lexical_hits, extra_calls=10_000, **budget):
        calls, _ = self.prior(receipts)
        budget.setdefault("max_calls", calls + extra_calls)
        mp = self.write_manifest(self.manifest_dict("live", receipts=receipts, **budget), "live-manifest.json")
        args = _mode_args("live", self.tmp / "live-result.json", mp)
        return eval_embeddings.run_live(args, lexical_provider=lexical)

    def set_oracle(self):
        for c in self.corpus["cases"]:
            if c["category"] != "empty":
                self.fake.oracle[c["query"]] = self.fact_by_id[c["expected_fact_id"]]["text"]

    def positive_ids(self):
        return seeding.plan_fact_ids(self.corpus, seeding.ROLE_POSITIVE)


class TestSeeding(LiveFixture):
    def test_seed_writes_hash_bound_receipts_and_proves_empty_accounts(self):
        result, code = self.seed()
        self.assertEqual((result["status"], code), ("PARTIAL", eval_embeddings.EXIT_PASS))
        self.assertEqual(len(self.fake.accounts["acct-pos"]), len(self.positive_ids()))
        # The empty-case account ends with ZERO current facts: 5 targets were
        # remembered and then removed (3 forgotten, 2 left to expire), not
        # never-authored or left as filler.
        self.assertEqual(self.fake.current_facts("acct-empty"), [])
        self.assertEqual(len(self.fake.forgotten["acct-empty"]), 3)
        self.assertEqual(self.fake.calls("forget"), 3)
        self.assertEqual(len(self.fake.accounts["acct-empty"]), 2)  # the two that lapsed, still stored but expired
        rows = {r["criterion"]: r["status"] for r in result["acceptance"]}
        self.assertTrue(all(v == "PASS" for v in rows.values()), rows)
        for role, info in result["seed_receipts"].items():
            receipt, errs = seeding.load_receipt(Path(info["path"]), info["confirmation"])
            self.assertEqual(errs, [])
            self.assertTrue(receipt["empty_account_proof"]["passed"])
            self.assertEqual(receipt["endpoint_url"], f"{self.origin}/mcp")
        # Seeding measures nothing, so it can never be PASS.
        self.assertNotEqual(result["status"], "PASS")

    def test_non_empty_account_blocks_before_any_remember(self):
        self.fake.preload("acct-pos", "a fact that was already here")
        result, code = self.seed()
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertEqual(self.fake.calls("remember"), 0)
        self.assertIn("not empty", result["blockers"][0]["required_input"])

    def test_two_credentials_for_one_account_are_caught_by_the_second_emptiness_proof(self):
        self.fake.tokens[EMPTY_TOKEN] = "acct-pos"  # both credentials resolve to one account
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("empty_case_hosted_mcp", result["blockers"][0]["required_input"])
        self.assertIn("not empty", result["blockers"][0]["required_input"])
        self.assertEqual(self.fake.calls("remember"), len(self.positive_ids()))  # nothing written for the second target

    def test_seed_refuses_without_explicit_authorization(self):
        m = self.manifest_dict("seed")
        m["seeding"]["authorized"] = "true"  # a truthy string is not the boolean true
        mp = self.write_manifest(m)
        args = _mode_args("seed", self.tmp / "r.json", mp, "--receipt-dir", str(self.tmp / "rc"))
        result, code = eval_embeddings.run_seed(args)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls(), 0)

    def test_seed_never_overwrites_an_existing_receipt(self):
        self.seed()
        calls_after_first = self.fake.calls()
        result, code = self.seed()  # same receipt dir
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("refusing to overwrite", result["blockers"][0]["required_input"])
        self.assertEqual(self.fake.calls(), calls_after_first)

    def test_no_vector_stored_blocks_at_the_first_remember(self):
        self.fake.search_state = "lexical"
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls("remember"), 1)
        self.assertIn("no vector was stored", result["blockers"][0]["required_input"])

    def test_degraded_search_at_the_empty_probe_blocks_before_any_write(self):
        self.fake.degraded = True
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls("remember"), 0)
        self.assertIn("provider unavailable", result["blockers"][0]["required_input"])

    def test_seed_token_cap_never_overshoots_on_a_request_larger_than_what_remains(self):
        """The regression the coordinator required: caps are charged against
        the exact next request BEFORE it is sent, so the server never receives
        more bytes than max_input_tokens, at any cap, including caps where the
        bytes already spent are still below the cap."""
        for cap in (1, 200, 900, 2000, 3300, 5000, 9000):
            with self.subTest(cap=cap):
                self.setUp()
                result, code = self.seed(name=f"cap{cap}", max_input_tokens=cap)
                self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
                self.assertLessEqual(self.fake.bytes_received, cap)
                self.assertLessEqual(result["usage"]["request_bytes"], cap)
                self.assertIn("max_input_tokens", result["blockers"][0]["required_input"])
                self.fake.stop()

    def test_seed_max_calls_is_exact(self):
        result, code = self.seed(max_calls=7)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls(), 7)
        self.assertIn("max_calls=7", result["blockers"][0]["required_input"])


class TestForgottenFactProtocol(LiveFixture):
    """T23.43.md: the 5 expected-empty cases are forgotten/expired cases and
    must return NO current fact. The seed run makes that real: each frozen
    empty query gets a synthetic target that is remembered, shown present,
    forgotten through `forget`, then shown absent. Any step the hosted service
    cannot perform leaves the seed BLOCKED; nothing is scored permissively."""

    def test_receipt_records_presence_forget_ids_and_verified_absence(self):
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_PASS)
        receipt = json.loads(Path(result["seed_receipts"]["empty_case"]["path"]).read_text())
        cases = seeding.empty_case_ids(self.corpus)
        self.assertEqual([p["case_id"] for p in receipt["presence"]], cases)
        self.assertTrue(all(1 <= p["rank"] <= 5 for p in receipt["presence"]))
        self.assertEqual([f["case_id"] for f in receipt["forget"]], seeding.ft.cases_with_mode("forget"))
        self.assertTrue(all(f["expired"] is True and f["remember_id"] in receipt["id_map"] for f in receipt["forget"]))
        self.assertEqual(receipt["post_forget"]["inventory_total"], 0)
        self.assertTrue(all(c["results_returned"] == 0 and c["facts_returned"] == 0 for c in receipt["post_forget"]["cases"]))
        self.assertTrue(receipt["cross_account_probe"]["passed"])
        forget_calls = [e for e in self.fake.log if e["tool"] == "forget"]
        self.assertEqual({e["args"]["id"] for e in forget_calls}, {f["remember_id"] for f in receipt["forget"]})
        row = next(r for r in result["acceptance"] if r["criterion"].startswith("Forgotten/expired protocol"))
        self.assertEqual(row["status"], "PASS")

    def test_targets_use_the_frozen_queries_and_do_not_touch_the_corpus(self):
        self.assertEqual(sorted(seeding.ft.FORGOTTEN_TARGET_TEXT), seeding.empty_case_ids(self.corpus))
        # corpus.json is byte-for-byte what corpus_gen.py freezes, so the
        # reviewed hash (queries, 95 positive cases) did not move.
        self.assertEqual(eval_embeddings.canonical_sha256(self.corpus["cases"]), _corpus_hash())
        for case in self.corpus["cases"]:
            self.assertNotIn(case["query"], seeding.ft.FORGOTTEN_TARGET_TEXT.values())

    def test_a_service_without_forget_blocks_the_seed(self):
        self.fake.forget_supported = False
        result, code = self.seed()
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertIn("isError", result["blockers"][0]["required_input"])

    def test_a_forget_that_reports_success_but_removes_nothing_blocks_the_seed(self):
        self.fake.forget_noop = True
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("still visible", result["blockers"][0]["required_input"])

    def test_a_forgotten_fact_that_stays_searchable_blocks_the_seed(self):
        self.fake.forgotten_search_leak = True
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("still visible", result["blockers"][0]["required_input"])

    def test_a_target_that_was_never_retrievable_makes_absence_meaningless_and_blocks(self):
        self.fake.search_blackhole = {"acct-empty"}
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("presence not demonstrated", result["blockers"][0]["required_input"])
        self.assertEqual(self.fake.calls("forget"), 0)  # never forgets what it could not first find

    def test_a_reachable_cross_account_sentinel_blocks_the_seed(self):
        self.fake.inject_on_query = {seeding.ft.SENTINEL_FACT_TEXT: ("acct-pos", seeding.ft.SENTINEL_FACT_TEXT)}
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("sentinel is reachable", result["blockers"][0]["required_input"])

    def test_receipt_without_the_forget_proofs_is_rejected(self):
        self.seed()
        receipts = self.seeded_receipts()
        for field in ("presence", "forget", "expiry", "expiry_check", "post_forget", "cross_account_probe"):
            with self.subTest(field=field):
                path = Path(receipts["empty_case"]["path"])
                original = path.read_text()
                r = json.loads(original)
                r.pop(field)
                path.write_text(json.dumps(r, indent=2, sort_keys=True) + "\n")
                mutated = json.loads(json.dumps(receipts))
                mutated["empty_case"]["confirmation"] = seeding.receipt_confirmation(json.loads(path.read_text()))
                before = self.fake.calls()
                result, code = self.live(mutated)
                path.write_text(original)
                self.assertEqual(code, eval_embeddings.EXIT_BLOCKED, field)
                self.assertEqual(self.fake.calls(), before, field)


class TestExpiryProtocol(LiveFixture):
    """The TTL half of forgotten/expired. Two of the five empty-case targets are
    remembered with one fixed absolute expiry, shown visible before it, and
    shown absent only after the service's own clock has passed it, with nothing
    called to remove them. The fake evaluates now >= valid_until at query time
    against a clock the harness sleeps on, so the expiry is real semantics, not
    a fact that was simply never stored."""

    ft = seeding.ft

    def empty_receipt(self, result) -> dict:
        return json.loads(Path(result["seed_receipts"]["empty_case"]["path"]).read_text())

    def assert_blocked(self, expect: str, **kw) -> tuple[dict, dict]:
        result, code = self.seed(**kw)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertIn(expect, result["blockers"][0]["required_input"])
        row = next(r for r in result["acceptance"] if r["criterion"].startswith("Forgotten/expired protocol"))
        self.assertEqual(row["status"], "BLOCKED")  # never a misleading PASS
        receipt = self.empty_receipt(result)
        self.assertIs(receipt["complete"], False)
        return result, receipt

    def test_expire_targets_carry_one_absolute_ttl_and_lapse_on_the_server_clock(self):
        ttl, margin = self.ft.EXPIRY_TTL_SECONDS, self.ft.EXPIRY_MARGIN_SECONDS
        started = self.clock.now()
        result, code = self.seed()
        self.assertEqual(code, eval_embeddings.EXIT_PASS)
        receipt = self.empty_receipt(result)
        valid_until, epoch = receipt["expiry"]["valid_until"], receipt["expiry"]["valid_until_epoch"]
        self.assertRegex(valid_until, r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$")  # absolute, whole seconds, UTC
        self.assertGreaterEqual(epoch, started + ttl)
        self.assertLess(epoch, started + ttl + 2)  # rounded up to a whole second, no more

        empty_log = [e for e in self.fake.log if e["account"] == "acct-empty"]
        remembers = {e["args"]["fact"]: e["args"] for e in empty_log if e["tool"] == "remember"}
        for cid in self.ft.cases_with_mode("expire"):
            self.assertEqual(remembers[self.ft.FORGOTTEN_TARGET_TEXT[cid]]["ttl"], valid_until)  # one shared instant
        for cid in self.ft.cases_with_mode("forget"):
            self.assertNotIn("ttl", remembers[self.ft.FORGOTTEN_TARGET_TEXT[cid]])
        forgot = {e["args"]["id"] for e in empty_log if e["tool"] == "forget"}
        self.assertEqual(forgot, {f["remember_id"] for f in receipt["forget"]})
        for e in empty_log:
            if e["tool"] == "forget":
                self.assertIn(receipt["id_map"][e["args"]["id"]], {self.ft.target_id(c) for c in self.ft.cases_with_mode("forget")})

        # On the SERVER's clock: the remembers and every presence probe happen
        # before the expiry, no recall lands inside the margin, and the first
        # recall after the wait is a full margin past it.
        self.assertTrue(all(e["now"] < epoch for e in empty_log if e["tool"] == "remember"))
        recalls = [e for e in empty_log if e["tool"] == "recall"]
        self.assertFalse([e for e in recalls if epoch <= e["now"] < epoch + margin])
        before = [e for e in recalls if e["now"] < epoch]
        after = [e for e in recalls if e["now"] >= epoch + margin]
        self.assertGreaterEqual(len(before), 5 + 2)  # 2 emptiness probes, 1 inventory, 5 presence
        self.assertTrue(after)
        self.assertGreaterEqual(self.clock.now() - started, ttl + margin)  # the wait really happened
        check = receipt["expiry_check"]
        self.assertTrue(check["expired_absent_from_inventory"] and check["forget_targets_still_present"])
        self.assertEqual(check["unexpected_facts"], 0)
        self.assertEqual([c["case_id"] for c in check["cases"]], self.ft.cases_with_mode("expire"))
        self.assertTrue(all(not c["target_in_results"] and not c["target_in_facts"] for c in check["cases"]))
        self.assertGreaterEqual(receipt["expiry"]["probed_after_valid_until_seconds"], margin)
        # ...and the account ends with zero current facts.
        self.assertEqual(self.fake.current_facts("acct-empty"), [])
        self.assertEqual(receipt["post_forget"]["inventory_total"], 0)

    def test_a_service_that_ignores_the_ttl_blocks_because_it_reports_no_expiry(self):
        self.fake.ttl_ignored = True
        self.assert_blocked("did not report the requested expiry")

    def test_a_service_that_does_not_report_valid_until_blocks(self):
        self.fake.ttl_not_echoed = True  # it would expire the fact, but the harness cannot know that
        self.assert_blocked("did not report the requested expiry")

    def test_a_service_that_reports_an_expiry_it_never_applies_blocks_after_the_wait(self):
        self.fake.ttl_lie = True
        started = self.clock.now()
        _, receipt = self.assert_blocked("still visible after its expiry")
        self.assertGreaterEqual(self.clock.now() - started, self.ft.EXPIRY_TTL_SECONDS + self.ft.EXPIRY_MARGIN_SECONDS)
        self.assertEqual(self.fake.calls("forget"), 0)  # blocked at the expiry check, before any forget
        self.assertFalse(receipt["expiry_check"]["expired_absent_from_inventory"])

    def test_an_expired_fact_that_stays_searchable_blocks(self):
        self.fake.expired_searchable = True
        _, receipt = self.assert_blocked("still visible after its expiry")
        check = receipt["expiry_check"]
        self.assertTrue(check["expired_absent_from_inventory"])  # the facts arm dropped them...
        self.assertTrue(any(c["target_in_results"] for c in check["cases"]))  # ...the search index did not

    def test_a_small_clock_skew_is_tolerated_and_a_large_one_blocks(self):
        margin = self.ft.EXPIRY_MARGIN_SECONDS
        self.fake.clock_skew_seconds = margin - 2  # server clock a little behind: still past valid_until after the margin
        result, code = self.seed()
        self.assertEqual((result["status"], code), ("PARTIAL", eval_embeddings.EXIT_PASS), result.get("blockers"))
        self.setUp()
        self.fake.clock_skew_seconds = margin + 30  # far behind: the fact is still alive on the server
        self.assert_blocked("still visible after its expiry")

    def test_a_ttl_that_lapses_before_presence_can_be_shown_blocks_and_names_the_ttl(self):
        self.fake.advance_per_call = 9  # a slow endpoint: the expiry passes between the inventory and the presence probes
        _, receipt = self.assert_blocked("TTL is too short")
        self.assertEqual(self.fake.calls("forget"), 0)
        self.assertNotIn("expiry_check", receipt)

    def test_a_ttl_that_lapses_during_the_remembers_blocks_at_the_inventory(self):
        self.fake.advance_per_call = 15
        self.assert_blocked("TTL is too short")

    def test_an_elapsed_cap_below_the_expiry_floor_blocks_before_any_call(self):
        floor = self.ft.expiry_wait_floor_seconds()
        result, code = self.seed(max_elapsed_seconds=floor - 1)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertIn("max_elapsed_seconds", result["blockers"][0]["required_input"])
        self.assertIn(str(floor), result["blockers"][0]["required_input"])
        self.assertEqual(self.fake.calls(), 0)  # nothing written, nothing to clean up
        # The floor is a necessary minimum, not a sufficient one: the expiry instant is rounded up
        # to a whole second and the run has already spent real time by the wait, so a cap of exactly
        # the floor blocks at the wait ("too little time") instead of starting a wait it cannot finish.
        result, code = self.seed(name="at-floor", max_elapsed_seconds=floor)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("too little time", result["blockers"][0]["required_input"])
        self.setUp()
        result, code = self.seed(name="above-floor", max_elapsed_seconds=floor + 10)
        self.assertEqual(code, eval_embeddings.EXIT_PASS, result.get("blockers"))

    def test_a_wait_that_no_longer_fits_the_remaining_cap_blocks_before_sleeping(self):
        clock = FakeClock()
        slept: list[float] = []
        expiry = seeding.plan_expiry(clock.now)
        with self.assertRaisesRegex(seeding.SeedBlocked, "too little time"):
            seeding._await_expiry(lambda: None, expiry, clock.now, slept.append, lambda: 10)
        self.assertEqual(slept, [])
        # exactly enough: wait + tail fits
        needed = expiry.epoch + self.ft.EXPIRY_MARGIN_SECONDS - clock.now()
        seeding._await_expiry(lambda: None, expiry, clock.now, clock.sleep, lambda: needed + self.ft.EXPIRY_TAIL_SECONDS)

    def test_the_wait_sleeps_in_bounded_chunks_and_is_never_shortened(self):
        clock = FakeClock()
        slept: list[float] = []
        expiry = seeding.plan_expiry(clock.now)
        start = clock.now()

        def sleep(n):
            slept.append(n)
            clock.sleep(n)

        seeding._await_expiry(lambda: None, expiry, clock.now, sleep, None)
        self.assertTrue(all(0 < n <= seeding.WAIT_CHUNK_SECONDS for n in slept))
        self.assertGreater(len(slept), 1)
        self.assertGreaterEqual(clock.now() - start, self.ft.EXPIRY_TTL_SECONDS + self.ft.EXPIRY_MARGIN_SECONDS)
        self.assertGreaterEqual(clock.now(), expiry.epoch + self.ft.EXPIRY_MARGIN_SECONDS)

    def test_a_cap_reached_mid_wait_stops_the_wait(self):
        clock = FakeClock()
        slept: list[float] = []
        expiry = seeding.plan_expiry(clock.now)
        checks = []

        def guard_check():
            checks.append(1)
            if len(checks) == 3:
                raise seeding.SeedBlocked("max_elapsed_seconds reached")

        def sleep(n):
            slept.append(n)
            clock.sleep(n)

        with self.assertRaisesRegex(seeding.SeedBlocked, "max_elapsed_seconds reached"):
            seeding._await_expiry(guard_check, expiry, clock.now, sleep, None)
        self.assertEqual(len(slept), 2)  # the third check refused a third chunk

    def test_a_clock_that_never_advances_blocks_instead_of_spinning(self):
        clock = FakeClock()
        expiry = seeding.plan_expiry(clock.now)
        with self.assertRaisesRegex(seeding.SeedBlocked, "did not advance"):
            seeding._await_expiry(lambda: None, expiry, clock.now, lambda n: None, None)

    def test_a_stray_fact_or_a_vanished_forget_target_during_the_wait_blocks(self):
        def stray(n):
            self.fake.preload("acct-empty", "a fact nobody seeded")
            self.clock.sleep(n)

        self.assert_blocked("changed while the expiry was pending", sleep=stray)
        self.setUp()

        def vanish(n):
            for f in list(self.fake.accounts["acct-empty"]):
                if f["fact"] == self.ft.FORGOTTEN_TARGET_TEXT["empty-01"]:
                    self.fake.accounts["acct-empty"].remove(f)  # the wait sleeps in chunks; only the first has a victim
            self.clock.sleep(n)

        self.assert_blocked("changed while the expiry was pending", sleep=vanish)

    def test_real_clock_real_wait_end_to_end(self):
        """No fake time anywhere: the real wall clock, a real sleep, and the
        server evaluating expiry against the real clock. Timing constants are
        shortened for the test only; the reviewed plan's values are untouched."""
        self.fake.clock = RealClock()
        with mock.patch.object(seeding.ft, "EXPIRY_TTL_SECONDS", 2), mock.patch.object(seeding.ft, "EXPIRY_MARGIN_SECONDS", 1), \
                mock.patch.object(seeding.ft, "EXPIRY_TAIL_SECONDS", 1):
            began = time.time()
            result, code = self.seed(clock=time.time, sleep=time.sleep, max_elapsed_seconds=60)
            elapsed = time.time() - began
        self.assertEqual((result["status"], code), ("PARTIAL", eval_embeddings.EXIT_PASS), result.get("blockers"))
        receipt = self.empty_receipt(result)
        self.assertGreaterEqual(elapsed, 3.0 - 1.0)  # the empty account alone waits >= ttl + margin after its own start
        self.assertGreaterEqual(receipt["expiry"]["probed_after_valid_until_seconds"], 1)
        self.assertEqual(self.fake.current_facts("acct-empty"), [])


class TestExpiryReceiptVerification(LiveFixture):
    """A receipt cannot claim an expiry it did not wait for: every expiry field
    is type- and value-checked, and a forged or truncated proof blocks the live
    run before its first call."""

    def setUp(self):
        super().setUp()
        self.seed()
        self.receipts = self.seeded_receipts()

    def blocked_after(self, mutate, label: str, expect: str | None = None):
        path = Path(self.receipts["empty_case"]["path"])
        original = path.read_text()
        r = json.loads(original)
        mutate(r)
        path.write_text(json.dumps(r, indent=2, sort_keys=True) + "\n")
        mutated = json.loads(json.dumps(self.receipts))
        mutated["empty_case"]["confirmation"] = seeding.receipt_confirmation(json.loads(path.read_text()))
        before = self.fake.calls()
        try:
            result, code = self.live(mutated)
        finally:
            path.write_text(original)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED, label)
        self.assertEqual(self.fake.calls(), before, label)
        if expect:
            self.assertIn(expect, result["blockers"][0]["required_input"], label)

    def test_the_untouched_receipt_is_accepted(self):
        result, code = self.live(self.receipts)
        self.assertNotEqual(code, eval_embeddings.EXIT_BLOCKED, result.get("blockers"))

    def test_forged_or_truncated_expiry_evidence_is_rejected(self):
        mutations = {
            "probed less than a margin after expiry": lambda r: r["expiry"].update(probed_after_valid_until_seconds=1),
            "probe evidence is a bool": lambda r: r["expiry"].update(probed_after_valid_until_seconds=True),
            "probe evidence is a string": lambda r: r["expiry"].update(probed_after_valid_until_seconds="60"),
            "valid_until is not the epoch": lambda r: r["expiry"].update(valid_until_epoch=r["expiry"]["valid_until_epoch"] + 1),
            "valid_until is not RFC 3339": lambda r: r["expiry"].update(valid_until="next tuesday"),
            "epoch is a bool": lambda r: r["expiry"].update(valid_until_epoch=True),
            "wrong expiry cases": lambda r: r["expiry"].update(cases=["empty-01"]),
            "wrong ttl": lambda r: r["expiry"].update(ttl_seconds=1),
            "wrong margin": lambda r: r["expiry"].update(margin_seconds=0),
            "an expire target has no valid_until": lambda r: next(x for x in r["seeded"] if x["corpus_fact_id"] == "forgotten-empty-03").pop("valid_until"),
            "the service reported a different expiry": lambda r: next(x for x in r["seeded"] if x["corpus_fact_id"] == "forgotten-empty-04").update(valid_until_returned="2001-01-01T00:00:00Z"),
            "a forget target carries an expiry": lambda r: next(x for x in r["seeded"] if x["corpus_fact_id"] == "forgotten-empty-01").update(valid_until=r["expiry"]["valid_until"]),
            "expired fact still in inventory": lambda r: r["expiry_check"].update(expired_absent_from_inventory=False),
            "control target was not present": lambda r: r["expiry_check"].update(forget_targets_still_present=False),
            "unknown fact during the wait": lambda r: r["expiry_check"].update(unexpected_facts=1),
            "unknown fact as a bool": lambda r: r["expiry_check"].update(unexpected_facts=False),
            "search still returned an expired target": lambda r: r["expiry_check"]["cases"][0].update(target_in_results=True),
            "facts arm still returned an expired target": lambda r: r["expiry_check"]["cases"][1].update(target_in_facts=True),
            "expiry check covers the wrong cases": lambda r: r["expiry_check"].update(cases=r["expiry_check"]["cases"][:1]),
            "an expire target was also forgotten": lambda r: r["forget"].append({"case_id": "empty-03", "remember_id": next(iter(r["id_map"])), "expired": True}),
            "a forget target is missing from forget": lambda r: r["forget"].pop(),
            "expiry block absent": lambda r: r.pop("expiry"),
            "expiry_check absent": lambda r: r.pop("expiry_check"),
            "expiry is not an object": lambda r: r.update(expiry=[]),
            "stale schema version 1": lambda r: r.update(schema_version=1),
            "schema version is a bool": lambda r: r.update(schema_version=True),
            "supplemental hash from an older plan": lambda r: r.update(forgotten_targets_sha256="0" * 64),
        }
        for label, mutate in mutations.items():
            with self.subTest(mutation=label):
                self.blocked_after(mutate, label)

    def test_the_positive_account_receipt_needs_no_expiry_block(self):
        r = json.loads(Path(self.receipts["positive"]["path"]).read_text())
        self.assertNotIn("expiry", r)
        self.assertEqual(r["schema_version"], seeding.RECEIPT_SCHEMA_VERSION)


class TestCostReporting(LiveFixture):
    """cost.actual_usd is null unless a billing measurement exists. None does,
    so it stays null; the guard's worst-case reservation has its own name."""

    CEILING = 0.001

    def assert_cost_shape(self, cost: dict):
        self.assertIsNone(cost["actual_usd"])
        self.assertIsInstance(cost["operator_ceiling_projection_usd"], float)
        self.assertGreaterEqual(cost["operator_ceiling_projection_usd"], 0)
        self.assertIsInstance(cost["approved_max_usd"], float)
        self.assertTrue(all(isinstance(x, str) for x in cost["authorization_refs"]))

    def test_seed_reports_null_actual_and_a_named_projection(self):
        result, _ = self.seed()
        self.assert_cost_shape(result["cost"])
        self.assertEqual(result["cost"]["operator_ceiling_projection_usd"], round(result["usage"]["calls_this_run"] * self.CEILING, 6))
        self.assertIn(eval_embeddings.COST_LIMITATION, result["limitations"])

    def test_live_projection_counts_seed_receipt_calls_and_actual_stays_null(self):
        self.seed()
        receipts = self.seeded_receipts()
        result, _ = self.live(receipts)
        self.assert_cost_shape(result["cost"])
        calls = self.prior(receipts)[0] + result["usage"]["calls_this_run"]
        self.assertEqual(result["cost"]["operator_ceiling_projection_usd"], round(calls * self.CEILING, 6))
        self.assertGreater(result["cost"]["operator_ceiling_projection_usd"], 0)
        self.assertIn(eval_embeddings.COST_LIMITATION, result["limitations"])
        self.assertNotIn("actual_usd is calls", " ".join(result["limitations"]))  # the old mislabel is gone

    def test_a_blocked_run_still_reports_null_actual(self):
        self.seed()
        receipts = self.seeded_receipts()
        result, code = self.live(receipts, extra_calls=4)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assert_cost_shape(result["cost"])
        self.assertEqual(result["cost"]["operator_ceiling_projection_usd"], round((self.prior(receipts)[0] + 4) * self.CEILING, 6))

    def test_a_seed_blocked_before_any_call_reports_zero_projection_and_null_actual(self):
        result, _ = self.seed(max_elapsed_seconds=seeding.ft.expiry_wait_floor_seconds() - 1)
        self.assertIsNone(result["cost"]["actual_usd"])
        self.assertEqual(result["cost"].get("operator_ceiling_projection_usd", 0), 0)

    def test_fixtures_mode_reports_null_actual(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = json.loads(TEMPLATE_PATH.read_text())
            manifest["task_id"] = "T23.43"
            mp = Path(tmp) / "m.json"
            mp.write_text(json.dumps(manifest))
            result, _ = eval_embeddings.run_fixtures(_args(Path(tmp) / "o.json", mp, fixtures=True))
        self.assertIsNone(result["cost"]["actual_usd"])


class TestLiveModeBudgetEnforcement(LiveFixture):
    """The prior pass's hard-cap regressions, kept with their assertions and
    now run against real seed receipts and a fake speaking the real wire
    shapes. Caps are cumulative with the seed run's recorded spend."""

    def setUp(self):
        super().setUp()
        self.seed()
        self.receipts = self.seeded_receipts()
        self.seed_server_calls = self.fake.calls()

    def live_calls(self) -> int:
        return self.fake.calls() - self.seed_server_calls

    def test_missing_credential_env_blocks_before_any_call(self):
        del os.environ[POS_ENV]
        result, code = self.live(self.receipts)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertEqual(self.live_calls(), 0)

    def test_missing_empty_case_credential_env_blocks_before_any_call(self):
        # Strengthened from the prior pass, which blocked only AFTER spending
        # 95 positive-phase calls: both credentials are now checked up front.
        del os.environ[EMPTY_ENV]
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(result["cases"]["executed"], 0)
        self.assertEqual(self.live_calls(), 0)
        self.assertIn(EMPTY_ENV, result["blockers"][0]["required_input"])

    def test_identical_credential_values_block_before_any_call(self):
        os.environ[EMPTY_ENV] = POS_TOKEN
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.live_calls(), 0)

    def test_budget_exhaustion_stops_at_exact_cap_with_partial_results(self):
        result, code = self.live(self.receipts, extra_calls=4)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        # initialize + inventory + 2 recall queries, then no further request.
        self.assertEqual(self.live_calls(), 4)
        self.assertEqual(result["cases"]["executed"], 2)
        self.assertGreater(result["cases"]["skipped"], 0)
        budget_row = next(r for r in result["acceptance"] if "Budget exhausted" in r["criterion"])
        self.assertEqual(budget_row["status"], "PASS")

    def test_max_calls_of_one_blocks_after_initialize_before_any_tool_call(self):
        result, code = self.live(self.receipts, extra_calls=1)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        # The single budgeted call is spent on initialize() itself.
        self.assertEqual(self.live_calls(), 1)
        self.assertEqual(result["cases"]["executed"], 0)

    def test_near_token_cap_blocks_after_initialize_before_tool_calls(self):
        # Cap = bytes already charged by the seed receipts + exactly the
        # initialize request. initialize fits; the next request cannot.
        _, prior_bytes = self.prior(self.receipts)
        init_bytes = len(mcp_client.request_body(1, "initialize", mcp_client.INITIALIZE_PARAMS))
        result, code = self.live(self.receipts, max_input_tokens=prior_bytes + init_bytes)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.live_calls(), 1)  # only initialize; the cap trips before any tool call
        self.assertIn("max_input_tokens", result["blockers"][0]["required_input"])

    def test_token_cap_smaller_than_the_first_request_sends_nothing(self):
        _, prior_bytes = self.prior(self.receipts)
        result, code = self.live(self.receipts, max_input_tokens=prior_bytes + 1)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.live_calls(), 0)

    def test_live_token_cap_never_overshoots_at_any_cap(self):
        _, prior_bytes = self.prior(self.receipts)
        for extra in (10, 150, 400, 1200, 3000):
            with self.subTest(extra=extra):
                before = self.fake.bytes_received
                result, code = self.live(self.receipts, max_input_tokens=prior_bytes + extra)
                self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
                self.assertLessEqual(self.fake.bytes_received - before, extra)

    def test_dollar_cap_blocks_before_any_network_call(self):
        result, code = self.live(self.receipts, approved_max_usd=0.0005, max_cost_per_call_usd=0.01)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.live_calls(), 0)
        self.assertIn("approved_max_usd", result["blockers"][0]["required_input"])

    def test_seed_spend_counts_against_the_live_caps(self):
        # A live cap that would be ample on its own is exhausted by the seed
        # receipts' recorded calls.
        prior_calls, _ = self.prior(self.receipts)
        result, code = self.live(self.receipts, max_calls=prior_calls)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.live_calls(), 0)

    def test_elapsed_deadline_is_monotonic_and_bounds_the_socket(self):
        import time
        guard = __import__("lib.budget", fromlist=["BudgetGuard"]).BudgetGuard(
            self.manifest_dict("live", receipts=self.receipts)["budget"] | {"max_elapsed_seconds": 0.3}
        )
        self.assertLessEqual(guard.remaining_seconds(), 0.3)
        time.sleep(0.35)
        with self.assertRaises(mcp_client.BudgetExceeded):
            guard.precharge(1)

    def test_full_run_authenticates_positive_and_empty_phases_separately(self):
        result, code = self.live(self.receipts)
        self.assertEqual(result["cases"]["executed"], 100)
        live_log = self.fake.log[self.seed_server_calls:]
        self.assertEqual({e["auth"] for e in live_log}, {f"Bearer {POS_TOKEN}", f"Bearer {EMPTY_TOKEN}"})
        self.assertEqual({e["account"] for e in live_log}, {"acct-pos", "acct-empty"})
        self.assertNotEqual(result["status"], "PASS")  # hash-bag ranking is far below the floor


class TestLiveScoring(LiveFixture):
    def setUp(self):
        super().setUp()
        self.seed()
        self.receipts = self.seeded_receipts()
        self.seed_server_calls = self.fake.calls()

    def test_live_ranks_match_the_local_scorer_over_the_same_facts(self):
        """Proves the wire decode and the source-<id> -> corpus id mapping:
        the hosted fake ranks with the same embedder the fixture arm uses, so
        every per-query top-5 must be identical. Before this repair the
        harness could not read a single real-shaped result."""
        facts = dict(self.fact_by_id)
        facts[seeding.ft.SENTINEL_FACT_ID] = {"text": seeding.ft.SENTINEL_FACT_TEXT}
        self.fake.tiebreak = {facts[fid]["text"]: fid for fid in self.positive_ids()}
        result, _ = self.live(self.receipts)
        local, _ = eval_embeddings.run_local_scorer_arm(HashBagEmbedder(), self.corpus, facts, self.positive_ids())
        by_id = {r.case_id: r for r in local}
        for row in result["per_query"]["positive"]:
            self.assertEqual(row["top5"], by_id[row["case_id"]].ranked_ids[:5], row["case_id"])
        self.assertGreater(sum(r["hit"] for r in result["per_query"]["positive"]), 0)

    def test_perfect_retrieval_and_missed_lexical_control_is_the_only_pass(self):
        self.set_oracle()
        result, code = self.live(self.receipts)
        self.assertEqual((result["status"], code), ("PASS", eval_embeddings.EXIT_PASS), result["acceptance"])
        self.assertEqual(result["cases"]["executed"], 100)
        self.assertEqual(result["per_query"]["empty"] and len(result["per_query"]["empty"]), 5)
        self.assertTrue(all(r["hit"] for r in result["per_query"]["positive"]))

    def test_pass_requires_the_lexical_negative_criterion(self):
        self.set_oracle()
        result, code = self.live(self.receipts, lexical=_all_lexical_hits)
        self.assertEqual((result["status"], code), ("FAIL", eval_embeddings.EXIT_TESTED_FAILURE))
        quality = next(r for r in result["acceptance"] if r["criterion"].startswith("Hit@5"))
        self.assertIn("lexical_negative=0/", quality["observed"])

    def test_missing_lexical_control_blocks_before_any_paid_call(self):
        def unavailable():
            raise eval_embeddings.lexical_control.LexicalControlUnavailable("no binaries")

        result, code = self.live(self.receipts, lexical=unavailable)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls(), self.seed_server_calls)

    def test_degraded_search_blocks_and_stops_calling(self):
        self.fake.degraded = True
        result, code = self.live(self.receipts)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertIn("provider unavailable", result["blockers"][0]["required_input"])
        queries = [e for e in self.fake.log[self.seed_server_calls:] if e["tool"] == "recall" and e["args"].get("query")]
        self.assertEqual(len(queries), 1)  # the first degraded response stops the run

    def test_cross_account_leakage_is_a_tested_failure(self):
        self.set_oracle()
        self.fake.leak_into = {"acct-empty": "acct-pos"}  # the empty account can surface positive facts
        result, code = self.live(self.receipts)
        self.assertEqual((result["status"], code), ("FAIL", eval_embeddings.EXIT_TESTED_FAILURE))
        quality = next(r for r in result["acceptance"] if r["criterion"].startswith("Hit@5"))
        self.assertNotIn("empty_leakage=0;", quality["observed"])
        self.assertTrue(any(r["forbidden_ids"] for r in result["per_query"]["empty"]))

    def test_every_expected_empty_query_must_return_nothing(self):
        # A result for an expected-empty query is a violation whatever it is;
        # here another account's fact surfaces in the empty account's search.
        self.set_oracle()
        self.fake.inject_on_query = {
            self.corpus["cases"][-1]["query"]: ("acct-pos", self.fact_by_id[self.positive_ids()[0]]["text"])
        }
        result, code = self.live(self.receipts)
        self.assertEqual((result["status"], code), ("FAIL", eval_embeddings.EXIT_TESTED_FAILURE))
        self.assertEqual(sum(1 for r in result["per_query"]["empty"] if not r["hit"]), 1)

    def test_a_fact_remembered_into_the_empty_account_after_seeding_blocks_before_any_query(self):
        self.fake.preload("acct-empty", "a fact that must not be here")
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("inventory differs", result["blockers"][0]["required_input"])
        empty_queries = [e for e in self.fake.log[self.seed_server_calls:]
                         if e["account"] == "acct-empty" and e["tool"] == "recall" and e["args"].get("query")]
        self.assertEqual(empty_queries, [])

    def test_the_cross_account_sentinel_probe_is_scored_and_required(self):
        self.set_oracle()
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_PASS)
        self.assertEqual(len(result["per_query"]["cross_account_sentinel"]), 1)
        self.fake.leak_into = {"acct-empty": "acct-pos"}
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_TESTED_FAILURE)
        self.assertTrue(result["per_query"]["cross_account_sentinel"][0]["forbidden_ids"])

    def test_foreign_result_in_the_positive_account_blocks_the_run(self):
        self.fake.foreign_slug = "source-" + "0" * 64
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("not in the seed receipt", result["blockers"][0]["required_input"])

    def test_account_drift_since_seeding_blocks_before_any_query(self):
        self.fake.preload("acct-pos", "a fact added after seeding")
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("inventory differs", result["blockers"][0]["required_input"])
        queries = [e for e in self.fake.log[self.seed_server_calls:] if e["tool"] == "recall" and e["args"].get("query")]
        self.assertEqual(queries, [])

    def test_result_records_pin_usage_elapsed_and_configuration_hash(self):
        result, _ = self.live(self.receipts)
        self.assertEqual(result["provider"]["model"], "test-model")
        self.assertEqual(result["provider"]["dimensions"], 8)
        self.assertEqual(len(result["configuration_sha256"]), 64)
        self.assertGreater(result["usage"]["request_bytes"], 0)
        self.assertGreater(result["elapsed_seconds"], 0)
        self.assertEqual(result["cost"]["approved_max_usd"], GENEROUS["approved_max_usd"])

    def test_dirty_tree_blocks_a_live_run(self):
        with mock.patch.object(eval_embeddings, "dirty_paths", return_value=["evals/hosted/lib/scoring.py"]):
            result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(self.fake.calls(), self.seed_server_calls)

    def test_tampered_receipt_fails_the_manifest_hash_before_any_call(self):
        path = Path(self.receipts["positive"]["path"])
        receipt = json.loads(path.read_text())
        receipt["usage"]["calls_made"] = 0
        path.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n")
        result, code = self.live(self.receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("does not match corpus_seeded_confirmation", "; ".join(b["required_input"] for b in result["blockers"]))
        self.assertEqual(self.fake.calls(), self.seed_server_calls)

    def _rewrite_receipt(self, mutate) -> dict:
        """A receipt edited AND re-hashed into the manifest: the hash binds it,
        so only structural validation stands between it and the budget."""
        receipts = json.loads(json.dumps(self.receipts))
        path = Path(receipts["positive"]["path"])
        receipt = json.loads(path.read_text())
        mutate(receipt)
        path.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n")
        receipts["positive"]["confirmation"] = seeding.receipt_confirmation(json.loads(path.read_text()))
        return receipts

    def test_negative_or_malformed_receipt_usage_blocks_instead_of_forging_credit(self):
        cases = {
            "negative": lambda r: r["usage"].update(calls_made=-100),
            "bool": lambda r: r["usage"].update(request_bytes=True),
            "string": lambda r: r["usage"].update(response_bytes="0"),
            "float": lambda r: r["usage"].update(calls_made=1.5),
            "missing": lambda r: r.pop("usage"),
            "schema_bool": lambda r: r.update(schema_version=True),
            "seeded_not_list": lambda r: r.update(seeded="x"),
            "id_map_list": lambda r: r.update(id_map=[]),
            "inventory_none": lambda r: r.update(post_seed_inventory=None),
            "proof_string": lambda r: r.update(empty_account_proof="passed"),
        }
        for name, mutate in cases.items():
            with self.subTest(name):
                before = self.fake.calls()
                result, code = self.live(self._rewrite_receipt(mutate))
                self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED), name)
                self.assertEqual(self.fake.calls(), before, name)
                self._reseed()

    def _reseed(self):
        # _rewrite_receipt edits the receipt file in place, so give the next
        # subtest fresh accounts and a fresh, unedited receipt set.
        self.fake.accounts = {a: [] for a in self.fake.accounts}
        self.seed(name=f"reseed{len(list(self.tmp.glob('reseed*')))}")
        self.receipts = self.seeded_receipts()

    def test_receipt_bound_to_a_different_endpoint_is_rejected(self):
        receipts = self._rewrite_receipt(lambda r: r.update(endpoint_url="http://127.0.0.1:1/mcp"))
        result, code = self.live(receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("endpoint_url", "; ".join(b["required_input"] for b in result["blockers"]))

    def test_empty_case_receipt_must_follow_the_positive_seeding(self):
        receipts = json.loads(json.dumps(self.receipts))
        path = Path(receipts["empty_case"]["path"])
        r = json.loads(path.read_text())
        r["preceded_by_seeded_facts"] = 0
        path.write_text(json.dumps(r, indent=2, sort_keys=True) + "\n")
        receipts["empty_case"]["confirmation"] = seeding.receipt_confirmation(json.loads(path.read_text()))
        result, code = self.live(receipts)
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertIn("after the positive account", "; ".join(b["required_input"] for b in result["blockers"]))


class TestNoCredentialOrUpstreamTextEverEscapes(LiveFixture):
    """A server can reflect the Authorization header into any field it
    controls. Whatever channel it uses, the token must reach no result, no
    receipt and no exception message."""

    def _scan(self, *texts):
        for t in texts:
            for token in (POS_TOKEN, EMPTY_TOKEN, "SENTINEL"):
                self.assertNotIn(token, t)

    def _all_files_text(self) -> str:
        return "".join(p.read_text() for p in self.tmp.rglob("*.json"))

    def test_reflection_during_seeding_never_reaches_results_or_receipts(self):
        for channel in ("jsonrpc_error", "status", "http_error"):
            with self.subTest(channel=channel):
                self.setUp()
                self.fake.reflect = channel
                result, code = self.seed(name=f"r-{channel}")
                self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
                self._scan(json.dumps(result), self._all_files_text())
                self.fake.stop()

    def test_reflection_during_live_never_reaches_the_result(self):
        self.seed()
        receipts = self.seeded_receipts()
        for channel in ("jsonrpc_error", "search_degraded", "slug", "http_error"):
            with self.subTest(channel=channel):
                self.fake.reflect = channel
                result, code = self.live(receipts)
                self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
                self._scan(json.dumps(result), self._all_files_text())

    def test_client_error_messages_contain_only_fixed_classes_and_integers(self):
        self.fake.reflect = "jsonrpc_error"
        client = mcp_client.MCPClient(f"{self.origin}/mcp", POS_TOKEN, allowed_origins=[self.origin])
        client.initialize()
        with self.assertRaises(mcp_client.MCPError) as ctx:
            client.call_tool("recall", {"limit": 1})
        self._scan(str(ctx.exception))
        self.assertIn("code=-32000", str(ctx.exception))

    def test_transport_error_reports_only_the_exception_class(self):
        client = mcp_client.MCPClient("http://127.0.0.1:1/mcp", POS_TOKEN, allowed_origins=["http://127.0.0.1:1"])
        with self.assertRaises(mcp_client.MCPError) as ctx:
            client.initialize()
        self.assertEqual(str(ctx.exception), "transport error calling the hosted endpoint: ConnectionRefusedError")

    def test_response_reads_are_bounded(self):
        self.fake.huge_response = True
        with mock.patch.object(mcp_client, "MAX_RESPONSE_BYTES", 1024):
            client = mcp_client.MCPClient(f"{self.origin}/mcp", POS_TOKEN, allowed_origins=[self.origin])
            with self.assertRaises(mcp_client.MCPError) as ctx:
                client.initialize()
        self.assertIn("exceeds 1024 bytes", str(ctx.exception))
        self.assertLessEqual(client.total_response_bytes, 1024)


class TestPreflight(LiveFixture):
    def preflight(self, phase="seed", receipts=None, **budget):
        m = self.manifest_dict(phase, receipts=receipts, **budget)
        mp = self.write_manifest(m)
        return eval_embeddings.run_preflight(_mode_args("preflight", self.tmp / "pf.json", mp, "--phase", phase))

    def test_ready_manifest_is_partial_and_makes_no_network_call(self):
        result, code = self.preflight()
        self.assertEqual((result["status"], code), ("PARTIAL", eval_embeddings.EXIT_PASS))
        self.assertEqual(self.fake.calls(), 0)
        self.assertEqual(result["plan"]["total_calls"], sum(r["seed_calls"] + r["live_calls"] for r in result["plan"]["per_role"].values()))

    def test_plan_is_a_true_upper_bound_on_bytes_and_exact_on_calls(self):
        result, _ = self.preflight()
        plan = result["plan"]
        self.seed()
        receipts = self.seeded_receipts()
        live, _ = self.live(receipts)
        actual_calls = self.prior(receipts)[0] + live["usage"]["calls_this_run"]
        actual_bytes = self.prior(receipts)[1] + live["usage"]["request_bytes"]
        self.assertEqual(actual_calls, plan["total_calls"])
        self.assertLessEqual(actual_bytes, plan["total_request_bytes"])
        self.assertLess(plan["total_request_bytes"] - actual_bytes, 4 * plan["total_calls"])  # only request-id digit slack

    def test_caps_below_the_plan_block(self):
        needed, _ = self.preflight()
        plan = needed["plan"]
        for field, low in (("max_calls", plan["total_calls"] - 1),
                           ("max_input_tokens", plan["input_token_upper_bound"] - 1),
                           ("approved_max_usd", plan["total_calls"] * 0.001 - 0.0001)):
            with self.subTest(field=field):
                result, code = self.preflight(**{field: low})
                self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
                self.assertIn(field, result["blockers"][0]["required_input"])

    def test_exactly_the_plan_is_accepted(self):
        needed, _ = self.preflight()
        plan = needed["plan"]
        result, code = self.preflight(max_calls=plan["total_calls"], max_input_tokens=plan["input_token_upper_bound"])
        self.assertEqual(code, eval_embeddings.EXIT_PASS)

    def test_missing_credential_and_dirty_tree_block(self):
        del os.environ[POS_ENV]
        result, code = self.preflight()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)
        self.assertTrue(any(POS_ENV in b["required_input"] for b in result["blockers"]))
        os.environ[POS_ENV] = POS_TOKEN
        with mock.patch.object(eval_embeddings, "dirty_paths", return_value=["scripts/hosted/eval_embeddings.py"]):
            result, code = self.preflight()
        self.assertEqual(code, eval_embeddings.EXIT_BLOCKED)

    def test_plan_reports_the_expiry_timing_and_the_supplemental_hash(self):
        result, _ = self.preflight()
        expiry = result["plan"]["expiry"]
        ft = seeding.ft
        self.assertEqual(expiry["expire_cases"], ft.cases_with_mode("expire"))
        self.assertEqual(expiry["forget_cases"], ft.cases_with_mode("forget"))
        self.assertEqual(
            (expiry["ttl_seconds"], expiry["margin_seconds"], expiry["tail_seconds"]),
            (ft.EXPIRY_TTL_SECONDS, ft.EXPIRY_MARGIN_SECONDS, ft.EXPIRY_TAIL_SECONDS),
        )
        self.assertEqual(expiry["min_elapsed_seconds"], ft.expiry_wait_floor_seconds())
        self.assertEqual(expiry["supplemental_sha256"], ft.targets_sha256())

    def test_seed_phase_blocks_an_elapsed_cap_below_the_expiry_floor(self):
        floor = seeding.ft.expiry_wait_floor_seconds()
        result, code = self.preflight(max_elapsed_seconds=floor - 1)
        self.assertEqual((result["status"], code), ("BLOCKED", eval_embeddings.EXIT_BLOCKED))
        self.assertIn("max_elapsed_seconds", result["blockers"][0]["required_input"])
        self.assertEqual(self.fake.calls(), 0)
        result, code = self.preflight(max_elapsed_seconds=floor)
        self.assertEqual(code, eval_embeddings.EXIT_PASS)  # the floor is necessary, and preflight checks only that

    def test_live_phase_has_no_expiry_wait_so_a_short_elapsed_cap_is_not_a_preflight_problem(self):
        self.seed()
        receipts = self.seeded_receipts()
        result, code = self.preflight("live", receipts, max_calls=10_000, max_elapsed_seconds=5)
        self.assertEqual(code, eval_embeddings.EXIT_PASS, result.get("blockers"))

    def test_live_phase_preflight_verifies_receipts_offline(self):
        self.seed()
        receipts = self.seeded_receipts()
        before = self.fake.calls()
        result, code = self.preflight("live", receipts, max_calls=10_000)
        self.assertEqual(code, eval_embeddings.EXIT_PASS, result.get("blockers"))
        self.assertEqual(self.fake.calls(), before)


class TestSupplementalPlanAndFrozenScope(unittest.TestCase):
    """The supplemental plan (targets, removal modes, expiry timing) is what
    task41's reviewer is asked to confirm by hash. It must not be able to move
    silently, and it must leave everything already frozen untouched."""

    ft = seeding.ft
    FROZEN_CORPUS_SHA256 = "f2593f81a5935a0e763672a057195f57c3b81475f21f78132689903ce56b56b6"

    def test_frozen_corpus_and_thresholds_are_unchanged(self):
        self.assertEqual(_corpus_hash(), self.FROZEN_CORPUS_SHA256)
        corpus = json.loads(CORPUS_PATH.read_text())
        self.assertEqual(eval_embeddings.canonical_sha256(corpus["cases"]), self.FROZEN_CORPUS_SHA256)
        self.assertEqual(len(corpus["cases"]), 100)
        self.assertEqual(sum(1 for c in corpus["cases"] if c["category"] != "empty"), 95)
        self.assertEqual(sum(1 for c in corpus["cases"] if c["category"] == "empty"), 5)
        self.assertEqual(
            scoring.CATEGORY_FLOORS,
            {"paraphrase": (36, 40), "name_entity": (19, 20), "preference": (19, 20), "multilingual": (9, 10), "temporal": (5, 5)},
        )
        self.assertEqual(scoring.OVERALL_HIT_AT_5_MIN, 0.90)
        self.assertEqual((scoring.LEXICAL_NEGATIVE_MIN_HITS, scoring.LEXICAL_NEGATIVE_MIN_DENOM), (18, 20))

    def test_every_empty_case_has_exactly_one_removal_mode_and_both_modes_are_used(self):
        empties = seeding.empty_case_ids(json.loads(CORPUS_PATH.read_text()))
        self.assertEqual(sorted(self.ft.TARGET_MODE), empties)
        self.assertEqual(sorted(self.ft.FORGOTTEN_TARGET_TEXT), empties)
        expire, forget = self.ft.cases_with_mode("expire"), self.ft.cases_with_mode("forget")
        self.assertTrue(expire and forget)
        self.assertEqual(sorted(expire + forget), empties)
        self.assertEqual(set(self.ft.TARGET_MODE.values()), {"expire", "forget"})

    def test_the_supplemental_hash_covers_texts_modes_timing_and_sentinel(self):
        base = self.ft.targets_sha256()
        self.assertRegex(base, r"^[0-9a-f]{64}$")
        self.assertEqual(base, self.ft.targets_sha256())  # deterministic
        for label, patch in (
            ("ttl", mock.patch.object(self.ft, "EXPIRY_TTL_SECONDS", self.ft.EXPIRY_TTL_SECONDS + 1)),
            ("margin", mock.patch.object(self.ft, "EXPIRY_MARGIN_SECONDS", self.ft.EXPIRY_MARGIN_SECONDS + 1)),
            ("tail", mock.patch.object(self.ft, "EXPIRY_TAIL_SECONDS", self.ft.EXPIRY_TAIL_SECONDS + 1)),
            ("a mode", mock.patch.dict(self.ft.TARGET_MODE, {"empty-01": "expire"})),
            ("a text", mock.patch.dict(self.ft.FORGOTTEN_TARGET_TEXT, {"empty-01": "changed"})),
            ("the sentinel", mock.patch.object(self.ft, "SENTINEL_FACT_TEXT", "changed")),
        ):
            with self.subTest(changed=label), patch:
                self.assertNotEqual(self.ft.targets_sha256(), base)

    def test_the_reviewed_docs_pin_the_current_supplemental_hash(self):
        doc = (REPO_ROOT / "docs" / "launch" / "hosted-completion" / "embedding-eval.md").read_text()
        self.assertTrue(self.ft.targets_sha256() in doc, "embedding-eval.md must name the supplemental hash the freeze review confirms")
        self.assertTrue(self.FROZEN_CORPUS_SHA256 in doc, "embedding-eval.md must name the frozen corpus hash")
        self.assertFalse("7f5e08628f903686403eb8a48c7841a4ff9bfa3cb3cd2d36d54de0f196251714" in doc, "the superseded supplemental hash must not linger")

    def test_the_docs_call_the_lexical_negative_draft_an_unapproved_raw_count(self):
        doc = (REPO_ROOT / "docs" / "launch" / "hosted-completion" / "embedding-eval.md").read_text()
        flat = " ".join(doc.split())
        self.assertIn("unapproved", flat)
        self.assertIn("raw count", flat)
        self.assertIn("18/21 = 85.7%", flat)
        self.assertIn("exact subset", flat)
        self.assertIn("exact ratio", flat)
        # "proportional" may appear only negated.
        for m in re.finditer(r"proportional", flat):
            self.assertRegex(flat[max(0, m.start() - 20) : m.start()], r"not (a )?$", "docs may name 'proportional' only to negate it")

    def test_targets_are_synthetic_and_unrelated_to_any_frozen_query_text(self):
        corpus = json.loads(CORPUS_PATH.read_text())
        queries = {c["query"] for c in corpus["cases"]}
        for text in self.ft.FORGOTTEN_TARGET_TEXT.values():
            self.assertNotIn(text, queries)


class TestGitAndCorpusIntegrity(unittest.TestCase):
    def test_dirty_paths_reports_uncommitted_harness_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            run = lambda *a: subprocess.run(a, cwd=tmp, check=True, capture_output=True)  # noqa: E731
            run("git", "init", "-q")
            run("git", "config", "user.email", "t@example.invalid")
            run("git", "config", "user.name", "t")
            (Path(tmp) / "evals" / "hosted").mkdir(parents=True)
            f = Path(tmp) / "evals" / "hosted" / "x.py"
            f.write_text("a = 1\n")
            run("git", "add", "-A")
            run("git", "commit", "-q", "-m", "init")
            self.assertEqual(eval_embeddings.dirty_paths(Path(tmp)), [])
            f.write_text("a = 2\n")
            self.assertEqual(eval_embeddings.dirty_paths(Path(tmp)), ["evals/hosted/x.py"])

    def _tampered(self, tmp: str, mutate_corpus=None, mutate_facts=None) -> tuple[Path, Path]:
        corpus = json.loads(CORPUS_PATH.read_text())
        facts = json.loads(FACTS_PATH.read_text())
        (mutate_corpus or (lambda c: None))(corpus)
        (mutate_facts or (lambda f: None))(facts)
        cp, fp = Path(tmp) / "corpus.json", Path(tmp) / "facts.json"
        cp.write_text(json.dumps(corpus))
        fp.write_text(json.dumps(facts))
        return cp, fp

    def test_edited_case_with_a_stale_meta_hash_is_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            cp, fp = self._tampered(tmp, mutate_corpus=lambda c: c["cases"][0].update(query="edited to make it pass"))
            with self.assertRaisesRegex(ValueError, "edited outside corpus_gen"):
                eval_embeddings.load_corpus(cp, fp)

    def test_edited_fact_text_with_a_stale_meta_hash_is_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            cp, fp = self._tampered(tmp, mutate_facts=lambda f: f["facts"][0].update(text="edited"))
            with self.assertRaisesRegex(ValueError, "edited outside corpus_gen"):
                eval_embeddings.load_corpus(cp, fp)

    def test_main_writes_a_result_and_exits_3_on_a_tampered_corpus_or_bad_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            cp, fp = self._tampered(tmp, mutate_corpus=lambda c: c["cases"][0].update(query="x"))
            manifest = Path(tmp) / "m.json"
            manifest.write_text("{}")
            out = Path(tmp) / "out.json"
            code = eval_embeddings.main(["--fixtures", "--manifest", str(manifest), "--output", str(out), "--corpus", str(cp), "--facts", str(fp)])
            self.assertEqual(code, eval_embeddings.EXIT_INVALID_INPUT)
            self.assertEqual(json.loads(out.read_text())["status"], "BLOCKED")
            manifest.write_text("not json")
            out2 = Path(tmp) / "out2.json"
            code = eval_embeddings.main(["--fixtures", "--manifest", str(manifest), "--output", str(out2)])
            self.assertEqual(code, eval_embeddings.EXIT_INVALID_INPUT)
            self.assertTrue(out2.exists())


if __name__ == "__main__":
    unittest.main()
