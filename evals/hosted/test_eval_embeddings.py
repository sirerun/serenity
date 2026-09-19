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
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
SCRIPTS_HOSTED = REPO_ROOT / "scripts" / "hosted"
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(SCRIPTS_HOSTED))

import eval_embeddings  # noqa: E402
from fake_hosted_mcp import FakeHostedMCP  # noqa: E402
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
        self.fake = FakeHostedMCP({POS_TOKEN: "acct-pos", EMPTY_TOKEN: "acct-empty"})
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

    def seed(self, name: str = "receipts", **budget) -> tuple[dict, int]:
        rdir = self.tmp / name
        mp = self.write_manifest(self.manifest_dict("seed", **budget), f"{name}-seed-manifest.json")
        args = _mode_args("seed", self.tmp / f"{name}-seed-result.json", mp, "--receipt-dir", str(rdir))
        result, code = eval_embeddings.run_seed(args)
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
        # remembered and then forgotten, not never-authored or left as filler.
        self.assertEqual(self.fake.accounts["acct-empty"], [])
        self.assertEqual(len(self.fake.forgotten["acct-empty"]), 5)
        self.assertEqual(self.fake.calls("forget"), 5)
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
        self.assertEqual([f["case_id"] for f in receipt["forget"]], cases)
        self.assertTrue(all(f["expired"] is True and f["remember_id"] in receipt["id_map"] for f in receipt["forget"]))
        self.assertEqual(receipt["post_forget"]["inventory_total"], 0)
        self.assertTrue(all(c["results_returned"] == 0 and c["facts_returned"] == 0 for c in receipt["post_forget"]["cases"]))
        self.assertTrue(receipt["cross_account_probe"]["passed"])
        forget_calls = [e for e in self.fake.log if e["tool"] == "forget"]
        self.assertEqual({e["args"]["id"] for e in forget_calls}, set(receipt["id_map"]))
        row = next(r for r in result["acceptance"] if r["criterion"].startswith("Forgotten-fact protocol"))
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
        for field in ("presence", "forget", "post_forget", "cross_account_probe"):
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

    def test_live_phase_preflight_verifies_receipts_offline(self):
        self.seed()
        receipts = self.seeded_receipts()
        before = self.fake.calls()
        result, code = self.preflight("live", receipts, max_calls=10_000)
        self.assertEqual(code, eval_embeddings.EXIT_PASS, result.get("blockers"))
        self.assertEqual(self.fake.calls(), before)


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
