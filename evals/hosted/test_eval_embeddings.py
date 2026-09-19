"""Integration tests for scripts/hosted/eval_embeddings.py: the harness
command contract (docs/launch/hosted-completion/evidence.md) end to end.
Run via: python3 -m unittest discover -s evals/hosted -p "test_*.py"

The lexical-only-control arm additionally runs only when SERENITY_BIN and
SEEDBRAIN_BIN point at real pre-built binaries (never built by this test
itself -- see lib/lexical_control.py's docstring on why). Its absence is a
genuine, explicit skip, not a silent pass.
"""

from __future__ import annotations

import http.server
import json
import os
import sys
import tempfile
import threading
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
SCRIPTS_HOSTED = REPO_ROOT / "scripts" / "hosted"
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(SCRIPTS_HOSTED))

import eval_embeddings  # noqa: E402
from lib import scoring  # noqa: E402

CORPUS_PATH = HERE / "corpus.json"
FACTS_PATH = HERE / "facts.json"
TEMPLATE_PATH = REPO_ROOT / "docs" / "launch" / "hosted-completion" / "qualification.example.json"


def _corpus_hash() -> str:
    return json.loads(CORPUS_PATH.read_text())["meta"]["corpus_hash_sha256"]


def _args(tmp_output: Path, manifest_path: Path, *, fixtures: bool, live: bool = False) -> "eval_embeddings.argparse.Namespace":
    argv = ["--manifest", str(manifest_path), "--output", str(tmp_output)]
    argv.append("--fixtures" if fixtures else "--live")
    return eval_embeddings.parse_args(argv)


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


class _RecallStub(http.server.BaseHTTPRequestHandler):
    calls = 0
    lock = threading.Lock()
    seen_auth_headers: list[str] = []

    def log_message(self, *a):  # silence
        pass

    def do_POST(self):
        with _RecallStub.lock:
            _RecallStub.calls += 1
            _RecallStub.seen_auth_headers.append(self.headers.get("Authorization"))
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length) or b"{}")
        req_id = body.get("id")
        method = body.get("method")
        if method == "initialize":
            payload = json.dumps({"jsonrpc": "2.0", "id": req_id, "result": {}}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Mcp-Session-Id", "test-session")
            self.end_headers()
            self.wfile.write(payload)
            return
        if method == "tools/call":
            payload = json.dumps(
                {"jsonrpc": "2.0", "id": req_id, "result": {"protocol_version": 1, "facts": [], "results": []}}
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(payload)
            return
        self.send_response(400)
        self.end_headers()


class TestLiveModeBudgetEnforcement(unittest.TestCase):
    """The manifest requires two separately credentialed targets --
    hosted_mcp for the 95 positive cases and empty_case_hosted_mcp for the 5
    empty cases (manifest.py's rationale: one shared account can't test
    cross-account leakage honestly). Both targets point at the same local
    stub server here since this suite only exercises the budget/credential
    mechanics, not real account isolation (that property is enforced by
    provisioning, not by this harness) -- but each target still carries its
    own credential_secret_ref, and _RecallStub records which Authorization
    header arrived with every request so a test can prove the two clients
    really did authenticate separately.
    """

    def setUp(self):
        _RecallStub.calls = 0
        _RecallStub.seen_auth_headers = []
        self.server = http.server.HTTPServer(("127.0.0.1", 0), _RecallStub)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.server.shutdown)
        self.addCleanup(self.thread.join, 2)
        self.addCleanup(self.server.server_close)

        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.cred_env = "T2343_TEST_CREDENTIAL"
        os.environ[self.cred_env] = "test-token"
        self.addCleanup(os.environ.pop, self.cred_env, None)
        self.empty_cred_env = "T2343_TEST_EMPTY_CREDENTIAL"
        os.environ[self.empty_cred_env] = "test-empty-token"
        self.addCleanup(os.environ.pop, self.empty_cred_env, None)

    def _manifest(
        self,
        max_calls: int = 100,
        *,
        approved_max_usd: float = 1.0,
        max_cost_per_call_usd: float = 0.001,
        max_input_tokens: float = 5000,
    ) -> Path:
        m = json.loads(TEMPLATE_PATH.read_text())
        m["task_id"] = "T23.43"
        m["budget"] = {
            "approved_max_usd": approved_max_usd,
            "max_calls": max_calls,
            "max_input_tokens": max_input_tokens,
            "max_elapsed_seconds": 600,
            "max_cost_per_call_usd": max_cost_per_call_usd,
            "authorization_ref": "test-authorization",
            "automatic_reset": False,
            "auto_top_up": False,
        }
        m["provider"]["secret_ref"] = "EMBEDDINGS_API_KEY"
        m["provider"]["allow_fallback"] = False
        port = self.server.server_address[1]
        origin = f"http://127.0.0.1:{port}"
        m["hosted_mcp"] = {
            "endpoint_url": f"{origin}/mcp",
            "credential_secret_ref": self.cred_env,
            "corpus_seeded_confirmation": "test-receipt",
            "allowed_origins": [origin],
        }
        m["empty_case_hosted_mcp"] = {
            "endpoint_url": f"{origin}/mcp",
            "credential_secret_ref": self.empty_cred_env,
            "corpus_seeded_confirmation": "test-receipt-empty",
            "allowed_origins": [origin],
        }
        m["corpus_sha256"] = _corpus_hash()
        path = Path(self.tmpdir.name) / "manifest.json"
        path.write_text(json.dumps(m))
        return path

    def test_missing_credential_env_blocks_before_any_call(self):
        manifest_path = self._manifest(max_calls=50)
        del os.environ[self.cred_env]
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual(_RecallStub.calls, 0)
        os.environ[self.cred_env] = "test-token"  # restore for addCleanup symmetry

    def test_missing_empty_case_credential_env_blocks_after_positive_phase(self):
        # A generous max_calls lets all 95 positive cases finish, so the
        # empty-case phase is reached and its own missing credential is
        # what blocks -- proving the two credentials are checked
        # independently, not just the first one.
        manifest_path = self._manifest(max_calls=200)
        del os.environ[self.empty_cred_env]
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(result["cases"]["executed"], 95)
        self.assertIn(self.empty_cred_env, result["blockers"][0]["required_input"])
        os.environ[self.empty_cred_env] = "test-empty-token"  # restore for addCleanup symmetry

    def test_budget_exhaustion_stops_at_exact_cap_with_partial_results(self):
        max_calls = 4
        manifest_path = self._manifest(max_calls=max_calls)
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(result["status"], "BLOCKED")
        # initialize() consumes one call, so exactly max_calls total requests reach the server.
        self.assertLessEqual(_RecallStub.calls, max_calls)
        self.assertGreater(result["cases"]["skipped"], 0)
        budget_row = next(r for r in result["acceptance"] if "Budget exhausted" in r["criterion"])
        self.assertEqual(budget_row["status"], "PASS")

    def test_max_calls_of_one_blocks_after_initialize_before_any_tool_call(self):
        manifest_path = self._manifest(max_calls=1)
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        # The single budgeted call is spent on initialize() itself -- proof
        # that initialize is charged against max_calls like any other
        # request, not given a free pass.
        self.assertEqual(_RecallStub.calls, 1)
        self.assertEqual(result["cases"]["executed"], 0)

    def test_near_token_cap_blocks_after_initialize_before_tool_calls(self):
        manifest_path = self._manifest(max_input_tokens=1)
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(_RecallStub.calls, 1)  # only initialize; the token cap trips before any tool call
        self.assertIn("max_input_tokens", result["blockers"][0]["required_input"])

    def test_dollar_cap_blocks_before_any_network_call(self):
        # approved_max_usd smaller than a single call's cost means even the
        # handshake is unaffordable -- must block closed before initialize
        # ever reaches the wire, per "if cost cannot be bounded, BLOCKED
        # before network."
        manifest_path = self._manifest(max_calls=100, approved_max_usd=0.0005, max_cost_per_call_usd=0.01)
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(exit_code, eval_embeddings.EXIT_BLOCKED)
        self.assertEqual(_RecallStub.calls, 0)
        self.assertIn("approved_max_usd", result["blockers"][0]["required_input"])

    def test_full_run_authenticates_positive_and_empty_phases_separately(self):
        manifest_path = self._manifest(max_calls=200)
        args = _args(Path(self.tmpdir.name) / "out.json", manifest_path, fixtures=False, live=True)
        result, exit_code = eval_embeddings.run_live(args)
        self.assertEqual(result["cases"]["executed"], 100)
        auths = set(_RecallStub.seen_auth_headers)
        self.assertIn("Bearer test-token", auths)
        self.assertIn("Bearer test-empty-token", auths)


if __name__ == "__main__":
    unittest.main()
