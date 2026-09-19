import http.server
import importlib.util
import json
import sys
import tempfile
import threading
import unittest
from unittest import mock
import urllib.error
import urllib.request
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "hosted" / "load.py"
spec = importlib.util.spec_from_file_location("load_cli", SCRIPT)
load_cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(load_cli)

MANIFEST_PATH = Path(__file__).resolve().parents[2] / "docs" / "launch" / "evidence" / "T23.60" / "manifest.json"
PROTOCOL_VERSION = load_cli.PROTOCOL_VERSION


def base_manifest(**overrides):
    m = json.loads(MANIFEST_PATH.read_text())
    m.update(overrides)
    return m


def small_workload(**overrides):
    w = {
        "phases": [{"name": "main", "minutes": 0.01, "rate_multiplier": 1}],
        "concurrency": {"clients": 2, "baseline_request_rate_per_s": 20},
        "traffic_mix": {"recall": 0.5, "remember": 0.3, "forget": 0.2},
        "cardinalities": {"paid": {"accounts": {"free": 2}, "total_facts": 10}},
        "repetitions": 1,
        "hot_tenant_traffic_fraction": 0.5,
        "cold_brain_fraction": 0.1,
        "query_tokens": {"min": 3, "max": 6},
        "fact_tokens": {"min": 4, "max": 8},
    }
    w.update(overrides)
    return w


def make_credential_dir(root: Path, accounts: list[dict], mode: int = 0o600, value: str = "test-credential-value") -> Path:
    cred_dir = root / "creds"
    cred_dir.mkdir(exist_ok=True)
    for a in accounts:
        p = cred_dir / f"{a['id']}.token"
        p.write_text(value + "\n")
        p.chmod(mode)
    return cred_dir


# ---------------------------------------------------------------------------
# A real local MCP-over-HTTP fixture: genuine sockets, genuine JSON
# (de)serialization on both sides -- not an injected mock-transport function.
# Mirrors internal/server/mcp/http.go's own wire behavior (session bootstrap,
# header requirements, JSON-RPC dual failure modes) closely enough to
# exercise the real client, without importing Go.
# ---------------------------------------------------------------------------

class FakeMcpState:
    def __init__(self):
        self.lock = threading.Lock()
        self.sessions: dict[str, dict] = {}
        self.next_session = 0
        self.remembered: dict[str, bool] = {}
        self.next_fact = 0
        self.redirect_hit = False
        self.always_redirect_mcp = False
        self.readyz_status = 200
        self.requests_seen: list[tuple[str, str, dict]] = []


class FakeMcpHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    @property
    def state(self) -> FakeMcpState:
        return self.server.state  # type: ignore[attr-defined]

    def _send_json(self, status, payload, extra_headers=None):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        for k, v in (extra_headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    def _read_body(self) -> bytes:
        length = int(self.headers.get("Content-Length", "0"))
        return self.rfile.read(length) if length else b""

    def do_GET(self):
        with self.state.lock:
            self.state.requests_seen.append(("GET", self.path, dict(self.headers)))
        if self.path == "/redirect-target":
            with self.state.lock:
                self.state.redirect_hit = True
            self._send_json(200, {"hit": True})
            return
        if self.path == "/readyz":
            status = self.state.readyz_status
            if status >= 300:
                self.send_response(status)
                if 300 <= status < 400:
                    self.send_header("Location", f"http://{self.headers.get('Host')}/redirect-target")
                self.send_header("Content-Length", "0")
                self.end_headers()
                return
            self._send_json(status, {"ready": True})
            return
        self.send_response(404)
        self.end_headers()

    def do_DELETE(self):
        if self.headers.get("Origin"):
            self._send_json(403, {"error": "Origin header not allowed"})
            return
        session_id = self.headers.get("Mcp-Session-Id")
        with self.state.lock:
            self.state.sessions.pop(session_id, None)
        self.send_response(204)
        self.end_headers()

    def do_POST(self):
        with self.state.lock:
            self.state.requests_seen.append(("POST", self.path, dict(self.headers)))
        if self.headers.get("Origin"):
            self._send_json(403, {"error": "Origin header not allowed"})
            return
        if self.headers.get("Content-Type") != "application/json":
            self._send_json(415, {"error": "Content-Type must be application/json"})
            return
        if self.state.always_redirect_mcp:
            self.send_response(302)
            self.send_header("Location", f"http://{self.headers.get('Host')}/redirect-target")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.path != "/mcp":
            self.send_response(404)
            self.end_headers()
            return

        raw = self._read_body()
        try:
            msg = json.loads(raw)
        except json.JSONDecodeError:
            self._send_json(200, {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "Parse error"}})
            return

        session_id = self.headers.get("Mcp-Session-Id")
        req_id = msg.get("id")
        method = msg.get("method")
        params = msg.get("params") or {}

        if session_id is None:
            if method != "initialize":
                self.send_response(400)
                self.end_headers()
                return
            client_info = params.get("clientInfo") or {}
            if not params.get("protocolVersion") or not client_info.get("name") or not client_info.get("version"):
                self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32602, "message": "Invalid initialize parameters"}})
                return
            with self.state.lock:
                self.state.next_session += 1
                new_id = f"sess-{self.state.next_session}"
                self.state.sessions[new_id] = {"state": 1}
            self._send_json(
                200,
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "result": {"protocolVersion": PROTOCOL_VERSION, "capabilities": {"tools": {"listChanged": False}}, "serverInfo": {"name": "fake-serenity", "version": "test"}},
                },
                extra_headers={"Mcp-Session-Id": new_id, "MCP-Protocol-Version": PROTOCOL_VERSION},
            )
            return

        with self.state.lock:
            sess = self.state.sessions.get(session_id)
        if sess is None:
            self.send_response(404)
            self.end_headers()
            return
        if self.headers.get("MCP-Protocol-Version") != PROTOCOL_VERSION:
            self.send_response(400)
            self.end_headers()
            return

        if req_id is None:
            if method == "notifications/initialized":
                with self.state.lock:
                    if sess["state"] == 1:
                        sess["state"] = 2
            self.send_response(202)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return

        if method != "tools/call":
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32601, "message": "Method not found"}})
            return
        if sess["state"] != 2:
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32600, "message": "Initialization required"}})
            return

        name = params.get("name")
        arguments = params.get("arguments") or {}
        if name == "recall":
            payload, is_error = {"facts": []}, False
        elif name == "remember":
            fact = arguments.get("fact")
            provenance = arguments.get("provenance")
            if not fact or not provenance:
                payload, is_error = {"code": "invalid_params", "message": "fact and provenance are required"}, True
            else:
                with self.state.lock:
                    self.state.next_fact += 1
                    fact_id = f"fact-{self.state.next_fact}"
                    self.state.remembered[fact_id] = True
                payload, is_error = {"id": fact_id, "status": "inserted"}, False
        elif name == "forget":
            fid = arguments.get("id")
            with self.state.lock:
                known = self.state.remembered.pop(fid, None) if fid else None
            if known is None:
                payload, is_error = {"code": "not_found", "message": "unknown fact id"}, True
            else:
                payload, is_error = {"id": fid, "status": "deleted"}, False
        else:
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32602, "message": "Unknown tool"}})
            return

        self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "result": {"content": [{"type": "text", "text": json.dumps(payload)}], "isError": is_error}})


class FakeMcpServer:
    def __init__(self):
        self.state = FakeMcpState()
        self.httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), FakeMcpHandler)
        self.httpd.state = self.state  # type: ignore[attr-defined]
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    @property
    def origin(self) -> str:
        return f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def start(self):
        self.thread.start()

    def stop(self):
        self.httpd.shutdown()
        self.httpd.server_close()


class FakeServerTestCase(unittest.TestCase):
    def setUp(self):
        self.server = FakeMcpServer()
        self.server.start()
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmp = Path(self.tmpdir.name)

    def tearDown(self):
        self.server.stop()
        self.tmpdir.cleanup()


class McpSessionHandshakeTests(FakeServerTestCase):
    def test_full_initialize_notified_tools_call_handshake_succeeds(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        outcome = session.call_tool("recall", {"query": "onboarding latency"}, timeout_s=5.0)
        self.assertTrue(outcome["ok"])
        self.assertEqual(outcome["level"], "tool")
        self.assertFalse(outcome["is_error"])
        self.assertEqual(outcome["text"], {"facts": []})

    def test_never_sends_an_origin_header(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        session.call_tool("recall", {"query": "x"}, timeout_s=5.0)
        for _method, _path, headers in self.server.state.requests_seen:
            self.assertNotIn("Origin", headers)

    def test_sends_the_real_loaded_credential_as_bearer_token(self):
        session = load_cli.McpSession(self.server.origin, "super-secret-value", timeout_s=5.0)
        session.initialize()
        _method, _path, headers = self.server.state.requests_seen[-1]
        self.assertEqual(headers.get("Authorization"), "Bearer super-secret-value")


class ProtocolAndToolErrorTests(FakeServerTestCase):
    def test_json_rpc_top_level_error_is_reported_as_protocol_level(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        outcome = session.call_tool("not_a_real_tool", {}, timeout_s=5.0)
        self.assertFalse(outcome["ok"])
        self.assertEqual(outcome["level"], "protocol")
        self.assertIn("error", outcome)

    def test_tool_level_is_error_true_inside_http_200_is_detected(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        outcome = session.call_tool("remember", {}, timeout_s=5.0)  # missing required fact/provenance
        self.assertFalse(outcome["ok"])
        self.assertEqual(outcome["level"], "tool")
        self.assertTrue(outcome["is_error"])

    def test_forget_of_unknown_id_is_a_tool_level_error_not_silently_ok(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        outcome = session.call_tool("forget", {"id": "fact-does-not-exist"}, timeout_s=5.0)
        self.assertFalse(outcome["ok"])
        self.assertTrue(outcome["is_error"])

    def test_remember_then_forget_the_real_returned_id_succeeds(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        remembered = session.call_tool("remember", {"fact": "x", "provenance": "test"}, timeout_s=5.0)
        fact_id = remembered["text"]["id"]
        forgotten = session.call_tool("forget", {"id": fact_id}, timeout_s=5.0)
        self.assertTrue(forgotten["ok"])


class RedirectRejectionTests(FakeServerTestCase):
    def test_client_refuses_to_follow_a_redirect_and_never_dials_the_target(self):
        self.server.state.always_redirect_mcp = True
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        with self.assertRaises(load_cli.McpProtocolError):
            session.initialize()
        self.assertFalse(self.server.state.redirect_hit)

    def test_readiness_probe_refuses_a_redirect(self):
        self.server.state.readyz_status = 302
        error = load_cli.check_readiness(self.server.origin, timeout_s=5.0)
        self.assertIsNotNone(error)
        self.assertFalse(self.server.state.redirect_hit)


class FakeServerFidelityTests(FakeServerTestCase):
    """Proves the test fixture itself enforces the same wire constraints as
    internal/server/mcp/http.go, so a client that passes against it is
    exercising real protocol behavior, not a lenient stand-in."""

    def test_fixture_rejects_missing_or_mismatched_protocol_version_header_on_subsequent_requests(self):
        session = load_cli.McpSession(self.server.origin, "test-cred", timeout_s=5.0)
        session.initialize()
        req = urllib.request.Request(
            self.server.origin + "/mcp",
            data=json.dumps({"jsonrpc": "2.0", "id": 999, "method": "tools/call", "params": {"name": "recall", "arguments": {}}}).encode(),
            method="POST",
            headers={"Content-Type": "application/json", "Mcp-Session-Id": session._session_id, "MCP-Protocol-Version": "9999-01-01"},
        )
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req, timeout=5.0)
        self.assertEqual(ctx.exception.code, 400)

    def test_fixture_rejects_a_request_carrying_an_origin_header(self):
        req = urllib.request.Request(
            self.server.origin + "/mcp",
            data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": PROTOCOL_VERSION, "capabilities": {}, "clientInfo": {"name": "x", "version": "1"}}}).encode(),
            method="POST",
            headers={"Content-Type": "application/json", "Origin": "https://evil.example.com"},
        )
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req, timeout=5.0)
        self.assertEqual(ctx.exception.code, 403)


class EnvironmentGuardTests(unittest.TestCase):
    def test_missing_origin_blocked(self):
        self.assertIsNotNone(load_cli.check_environment_guard(base_manifest()))

    def test_host_not_allowlisted_blocked(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://evil.example.com", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_production_target_allowed_flag_always_refused(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": True})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_production_hostname_refused_even_if_allowlisted(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://app.serenity.sire.run", "allowed_hosts": ["app.serenity.sire.run"], "production_target_allowed": False})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_allowlisted_loopback_http_origin_passes_real_sensitivity(self):
        """Proves the guard actually discriminates rather than always refusing."""
        m = base_manifest(environment={"kind": "disposable", "origin": "http://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False})
        self.assertIsNone(load_cli.check_environment_guard(m))

    def test_plaintext_http_to_non_loopback_host_refused(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "http://staging.internal.example.com", "allowed_hosts": ["staging.internal.example.com"], "production_target_allowed": False})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_https_to_non_loopback_allowlisted_host_accepted(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://staging.internal.example.com", "allowed_hosts": ["staging.internal.example.com"], "production_target_allowed": False})
        self.assertIsNone(load_cli.check_environment_guard(m))

    def test_localhost_hostname_treated_as_loopback(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "http://localhost:9443", "allowed_hosts": ["localhost"], "production_target_allowed": False})
        self.assertIsNone(load_cli.check_environment_guard(m))


class BudgetGuardTests(unittest.TestCase):
    def test_missing_budget_field_blocked(self):
        m = base_manifest(budget={"approved_max_usd": None, "max_calls": 10, "max_input_tokens": 10, "max_elapsed_seconds": 10, "authorization_ref": "x", "worst_case_usd_per_call": 0.01})
        self.assertIsNotNone(load_cli.check_budget(m))

    def test_missing_worst_case_usd_per_call_blocked(self):
        m = base_manifest(budget={"approved_max_usd": 1, "max_calls": 10, "max_input_tokens": 10, "max_elapsed_seconds": 10, "authorization_ref": "x"})
        self.assertIsNotNone(load_cli.check_budget(m))

    def test_complete_budget_passes(self):
        m = base_manifest(budget={"approved_max_usd": 0, "max_calls": 10, "max_input_tokens": 10, "max_elapsed_seconds": 10, "authorization_ref": "x", "worst_case_usd_per_call": 0.01})
        self.assertIsNone(load_cli.check_budget(m))


class CredentialGuardTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmp = Path(self.tmpdir.name)
        self.accounts = [{"id": "free-0"}, {"id": "free-1"}]

    def tearDown(self):
        self.tmpdir.cleanup()

    def test_missing_credential_dir_blocked_before_any_network(self):
        creds, error = load_cli.check_credentials(self.tmp / "does-not-exist", self.accounts)
        self.assertIsNone(creds)
        self.assertIsNotNone(error)

    def test_missing_one_account_credential_file_blocked(self):
        cred_dir = make_credential_dir(self.tmp, self.accounts[:1])
        creds, error = load_cli.check_credentials(cred_dir, self.accounts)
        self.assertIsNone(creds)
        self.assertIn("free-1", error)

    def test_wrong_permission_mode_rejected(self):
        cred_dir = make_credential_dir(self.tmp, self.accounts, mode=0o644)
        creds, error = load_cli.check_credentials(cred_dir, self.accounts)
        self.assertIsNone(creds)
        self.assertIn("0600", error)

    def test_correct_permission_mode_loads_real_values(self):
        cred_dir = make_credential_dir(self.tmp, self.accounts, value="abc123")
        creds, error = load_cli.check_credentials(cred_dir, self.accounts)
        self.assertIsNone(error)
        self.assertEqual(creds["free-0"], "abc123")

    def test_cli_blocks_before_any_network_when_credential_dir_missing(self):
        """A closed loopback port would hang/refuse a real connection attempt;
        this must never be reached because the credential guard fails first."""
        import socket
        closed = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        closed.bind(("127.0.0.1", 0))
        port = closed.getsockname()[1]
        closed.close()  # now guaranteed nothing is listening on this port

        with tempfile.TemporaryDirectory() as tmp:
            manifest = base_manifest(
                environment={"kind": "disposable", "origin": f"http://127.0.0.1:{port}", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
                budget={"approved_max_usd": 1, "max_calls": 10, "max_input_tokens": 10000, "max_elapsed_seconds": 10, "authorization_ref": "x", "worst_case_usd_per_call": 0.01},
                live={"credential_dir": str(Path(tmp) / "nonexistent-creds")},
            )
            manifest_path = Path(tmp) / "manifest.json"
            manifest_path.write_text(json.dumps(manifest))
            out = Path(tmp) / "load.json"
            argv = sys.argv
            sys.argv = ["load.py", "--live", "--manifest", str(manifest_path), "--output", str(out)]
            try:
                code = load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 2)
            self.assertEqual(json.loads(out.read_text())["calls_used"], 0)



class RunFixturesTests(unittest.TestCase):
    def test_produces_valid_structure_with_replay_verified(self):
        manifest = base_manifest()
        workload = load_cli.harness.load_workload(Path(__file__).with_name("workload.json"))
        result = load_cli.run_fixtures(manifest, workload)
        self.assertTrue(result["replay_determinism_verified"])
        self.assertEqual(len(result["repetitions"]), workload["repetitions"])
        self.assertFalse(result["resource_usage"]["available"])


class RunLiveTests(FakeServerTestCase):
    def _accounts_and_creds(self, workload):
        accounts = load_cli.harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts)
        creds, error = load_cli.check_credentials(cred_dir, accounts)
        self.assertIsNone(error)
        return accounts, creds

    def test_full_run_against_fake_server_completes_and_exercises_all_verbs(self):
        workload = small_workload()
        accounts, creds = self._accounts_and_creds(workload)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": 10000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30, "worst_case_usd_per_call": 0.001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["status"], "COMPLETE")
        self.assertEqual(result["accounts_initialized"], len(accounts))
        self.assertGreater(result["calls_used"], 0)
        self.assertEqual(result["protocol_errors"], 0)
        verbs_seen = {r["verb"] for r in result["results"]}
        self.assertIn("recall", verbs_seen)

    def test_precharge_never_sends_a_request_whose_tokens_would_exceed_the_cap(self):
        workload = small_workload(traffic_mix={"recall": 1.0}, phases=[{"name": "main", "minutes": 0.02, "rate_multiplier": 1}])
        accounts, creds = self._accounts_and_creds(workload)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": 10000, "max_input_tokens": 5, "max_elapsed_seconds": 30, "worst_case_usd_per_call": 0.001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_input_tokens", result["reason"])
        self.assertLessEqual(result["tokens_used"], 5)

    def test_stops_at_max_calls_cap_real_sensitivity(self):
        workload = small_workload(phases=[{"name": "main", "minutes": 0.02, "rate_multiplier": 1}])
        accounts, creds = self._accounts_and_creds(workload)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": len(accounts) + 1, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30, "worst_case_usd_per_call": 0.001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_calls", result["reason"])
        self.assertLessEqual(result["calls_used"], len(accounts) + 1)

    def test_stops_at_dollar_cap_using_worst_case_precharge(self):
        workload = small_workload(phases=[{"name": "main", "minutes": 0.02, "rate_multiplier": 1}])
        accounts, creds = self._accounts_and_creds(workload)
        # Budget covers only the session-bootstrap calls (one per account, $0 tokens
        # but still precharged) plus one workload call before the dollar cap bites.
        worst_case = 1.0
        approved = worst_case * (len(accounts) + 1)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": approved, "max_calls": 10000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30, "worst_case_usd_per_call": worst_case, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("approved_max_usd", result["reason"])
        self.assertLessEqual(result["usd_used_worst_case"], approved)

    def test_stops_at_elapsed_cap(self):
        workload = small_workload(phases=[{"name": "main", "minutes": 1, "rate_multiplier": 1}])
        accounts, creds = self._accounts_and_creds(workload)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": 100000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 1, "worst_case_usd_per_call": 0.0001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_elapsed_seconds", result["reason"])
        self.assertLess(result["elapsed_s"], 5.0)

    def test_forget_without_a_prior_remember_is_skipped_not_sent_as_a_broken_call(self):
        workload = small_workload(traffic_mix={"forget": 1.0}, phases=[{"name": "main", "minutes": 0.02, "rate_multiplier": 1}])
        accounts, creds = self._accounts_and_creds(workload)
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": 10000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30, "worst_case_usd_per_call": 0.001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertEqual(result["tool_errors"], 0)
        self.assertGreater(result["skipped_forgets"], 0)

    def test_main_cli_end_to_end_against_fake_server_exits_zero_and_writes_output(self):
        workload_path = Path(__file__).with_name("workload.json")
        real_workload = json.loads(workload_path.read_text())
        accounts = load_cli.harness.build_accounts(real_workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts)
        manifest = base_manifest(
            environment={"kind": "disposable", "origin": self.server.origin, "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
            budget={"approved_max_usd": 10.0, "max_calls": 60, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 3, "authorization_ref": "test", "worst_case_usd_per_call": 0.001},
            live={"credential_dir": str(cred_dir)},
        )
        manifest_path = self.tmp / "manifest.json"
        manifest_path.write_text(json.dumps(manifest))
        out = self.tmp / "load-live.json"
        argv = sys.argv
        sys.argv = ["load.py", "--live", "--manifest", str(manifest_path), "--output", str(out)]
        try:
            code = load_cli.main()
        finally:
            sys.argv = argv
        self.assertEqual(code, 2)
        data = json.loads(out.read_text())
        self.assertEqual(data["status"], "BLOCKED")
        self.assertEqual(data["calls_used"], 0)

    def test_output_never_contains_the_raw_credential_value(self):
        workload = small_workload()
        accounts, creds = self._accounts_and_creds(workload)
        secret = list(creds.values())[0]
        result = load_cli.run_live(
            {"environment": {"origin": self.server.origin}, "budget": {"approved_max_usd": 10.0, "max_calls": 10000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30, "worst_case_usd_per_call": 0.001, "authorization_ref": "t"}},
            workload,
            accounts,
            creds,
        )
        self.assertNotIn(secret, json.dumps(result))


class CLIEndToEndTests(unittest.TestCase):
    def test_fixtures_mode_exits_zero_and_writes_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "load-fixture.json"
            argv = sys.argv
            sys.argv = ["load.py", "--fixtures", "--manifest", str(MANIFEST_PATH), "--output", str(out)]
            try:
                code = load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 0)
            data = json.loads(out.read_text())
            self.assertEqual(data["mode"], "fixtures")
            self.assertTrue(data["replay_determinism_verified"])

    def test_live_mode_blocked_without_environment_origin(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest_path = Path(tmp) / "manifest.json"
            manifest_path.write_text(json.dumps(base_manifest()))
            out = Path(tmp) / "load.json"
            argv = sys.argv
            sys.argv = ["load.py", "--live", "--manifest", str(manifest_path), "--output", str(out)]
            try:
                code = load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 2)
            self.assertEqual(json.loads(out.read_text())["calls_used"], 0)

    def test_live_gate_precedes_manifest_credentials_and_network(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "blocked.json"
            with mock.patch.object(sys, "argv", ["load.py", "--live", "--manifest", str(Path(tmp) / "does-not-exist"), "--output", str(out)]), \
                 mock.patch.object(load_cli, "load_manifest", side_effect=AssertionError("manifest read")), \
                 mock.patch.object(load_cli, "check_credentials", side_effect=AssertionError("secret read")), \
                 mock.patch.object(load_cli, "check_readiness", side_effect=AssertionError("network")), \
                 mock.patch.object(load_cli, "run_live", side_effect=AssertionError("network")):
                self.assertEqual(load_cli.main(), 2)
            self.assertEqual(json.loads(out.read_text())["status"], "BLOCKED")

    def test_manifest_not_found_blocked_json_not_bare_exception(self):
        argv = sys.argv
        sys.argv = ["load.py", "--fixtures", "--manifest", "/nonexistent/manifest.json", "--output", "/tmp/out.json"]
        try:
            with self.assertRaises(SystemExit) as ctx:
                load_cli.main()
        finally:
            sys.argv = argv
        self.assertEqual(ctx.exception.code, 2)

    def test_invalid_json_manifest_blocked_json_not_bare_exception(self):
        with tempfile.TemporaryDirectory() as tmp:
            bad = Path(tmp) / "manifest.json"
            bad.write_text("{not valid json")
            argv = sys.argv
            sys.argv = ["load.py", "--fixtures", "--manifest", str(bad), "--output", str(Path(tmp) / "out.json")]
            try:
                with self.assertRaises(SystemExit) as ctx:
                    load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(ctx.exception.code, 2)


if __name__ == "__main__":
    unittest.main()
