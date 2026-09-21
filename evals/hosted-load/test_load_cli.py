import http.server
import importlib.util
import ipaddress
import json
import os
import random
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from unittest import mock

SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "hosted" / "load.py"
spec = importlib.util.spec_from_file_location("load_cli", SCRIPT)
load_cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(load_cli)
harness = load_cli.harness

MANIFEST_PATH = Path(__file__).resolve().parents[2] / "docs" / "launch" / "evidence" / "T23.60" / "manifest.json"
WORKLOAD_PATH = Path(__file__).with_name("workload.json")
PROTOCOL_VERSION = load_cli.PROTOCOL_VERSION
THRESHOLDS = json.loads(WORKLOAD_PATH.read_text())["thresholds"]

# A long printable credential, so a truncation boundary can fall inside it.
LONG_CRED = "cr3d-" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789" * 9


def base_manifest(**overrides):
    m = json.loads(MANIFEST_PATH.read_text())
    m.update(overrides)
    return m


def small_workload(**overrides):
    w = {
        "phases": [{"name": "steady", "minutes": 0.01, "rate_multiplier": 1}],
        "concurrency": {"clients": 2, "baseline_request_rate_per_s": 20},
        "traffic_mix": {"recall": 0.6, "remember": 0.4},
        "cardinalities": {"paid": {"accounts": {"free": 2}, "total_facts": 10}},
        "repetitions": 1,
        "hot_tenant_traffic_fraction": 0.5,
        "cold_brain_fraction": 0.1,
        "query_tokens": {"min": 3, "max": 6},
        "fact_tokens": {"min": 4, "max": 8},
        "thresholds": THRESHOLDS,
    }
    w.update(overrides)
    return w


def budget_dict(**overrides):
    """A manifest.budget for tests. Values are test-only; none is an authorization."""
    b = {
        "approved_max_usd": 10.0, "max_calls": 10000, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 30,
        "worst_case_usd_per_call": 0.001, "authorization_ref": "test-only",
        "provider_work_bound": {"readiness_tokens": 0, "cold_open_tokens": 0, "basis": "test-only loopback fixture"},
    }
    b.update(overrides)
    return b


def live_manifest(origin, **budget_overrides):
    return {
        "environment": {"kind": "disposable", "origin": origin, "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
        "budget": budget_dict(**budget_overrides),
    }


def make_budget(**overrides):
    fields = dict(
        approved_max_usd=10.0, max_calls=10000, max_input_tokens=10_000_000, max_elapsed_seconds=30, worst_case_usd_per_call=0.001,
        authorization_ref="test-only", readiness_tokens=0, cold_open_tokens=0, provider_work_basis="test-only loopback fixture",
    )
    fields.update(overrides)
    return load_cli.Budget(**fields)


def make_run(**overrides):
    return load_cli.RunBudget(make_budget(**overrides))


def make_credential_dir(root: Path, accounts: list[dict], mode: int = 0o600, value: str = "test-credential-value") -> Path:
    cred_dir = root / "creds"
    cred_dir.mkdir(exist_ok=True)
    for a in accounts:
        p = cred_dir / f"{a['id']}.token"
        p.write_text(value + "\n")
        p.chmod(mode)
    return cred_dir


def unused_port() -> int:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()  # nothing listens here now
    return port


class SilentListener:
    """Accepts TCP connections and never writes a byte (a stalled peer, and for
    https a stalled TLS handshake). Real sockets; no HTTP semantics at all."""

    def __init__(self):
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.bind(("127.0.0.1", 0))
        self.sock.listen(8)
        self.port = self.sock.getsockname()[1]
        self.accepted: list[socket.socket] = []
        self.thread = threading.Thread(target=self._accept, daemon=True)
        self.thread.start()

    def _accept(self):
        while True:
            try:
                conn, _addr = self.sock.accept()
            except OSError:
                return
            self.accepted.append(conn)

    def close(self):
        self.sock.close()
        for conn in self.accepted:
            conn.close()


# ---------------------------------------------------------------------------
# A real local MCP-over-HTTP fixture: genuine sockets, genuine JSON
# (de)serialization on both sides -- not an injected mock-transport function.
# Mirrors internal/server/mcp/http.go's own wire behavior (session bootstrap,
# header requirements, JSON-RPC dual failure modes) closely enough to exercise
# the real client, plus deterministic fault knobs (drip, drop, echo, status).
# ---------------------------------------------------------------------------

class FakeMcpState:
    def __init__(self):
        self.lock = threading.Lock()
        self.stopped = threading.Event()
        self.sessions: dict[str, dict] = {}
        self.next_session = 0
        self.remembered: dict[str, bool] = {}
        self.next_fact = 0
        self.redirect_hit = False
        self.always_redirect_mcp = False
        self.readyz_status = 200
        self.readyz_body = b'{"ready": true}'
        self.requests_seen: list[tuple[str, str, dict]] = []
        self.rpc_seen: list[tuple[str, float]] = []  # (JSON-RPC method, server-side arrival time)
        self.tool_arg_bytes = 0
        self.delete_seen = 0
        self.tools_mode = "ok"
        self.forget_loses = False
        self.tools_delay_s = 0.0
        self.init_mode = "ok"
        self.init_delay_s = 0.0
        self.delete_mode = "ok"  # ok | status:<n> | drop | redirect
        self.deleted_ids: list[str] = []  # session ids the server saw a DELETE for
        self.fail_verb: str | None = None  # with tools_mode "tool_code": fail only this verb (None: every verb)
        self.fail_code = "internal"  # the gateway error code tools_mode "tool_code" returns
        self.fail_extra: dict = {}  # extra fields on that failure (the gateway adds reset_at and upgrade_url to limit_exceeded)
        self.drip_interval_s = 0.02

    def tools_calls_seen(self) -> list[float]:
        with self.lock:
            return [t for m, t in self.rpc_seen if m == "tools/call"]


def gateway_failure(code, **extra):
    """The tool-result payload internal/hosted/gateway/gateway.go failure() and the memory
    verbs' envelope build: error/message/suggestion at the top level, returned as an
    isError tool result over HTTP 200 (never as an HTTP error status)."""
    return {"protocol_version": 1, "error": code, "message": code, "suggestion": "Review your connection and account limits.", **extra}


class FakeMcpHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    @property
    def state(self) -> FakeMcpState:
        return self.server.state  # type: ignore[attr-defined]

    def _send_json(self, status, payload, extra_headers=None):
        self._send_status(status, json.dumps(payload).encode(), {"Content-Type": "application/json", **(extra_headers or {})})

    def _send_status(self, status, body=b"", extra_headers=None):
        self.send_response(status)
        self.send_header("Content-Length", str(len(body)))
        for k, v in (extra_headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    def _write_slowly(self, chunks):
        for chunk in chunks:
            if self.state.stopped.is_set():
                break
            try:
                self.wfile.write(chunk)
                self.wfile.flush()
            except OSError:
                break
            time.sleep(self.state.drip_interval_s)
        self.close_connection = True

    def _drip_body(self, declared_length, sent_bytes):
        self.send_response(200)
        self.send_header("Content-Length", str(declared_length))
        self.end_headers()
        self.wfile.flush()
        self._write_slowly(b"x" for _ in range(sent_bytes))

    def _drip_headers(self):
        # A status line, then a header line that never ends: one byte per interval.
        self.wfile.write(b"HTTP/1.1 200 OK\r\nX-Slow: ")
        self.wfile.flush()
        self._write_slowly(b"a" for _ in range(2000))

    def _stall_after_first_byte(self, http10=False):
        """Headers at once, first body byte at 0.5s, then a stall. Both the
        `Connection: close` and the HTTP/1.0 forms make http.client's response
        take ownership of the socket (conn.sock is cleared by getresponse)."""
        if http10:
            self.protocol_version = "HTTP/1.0"
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.flush()
        self.close_connection = True
        if self.state.stopped.wait(0.5):
            return
        try:
            self.wfile.write(b"x")
            self.wfile.flush()
            self.state.stopped.wait(1.0)
            self.wfile.write(b"y")
            self.wfile.flush()
        except OSError:
            pass

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
        elif self.path == "/readyz":
            status = self.state.readyz_status
            if status >= 300:
                extra = {"Location": f"http://{self.headers.get('Host')}/redirect-target"} if 300 <= status < 400 else {}
                self._send_status(status, self.state.readyz_body if status >= 400 else b"", extra)
            else:
                self._send_json(status, {"ready": True})
        elif self.path == "/stall-after-first-byte":
            self._stall_after_first_byte()
        elif self.path == "/stall-after-first-byte-http10":
            self._stall_after_first_byte(http10=True)
        elif self.path == "/drip-body-30":
            self._drip_body(30, 30)  # a complete 30-byte body, delivered slowly
        elif self.path == "/drip-body":
            self._drip_body(100000, 2000)
        elif self.path == "/drip-headers" or self.path == "/readyz-drip-headers":
            self._drip_headers()
        else:
            self._send_status(404)

    def do_DELETE(self):
        with self.state.lock:
            self.state.delete_seen += 1
        if self.headers.get("Origin"):
            self._send_json(403, {"error": "Origin header not allowed"})
            return
        mode = self.state.delete_mode
        if mode == "drop":
            self.close_connection = True
            return
        if mode == "redirect":
            self._send_status(302, b"", {"Location": f"http://{self.headers.get('Host')}/redirect-target"})
            return
        if mode.startswith("status:"):
            self._send_status(int(mode.split(":")[1]), b"cleanup refused")
            return
        session_id = self.headers.get("Mcp-Session-Id")
        with self.state.lock:
            self.state.sessions.pop(session_id, None)
            self.state.deleted_ids.append(session_id)
        self._send_status(204)

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
            self._send_status(302, b"", {"Location": f"http://{self.headers.get('Host')}/redirect-target"})
            return
        if self.path != "/mcp":
            self._send_status(404)
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
        auth = self.headers.get("Authorization", "")
        with self.state.lock:
            self.state.rpc_seen.append((method, time.monotonic()))

        if session_id is None:
            if method != "initialize":
                self._send_status(400)
                return
            client_info = params.get("clientInfo") or {}
            if not params.get("protocolVersion") or not client_info.get("name") or not client_info.get("version"):
                self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32602, "message": "Invalid initialize parameters"}})
                return
            if self.state.init_delay_s:
                time.sleep(self.state.init_delay_s)
            if self.state.init_mode == "echo_jsonrpc_error":
                self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32000, "message": "x" * 150 + auth}})
                return
            with self.state.lock:
                self.state.next_session += 1
                new_id = f"sess-{self.state.next_session}"
                self.state.sessions[new_id] = {"state": 1}
            if self.state.init_mode in ("deep_json", "not_json"):  # issued a session, then a body that cannot be parsed
                body = b"[" * 200000 if self.state.init_mode == "deep_json" else b"secret upstream text, not JSON"
                self._send_status(200, body, {"Content-Type": "application/json", "Mcp-Session-Id": new_id})
                return
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
            self._send_status(404)
            return
        if self.headers.get("MCP-Protocol-Version") != PROTOCOL_VERSION:
            self._send_status(400)
            return

        if req_id is None:
            if method == "notifications/initialized":
                with self.state.lock:
                    if sess["state"] == 1:
                        sess["state"] = 2
            self._send_status(202)
            return

        if method != "tools/call":
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32601, "message": "Method not found"}})
            return
        if sess["state"] != 2:
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32600, "message": "Initialization required"}})
            return

        name = params.get("name")
        arguments = params.get("arguments") or {}
        with self.state.lock:
            self.state.tool_arg_bytes += len(json.dumps(arguments))
        if self.state.tools_delay_s:
            time.sleep(self.state.tools_delay_s)
        mode = self.state.tools_mode
        if mode == "http500":
            self._send_status(500, b"upstream exploded")
            return
        if mode == "echo_http500":
            self._send_status(500, ("x" * 150 + auth).encode())
            return
        if mode == "http429":
            self._send_status(429, b"slow down", {"Retry-After": "60"})
            return
        if mode == "jsonrpc_error":
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32000, "message": "forced"}})
            return
        if mode == "echo_jsonrpc":
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32000, "message": "y" * 150 + auth}})
            return
        if mode == "drop":
            self.close_connection = True
            return
        if mode == "deep_json":
            self._send_status(200, b"[" * 200000, {"Content-Type": "application/json"})
            return
        if mode == "drip_body":
            self._drip_body(100000, 2000)
            return
        if mode == "stall_body":
            self._stall_after_first_byte()
            return
        if mode == "tool_error":
            payload, is_error = gateway_failure("internal"), True
        elif mode == "echo_tool":
            payload, is_error = {**gateway_failure("internal"), "message": "z" * 150 + auth}, True
        elif mode == "echo_code":  # an unknown error code that carries the credential
            payload, is_error = gateway_failure(auth), True
        elif mode == "tool_code" and self.state.fail_verb in (None, name):
            payload, is_error = gateway_failure(self.state.fail_code, **self.state.fail_extra), True
        elif name == "recall":
            payload, is_error = {"facts": []}, False
        elif name == "remember":
            fact = arguments.get("fact")
            provenance = arguments.get("provenance")
            if not fact or not provenance:
                payload, is_error = gateway_failure("invalid_params"), True
            elif len(fact.encode()) > 4096:  # gateway.go: len(input.Fact) > 4096
                payload, is_error = gateway_failure("input_too_large"), True
            else:
                with self.state.lock:
                    self.state.next_fact += 1
                    fact_id = f"fact-{self.state.next_fact}"
                    self.state.remembered[fact_id] = True
                payload, is_error = {"id": fact_id, "status": "inserted"}, False
        elif name == "forget":
            fid = arguments.get("id")
            with self.state.lock:
                known = self.state.remembered.pop(fid, None) if fid and not self.state.forget_loses else None
            if known is None:
                payload, is_error = gateway_failure("not_found"), True
            else:
                payload, is_error = {"id": fid, "status": "deleted"}, False
        else:
            self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32602, "message": "Unknown tool"}})
            return

        self._send_json(200, {"jsonrpc": "2.0", "id": req_id, "result": {"content": [{"type": "text", "text": json.dumps(payload)}], "isError": is_error}})


class FakeMcpServer:
    def __init__(self, tls_context: ssl.SSLContext | None = None):
        self.state = FakeMcpState()
        self.scheme = "https" if tls_context else "http"
        self.httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), FakeMcpHandler)
        if tls_context:
            self.httpd.socket = tls_context.wrap_socket(self.httpd.socket, server_side=True)
        self.httpd.state = self.state  # type: ignore[attr-defined]
        self.thread = threading.Thread(target=self.httpd.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)  # shutdown() waits one poll

    @property
    def origin(self) -> str:
        return f"{self.scheme}://127.0.0.1:{self.httpd.server_address[1]}"

    @property
    def parsed_origin(self):
        return load_cli.canonical_origin(self.origin)[0]

    def start(self):
        self.thread.start()

    def stop(self):
        self.state.stopped.set()
        self.httpd.shutdown()
        self.httpd.server_close()


class ThresholdEvaluationTests(unittest.TestCase):
    def test_expected_quota_refusals_are_excluded_only_from_admission_denominator(self):
        workload = small_workload()
        records = [
            {"phase": "steady", "outcome": "ok", "verb": "recall", "latency_s": 0.1}
            for _ in range(8)
        ]
        records.extend([
            {"phase": "steady", "outcome": load_cli.TOOL_ERROR, "verb": "remember", "error_code": "limit_exceeded"},
            {"phase": "steady", "outcome": load_cli.REJECTED_ADMISSION, "verb": "recall"},
        ])

        checks = load_cli.evaluate_live_thresholds(workload, records)

        admission = checks["unexpected_admission_rejection_max_pct"]
        self.assertEqual(admission["denominator"], 9)
        self.assertEqual(admission["excluded_expected_quota_refusals"], 1)
        self.assertAlmostEqual(admission["observed"], 100 / 9)
        self.assertEqual(checks["min_offered_completion_pct"]["observed"], 80.0)
        self.assertEqual(checks["unexpected_5xx_max_pct"]["observed"], 0.0)


class FakeServerTestCase(unittest.TestCase):
    def setUp(self):
        self.server = FakeMcpServer()
        self.server.start()
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmp = Path(self.tmpdir.name)

    def tearDown(self):
        self.server.stop()
        self.tmpdir.cleanup()

    def session(self, credential="test-cred", **budget_overrides):
        return load_cli.McpSession(self.server.origin, credential, make_run(**budget_overrides))

    def run_live(self, workload=None, credential="test-credential-value", origin=None, allowed_hosts=None, **budget_overrides):
        workload = workload or small_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts, value=credential)
        creds, error = load_cli.check_credentials(cred_dir, accounts)
        self.assertIsNone(error)
        manifest = live_manifest(origin or self.server.origin, **budget_overrides)
        if allowed_hosts is not None:
            manifest["environment"]["allowed_hosts"] = allowed_hosts
        return load_cli.run_live(manifest, workload, accounts, creds)


# ---------------------------------------------------------------------------
# Handshake, protocol vs tool errors, redirects, fixture fidelity.
# ---------------------------------------------------------------------------

class McpSessionHandshakeTests(FakeServerTestCase):
    def test_full_initialize_notified_tools_call_handshake_succeeds(self):
        session = self.session()
        session.initialize()
        outcome = session.call_tool("recall", {"query": "onboarding latency"})
        self.assertTrue(outcome["ok"])
        self.assertEqual(outcome["level"], "tool")
        self.assertFalse(outcome["is_error"])
        self.assertEqual(outcome["text"], {"facts": []})

    def test_never_sends_an_origin_header(self):
        session = self.session()
        session.initialize()
        session.call_tool("recall", {"query": "x"})
        self.assertEqual(session.close(), ("closed", None))
        for _method, _path, headers in self.server.state.requests_seen:
            self.assertNotIn("Origin", headers)

    def test_sends_the_real_loaded_credential_as_bearer_token(self):
        session = self.session("super-secret-value")
        session.initialize()
        _method, _path, headers = self.server.state.requests_seen[-1]
        self.assertEqual(headers.get("Authorization"), "Bearer super-secret-value")

    def test_rejects_a_malformed_credential_before_any_socket(self):
        for bad in ("", "has space", "line\nbreak", "tab\there", "nonasciié"):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                self.session(bad)
        self.assertEqual(self.server.state.requests_seen, [])


class ProtocolAndToolErrorTests(FakeServerTestCase):
    def test_json_rpc_top_level_error_is_reported_as_protocol_level(self):
        session = self.session()
        session.initialize()
        outcome = session.call_tool("not_a_real_tool", {})
        self.assertFalse(outcome["ok"])
        self.assertEqual(outcome["level"], "protocol")
        self.assertEqual(outcome["jsonrpc_code"], -32602)

    def test_tool_level_is_error_true_inside_http_200_is_detected(self):
        session = self.session()
        session.initialize()
        outcome = session.call_tool("remember", {})  # missing required fact/provenance
        self.assertFalse(outcome["ok"])
        self.assertEqual(outcome["level"], "tool")
        self.assertTrue(outcome["is_error"])

    def test_forget_of_unknown_id_is_a_tool_level_error_not_silently_ok(self):
        session = self.session()
        session.initialize()
        outcome = session.call_tool("forget", {"id": "fact-does-not-exist"})
        self.assertFalse(outcome["ok"])
        self.assertTrue(outcome["is_error"])

    def test_remember_then_forget_the_real_returned_id_succeeds(self):
        session = self.session()
        session.initialize()
        remembered = session.call_tool("remember", {"fact": "x", "provenance": "test"})
        forgotten = session.call_tool("forget", {"id": remembered["text"]["id"]})
        self.assertTrue(forgotten["ok"])

    def test_a_non_object_reply_is_a_protocol_error_not_a_crash(self):
        session = self.session()
        with mock.patch.object(load_cli, "_http_exchange", return_value=(200, {}, b"[1, 2, 3]")):
            with self.assertRaises(load_cli.McpProtocolError):
                session.initialize()

    def test_a_malformed_session_id_from_the_server_is_refused(self):
        session = self.session()
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "result": {}}).encode()
        with mock.patch.object(load_cli, "_http_exchange", return_value=(200, {"Mcp-Session-Id": "bad id\r\nInjected: 1"}, body)):
            with self.assertRaises(load_cli.McpProtocolError):
                session.initialize()


class RedirectRejectionTests(FakeServerTestCase):
    def test_client_refuses_to_follow_a_redirect_and_never_dials_the_target(self):
        self.server.state.always_redirect_mcp = True
        session = self.session()
        with self.assertRaises(load_cli.McpHttpStatusError) as ctx:
            session.initialize()
        self.assertEqual(ctx.exception.status, 302)
        self.assertFalse(self.server.state.redirect_hit)

    def test_readiness_probe_refuses_a_redirect(self):
        self.server.state.readyz_status = 302
        error = load_cli.check_readiness(self.server.origin, make_run())
        self.assertIn("redirect", error)
        self.assertFalse(self.server.state.redirect_hit)

    def test_environment_proxy_variables_are_not_honored(self):
        # A proxy env var must never receive the bearer credential.
        proxy = f"http://127.0.0.1:{unused_port()}"
        with mock.patch.dict(os.environ, {"HTTP_PROXY": proxy, "http_proxy": proxy, "ALL_PROXY": proxy}):
            self.session().initialize()
        self.assertTrue(self.server.state.requests_seen)


class FakeServerFidelityTests(FakeServerTestCase):
    """Proves the test fixture itself enforces the same wire constraints as
    internal/server/mcp/http.go, so a client that passes against it is
    exercising real protocol behavior, not a lenient stand-in."""

    def _raw_post(self, payload, headers):
        req = urllib.request.Request(self.server.origin + "/mcp", data=json.dumps(payload).encode(), method="POST", headers=headers)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        return opener.open(req, timeout=5.0)

    def test_fixture_rejects_missing_or_mismatched_protocol_version_header_on_subsequent_requests(self):
        session = self.session()
        session.initialize()
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            self._raw_post(
                {"jsonrpc": "2.0", "id": 999, "method": "tools/call", "params": {"name": "recall", "arguments": {}}},
                {"Content-Type": "application/json", "Mcp-Session-Id": session._session_id, "MCP-Protocol-Version": "9999-01-01"},
            )
        ctx.exception.close()
        self.assertEqual(ctx.exception.code, 400)

    def test_fixture_rejects_a_request_carrying_an_origin_header(self):
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            self._raw_post(
                {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": PROTOCOL_VERSION, "capabilities": {}, "clientInfo": {"name": "x", "version": "1"}}},
                {"Content-Type": "application/json", "Origin": "https://evil.example.com"},
            )
        ctx.exception.close()
        self.assertEqual(ctx.exception.code, 403)


# ---------------------------------------------------------------------------
# Environment guard, canonical origins, budget parse, credentials.
# ---------------------------------------------------------------------------

def env_manifest(origin, allowed, production_target_allowed=False):
    return base_manifest(environment={"kind": "disposable", "origin": origin, "allowed_hosts": allowed, "production_target_allowed": production_target_allowed})


class EnvironmentGuardTests(unittest.TestCase):
    def test_missing_origin_blocked(self):
        self.assertIsNotNone(load_cli.check_environment_guard(base_manifest()))

    def test_host_not_allowlisted_blocked(self):
        self.assertIsNotNone(load_cli.check_environment_guard(env_manifest("https://evil.example.com", ["127.0.0.1"])))

    def test_production_target_allowed_flag_always_refused(self):
        self.assertIsNotNone(load_cli.check_environment_guard(env_manifest("https://127.0.0.1:9443", ["127.0.0.1"], production_target_allowed=True)))

    def test_production_target_allowed_must_be_exactly_false(self):
        for value in (None, "false", 0, []):
            with self.subTest(value=value):
                m = env_manifest("http://127.0.0.1:9443", ["127.0.0.1"], production_target_allowed=value)
                self.assertIn("exactly false", load_cli.check_environment_guard(m))
        m = env_manifest("http://127.0.0.1:9443", ["127.0.0.1"])
        del m["environment"]["production_target_allowed"]
        self.assertIn("exactly false", load_cli.check_environment_guard(m))

    def test_production_hostname_refused_even_if_allowlisted(self):
        self.assertIn("production", load_cli.check_environment_guard(env_manifest("https://app.serenity.sire.run", ["app.serenity.sire.run"])))

    def test_production_hostname_variants_are_refused_as_production(self):
        cases = [
            ("https://APP.serenity.sire.run", ["app.serenity.sire.run"]),
            ("https://app.serenity.sire.run.", ["app.serenity.sire.run."]),
            ("https://SERENITY.sire.run", ["serenity.sire.run"]),
            ("https://serenity.sire.run.", ["serenity.sire.run"]),
            ("https://staging.example.com", ["staging.example.com", "APP.serenity.sire.run"]),
            ("https://staging.example.com", ["staging.example.com", "serenity.sire.run."]),
        ]
        for origin, allowed in cases:
            with self.subTest(origin=origin, allowed=allowed):
                self.assertIn("production", load_cli.check_environment_guard(env_manifest(origin, allowed)))

    def test_allowlisted_loopback_http_origin_passes_real_sensitivity(self):
        """Proves the guard actually discriminates rather than always refusing."""
        self.assertIsNone(load_cli.check_environment_guard(env_manifest("http://127.0.0.1:9443", ["127.0.0.1"])))

    def test_plaintext_http_to_non_loopback_host_refused(self):
        self.assertIsNotNone(load_cli.check_environment_guard(env_manifest("http://staging.internal.example.com", ["staging.internal.example.com"])))

    def test_https_to_non_loopback_allowlisted_host_accepted(self):
        self.assertIsNone(load_cli.check_environment_guard(env_manifest("https://staging.internal.example.com", ["staging.internal.example.com"])))

    def test_localhost_hostname_treated_as_loopback(self):
        self.assertIsNone(load_cli.check_environment_guard(env_manifest("http://localhost:9443", ["localhost"])))

    def test_ipv6_loopback_literal_is_canonical_and_accepted(self):
        self.assertIsNone(load_cli.check_environment_guard(env_manifest("http://[::1]:9443", ["::1"])))

    def test_allowed_hosts_entries_must_themselves_be_canonical(self):
        for entry in ("*", "LOCALHOST", "localhost.", "127.1", "http://localhost", "localhost:9443", ""):
            with self.subTest(entry=entry):
                self.assertIsNotNone(load_cli.check_environment_guard(env_manifest("http://localhost:9443", ["localhost", entry])))
        self.assertIsNotNone(load_cli.check_environment_guard(env_manifest("http://localhost:9443", [])))


class CanonicalOriginTests(unittest.TestCase):
    GOOD = ["http://127.0.0.1:9443", "http://localhost:9443", "https://staging.example.com", "https://staging.example.com:8443", "http://[::1]:9443"]
    BAD = [
        "http://127.0.0.1:9443/", "http://127.0.0.1:9443/mcp", "HTTP://127.0.0.1:9443", "http://LOCALHOST:9443", "http://localhost.:9443",
        "http://user@127.0.0.1:9443", "http://user:pass@127.0.0.1:9443", "http://127.0.0.1:9443?x=1", "http://127.0.0.1:9443#frag",
        "http://127.0.0.1:80", "https://staging.example.com:443", "http://127.0.0.1:0", "http://127.0.0.1:99999", "http://127.1:9443",
        "http://0x7f.1:9443", "http://2130706433:9443", " http://127.0.0.1:9443", "http://127.0.0.1:9443 ", "http://127.0.0.1:9443\\@evil.example.com",
        "http://127.0.0.1\t:9443", "http://exa mple.com", "http://café.example.com", "ftp://127.0.0.1:9443", "//127.0.0.1:9443", "127.0.0.1:9443",
        "http://", "http://:9443", "http://a..b:9443", "http://-a.example.com", "", None, 9443, ["http://127.0.0.1:9443"],
    ]

    def test_canonical_origins_parse_to_scheme_host_port(self):
        for origin in self.GOOD:
            with self.subTest(origin=origin):
                parsed, error = load_cli.canonical_origin(origin)
                self.assertIsNone(error)
                self.assertEqual(parsed[0], origin.split(":")[0])

    def test_every_noncanonical_origin_is_rejected(self):
        for origin in self.BAD:
            with self.subTest(origin=origin):
                parsed, error = load_cli.canonical_origin(origin)
                self.assertIsNone(parsed)
                self.assertTrue(error)

    def test_the_client_never_dials_a_noncanonical_origin(self):
        run = make_run()
        for origin in self.BAD:
            with self.subTest(origin=origin), mock.patch.object(load_cli, "_http_exchange", side_effect=AssertionError("socket opened")):
                with self.assertRaises(ValueError):
                    load_cli.McpSession(origin, "test-cred", run)
                self.assertTrue(load_cli.check_readiness(origin, run))
        self.assertEqual(run.calls, 0)


class BudgetGuardTests(unittest.TestCase):
    def manifest(self, **overrides):
        return base_manifest(budget=budget_dict(**overrides))

    def test_missing_budget_field_blocked(self):
        self.assertIsNotNone(load_cli.check_budget(self.manifest(approved_max_usd=None)))

    def test_missing_worst_case_usd_per_call_blocked(self):
        b = budget_dict()
        del b["worst_case_usd_per_call"]
        self.assertIsNotNone(load_cli.check_budget(base_manifest(budget=b)))

    def test_complete_budget_passes_and_zero_usd_is_a_valid_value(self):
        self.assertIsNone(load_cli.check_budget(self.manifest()))
        budget, error = load_cli.parse_budget(self.manifest(approved_max_usd=0))
        self.assertIsNone(error)
        self.assertEqual(budget.approved_max_usd, 0)

    def test_nonfinite_boolean_negative_zero_and_malformed_values_are_rejected(self):
        nan, inf = float("nan"), float("inf")
        bad = {
            "approved_max_usd": [nan, inf, -inf, -0.01, True, False, "5", [1], {}],
            "max_calls": [0, -1, 10.5, 10.0, nan, inf, True, "10", None],
            "max_input_tokens": [0, -1, 1.5, nan, inf, True, "10"],
            "max_elapsed_seconds": [0, -1, nan, inf, -inf, True, False, "30", [30]],
            "worst_case_usd_per_call": [0, -0.001, nan, inf, True, "0.01"],
            "authorization_ref": ["", "   ", 7, True, ["x"]],
            "automatic_reset": [True, "false", 0],
            "auto_top_up": [True, "false", 0],
        }
        for field, values in bad.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    self.assertIsNotNone(load_cli.check_budget(self.manifest(**{field: value})))

    def test_nan_and_infinity_literals_in_manifest_json_are_rejected(self):
        # json.loads accepts NaN/Infinity by default; the budget parse must not.
        text = json.dumps(self.manifest()).replace('"max_elapsed_seconds": 30', '"max_elapsed_seconds": NaN').replace('"approved_max_usd": 10.0', '"approved_max_usd": Infinity')
        loaded = json.loads(text)
        self.assertTrue(load_cli.check_budget(loaded))
        self.assertTrue(load_cli.check_budget(json.loads(text.replace("Infinity", "-Infinity"))))

    def test_undeclared_provider_work_bound_blocks_the_run(self):
        for bound in (None, {}, {"readiness_tokens": 0, "cold_open_tokens": 0}, {"readiness_tokens": 0, "cold_open_tokens": 0, "basis": ""}, "declared"):
            with self.subTest(bound=bound):
                self.assertIn("provider_work_bound", load_cli.check_budget(self.manifest(provider_work_bound=bound)))
        for field in ("readiness_tokens", "cold_open_tokens"):
            for value in (-1, 1.5, True, float("nan"), "0", None):
                with self.subTest(field=field, value=value):
                    bound = {"readiness_tokens": 0, "cold_open_tokens": 0, "basis": "test-only"}
                    bound[field] = value
                    self.assertIsNotNone(load_cli.check_budget(self.manifest(provider_work_bound=bound)))

    def test_committed_manifest_budget_is_not_liveable(self):
        self.assertIn("provider_work_bound", load_cli.check_budget(json.loads(MANIFEST_PATH.read_text())))


class RunBudgetLedgerTests(unittest.TestCase):
    def test_a_hand_built_budget_is_held_to_the_same_numeric_rules(self):
        for overrides in ({"max_elapsed_seconds": float("nan")}, {"max_calls": True}, {"approved_max_usd": -1}, {"worst_case_usd_per_call": float("inf")}, {"cold_open_tokens": -1}, {"authorization_ref": ""}):
            with self.subTest(overrides=overrides), self.assertRaises(ValueError):
                make_run(**overrides)

    def test_reserve_rejects_bad_arguments_without_changing_state(self):
        run = make_run()
        bad_calls = [
            dict(operation="tools_call", tokens=-1), dict(operation="tools_call", tokens=True), dict(operation="tools_call", tokens=1.5),
            dict(operation="tools_call", tokens=float("nan")), dict(operation="tools_call", default_timeout_s=float("nan")),
            dict(operation="tools_call", default_timeout_s=float("inf")), dict(operation="tools_call", default_timeout_s=0),
            dict(operation="tools_call", default_timeout_s=-1), dict(operation="tools_call", default_timeout_s=True), dict(operation="teleport"),
        ]
        for kwargs in bad_calls:
            with self.subTest(kwargs=kwargs), self.assertRaises(ValueError):
                run.reserve(**kwargs)
        self.assertEqual((run.calls, run.tokens, run.usd), (0, 0, 0.0))
        self.assertFalse(run.stop_event.is_set())

    def test_negative_tokens_cannot_credit_the_budget(self):
        run = make_run(max_input_tokens=10)
        with self.assertRaises(ValueError):
            run.reserve("tools_call", -1000)
        run.reserve("tools_call", 10)
        with self.assertRaises(load_cli.BudgetExhausted):
            run.reserve("tools_call", 1)

    def test_timeout_is_the_remaining_elapsed_budget_with_no_floor_above_it(self):
        now = [100.0]
        run = load_cli.RunBudget(make_budget(max_elapsed_seconds=5), clock=lambda: now[0])
        self.assertEqual(run.reserve("tools_call"), 5.0)
        now[0] = 104.95
        self.assertAlmostEqual(run.reserve("tools_call"), 0.05, places=6)  # not raised to any minimum
        now[0] = 105.0
        with self.assertRaises(load_cli.BudgetExhausted) as ctx:
            run.reserve("tools_call")
        self.assertIn("max_elapsed_seconds", ctx.exception.reason)

    def test_each_cap_refuses_the_reservation_that_would_cross_it(self):
        cases = [
            ({"max_calls": 2}, {}, "max_calls"),
            ({"max_input_tokens": 9}, {"tokens": 5}, "max_input_tokens"),
            ({"approved_max_usd": 0.0025}, {}, "approved_max_usd"),
        ]
        for overrides, kwargs, needle in cases:
            with self.subTest(needle=needle):
                run = make_run(**overrides)
                with self.assertRaises(load_cli.BudgetExhausted) as ctx:
                    for _ in range(5):
                        run.reserve("tools_call", **kwargs)
                self.assertIn(needle, ctx.exception.reason)
                self.assertTrue(run.stop_event.is_set())


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

    def test_a_credential_with_whitespace_or_control_bytes_is_rejected(self):
        for value in ("two words", "a\tb", "a\x01b"):
            with self.subTest(value=value):
                cred_dir = make_credential_dir(self.tmp, self.accounts, value=value)
                creds, error = load_cli.check_credentials(cred_dir, self.accounts)
                self.assertIsNone(creds)
                self.assertIn("printable ASCII", error)

    def test_a_symlinked_credential_file_is_rejected(self):
        cred_dir = make_credential_dir(self.tmp, self.accounts)
        real = self.tmp / "elsewhere.token"
        real.write_text("x\n")
        real.chmod(0o600)
        link = cred_dir / "free-0.token"
        link.unlink()
        link.symlink_to(real)
        creds, error = load_cli.check_credentials(cred_dir, self.accounts)
        self.assertIsNone(creds)
        self.assertIn("symlink", error)


# ---------------------------------------------------------------------------
# Wall-clock deadline: urllib/http.client timeouts are per-socket-read inactivity
# timeouts; a drip feed defeats them. Real slow servers, real sockets.
# ---------------------------------------------------------------------------

class ExchangeWallDeadlineTests(FakeServerTestCase):
    def exchange(self, path, timeout_s):
        start = time.monotonic()
        try:
            return load_cli._http_exchange("GET", self.server.parsed_origin, path, {}, None, timeout_s), time.monotonic() - start
        except load_cli.ExchangeDeadlineExceeded:
            return None, time.monotonic() - start

    def test_control_plain_urllib_read_outlasts_its_own_socket_timeout(self):
        """The failure this whole class guards against, reproduced: a 30-byte body
        dripped at 20ms/byte takes ~0.6s although the socket timeout is 0.1s."""
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        start = time.monotonic()
        with opener.open(self.server.origin + "/drip-body-30", timeout=0.1) as resp:
            body = resp.read()
        elapsed = time.monotonic() - start
        self.assertEqual(len(body), 30)
        self.assertGreater(elapsed, 0.3)

    def test_the_same_slow_body_is_cut_off_at_the_wall_deadline(self):
        result, elapsed = self.exchange("/drip-body-30", 0.2)
        self.assertIsNone(result)
        self.assertLess(elapsed, 0.2 + 0.4)
        self.assertGreaterEqual(elapsed, 0.19)

    def test_a_response_that_finishes_inside_the_deadline_is_returned_whole(self):
        result, elapsed = self.exchange("/readyz", 5.0)
        self.assertEqual(result[0], 200)
        self.assertLess(elapsed, 2.0)

    def test_an_endless_body_drip_is_cut_off_at_the_deadline(self):
        result, elapsed = self.exchange("/drip-body", 0.3)
        self.assertIsNone(result)
        self.assertLess(elapsed, 0.3 + 0.5)

    def test_delayed_first_byte_then_stall_is_cut_off_at_the_deadline_on_a_closing_response(self):
        """The measured defect: on `Connection: close` (or HTTP/1.0) getresponse()
        clears conn.sock and the response owns the socket, so a watchdog that shuts
        down conn.sock has nothing to act on and read1 waits out its own timeout.
        Budget 0.6s, first byte 0.5s, then a 1s stall: must end at ~0.6s, not ~1.1s."""
        for path in ("/stall-after-first-byte", "/stall-after-first-byte-http10"):
            with self.subTest(path=path):
                result, elapsed = self.exchange(path, 0.6)
                self.assertIsNone(result)
                self.assertGreaterEqual(elapsed, 0.59)
                self.assertLessEqual(elapsed, 0.6 + load_cli.DEADLINE_JITTER_S)

    def test_a_stall_after_the_first_byte_is_cut_off_even_with_a_tiny_inactivity_budget(self):
        result, elapsed = self.exchange("/stall-after-first-byte", 0.55)
        self.assertIsNone(result)
        self.assertLessEqual(elapsed, 0.55 + load_cli.DEADLINE_JITTER_S)

    def test_a_response_that_completes_after_a_late_first_byte_inside_the_deadline_is_returned(self):
        result, elapsed = self.exchange("/stall-after-first-byte", 3.0)  # 0.5s + 1.0s stall, then the second byte
        self.assertEqual(result[0], 200)
        self.assertEqual(result[2], b"xy")
        self.assertLess(elapsed, 2.5)

    def test_an_accepted_connection_that_never_answers_is_cut_off_at_the_deadline(self):
        silent = SilentListener()
        self.addCleanup(silent.close)
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._http_exchange("GET", ("http", "127.0.0.1", silent.port), "/x", {}, None, 0.4)
        self.assertLessEqual(time.monotonic() - start, 0.4 + load_cli.DEADLINE_JITTER_S)

    def test_a_stalled_tls_handshake_is_cut_off_at_the_deadline(self):
        silent = SilentListener()
        self.addCleanup(silent.close)
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._http_exchange("GET", ("https", "127.0.0.1", silent.port), "/x", {}, None, 0.4)
        self.assertLessEqual(time.monotonic() - start, 0.4 + load_cli.DEADLINE_JITTER_S)

    def test_every_socket_the_exchange_owned_is_really_closed_on_every_exit_path(self):
        opened: list[socket.socket] = []
        real_connect = load_cli._connect

        def recording_connect(scheme, host, port, deadline, hold):
            def recording_hold(sock):
                opened.append(sock)
                hold(sock)
            return real_connect(scheme, host, port, deadline, recording_hold)

        silent = SilentListener()
        self.addCleanup(silent.close)
        with mock.patch.object(load_cli, "_connect", recording_connect):
            self.exchange("/readyz", 5.0)  # completes
            self.exchange("/stall-after-first-byte", 0.6)  # deadline while the response owns the socket
            self.exchange("/drip-headers", 0.3)  # deadline in the header phase
            with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
                load_cli._http_exchange("GET", ("https", "127.0.0.1", silent.port), "/x", {}, None, 0.3)  # deadline in a TLS handshake
        self.assertGreaterEqual(len(opened), 5)  # plain sockets, plus the TLS wrapper
        self.assertEqual([s for s in opened if s.fileno() != -1], [])

    def test_a_stalled_response_body_cannot_hold_the_run_past_its_elapsed_cap(self):
        self.server.state.tools_mode = "stall_body"
        workload = small_workload(traffic_mix={"recall": 1.0}, phases=[{"name": "steady", "minutes": 0.5, "rate_multiplier": 1}])
        cap = 1.0
        start = time.monotonic()
        result = self.run_live(workload, max_elapsed_seconds=cap)
        wall = time.monotonic() - start
        self.assertLess(wall, cap + 1.0)
        self.assertLessEqual(result["elapsed_over_cap_s"], load_cli.DEADLINE_JITTER_S)
        classes = {r.get("error_class") for r in result["results"] if r["outcome"] == "network_error"}
        self.assertEqual(classes, {"ExchangeDeadlineExceeded"})
        self.assertNotEqual(result["status"], "PARTIAL")

    def test_an_endless_header_drip_is_cut_off_at_the_deadline(self):
        """Header drip: no chunk loop to check the clock in. Only the watchdog that
        shuts the socket down can end a read blocked inside getresponse()."""
        result, elapsed = self.exchange("/drip-headers", 0.3)
        self.assertIsNone(result)
        self.assertLess(elapsed, 0.3 + 0.5)
        self.assertGreaterEqual(elapsed, 0.29)

    def test_readiness_header_drip_is_bounded_and_reports_a_fixed_class(self):
        self.server.state.readyz_status = 200
        with mock.patch.object(load_cli, "READINESS_TIMEOUT_S", 0.3):
            start = time.monotonic()
            error = self._readiness_against("/readyz-drip-headers")
        self.assertLess(time.monotonic() - start, 0.3 + 0.5)
        self.assertEqual(error, "readiness probe failed: ExchangeDeadlineExceeded")

    def _readiness_against(self, path):
        run = make_run()
        timeout_s = run.reserve("readiness", 0, load_cli.READINESS_TIMEOUT_S)
        try:
            load_cli._http_exchange("GET", self.server.parsed_origin, path, {}, None, timeout_s)
        except (*load_cli.TRANSPORT_ERRORS, load_cli.McpProtocolError) as e:
            return f"readiness probe failed: {load_cli.error_class(e)}"
        return None

    def test_the_deadline_is_the_tighter_of_the_default_and_the_remaining_elapsed_budget(self):
        run = make_run(max_elapsed_seconds=0.4)
        timeout_s = run.reserve("readiness", 0, 5.0)
        self.assertLessEqual(timeout_s, 0.4)

    def test_a_drip_response_cannot_hold_the_run_past_its_elapsed_cap_and_shutdown_does_not_hang(self):
        self.server.state.tools_mode = "drip_body"
        workload = small_workload(traffic_mix={"recall": 1.0}, phases=[{"name": "steady", "minutes": 0.5, "rate_multiplier": 1}])
        cap = 1.0
        start = time.monotonic()
        result = self.run_live(workload, max_elapsed_seconds=cap)
        wall = time.monotonic() - start
        self.assertLess(wall, cap + 1.5)
        self.assertLessEqual(result["elapsed_over_cap_s"], load_cli.DEADLINE_JITTER_S)
        self.assertGreater(result["outcome_counts"]["network_error"], 0)
        classes = {r.get("error_class") for r in result["results"] if r["outcome"] == "network_error"}
        self.assertEqual(classes, {"ExchangeDeadlineExceeded"})
        self.assertNotEqual(result["status"], "PARTIAL")


@unittest.skipUnless(shutil.which("openssl"), "openssl is needed to mint a throwaway self-signed certificate")
class TlsExchangeTests(unittest.TestCase):
    """A completed TLS exchange against a real TLS listener, with a throwaway
    self-signed certificate minted into a temp dir for 127.0.0.1 only."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        tmp = Path(self.tmpdir.name)
        self.cert, key = tmp / "cert.pem", tmp / "key.pem"
        subprocess.run(
            ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", str(key), "-out", str(self.cert),
             "-days", "1", "-subj", "/CN=127.0.0.1", "-addext", "subjectAltName=IP:127.0.0.1"],
            check=True, capture_output=True,
        )
        server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        server_ctx.load_cert_chain(str(self.cert), str(key))
        self.server = FakeMcpServer(tls_context=server_ctx)
        self.server.start()
        self.addCleanup(self.server.stop)
        self.trusting_context = ssl.create_default_context(cafile=str(self.cert))

    def exchange(self, path, timeout_s):
        with mock.patch.object(load_cli.ssl, "create_default_context", lambda: self.trusting_context):
            start = time.monotonic()
            try:
                return load_cli._http_exchange("GET", self.server.parsed_origin, path, {}, None, timeout_s), time.monotonic() - start
            except load_cli.ExchangeDeadlineExceeded:
                return None, time.monotonic() - start

    def test_a_completed_tls_exchange_returns_the_response_and_closes_every_socket(self):
        opened: list[socket.socket] = []
        real_connect = load_cli._connect

        def recording_connect(scheme, host, port, deadline, hold):
            return real_connect(scheme, host, port, deadline, lambda sock: (opened.append(sock), hold(sock)))

        with mock.patch.object(load_cli, "_connect", recording_connect):
            result, _elapsed = self.exchange("/readyz", 5.0)
        self.assertEqual(result[0], 200)
        self.assertEqual(json.loads(result[2]), {"ready": True})
        self.assertGreaterEqual(len(opened), 2)  # the plain socket and its TLS wrapper
        self.assertEqual([s for s in opened if s.fileno() != -1], [])

    def test_certificate_verification_is_on_by_default(self):
        with self.assertRaises(ssl.SSLCertVerificationError):
            load_cli._http_exchange("GET", self.server.parsed_origin, "/readyz", {}, None, 5.0)

    def test_a_tls_response_that_stalls_after_its_first_byte_is_cut_off_at_the_deadline(self):
        result, elapsed = self.exchange("/stall-after-first-byte", 0.6)
        self.assertIsNone(result)
        self.assertGreaterEqual(elapsed, 0.59)
        self.assertLessEqual(elapsed, 0.6 + load_cli.DEADLINE_JITTER_S)

    def test_a_tls_header_drip_is_cut_off_at_the_deadline(self):
        result, elapsed = self.exchange("/drip-headers", 0.4)
        self.assertIsNone(result)
        self.assertLessEqual(elapsed, 0.4 + load_cli.DEADLINE_JITTER_S)


# ---------------------------------------------------------------------------
# Bounded name resolution. getaddrinfo() cannot be interrupted, so a hostname is
# resolved in a disposable child process that the parent kills at the deadline.
# Resolver behavior is scripted by swapping the child's command for a real
# Python process (a real stuck child, a real exit code), never a mocked result.
# ---------------------------------------------------------------------------

LOOPBACK_RESOLVER = "import json, sys\nprint(json.dumps([[2, ['127.0.0.1', int(sys.argv[2])]]]))\n"


def patch_resolver(testcase, script):
    """Run `script` in place of the real resolver child for the rest of the test."""
    def command(host, port, limit_s):
        return [sys.executable, "-I", "-S", "-c", script, host, str(port), str(limit_s)]
    patcher = mock.patch.object(load_cli, "_resolver_command", command)
    patcher.start()
    testcase.addCleanup(patcher.stop)


def stuck_resolver(pid_file, ignore_sigterm=False):
    """A child that records its pid and then never answers (optionally ignoring SIGTERM)."""
    return (
        "import os, signal, time\n"
        + ("signal.signal(signal.SIGTERM, signal.SIG_IGN)\n" if ignore_sigterm else "")
        + f"with open({str(pid_file)!r}, 'a') as f:\n    f.write(str(os.getpid()) + '\\n')\n"
        + "time.sleep(60)\n"
    )


def stalls_after_resolver(pid_file, answered):
    """A child that records its pid, answers loopback for the first `answered` runs, then never answers."""
    return (
        "import json, os, sys, time\n"
        + f"pid_file = {str(pid_file)!r}\n"
        + "with open(pid_file, 'a') as f:\n    f.write(str(os.getpid()) + '\\n')\n"
        + f"if sum(1 for _ in open(pid_file)) > {answered}:\n    time.sleep(60)\n"
        + "print(json.dumps([[2, ['127.0.0.1', int(sys.argv[2])]]]))\n"
    )


def recorded_pids(pid_file):
    return [int(p) for p in pid_file.read_text().split()] if pid_file.exists() else []


def process_is_gone(pid):
    """True only once the pid is fully reaped: a zombie still answers signal 0."""
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return True
    except PermissionError:
        return False
    return False


class UnkillableProcess:
    """Stands in for a child that survives SIGKILL (uninterruptible sleep)."""

    stdout = None

    def __init__(self):
        self.calls = []
        self.reaper_waiting = threading.Event()
        self.release = threading.Event()

    def poll(self):
        return None

    def terminate(self):
        self.calls.append("terminate")

    def kill(self):
        self.calls.append("kill")

    def wait(self, timeout=None):
        self.calls.append(("wait", timeout))
        if timeout is not None:
            raise subprocess.TimeoutExpired("resolver", timeout)
        self.reaper_waiting.set()
        self.release.wait(5)
        return -9


class BoundedResolverTests(FakeServerTestCase):
    def setUp(self):
        super().setUp()
        self.pids = self.tmp / "resolver.pids"
        self.port = self.server.httpd.server_address[1]

    def hostname_origin(self):
        return ("http", "localhost", self.port)

    def assert_children_reaped(self, expected_at_least=1):
        pids = recorded_pids(self.pids)
        self.assertGreaterEqual(len(pids), expected_at_least)
        for pid in pids:
            self.assertTrue(process_is_gone(pid), f"resolver child {pid} was not killed and reaped")

    def test_a_stuck_resolver_is_cut_off_at_the_deadline_and_its_child_is_reaped(self):
        patch_resolver(self, stuck_resolver(self.pids))
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._resolve("stuck.test", 80, time.monotonic() + 0.4)
        elapsed = time.monotonic() - start
        self.assertGreaterEqual(elapsed, 0.39)
        self.assertLessEqual(elapsed, 0.4 + load_cli.DEADLINE_JITTER_S)
        self.assert_children_reaped()
        self.assertEqual(len(recorded_pids(self.pids)), 1)

    def test_a_resolver_that_ignores_sigterm_is_killed_and_reaped_within_the_grace(self):
        patch_resolver(self, stuck_resolver(self.pids, ignore_sigterm=True))
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._resolve("stuck.test", 80, time.monotonic() + 0.4)
        elapsed = time.monotonic() - start
        self.assertGreaterEqual(elapsed, 0.4 + load_cli.RESOLVER_TERMINATE_GRACE_S - 0.05)  # SIGTERM was ignored, so SIGKILL ended it
        self.assertLessEqual(elapsed, 0.4 + load_cli.RESOLVER_TERMINATE_GRACE_S + load_cli.DEADLINE_JITTER_S)
        self.assert_children_reaped()

    def test_a_child_that_survives_sigkill_is_handed_to_a_reaper_and_never_holds_the_exchange(self):
        proc = UnkillableProcess()
        self.addCleanup(proc.release.set)
        with mock.patch.object(load_cli, "RESOLVER_TERMINATE_GRACE_S", 0.01), mock.patch.object(load_cli, "RESOLVER_REAP_TIMEOUT_S", 0.01):
            start = time.monotonic()
            load_cli._stop_process(proc)
            elapsed = time.monotonic() - start
        self.assertLess(elapsed, 0.5)
        self.assertEqual(proc.calls[:4], ["terminate", ("wait", 0.01), "kill", ("wait", 0.01)])
        self.assertTrue(proc.reaper_waiting.wait(2), "no daemon thread was left waiting to reap the child")

    def test_a_stuck_resolver_ends_a_hostname_exchange_at_its_deadline_before_any_socket(self):
        patch_resolver(self, stuck_resolver(self.pids))
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._http_exchange("GET", self.hostname_origin(), "/readyz", {}, None, 0.5)
        self.assertLessEqual(time.monotonic() - start, 0.5 + load_cli.DEADLINE_JITTER_S)
        self.assert_children_reaped()
        self.assertEqual(self.server.state.requests_seen, [])

    def test_a_stuck_resolver_cannot_hold_the_run_or_executor_shutdown_past_the_cap(self):
        """Readiness and both sessions' initialize/notification resolve (5 children);
        every later resolution, in the executor's worker threads, never answers."""
        patch_resolver(self, stalls_after_resolver(self.pids, answered=5))
        workload = small_workload(traffic_mix={"recall": 1.0}, phases=[{"name": "steady", "minutes": 0.5, "rate_multiplier": 1}])
        cap = 1.5
        start = time.monotonic()
        result = self.run_live(workload, origin=f"http://localhost:{self.port}", allowed_hosts=["localhost"], max_elapsed_seconds=cap)
        wall = time.monotonic() - start
        self.assertLess(wall, cap + 1.0)
        self.assertLessEqual(result["elapsed_over_cap_s"], load_cli.DEADLINE_JITTER_S)
        self.assertGreater(result["outcome_counts"]["network_error"], 0)  # the stuck workers, not a setup failure
        classes = {r.get("error_class") for r in result["results"] if r["outcome"] == "network_error"}
        self.assertEqual(classes, {"ExchangeDeadlineExceeded"})
        self.assertNotEqual(result["status"], "PARTIAL")
        self.assert_children_reaped(expected_at_least=7)

    def test_the_resolution_time_and_the_tls_handshake_share_one_deadline(self):
        """Resolution takes 0.3s, then the TLS handshake stalls: the exchange must end at
        the 0.6s budget, not at 0.3s + 0.6s."""
        silent = SilentListener()
        self.addCleanup(silent.close)
        patch_resolver(self, "import json, sys, time\ntime.sleep(0.3)\nprint(json.dumps([[2, ['127.0.0.1', int(sys.argv[2])]]]))\n")
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._http_exchange("GET", ("https", "slow.test", silent.port), "/x", {}, None, 0.6)
        elapsed = time.monotonic() - start
        self.assertGreaterEqual(elapsed, 0.59)
        self.assertLessEqual(elapsed, 0.6 + load_cli.DEADLINE_JITTER_S)

    def test_an_expired_deadline_never_starts_a_child(self):
        with mock.patch.object(load_cli, "_resolver_command", side_effect=AssertionError("resolver child started")):
            with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
                load_cli._resolve("late.test", 80, time.monotonic() - 1)

    def test_an_ip_literal_never_starts_a_resolver_child(self):
        with mock.patch.object(load_cli, "_resolver_command", side_effect=AssertionError("resolver child started")):
            deadline = time.monotonic() + 5
            self.assertEqual(load_cli._resolve("127.0.0.1", 80, deadline), [(socket.AF_INET, ("127.0.0.1", 80))])
            self.assertEqual(load_cli._resolve("::1", 80, deadline), [(socket.AF_INET6, ("::1", 80, 0, 0))])
            status, _headers, body = load_cli._http_exchange("GET", self.server.parsed_origin, "/readyz", {}, None, 5.0)  # a whole exchange
        self.assertEqual(status, 200)
        self.assertEqual(json.loads(body), {"ready": True})

    def test_localhost_resolves_through_a_child_to_loopback_addresses_only(self):
        addresses = load_cli._resolve("localhost", 4321, time.monotonic() + 10)
        self.assertGreaterEqual(len(addresses), 1)
        for family, sockaddr in addresses:
            self.assertIn(family, (socket.AF_INET, socket.AF_INET6))
            self.assertEqual(sockaddr[1], 4321)
            self.assertTrue(ipaddress.ip_address(sockaddr[0]).is_loopback)

    def test_a_localhost_hostname_exchange_succeeds_and_keeps_the_origin_host_header(self):
        status, _headers, body = load_cli._http_exchange("GET", self.hostname_origin(), "/readyz", {}, None, 10.0)
        self.assertEqual(status, 200)
        self.assertEqual(json.loads(body), {"ready": True})
        _method, _path, headers = self.server.state.requests_seen[-1]
        self.assertEqual(headers["Host"], f"localhost:{self.port}")  # the origin, not the resolved IP

    def test_a_full_run_against_a_localhost_hostname_origin_completes_and_states_the_scope(self):
        result = self.run_live(origin=f"http://localhost:{self.port}", allowed_hosts=["localhost"])
        self.assertEqual(result["status"], "PARTIAL")
        self.assertEqual(result["outcome_counts"]["ok"], result["offered_total"])
        self.assertTrue(result["exchange_deadline_scope"].startswith("bounded resolver"))
        self.assertFalse(result["qualified"])

    def test_addresses_are_tried_in_order_and_a_refused_one_falls_through(self):
        patch_resolver(self, f"import json\nprint(json.dumps([[30, ['::1', {self.port}, 0, 0]], [2, ['127.0.0.1', {self.port}]]]))\n")
        status, _headers, _body = load_cli._http_exchange("GET", ("http", "alias.test", self.port), "/readyz", {}, None, 5.0)
        self.assertEqual(status, 200)

    def test_plain_http_refuses_only_when_no_resolved_address_is_loopback_and_dials_no_socket(self):
        patch_resolver(self, "import json, sys\nprint(json.dumps([[2, ['192.0.2.1', int(sys.argv[2])]]]))\n")
        opened = []
        with self.assertRaises(load_cli.ResolutionError) as ctx:
            load_cli._connect("http", "localhost", self.port, time.monotonic() + 5, opened.append)
        self.assertEqual(str(ctx.exception), "plain http resolved to no loopback address")
        self.assertEqual(opened, [])

    def test_plain_http_filters_a_mixed_list_to_its_loopback_addresses_and_never_dials_the_rest(self):
        """The non-loopback answer comes first: it is dropped, not tried and not a reason to refuse."""
        patch_resolver(self, f"import json\nprint(json.dumps([[2, ['192.0.2.1', {self.port}]], [2, ['127.0.0.1', {self.port}]]]))\n")
        opened = []
        sock = load_cli._connect("http", "localhost", self.port, time.monotonic() + 5, opened.append)
        self.addCleanup(sock.close)
        self.assertEqual(len(opened), 1)  # exactly one socket was ever created: no attempt at 192.0.2.1
        self.assertEqual(sock.getpeername(), ("127.0.0.1", self.port))

    def test_resolution_failure_from_the_real_resolver_is_a_fixed_class(self):
        with self.assertRaises(load_cli.ResolutionError) as ctx:
            load_cli._resolve("nonexistent.invalid", 80, time.monotonic() + 10)  # reserved: never resolves
        self.assertEqual(load_cli.error_class(ctx.exception), "ResolutionError")
        self.assertEqual(str(ctx.exception), "name resolution failed")

    def test_resolution_failure_blocks_the_run_at_readiness_with_a_fixed_class(self):
        patch_resolver(self, "import sys\nsys.exit(3)\n")
        result = self.run_live(origin=f"http://localhost:{self.port}", allowed_hosts=["localhost"])
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("readiness probe failed: ResolutionError", result["reason"])
        self.assertEqual(self.server.state.requests_seen, [])

    def test_hostile_or_malformed_resolver_output_is_rejected_with_a_fixed_message(self):
        many = "[" + ",".join(["[2, ['127.0.0.1', PORT]]"] * (load_cli.RESOLVER_MAX_ADDRESSES + 1)) + "]"
        cases = {
            "not json": ("print('SECRET-resolver-text')", "resolver output is malformed"),
            "empty list": ("print('[]')", "resolver output is malformed"),
            "wrong port": ("print('[[2, [\"127.0.0.1\", 1]]]')", "resolver output is malformed"),
            "family and address disagree": ("import sys\nprint('[[2, [\"::1\", %s]]]' % sys.argv[2])", "resolver output is malformed"),
            "not an address": ("import sys\nprint('[[2, [\"SECRET-name\", %s]]]' % sys.argv[2])", "resolver output is malformed"),
            "unknown family": ("import sys\nprint('[[99, [\"127.0.0.1\", %s]]]' % sys.argv[2])", "resolver output is malformed"),
            "too many addresses": ("import sys\nprint(%r.replace('PORT', sys.argv[2]))" % many, "resolver output is malformed"),
            "oversize output": (f"import sys\nsys.stdout.write('x' * {load_cli.RESOLVER_MAX_OUTPUT_BYTES * 3})", "resolver output exceeds the size bound"),
            "non-zero exit": ("import sys\nprint('SECRET-stderr-text')\nsys.exit(5)", "name resolution failed"),
        }
        for name, (script, message) in cases.items():
            with self.subTest(name):
                patch_resolver(self, script + "\n")
                with self.assertRaises(load_cli.ResolutionError) as ctx:
                    load_cli._resolve("hostile.test", 4321, time.monotonic() + 10)
                self.assertEqual(str(ctx.exception), message)
                self.assertNotIn("SECRET", str(ctx.exception))

    def test_the_resolver_child_gets_only_the_hostname_port_and_limit_and_never_a_credential(self):
        recorded = []
        real_popen = subprocess.Popen

        def recording_popen(args, **kwargs):
            recorded.append((list(args), kwargs))
            return real_popen(args, **kwargs)

        with mock.patch.dict(os.environ, {"T2360_PARENT_SECRET": LONG_CRED}), mock.patch.object(load_cli.subprocess, "Popen", recording_popen):
            session = load_cli.McpSession(f"http://localhost:{self.port}", LONG_CRED, make_run())
            session.initialize()  # initialize and notification: two exchanges, two children
        self.assertTrue(session.has_server_session)
        self.assertEqual(len(recorded), 2)
        for args, kwargs in recorded:
            self.assertEqual(args[:4], [sys.executable, "-I", "-S", "-c"])
            self.assertEqual(len(args), 8)  # interpreter flags, fixed source, then exactly hostname, port, limit
            self.assertEqual((args[5], args[6]), ("localhost", str(self.port)))
            self.assertGreaterEqual(int(args[7]), 1)
            self.assertNotIn(LONG_CRED, " ".join(args))
            self.assertEqual(kwargs["env"], {})
            self.assertEqual(kwargs["stdin"], subprocess.DEVNULL)
            self.assertEqual(kwargs["stderr"], subprocess.DEVNULL)
            self.assertNotIn(LONG_CRED, repr(kwargs))
        _method, _path, headers = self.server.state.requests_seen[-1]
        self.assertEqual(headers["Authorization"], f"Bearer {LONG_CRED}")  # the credential reached the server, not the child

    def test_the_resolver_child_sees_an_empty_environment(self):
        env_file = self.tmp / "child.env"
        patch_resolver(self, f"import json, os, sys\nopen({str(env_file)!r}, 'w').write(json.dumps(sorted(os.environ)))\n" + LOOPBACK_RESOLVER)
        with mock.patch.dict(os.environ, {"T2360_PARENT_SECRET": LONG_CRED}):
            load_cli._resolve("env.test", 4321, time.monotonic() + 10)
        names = json.loads(env_file.read_text())
        for name in ("T2360_PARENT_SECRET", "PATH", "HOME"):
            self.assertNotIn(name, names)

    def test_the_real_resolver_child_prints_only_the_address_list(self):
        real = subprocess.run(load_cli._resolver_command("localhost", 4321, 5), stdin=subprocess.DEVNULL, capture_output=True, env={}, timeout=10)
        self.assertEqual(real.returncode, 0)
        self.assertEqual(real.stderr, b"")
        self.assertEqual(json.dumps(json.loads(real.stdout)), real.stdout.decode())  # one JSON list, no trailing text
        self.assertEqual(len(load_cli._parse_resolver_output(real.stdout, 4321)), len(json.loads(real.stdout)))


def mint_certificate(tmp: Path, common_name: str, san: str):
    cert, key = tmp / "cert.pem", tmp / "key.pem"
    subprocess.run(
        ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", str(key), "-out", str(cert),
         "-days", "1", "-subj", f"/CN={common_name}", "-addext", f"subjectAltName={san}"],
        check=True, capture_output=True,
    )
    return cert, key


@unittest.skipUnless(shutil.which("openssl"), "openssl is needed to mint a throwaway certificate")
class TlsHostnameTests(unittest.TestCase):
    """Real TLS through the bounded resolver. The connection goes to a resolved IP
    while the certificate is checked, and the SNI sent, for the origin's hostname.
    The throwaway certificate is valid for the DNS name localhost only."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        tmp = Path(self.tmpdir.name)
        self.cert, key = mint_certificate(tmp, "localhost", "DNS:localhost")
        server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        server_ctx.load_cert_chain(str(self.cert), str(key))
        self.sni_seen = []
        server_ctx.sni_callback = lambda _sock, name, _ctx: self.sni_seen.append(name)  # returns None: carry on
        self.server = FakeMcpServer(tls_context=server_ctx)
        self.server.start()
        self.addCleanup(self.server.stop)
        self.port = self.server.httpd.server_address[1]
        self.trusting_context = ssl.create_default_context(cafile=str(self.cert))

    def exchange(self, host, trusted=True, timeout_s=10.0):
        origin = ("https", host, self.port)
        if not trusted:
            return load_cli._http_exchange("GET", origin, "/readyz", {}, None, timeout_s)
        with mock.patch.object(load_cli.ssl, "create_default_context", lambda: self.trusting_context):
            return load_cli._http_exchange("GET", origin, "/readyz", {}, None, timeout_s)

    def test_a_hostname_is_verified_against_its_certificate_and_sent_as_sni(self):
        status, _headers, body = self.exchange("localhost")  # real resolver child; connects to a resolved loopback IP
        self.assertEqual(status, 200)
        self.assertEqual(json.loads(body), {"ready": True})
        self.assertEqual(self.sni_seen, ["localhost"])  # the hostname, never the resolved IP

    def test_the_hostname_not_the_resolved_ip_decides_certificate_acceptance(self):
        """The same server, the same trusted certificate and the same resolved IP as
        the success case: only the origin's hostname differs, and it is rejected."""
        patch_resolver(self, LOOPBACK_RESOLVER)
        with self.assertRaises(ssl.SSLCertVerificationError) as ctx:
            self.exchange("other.test")
        self.assertIn("mismatch", ctx.exception.verify_message.lower())
        self.assertEqual(self.sni_seen, ["other.test"])

    def test_an_ip_literal_origin_is_still_checked_and_a_hostname_only_certificate_fails(self):
        with mock.patch.object(load_cli, "_resolver_command", side_effect=AssertionError("resolver child started")):
            with self.assertRaises(ssl.SSLCertVerificationError):
                self.exchange("127.0.0.1")

    def test_default_verification_rejects_an_untrusted_certificate_for_a_hostname_origin(self):
        with self.assertRaises(ssl.SSLCertVerificationError):
            self.exchange("localhost", trusted=False)

    def test_a_resolved_hostname_whose_peer_never_answers_tls_ends_at_the_deadline(self):
        silent = SilentListener()
        self.addCleanup(silent.close)
        start = time.monotonic()
        with self.assertRaises(load_cli.ExchangeDeadlineExceeded):
            load_cli._http_exchange("GET", ("https", "localhost", silent.port), "/x", {}, None, 0.5)
        self.assertLessEqual(time.monotonic() - start, 0.5 + load_cli.DEADLINE_JITTER_S)


# ---------------------------------------------------------------------------
# Every socket operation is guarded immediately before it opens.
# ---------------------------------------------------------------------------

class PreSocketGuardTests(FakeServerTestCase):
    def test_notification_is_guarded_before_its_socket(self):
        session = self.session(max_calls=1)  # initialize spends the only call
        with self.assertRaises(load_cli.BudgetExhausted):
            session.initialize()
        methods = [m for m, _t in self.server.state.rpc_seen]
        self.assertEqual(methods, ["initialize"])  # the notification never reached the server
        self.assertTrue(session.has_server_session)
        self.assertFalse(session.initialized)

    def test_initialize_is_guarded_before_its_socket(self):
        session = self.session(approved_max_usd=0)
        with self.assertRaises(load_cli.BudgetExhausted):
            session.initialize()
        self.assertEqual(self.server.state.requests_seen, [])

    def test_readiness_is_guarded_before_its_socket(self):
        run = make_run(max_elapsed_seconds=0.001)
        time.sleep(0.01)
        error = load_cli.check_readiness(self.server.origin, run)
        self.assertIn("not sent", error)
        self.assertEqual(self.server.state.requests_seen, [])

    def test_run_live_with_no_dollar_budget_sends_nothing_at_all(self):
        result = self.run_live(approved_max_usd=0)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("approved_max_usd", result["reason"])
        self.assertEqual(self.server.state.requests_seen, [])
        self.assertEqual(result["calls_attempted"], 0)
        self.assertEqual(result["outcome_counts"]["not_dispatched"], result["offered_total"])

    def test_run_live_revalidates_a_bad_budget_and_sends_nothing(self):
        for overrides in ({"max_elapsed_seconds": float("nan")}, {"max_calls": True}, {"approved_max_usd": -1}, {"worst_case_usd_per_call": 0}, {"provider_work_bound": None}):
            with self.subTest(overrides=overrides):
                result = self.run_live(**overrides)
                self.assertEqual(result["status"], "BLOCKED")
                self.assertEqual(result["calls_attempted"], 0)
        self.assertEqual(self.server.state.requests_seen, [])

    def test_run_live_refuses_a_noncanonical_or_production_origin_and_sends_nothing(self):
        for origin in (self.server.origin + "/", self.server.origin.upper(), "https://app.serenity.sire.run", "https://APP.serenity.sire.run."):
            with self.subTest(origin=origin):
                result = self.run_live(origin=origin)
                self.assertEqual(result["status"], "BLOCKED")
                self.assertEqual(result["calls_attempted"], 0)
        self.assertEqual(self.server.state.requests_seen, [])

    def test_a_dead_origin_counts_an_attempt_but_not_a_sent_or_completed_exchange(self):
        result = self.run_live(origin=f"http://127.0.0.1:{unused_port()}")
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("readiness probe failed", result["reason"])
        self.assertEqual(result["calls_attempted_by_operation"], {"readiness": 1})
        self.assertEqual(result["calls_sent"], 0)
        self.assertEqual(result["calls_completed"], 0)


class QueuedCancellationTests(FakeServerTestCase):
    def test_queued_requests_are_cancelled_not_launched_after_the_elapsed_deadline(self):
        self.server.state.tools_delay_s = 0.3
        workload = small_workload(
            traffic_mix={"recall": 1.0}, concurrency={"clients": 1, "baseline_request_rate_per_s": 50},
            phases=[{"name": "steady", "minutes": 0.1, "rate_multiplier": 1}],  # ~300 arrivals over 6s
        )
        cap = 1.5
        t0 = time.monotonic()
        result = self.run_live(workload, max_elapsed_seconds=cap)
        wall = time.monotonic() - t0
        arrivals_at_server = self.server.state.tools_calls_seen()
        attempted = result["calls_attempted_by_operation"]["tools_call"]
        self.assertGreater(len(arrivals_at_server), 0)
        self.assertLessEqual(max(arrivals_at_server), t0 + cap + load_cli.DEADLINE_JITTER_S)  # nothing launched after the deadline
        self.assertLessEqual(attempted - 1, len(arrivals_at_server))
        self.assertLessEqual(len(arrivals_at_server), attempted)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertGreater(result["outcome_counts"]["not_dispatched"], 100)
        self.assertEqual(sum(result["outcome_counts"].values()), result["offered_total"])
        self.assertLess(wall, cap + 1.5)


# ---------------------------------------------------------------------------
# Conservative input-token precharge.
# ---------------------------------------------------------------------------

class TokenPrechargeTests(FakeServerTestCase):
    def test_precharge_is_the_encoded_argument_bytes_actually_sent(self):
        result = self.run_live(small_workload(traffic_mix={"recall": 0.5, "remember": 0.5}))
        sent = self.server.state.tool_arg_bytes
        synthetic_words = sum(r["tokens"] for r in result["results"])
        self.assertGreater(sent, synthetic_words)  # the synthetic word count understates the input
        self.assertEqual(result["input_tokens_precharged_upper_bound"], sent)

    def test_the_byte_based_cap_binds_where_the_synthetic_word_count_would_not(self):
        workload = small_workload(phases=[{"name": "steady", "minutes": 0.02, "rate_multiplier": 1}])
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(load_cli.BASE_SEED))
        cap = 600
        self.assertLess(sum(a["tokens"] for a in arrivals), cap)  # word counts alone would admit every request
        result = self.run_live(workload, max_input_tokens=cap)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_input_tokens", result["reason"])
        self.assertLessEqual(result["input_tokens_precharged_upper_bound"], cap)
        self.assertLessEqual(self.server.state.tool_arg_bytes, cap)
        self.assertGreater(result["outcome_counts"]["not_dispatched"], 0)

    def test_declared_provider_work_bound_is_charged_on_readiness_and_every_authenticated_operation(self):
        bound = {"readiness_tokens": 7, "cold_open_tokens": 5, "basis": "test-only loopback fixture"}
        result = self.run_live(small_workload(traffic_mix={"recall": 1.0}), provider_work_bound=bound)
        authenticated = sum(result["calls_attempted_by_operation"].get(op, 0) for op in ("initialize", "notification", "tools_call", "close"))
        self.assertGreater(result["calls_attempted_by_operation"]["tools_call"], 0)
        self.assertEqual(result["input_tokens_precharged_upper_bound"], 7 + 5 * authenticated + self.server.state.tool_arg_bytes)

    def test_a_declared_cold_open_bound_can_exhaust_the_token_cap(self):
        bound = {"readiness_tokens": 0, "cold_open_tokens": 1000, "basis": "test-only loopback fixture"}
        result = self.run_live(max_input_tokens=1500, provider_work_bound=bound)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_input_tokens", result["reason"])
        self.assertLessEqual(result["input_tokens_precharged_upper_bound"], 1500)

    def test_preflight_upper_bound_is_never_below_the_actual_argument_bytes(self):
        workload = json.loads(WORKLOAD_PATH.read_text())
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(load_cli.BASE_SEED))
        for seq, a in list(enumerate(arrivals))[:400]:
            actual = load_cli.argument_bytes(load_cli.tool_arguments(a["verb"], a["account"], 0, seq, a["tokens"], fact_id="f" * 40))
            self.assertLessEqual(actual, load_cli.argument_bytes_upper_bound(a["verb"], a["tokens"]))


# ---------------------------------------------------------------------------
# Upstream text never reaches the output: fixed classes only.
# ---------------------------------------------------------------------------

class ErrorSanitizationTests(FakeServerTestCase):
    def assert_no_credential(self, result):
        text = json.dumps(result)
        for fragment in (LONG_CRED, LONG_CRED[:8], LONG_CRED[:14], LONG_CRED[-8:], LONG_CRED[40:52]):
            self.assertNotIn(fragment, text)

    def test_initialize_error_echoing_the_credential_across_a_truncation_boundary_leaks_no_prefix(self):
        self.server.state.init_mode = "echo_jsonrpc_error"
        result = self.run_live(credential=LONG_CRED)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("McpProtocolError", result["reason"])
        self.assert_no_credential(result)

    def test_error_bodies_and_messages_echoing_the_credential_leak_no_prefix(self):
        for mode, expected in (("echo_http500", "unexpected_5xx"), ("echo_jsonrpc", "protocol_error"), ("echo_tool", "tool_error")):
            with self.subTest(mode=mode):
                self.server.state.tools_mode = mode
                result = self.run_live(small_workload(traffic_mix={"recall": 1.0}), credential=LONG_CRED)
                self.assertGreater(result["outcome_counts"][expected], 0)
                self.assert_no_credential(result)

    def test_readiness_failure_text_is_a_fixed_string_not_the_response_body(self):
        self.server.state.readyz_status = 500
        self.server.state.readyz_body = b"BODYSENTINEL " + LONG_CRED.encode()
        result = self.run_live(credential=LONG_CRED)
        self.assertEqual(result["reason"], "readiness probe returned HTTP 500")
        self.assertNotIn("BODYSENTINEL", json.dumps(result))
        self.assert_no_credential(result)

    def test_failed_records_carry_only_a_status_or_a_fixed_class(self):
        self.server.state.tools_mode = "http500"
        result = self.run_live(small_workload(traffic_mix={"recall": 1.0}))
        failed = [r for r in result["results"] if r["outcome"] == "unexpected_5xx"]
        self.assertTrue(failed)
        for r in failed:
            self.assertEqual(r["http_status"], 500)
            self.assertNotIn("exploded", json.dumps(r))
            self.assertNotIn("text", r)

    def test_error_class_is_a_class_name_and_errno_only(self):
        self.assertEqual(load_cli.error_class(ConnectionRefusedError(61, "Connection refused: SECRET")), "ConnectionRefusedError:ECONNREFUSED")
        self.assertEqual(load_cli.error_class(RuntimeError("SECRET")), "RuntimeError")


# ---------------------------------------------------------------------------
# Truthful cleanup and attempted/sent/completed accounting.
# ---------------------------------------------------------------------------

class CleanupStatusTests(FakeServerTestCase):
    def cleanup_of(self, **kwargs):
        return self.run_live(small_workload(traffic_mix={"recall": 1.0}), **kwargs)

    def test_confirmed_cleanup_is_204_and_leaves_no_session(self):
        result = self.cleanup_of()
        c = result["cleanup"]
        self.assertEqual((c["server_sessions_created"], c["closed_confirmed"], c["sessions_left_open"]), (2, 2, 0))
        self.assertEqual(self.server.state.sessions, {})
        self.assertEqual(self.server.state.delete_seen, 2)

    def test_an_unexpected_delete_status_is_a_cleanup_failure_not_a_close(self):
        for mode, detail in (("status:500", "http_500"), ("status:404", "http_404"), ("status:401", "http_401"), ("status:200", None)):
            with self.subTest(mode=mode):
                self.server.state.delete_mode = mode
                self.server.state.sessions.clear()
                result = self.cleanup_of()
                c = result["cleanup"]
                if detail is None:  # 200 is an acceptable confirmation
                    self.assertEqual(c["closed_confirmed"], 2)
                    continue
                self.assertEqual(c["closed_confirmed"], 0)
                self.assertEqual(c["close_failed"], {detail: 2})
                self.assertEqual(c["sessions_left_open"], 2)
                self.assertTrue(any("not confirmed closed" in g for g in result["qualification_gaps"]))

    def test_a_redirected_delete_is_a_failure_and_the_target_is_never_dialed(self):
        self.server.state.delete_mode = "redirect"
        c = self.cleanup_of()["cleanup"]
        self.assertEqual(c["close_failed"], {"http_302": 2})
        self.assertFalse(self.server.state.redirect_hit)

    def test_a_dropped_delete_connection_is_recorded_as_a_failure_class(self):
        self.server.state.delete_mode = "drop"
        c = self.cleanup_of()["cleanup"]
        self.assertEqual(c["closed_confirmed"], 0)
        self.assertEqual(sum(c["close_failed"].values()), 2)
        self.assertEqual(c["sessions_left_open"], 2)
        for detail in c["close_failed"]:
            self.assertNotIn(" ", detail)  # a class name, never message text

    def test_cleanup_the_budget_forbids_is_reported_not_attempted_and_not_closed(self):
        # readiness + (initialize, notification) for two accounts = 5 calls; close gets none.
        result = self.cleanup_of(max_calls=5)
        c = result["cleanup"]
        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual((c["closed_confirmed"], c["close_not_attempted"], c["sessions_left_open"]), (0, 2, 2))
        self.assertEqual(self.server.state.delete_seen, 0)
        self.assertEqual(len(self.server.state.sessions), 2)

    def test_attempted_sent_and_completed_are_counted_separately(self):
        self.server.state.tools_mode = "drop"
        result = self.cleanup_of()
        attempted = result["calls_attempted_by_operation"]["tools_call"]
        self.assertGreater(attempted, 0)
        self.assertEqual(result["calls_sent_by_operation"]["tools_call"], attempted)  # fully written
        self.assertEqual(result["calls_completed_by_operation"].get("tools_call", 0), 0)  # never answered
        self.assertGreater(result["calls_attempted"], result["calls_completed"])
        self.assertEqual(result["outcome_counts"]["network_error"], attempted)

    def test_a_healthy_run_attempts_sends_and_completes_the_same_calls(self):
        result = self.cleanup_of()
        self.assertEqual(result["calls_attempted"], result["calls_sent"])
        self.assertEqual(result["calls_sent"], result["calls_completed"])


# ---------------------------------------------------------------------------
# All-offered outcome semantics.
# ---------------------------------------------------------------------------

class OfferedOutcomeTests(FakeServerTestCase):
    def assert_accounted(self, result):
        self.assertEqual(sum(result["outcome_counts"].values()), result["offered_total"])
        self.assertEqual(len(result["results"]), result["offered_total"])
        self.assertTrue(all(r["outcome"] in load_cli.ALL_OUTCOMES for r in result["results"]))

    def test_a_healthy_run_is_never_better_than_partial_and_lists_its_gaps(self):
        result = self.run_live()
        self.assert_accounted(result)
        self.assertEqual(result["status"], "PARTIAL")
        self.assertFalse(result["qualified"])
        self.assertEqual(result["outcome_counts"]["ok"], result["offered_total"])
        self.assertFalse(result["seeded_state_verified"])
        self.assertFalse(result["cold_workload_verified"])
        joined = " ".join(result["qualification_gaps"])
        for needle in ("seeded", "cold flag", "provider_work_bound", "unmeasured is not passed", "not reviewer-frozen"):
            self.assertIn(needle, joined)

    def test_the_deadline_scope_is_stated_with_its_limits_and_no_live_claim(self):
        result = self.run_live()
        self.assertTrue(result["exchange_deadline_scope"].startswith("end to end"))  # loopback IP literal
        gaps = result["qualification_gaps"]
        self.assertTrue(any("per-exchange deadline" in g and "DEADLINE_JITTER_S" in g and "not observed behavior of a live target" in g for g in gaps))
        self.assertTrue(any("resolver child" in g and "not comparable" in g and "operating-system resolver" in g for g in gaps))
        self.assertFalse(any("getaddrinfo" in g for g in gaps))  # the old blanket "unqualified" gap is gone: the resolver is bounded
        self.assertFalse(result["qualified"])
        self.assertNotEqual(result["status"], "COMPLETE")
        for origin, expected in (("https://staging.example.com", "bounded resolver"), ("http://localhost:9443", "bounded resolver"), ("http://[::1]:9443", "end to end"), (None, "not applicable")):
            with self.subTest(origin=origin):
                self.assertTrue(load_cli._deadline_scope(origin).startswith(expected))

    def test_unmeasured_thresholds_are_none_never_true(self):
        checks = self.run_live()["by_repetition"][0]["threshold_evaluation"]
        for name in ("cpu_max_pct", "rss_max_pct_of_ram", "disk_max_pct", "cold_ready_max_s", "isolation_durability_errors_max"):
            self.assertIsNone(checks[name]["pass"], name)
        for name in ("min_offered_completion_pct", "unexpected_5xx_max_pct", "unexpected_admission_rejection_max_pct", "recall_p95_max_s"):
            self.assertIs(checks[name]["pass"], True, name)

    def test_each_failure_class_is_counted_and_fails_the_run(self):
        cases = [
            ("tool_error", "tool_error"), ("http500", "unexpected_5xx"), ("http429", "rejected_admission"),
            ("jsonrpc_error", "protocol_error"), ("drop", "network_error"),
        ]
        for mode, outcome in cases:
            with self.subTest(mode=mode):
                self.server.state.tools_mode = mode
                result = self.run_live(small_workload(traffic_mix={"recall": 1.0}))
                self.assert_accounted(result)
                self.assertEqual(result["status"], "FAIL")
                self.assertEqual(result["outcome_counts"][outcome], result["offered_total"])
                self.assertIn("min_offered_completion_pct", result["reason"])
                self.assertEqual(result["by_repetition"][0]["threshold_evaluation"]["min_offered_completion_pct"]["observed"], 0.0)

    def test_5xx_and_admission_rejections_fail_their_own_thresholds(self):
        self.server.state.tools_mode = "http500"
        checks = self.run_live(small_workload(traffic_mix={"recall": 1.0}))["by_repetition"][0]["threshold_evaluation"]
        self.assertIs(checks["unexpected_5xx_max_pct"]["pass"], False)
        self.server.state.tools_mode = "http429"
        checks = self.run_live(small_workload(traffic_mix={"recall": 1.0}))["by_repetition"][0]["threshold_evaluation"]
        self.assertIs(checks["unexpected_admission_rejection_max_pct"]["pass"], False)

    def test_offered_forgets_with_no_fact_id_stay_in_the_denominator(self):
        result = self.run_live(small_workload(traffic_mix={"forget": 1.0}))
        self.assert_accounted(result)
        self.assertEqual(result["outcome_counts"]["skipped_forget_no_fact"], result["offered_total"])
        self.assertEqual(result["outcome_counts"]["tool_error"], 0)
        self.assertEqual(result["calls_attempted_by_operation"].get("tools_call", 0), 0)  # nothing was sent for them
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any("seeding is not implemented" in g for g in result["qualification_gaps"]))

    def test_a_forget_failure_on_an_id_this_run_remembered_is_a_durability_error(self):
        self.server.state.forget_loses = True
        workload = small_workload(traffic_mix={"remember": 0.5, "forget": 0.5}, concurrency={"clients": 2, "baseline_request_rate_per_s": 60})
        result = self.run_live(workload)
        check = result["by_repetition"][0]["threshold_evaluation"]["isolation_durability_errors_max"]
        self.assertGreaterEqual(check["observed"], 1)
        self.assertIs(check["pass"], False)
        self.assertEqual(result["status"], "FAIL")

    def test_all_frozen_repetitions_replay_the_same_arrivals(self):
        result = self.run_live(small_workload(repetitions=3))
        self.assert_accounted(result)
        self.assertEqual(result["repetitions_planned"], 3)
        self.assertEqual(result["repetitions_fully_dispatched"], 3)
        self.assertEqual(len(result["by_repetition"]), 3)
        per_rep = [[(r["phase"], r["account"], r["verb"], r["tokens"]) for r in result["results"] if r["rep"] == rep] for rep in range(3)]
        self.assertEqual(per_rep[0], per_rep[1])
        self.assertEqual(per_rep[1], per_rep[2])
        self.assertEqual(result["offered_total"], 3 * len(per_rep[0]))

    def test_a_run_cut_short_reports_the_repetitions_it_never_dispatched(self):
        result = self.run_live(small_workload(repetitions=3), max_calls=12)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertLess(result["repetitions_fully_dispatched"], 3)
        self.assert_accounted(result)

    def test_the_cold_flag_is_reported_as_a_label_not_a_cold_workload(self):
        workload = small_workload(cold_brain_fraction=1.0)
        result = self.run_live(workload)
        self.assertEqual(result["cold_requests_flagged"], result["offered_total"])
        self.assertFalse(result["cold_workload_verified"])
        self.assertIsNone(result["by_repetition"][0]["threshold_evaluation"]["cold_ready_max_s"]["pass"])

    def test_without_a_steady_phase_nothing_is_evaluated_and_the_gap_says_so(self):
        workload = small_workload(phases=[{"name": "main", "minutes": 0.01, "rate_multiplier": 1}])
        result = self.run_live(workload)
        checks = result["by_repetition"][0]["threshold_evaluation"]
        self.assertIsNone(checks["min_offered_completion_pct"]["pass"])
        self.assertTrue(any("min_offered_completion_pct" in g for g in result["qualification_gaps"]))

    def test_a_slow_and_a_failed_request_never_yield_a_zero_exit_shaped_result(self):
        for mode in ("tool_error", "drop", "http500"):
            self.server.state.tools_mode = mode
            self.assertNotIn(self.run_live()["status"], ("PARTIAL", "COMPLETE", "PASS"), mode)

    def test_output_never_contains_the_raw_credential_value(self):
        result = self.run_live(credential="a-distinct-credential-value")
        self.assertNotIn("a-distinct-credential-value", json.dumps(result))


# ---------------------------------------------------------------------------
# prepare_live: authority inputs, guards and full-workload budget coverage.
# ---------------------------------------------------------------------------

def authorized_workload(**overrides):
    # TEST-ONLY reviewer values: no authority is implied by these.
    fields = dict(repetitions=3, frozen=True, reviewer="test-only reviewer", review_date="test-only date")
    fields.update(overrides)
    return small_workload(**fields)


def authorized_manifest(cred_dir="/nonexistent-test-creds", **budget_overrides):
    # TEST-ONLY provider/source values: no authority is implied by these.
    return base_manifest(
        environment={"kind": "disposable", "origin": "http://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
        budget=budget_dict(**{"max_elapsed_seconds": 120, **budget_overrides}),  # the elapsed requirement includes setup, drain and cleanup
        provider={"version_pin": "test-only", "serving_provider": "test-only", "privacy_review_ref": "test-only", "dimensions": 8},
        source_sha="a" * 40,
        binary_sha256="b" * 64,
        live={"credential_dir": str(cred_dir)},
    )


class PrepareLiveTests(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.object(load_cli, "_http_exchange", side_effect=AssertionError("prepare_live opened a socket"))
        patcher.start()
        self.addCleanup(patcher.stop)
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.tmp = Path(self.tmpdir.name)

    def test_committed_manifest_and_workload_are_not_liveable(self):
        workload = load_cli.harness.load_workload(WORKLOAD_PATH)
        plan, error = load_cli.prepare_live(json.loads(MANIFEST_PATH.read_text()), workload)
        self.assertIsNone(plan)
        self.assertIn("reviewer-frozen", error)

    def test_a_fully_specified_input_passes_every_guard_with_zero_sockets(self):
        workload = authorized_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts)
        plan, error = load_cli.prepare_live(authorized_manifest(cred_dir), workload)
        self.assertIsNone(error)
        self.assertEqual(len(plan.accounts), 2)
        self.assertEqual(set(plan.credentials), {a["id"] for a in accounts})

    def test_each_missing_authority_input_blocks(self):
        workload = authorized_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts)

        def blocked(manifest=None, workload=workload):
            plan, error = load_cli.prepare_live(manifest or authorized_manifest(cred_dir), workload)
            self.assertIsNone(plan)
            self.assertTrue(error)
            return error

        for key in ("frozen", "reviewer", "review_date"):
            with self.subTest(workload_key=key):
                w = dict(workload)
                w[key] = None
                blocked(workload=w)
        blocked(workload=authorized_workload(repetitions=1))
        for path in (("provider", "version_pin"), ("provider", "serving_provider"), ("provider", "privacy_review_ref"), ("provider", "dimensions"), ("source_sha",), ("binary_sha256",)):
            with self.subTest(path=path):
                m = authorized_manifest(cred_dir)
                target = m
                for part in path[:-1]:
                    target = target[part]
                target[path[-1]] = None
                blocked(m)
        blocked(authorized_manifest(cred_dir, provider_work_bound=None))

    def test_a_budget_that_cannot_cover_the_frozen_workload_blocks_before_any_spend(self):
        workload = authorized_workload()
        for field, value in (("max_calls", 5), ("max_input_tokens", 100), ("approved_max_usd", 0.001), ("max_elapsed_seconds", 1)):
            with self.subTest(field=field):
                plan, error = load_cli.prepare_live(authorized_manifest(**{field: value}), workload)
                self.assertIsNone(plan)
                self.assertIn(field, error)
                self.assertIn("cannot cover", error)

    def test_the_committed_budget_cannot_cover_the_real_frozen_workload_even_with_authority_filled(self):
        workload = json.loads(WORKLOAD_PATH.read_text())
        workload.update(frozen=True, reviewer="test-only reviewer", review_date="test-only date")
        committed = json.loads(MANIFEST_PATH.read_text())["budget"]
        manifest = authorized_manifest(**{**committed, "provider_work_bound": budget_dict()["provider_work_bound"]})
        plan, error = load_cli.prepare_live(manifest, workload)
        self.assertIsNone(plan)
        for field in ("max_calls", "max_input_tokens", "max_elapsed_seconds"):
            self.assertIn(field, error)

    def test_preflight_counts_bytes_and_the_declared_bound_per_authenticated_operation(self):
        workload = authorized_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(load_cli.BASE_SEED))
        bound = {"readiness_tokens": 11, "cold_open_tokens": 3, "basis": "test-only"}
        floor = 11 + 3 * (3 * len(accounts) + 3 * len(arrivals)) + 3 * sum(load_cli.argument_bytes_upper_bound(a["verb"], a["tokens"]) for a in arrivals)
        budget = load_cli.parse_budget(authorized_manifest(provider_work_bound=bound, max_input_tokens=floor))[0]
        self.assertIsNone(load_cli.preflight_budget(budget, workload, accounts, arrivals))
        tight = budget._replace(max_input_tokens=floor - 1)
        self.assertIn("max_input_tokens", load_cli.preflight_budget(tight, workload, accounts, arrivals))


# ---------------------------------------------------------------------------
# Repairs from the independent load review (T23.41-independent-load-review.md):
# C1 gateway error classification, B1 elapsed preflight, E1/E2 exceptions,
# S1 status masking, F1 fact size. Each test states the review finding it guards.
# ---------------------------------------------------------------------------

def recall_only(**overrides):
    return small_workload(traffic_mix={"recall": 1.0}, **overrides)


class GatewayErrorClassificationTests(FakeServerTestCase):
    """C1. The gateway returns pool and quota refusals as isError tool results over
    HTTP 200 with the payload {"protocol_version", "error", "message", "suggestion"};
    only the rate limits are HTTP 429. The fixture emits exactly that shape."""

    def fail(self, code, verb=None, **extra):
        self.server.state.tools_mode = "tool_code"
        self.server.state.fail_code = code
        self.server.state.fail_verb = verb
        self.server.state.fail_extra = extra

    def results(self, result, outcome):
        return [r for r in result["results"] if r["outcome"] == outcome]

    def test_the_decoder_maps_every_known_code_and_never_reflects_anything_else(self):
        for code in load_cli.TOOL_ERROR_CLASSES:
            with self.subTest(code=code):
                self.assertEqual(load_cli.decode_tool_error(gateway_failure(code)), code)
        for hostile in (None, "capacity", 7, True, ["capacity"], {"error": "capacity"}, "Bearer secret-token", "CAPACITY", "capacity ", ""):
            with self.subTest(error=hostile):
                self.assertEqual(load_cli.decode_tool_error({"error": hostile}), load_cli.UNRECOGNIZED)
        for not_a_dict in (None, "capacity", ["capacity"], 5, {}):
            with self.subTest(payload=not_a_dict):
                self.assertEqual(load_cli.decode_tool_error(not_a_dict), load_cli.UNRECOGNIZED)

    def test_capacity_and_operation_in_progress_are_counted_as_admission_rejections(self):
        """Reproduction of the finding: with the pool refusing every recall as `capacity`
        the old client reported admission rejections 0.0%, pass true."""
        for code in ("capacity", "operation_in_progress"):
            with self.subTest(code=code):
                self.fail(code)
                result = self.run_live(recall_only())
                offered = result["offered_total"]
                self.assertEqual(result["outcome_counts"]["rejected_admission"], offered)
                self.assertEqual(result["outcome_counts"]["tool_error"], 0)
                self.assertEqual(result["admission_rejections_by_source"], {"tool_result": offered})
                self.assertEqual(result["tool_errors_by_code"], {code: offered})
                check = result["by_repetition"][0]["threshold_evaluation"]["unexpected_admission_rejection_max_pct"]
                self.assertEqual(check["observed"], 100.0)
                self.assertIs(check["pass"], False)
                record = self.results(result, "rejected_admission")[0]
                self.assertEqual((record["error_code"], record["failure_class"], record["admission_source"]), (code, "admission", "tool_result"))
                self.assertEqual(result["status"], "FAIL")

    def test_an_http_429_is_still_an_admission_rejection_with_its_own_source(self):
        self.server.state.tools_mode = "http429"
        result = self.run_live(recall_only())
        self.assertEqual(result["admission_rejections_by_source"], {"http_429": result["offered_total"]})
        self.assertEqual(self.results(result, "rejected_admission")[0]["http_status"], 429)

    def test_a_quota_refusal_is_distinguished_from_admission_and_from_client_faults(self):
        self.fail("limit_exceeded", reset_at="2026-10-01T00:00:00Z", upgrade_url="/billing")
        result = self.run_live(recall_only())
        self.assertEqual(result["outcome_counts"]["rejected_admission"], 0)
        self.assertEqual(result["outcome_counts"]["tool_error"], result["offered_total"])
        record = self.results(result, "tool_error")[0]
        self.assertEqual((record["error_code"], record["failure_class"]), ("limit_exceeded", "quota"))
        check = result["by_repetition"][0]["threshold_evaluation"]["unexpected_admission_rejection_max_pct"]
        self.assertIsNone(check["observed"])  # all outcomes were expected quota refusals; no eligible denominator exists
        self.assertIsNone(check["pass"])
        self.assertIs(result["by_repetition"][0]["threshold_evaluation"]["min_offered_completion_pct"]["pass"], False)  # but it is a failure to complete
        self.assertNotIn("2026-10-01", json.dumps(result))  # reset_at and upgrade_url are not recorded

    def test_client_and_server_faults_keep_their_own_classes(self):
        expected = {
            "input_too_large": "client_fault", "invalid_params": "client_fault", "provenance_required": "client_fault", "scope_denied": "client_fault",
            "internal": "server_fault", "unavailable": "server_fault", "operation_conflict": "operation", "operation_canceled": "operation", "not_found": "not_found",
        }
        for code, failure_class in expected.items():
            with self.subTest(code=code):
                self.fail(code)
                result = self.run_live(recall_only())
                record = self.results(result, "tool_error")[0]
                self.assertEqual((record["error_code"], record["failure_class"]), (code, failure_class))
                self.assertEqual(result["tool_errors_by_code"], {code: result["offered_total"]})

    def test_a_forget_refused_for_any_reason_other_than_not_found_is_not_a_durability_error(self):
        """Reproduction of the finding: four forgets refused as `capacity` gave
        isolation_durability_errors_max observed 4, pass false."""
        for code in ("capacity", "limit_exceeded", "internal", "unavailable", "scope_denied", "operation_in_progress"):
            with self.subTest(code=code):
                self.fail(code, verb="forget")
                workload = small_workload(traffic_mix={"remember": 0.5, "forget": 0.5}, concurrency={"clients": 2, "baseline_request_rate_per_s": 60})
                result = self.run_live(workload)
                forgets = [r for r in result["results"] if r["verb"] == "forget" and r["outcome"] in ("rejected_admission", "tool_error")]
                self.assertGreater(len(forgets), 0)  # forgets were offered, dispatched and refused
                self.assertFalse(any(r.get("durability_error") for r in result["results"]))
                check = result["by_repetition"][0]["threshold_evaluation"]["isolation_durability_errors_max"]
                self.assertEqual(check["observed"], 0)
                self.assertIsNot(check["pass"], False)

    def test_a_forget_answered_not_found_for_an_id_this_run_remembered_is_the_durability_error(self):
        self.server.state.forget_loses = True
        workload = small_workload(traffic_mix={"remember": 0.5, "forget": 0.5}, concurrency={"clients": 2, "baseline_request_rate_per_s": 60})
        result = self.run_live(workload)
        lost = [r for r in result["results"] if r.get("durability_error")]
        self.assertGreaterEqual(len(lost), 1)
        self.assertTrue(all(r["verb"] == "forget" and r["error_code"] == "not_found" for r in lost))
        self.assertIs(result["by_repetition"][0]["threshold_evaluation"]["isolation_durability_errors_max"]["pass"], False)

    def test_an_unknown_or_hostile_error_code_is_unrecognized_and_never_reflected(self):
        self.server.state.tools_mode = "echo_code"  # the error field carries the request's Authorization header
        result = self.run_live(recall_only(), credential=LONG_CRED)
        self.assertEqual(result["tool_errors_by_code"], {"unrecognized": result["offered_total"]})
        record = self.results(result, "tool_error")[0]
        self.assertEqual((record["error_code"], record["failure_class"]), ("unrecognized", "unrecognized"))
        blob = json.dumps(result)
        self.assertNotIn(LONG_CRED, blob)
        self.assertNotIn("Bearer", blob)


class ElapsedPreflightTests(unittest.TestCase):
    """B1. The run clock starts at construction, so setup, the final drain and cleanup
    are elapsed time the preflight must count, not only the schedule."""

    def setUp(self):
        # Two accounts; 3 repetitions of (6 s steady + 3 s burst) = 27 s of schedule.
        self.workload = authorized_workload(phases=[{"name": "steady", "minutes": 0.1, "rate_multiplier": 1}, {"name": "burst", "minutes": 0.05, "rate_multiplier": 1}])
        self.accounts = harness.build_accounts(self.workload["cardinalities"], "paid")
        self.arrivals = harness.generate_arrivals(self.workload, self.accounts, random.Random(load_cli.BASE_SEED))

    def preflight(self, cap):
        budget = load_cli.parse_budget(authorized_manifest(max_elapsed_seconds=cap))[0]
        return load_cli.preflight_budget(budget, self.workload, self.accounts, self.arrivals)

    def test_the_requirement_is_the_exact_sum_of_its_components(self):
        need = load_cli.elapsed_requirement(self.workload, self.accounts)
        self.assertAlmostEqual(need["schedule"], 27.0)
        self.assertAlmostEqual(need["setup"], load_cli.READINESS_TIMEOUT_S + 2 * 2 * load_cli.DEFAULT_CALL_TIMEOUT_S)  # 5 + 40
        self.assertAlmostEqual(need["drain"], load_cli.DEFAULT_CALL_TIMEOUT_S)  # 10
        self.assertAlmostEqual(need["cleanup"], 2 * load_cli.DEFAULT_CALL_TIMEOUT_S)  # 20
        self.assertAlmostEqual(need["total"], 27.0 + 45.0 + 10.0 + 20.0)
        self.assertEqual((load_cli.READINESS_TIMEOUT_S, load_cli.DEFAULT_CALL_TIMEOUT_S), (5.0, 10.0))  # the arithmetic above assumes these

    def test_a_cap_exactly_at_the_requirement_passes_and_one_millisecond_less_is_refused_with_the_breakdown(self):
        total = load_cli.elapsed_requirement(self.workload, self.accounts)["total"]
        self.assertIsNone(self.preflight(total))
        error = self.preflight(total - 0.001)
        self.assertIn(f"max_elapsed_seconds {total - 0.001} < required {total}", error)
        for part in ("schedule 27.0", "setup 45.0", "final drain 10.0", "cleanup 20.0"):
            self.assertIn(part, error)

    def test_a_cap_equal_to_the_schedule_alone_is_now_refused(self):
        """Reproduction of the finding: a cap equal to the schedule passed preflight and was certain to be cut short."""
        self.assertIn("required", self.preflight(27.0))

    def test_the_reviewers_fixture_arithmetic(self):
        """3 x 6 s, one account: 18 + (5 + 20) + 10 + 10 = 63. Caps 18 and 22 (which the old preflight accepted) are refused."""
        workload = authorized_workload(phases=[{"name": "steady", "minutes": 0.1}], cardinalities={"paid": {"accounts": {"scale": 1}, "total_facts": 10}})
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(load_cli.BASE_SEED))
        need = load_cli.elapsed_requirement(workload, accounts)
        self.assertAlmostEqual(need["total"], 63.0)
        for cap, refused in ((18.0, True), (22.0, True), (62.999, True), (63.0, False)):
            budget = load_cli.parse_budget(authorized_manifest(max_elapsed_seconds=cap))[0]
            self.assertEqual(load_cli.preflight_budget(budget, workload, accounts, arrivals) is not None, refused, cap)

    def test_the_real_frozen_workload_needs_8535_seconds(self):
        workload = json.loads(WORKLOAD_PATH.read_text())
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        need = load_cli.elapsed_requirement(workload, accounts)
        self.assertEqual(len(accounts), 14)
        self.assertAlmostEqual(need["schedule"], 3 * 45 * 60)  # 8100
        self.assertAlmostEqual(need["setup"], 5 + 14 * 2 * 10)  # 285
        self.assertAlmostEqual(need["cleanup"], 14 * 10)  # 140
        self.assertAlmostEqual(need["total"], 8100 + 285 + 10 + 140)  # 8535

    def test_prepare_live_reports_the_setup_and_cleanup_shortfall(self):
        workload = authorized_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, cred_dir, True)
        make_credential_dir(cred_dir, accounts)
        plan, error = load_cli.prepare_live(authorized_manifest(cred_dir / "creds", max_elapsed_seconds=30), workload)
        self.assertIsNone(plan)
        self.assertIn("setup", error)
        self.assertIn("cleanup", error)


class SlowSetupRunTests(FakeServerTestCase):
    """B1 end to end: a cap that the elapsed requirement accepts is enough for a run
    with a slow setup, and a cap equal to the schedule alone is cut short."""

    def workload(self):
        return small_workload(repetitions=3, traffic_mix={"recall": 1.0})  # 3 x 0.6 s = 1.8 s of schedule, 2 accounts

    def test_a_schedule_only_cap_is_cut_short_by_a_slow_setup(self):
        self.server.state.init_delay_s = 0.6  # two initializes: 1.2 s of setup before the schedule origin
        result = self.run_live(self.workload(), max_elapsed_seconds=1.8)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertGreater(result["outcome_counts"]["not_dispatched"], 0)

    def test_a_cap_that_covers_the_requirement_is_not_cut_short(self):
        self.server.state.init_delay_s = 0.6
        # Scale the two timeouts the requirement is built from down to this fixture's speed (every exchange here takes well under 0.7 s).
        with mock.patch.object(load_cli, "READINESS_TIMEOUT_S", 0.7), mock.patch.object(load_cli, "DEFAULT_CALL_TIMEOUT_S", 0.7):
            workload = self.workload()
            accounts = harness.build_accounts(workload["cardinalities"], "paid")
            need = load_cli.elapsed_requirement(workload, accounts)
        self.assertAlmostEqual(need["total"], 1.8 + (0.7 + 2 * 2 * 0.7) + 0.7 + 2 * 0.7)
        result = self.run_live(workload, max_elapsed_seconds=need["total"])
        self.assertEqual(result["status"], "PARTIAL")
        self.assertEqual(result["outcome_counts"]["not_dispatched"], 0)
        self.assertEqual(result["outcome_counts"]["ok"], result["offered_total"])
        self.assertEqual(result["cleanup"]["closed_confirmed"], 2)


class MalformedInitializeTests(FakeServerTestCase):
    """E1. A 200 reply to initialize that issues a session id and then cannot be parsed
    used to raise RecursionError out of run_live: no receipt and no cleanup."""

    def test_a_deeply_nested_initialize_reply_still_returns_a_sanitized_receipt(self):
        self.server.state.init_mode = "deep_json"
        result = self.run_live(recall_only())
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("session initialize failed for account", result["reason"])
        self.assertIn("McpProtocolError", result["reason"])
        self.assertEqual(result["outcome_counts"]["not_dispatched"], result["offered_total"])
        self.assertNotIn("[[[[", json.dumps(result))

    def test_the_session_id_issued_with_an_unparseable_reply_is_closed(self):
        self.server.state.init_mode = "deep_json"
        result = self.run_live(recall_only())
        self.assertEqual(result["cleanup"]["server_sessions_created"], 1)
        self.assertEqual(result["cleanup"]["closed_confirmed"], 1)
        self.assertEqual(result["cleanup"]["sessions_left_open"], 0)
        self.assertEqual(self.server.state.deleted_ids, ["sess-1"])

    def test_the_session_object_binds_the_id_before_the_body_is_parsed(self):
        self.server.state.init_mode = "deep_json"
        session = self.session()
        with self.assertRaises(load_cli.McpProtocolError) as ctx:
            session.initialize()
        self.assertEqual(str(ctx.exception), "response body is nested too deeply")
        self.assertTrue(session.has_server_session)
        self.assertFalse(session.initialized)
        self.assertEqual(session.close(), ("closed", None))

    def test_a_reply_that_is_not_json_still_leaves_a_closable_session(self):
        self.server.state.init_mode = "not_json"
        session = self.session()
        with self.assertRaises(load_cli.McpProtocolError) as ctx:
            session.initialize()
        self.assertEqual(str(ctx.exception), "non-JSON response body")  # fixed text: the upstream body is never echoed
        self.assertTrue(session.has_server_session)
        self.assertEqual(session.close(), ("closed", None))

    def test_a_malformed_session_id_is_not_bound_and_is_reported(self):
        session = self.session()
        session._bind_issued_session({"Mcp-Session-Id": "has a space"})
        self.assertFalse(session.has_server_session)
        session._bind_issued_session({"Mcp-Session-Id": "ok-id-1"})
        self.assertTrue(session.has_server_session)

    def test_a_deeply_nested_tools_call_reply_is_a_protocol_error_not_a_lost_run(self):
        self.server.state.tools_mode = "deep_json"
        result = self.run_live(recall_only())
        self.assertEqual(result["outcome_counts"]["protocol_error"], result["offered_total"])
        self.assertEqual({r["error"] for r in result["results"]}, {"response body is nested too deeply"})

    def test_any_exception_from_the_run_becomes_a_receipt_and_the_session_it_made_is_closed(self):
        def crashing_execute(origin, workload, accounts, credentials, arrivals, run, records, created):
            session = load_cli.McpSession(origin, credentials[accounts[0]["id"]], run)
            created.append(session)
            session.initialize()
            raise RuntimeError("SECRET-upstream-text " + LONG_CRED)

        with mock.patch.object(load_cli, "_execute", crashing_execute):
            result = self.run_live(recall_only(), credential=LONG_CRED)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("client error during the run: RuntimeError", result["reason"])
        blob = json.dumps(result)
        self.assertNotIn("SECRET-upstream-text", blob)
        self.assertNotIn(LONG_CRED, blob)
        self.assertEqual(result["cleanup"]["closed_confirmed"], 1)
        self.assertEqual(self.server.state.deleted_ids, ["sess-1"])

    def test_a_client_error_is_named_even_when_an_earlier_stop_reason_won(self):
        def crashing_execute(origin, workload, accounts, credentials, arrivals, run, records, created):
            run.stop("max_calls budget exhausted")
            raise ZeroDivisionError

        with mock.patch.object(load_cli, "_execute", crashing_execute):
            result = self.run_live(recall_only())
        self.assertIn("max_calls budget exhausted", result["reason"])
        self.assertIn("client error during the run: ZeroDivisionError", result["reason"])

    def test_an_initialize_that_raises_anything_at_all_is_recorded_by_class_only(self):
        with mock.patch.object(load_cli.McpSession, "initialize", side_effect=RuntimeError("SECRET-upstream-text")):
            result = self.run_live(recall_only())
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("RuntimeError", result["reason"])
        self.assertNotIn("SECRET-upstream-text", json.dumps(result))

    def test_an_interrupt_is_not_swallowed_but_the_session_is_still_closed(self):
        def interrupted_execute(origin, workload, accounts, credentials, arrivals, run, records, created):
            session = load_cli.McpSession(origin, credentials[accounts[0]["id"]], run)
            created.append(session)
            session.initialize()
            raise KeyboardInterrupt

        with mock.patch.object(load_cli, "_execute", interrupted_execute):
            with self.assertRaises(KeyboardInterrupt):
                self.run_live(recall_only())
        self.assertEqual(self.server.state.deleted_ids, ["sess-1"])

    def test_a_close_that_raises_cannot_discard_the_results(self):
        with mock.patch.object(load_cli.McpSession, "close", side_effect=RuntimeError("SECRET-upstream-text")):
            result = self.run_live(recall_only())
        self.assertEqual(result["outcome_counts"]["ok"], result["offered_total"])
        self.assertEqual(result["cleanup"]["close_failed"], {"RuntimeError": 2})
        self.assertEqual(result["cleanup"]["sessions_left_open"], 2)
        self.assertNotIn("SECRET-upstream-text", json.dumps(result))

    def test_a_close_exchange_that_raises_a_non_transport_error_is_recorded_by_class(self):
        session = self.session()
        session.initialize()
        real = load_cli._http_exchange

        def failing_on_delete(method, *args, **kwargs):
            if method == "DELETE":
                raise ValueError("SECRET-upstream-text")
            return real(method, *args, **kwargs)

        with mock.patch.object(load_cli, "_http_exchange", failing_on_delete):
            self.assertEqual(session.close(), ("failed", "ValueError"))


class HugeIntegerTests(unittest.TestCase):
    """E2. JSON integer literals are unbounded: 10**400 made math.isfinite raise OverflowError."""

    def budget(self, **overrides):
        return {"budget": budget_dict(**overrides)}

    def test_a_huge_integer_in_any_numeric_field_is_an_invalid_budget_not_an_overflow(self):
        huge = 10**400
        for field in ("max_elapsed_seconds", "approved_max_usd", "worst_case_usd_per_call", "max_calls", "max_input_tokens"):
            with self.subTest(field=field):
                budget, error = load_cli.parse_budget(self.budget(**{field: huge}))
                self.assertIsNone(budget)
                self.assertIn(field, error)
                self.assertLess(len(error), 400)  # the value is not echoed
        for field in ("readiness_tokens", "cold_open_tokens"):
            with self.subTest(field=field):
                bound = {"readiness_tokens": 0, "cold_open_tokens": 0, "basis": "test-only", field: huge}
                self.assertIsNotNone(load_cli.parse_budget(self.budget(provider_work_bound=bound))[1])

    def test_an_integer_past_the_string_conversion_limit_does_not_crash_the_error_message(self):
        budget, error = load_cli.parse_budget(self.budget(max_elapsed_seconds=10**5000))
        self.assertIsNone(budget)
        self.assertIn("too large to show", error)

    def test_the_machine_sized_limit_is_inclusive(self):
        limit = load_cli.MAX_BUDGET_COUNT
        self.assertIsNotNone(load_cli.parse_budget(self.budget(max_calls=limit + 1))[1])
        self.assertIsNone(load_cli.parse_budget(self.budget(max_calls=limit, max_input_tokens=limit))[1])

    def test_a_hand_built_budget_and_the_ledger_are_held_to_the_same_rules(self):
        with self.assertRaises(ValueError):
            load_cli.RunBudget(make_budget(max_elapsed_seconds=10**400))
        run = make_run()
        with self.assertRaises(ValueError):
            run.reserve("readiness", 0, 10**400)
        self.assertEqual(run.calls, 0)

    def test_run_live_and_prepare_live_return_blocked_and_send_nothing(self):
        server = FakeMcpServer()
        server.start()
        self.addCleanup(server.stop)
        workload = small_workload()
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        manifest = live_manifest(server.origin, max_elapsed_seconds=10**400)
        result = load_cli.run_live(manifest, workload, accounts, {a["id"]: "test-credential-value" for a in accounts})
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("max_elapsed_seconds", result["reason"])
        self.assertEqual(server.state.requests_seen, [])
        plan, error = load_cli.prepare_live(authorized_manifest(max_elapsed_seconds=10**400), authorized_workload())
        self.assertIsNone(plan)
        self.assertIn("max_elapsed_seconds", error)

    def test_a_manifest_file_with_an_absurd_integer_is_a_blocked_receipt_not_a_crash(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "manifest.json"
            path.write_text('{"budget": {"max_calls": ' + "9" * 5000 + "}}")
            out = Path(tmp) / "out.json"
            with self.assertRaises(SystemExit) as ctx:
                run_main("--fixtures", "--manifest", str(path), "--output", str(out))
        self.assertEqual(ctx.exception.code, 2)


class MeasuredFailureStatusTests(FakeServerTestCase):
    """S1. A budget stop used to report BLOCKED and hide checks that had already failed."""

    def workload(self):
        return small_workload(repetitions=3, traffic_mix={"recall": 1.0})  # 3 x 0.6 s

    def test_measured_failures_are_reported_as_fail_even_when_a_cap_stopped_the_run(self):
        """Reproduction of the finding: every call answered 500 and a cap ended the run in repetition 1: BLOCKED."""
        self.server.state.tools_mode = "http500"
        result = self.run_live(self.workload(), max_elapsed_seconds=1.0)
        self.assertEqual(result["status"], "FAIL")
        self.assertIn("rep 0: unexpected_5xx_max_pct", result["reason"])
        self.assertIn("rep 0: min_offered_completion_pct", result["reason"])
        self.assertIn("the run also stopped early: max_elapsed_seconds budget exhausted", result["reason"])
        self.assertGreater(result["outcome_counts"]["not_dispatched"], 0)
        self.assertGreater(result["outcome_counts"]["unexpected_5xx"], 0)
        self.assertFalse(result["qualified"])

    def test_a_cut_short_run_whose_dispatched_requests_all_succeeded_stays_blocked(self):
        result = self.run_live(self.workload(), max_elapsed_seconds=1.0)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertGreater(result["outcome_counts"]["not_dispatched"], 0)
        self.assertEqual(result["outcome_counts"]["ok"] + result["outcome_counts"]["not_dispatched"], result["offered_total"])
        rep1 = result["by_repetition"][1]["threshold_evaluation"]["min_offered_completion_pct"]
        self.assertIs(rep1["pass"], False)  # the full denominator still counts undispatched arrivals
        self.assertIs(rep1["pass_if_undispatched_were_ok"], True)  # but dispatched requests alone did not cause the miss
        self.assertTrue(any("only because offered requests were never dispatched" in g for g in result["qualification_gaps"]))

    def test_a_wholly_undispatched_run_is_blocked_and_its_synthetic_completion_failure_is_not_a_measured_one(self):
        self.server.state.readyz_status = 503
        result = self.run_live(self.workload())
        self.assertEqual(result["status"], "BLOCKED")
        self.assertIn("readiness probe returned HTTP 503", result["reason"])
        self.assertEqual(result["outcome_counts"]["not_dispatched"], result["offered_total"])
        completion = result["by_repetition"][0]["threshold_evaluation"]["min_offered_completion_pct"]
        self.assertEqual((completion["observed"], completion["pass"]), (0.0, False))
        self.assertIs(completion["pass_if_undispatched_were_ok"], True)
        self.assertNotIn("min_offered_completion_pct", result["reason"])

    def test_a_completion_miss_caused_by_dispatched_failures_is_measured_even_with_undispatched_arrivals(self):
        self.server.state.tools_mode = "http500"
        result = self.run_live(self.workload(), max_elapsed_seconds=1.0)
        completion = result["by_repetition"][0]["threshold_evaluation"]["min_offered_completion_pct"]
        self.assertIs(completion["pass"], False)
        self.assertIs(completion["pass_if_undispatched_were_ok"], False)

    def test_an_uncapped_run_with_a_failing_server_is_still_fail_with_the_same_reason_format(self):
        self.server.state.tools_mode = "http500"
        result = self.run_live(self.workload())
        self.assertEqual(result["status"], "FAIL")
        self.assertNotIn("stopped early", result["reason"])
        self.assertIn("rep 0: unexpected_5xx_max_pct", result["reason"])


class FactSizeTests(FakeServerTestCase):
    """F1. 12 steady remember calls rendered past the gateway's 4096-byte fact limit.
    The fixture now enforces that limit exactly as gateway.go does."""

    def test_the_frozen_fact_range_renders_inside_the_limit_without_truncation(self):
        workload = harness.load_workload(WORKLOAD_PATH)
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        arrivals = harness.generate_arrivals(workload, accounts, random.Random(load_cli.BASE_SEED))
        low, high = workload["fact_tokens"]["min"], workload["fact_tokens"]["max"]
        remembers = [a for a in arrivals if a["verb"] == "remember"]
        self.assertGreater(len(remembers), 1000)
        largest = 0
        for rep in range(workload["repetitions"]):
            for seq, arrival in enumerate(arrivals):
                if arrival["verb"] != "remember":
                    continue
                fact = load_cli.tool_arguments("remember", arrival["account"], rep, seq, arrival["tokens"])["fact"]
                self.assertEqual(len(fact.split()), arrival["tokens"])  # exactly the requested size: not truncated
                self.assertTrue(low <= arrival["tokens"] <= high)
                largest = max(largest, len(fact.encode()))
        self.assertLessEqual(largest, harness.FACT_MAX_BYTES)
        self.assertLessEqual(harness.max_rendered_bytes(high), harness.FACT_MAX_BYTES)  # the seed-independent bound also fits

    def test_a_run_at_the_top_of_the_frozen_range_gets_no_input_too_large(self):
        high = harness.load_workload(WORKLOAD_PATH)["fact_tokens"]["max"]
        workload = small_workload(traffic_mix={"remember": 1.0}, fact_tokens={"min": high, "max": high})
        result = self.run_live(workload)
        self.assertEqual(result["outcome_counts"]["ok"], result["offered_total"])
        self.assertEqual(result["tool_errors_by_code"], {})

    def test_the_fixture_really_enforces_the_limit(self):
        """Control: an oversized fact is refused as input_too_large, so the run above is not vacuous."""
        session = self.session()
        session.initialize()
        oversized = {"fact": "x" * 4097, "provenance": "test-only"}
        outcome = session.call_tool("remember", oversized)
        self.assertEqual((outcome["is_error"], outcome["error_code"]), (True, "input_too_large"))

    def test_a_range_that_can_exceed_the_limit_blocks_prepare_live_and_is_never_truncated(self):
        workload = authorized_workload(fact_tokens={"min": 32, "max": 800})
        plan, error = load_cli.prepare_live(authorized_manifest(), workload)
        self.assertIsNone(plan)
        self.assertIn("fact limit", error)
        self.assertIn("never truncates", error)
        with self.assertRaises(ValueError):
            load_cli.tool_arguments("remember", "free-0", 0, 0, 800)

    def test_word_counts_are_documented_as_nominal_not_token_counts(self):
        self.assertIn("not a token count", harness.render_text.__doc__)
        result = self.run_live()
        self.assertTrue(any("nominal word counts" in g and "no provider tokenizer is assumed" in g for g in result["qualification_gaps"]))


class LatencyAndAdmissionLimitationTests(FakeServerTestCase):
    """Fresh-connection overhead, dispatch-versus-arrival latency and the hot-tenant
    rate limit are recorded limitations of every live result."""

    def test_every_result_records_the_three_limitations(self):
        gaps = " | ".join(self.run_live()["qualification_gaps"])
        self.assertIn("Connection: close", gaps)
        self.assertIn("fresh TCP", gaps)
        self.assertIn("TLS handshake", gaps)
        self.assertIn("latency_s runs from dispatch, not from the scheduled arrival", gaps)
        self.assertIn("queue_delay_s", gaps)
        self.assertIn("per-account limit (120 requests a minute", gaps)
        self.assertIn("counted as failures here, not excluded as expected saturation", gaps)
        self.assertIn("decision-request-hot-tenant-rate-limit.md", gaps)

    def test_the_client_really_sends_connection_close_on_every_request(self):
        self.run_live()
        self.assertTrue(self.server.state.requests_seen)
        self.assertEqual({headers.get("Connection") for _m, _p, headers in self.server.state.requests_seen}, {"close"})


# ---------------------------------------------------------------------------
# The CLI: fixtures mode works; --live is unconditionally BLOCKED.
# ---------------------------------------------------------------------------

def run_main(*argv):
    saved = sys.argv
    sys.argv = ["load.py", *argv]
    try:
        return load_cli.main()
    finally:
        sys.argv = saved


class RunFixturesTests(unittest.TestCase):
    def test_produces_valid_structure_with_replay_verified(self):
        workload = load_cli.harness.load_workload(WORKLOAD_PATH)
        result = load_cli.run_fixtures(base_manifest(), workload)
        self.assertTrue(result["replay_determinism_verified"])
        self.assertEqual(len(result["repetitions"]), workload["repetitions"])
        self.assertFalse(result["resource_usage"]["available"])


class CLIEndToEndTests(FakeServerTestCase):
    def test_fixtures_mode_exits_zero_and_writes_output(self):
        out = self.tmp / "load-fixture.json"
        self.assertEqual(run_main("--fixtures", "--manifest", str(MANIFEST_PATH), "--output", str(out)), 0)
        data = json.loads(out.read_text())
        self.assertEqual(data["mode"], "fixtures")
        self.assertTrue(data["replay_determinism_verified"])

    def test_live_mode_is_blocked_with_a_zero_call_receipt(self):
        manifest_path = self.tmp / "manifest.json"
        manifest_path.write_text(json.dumps(base_manifest()))
        out = self.tmp / "load.json"
        self.assertEqual(run_main("--live", "--manifest", str(manifest_path), "--output", str(out)), 2)
        data = json.loads(out.read_text())
        self.assertEqual(data["status"], "BLOCKED")
        self.assertEqual((data["calls_used"], data["tokens_used"], data["usd_used_worst_case"]), (0, 0, 0))

    def test_live_stays_blocked_even_with_fully_valid_inputs_and_a_live_server(self):
        workload = json.loads(WORKLOAD_PATH.read_text())
        accounts = harness.build_accounts(workload["cardinalities"], "paid")
        cred_dir = make_credential_dir(self.tmp, accounts)
        manifest = authorized_manifest(cred_dir, max_calls=10**7, max_input_tokens=10**9, approved_max_usd=10**6, max_elapsed_seconds=10**6)
        manifest["environment"]["origin"] = self.server.origin
        manifest_path = self.tmp / "manifest.json"
        manifest_path.write_text(json.dumps(manifest))
        out = self.tmp / "load-live.json"
        with mock.patch.object(load_cli, "check_credentials", side_effect=AssertionError("secret read")), \
             mock.patch.object(load_cli, "prepare_live", side_effect=AssertionError("prepared")), \
             mock.patch.object(load_cli, "run_live", side_effect=AssertionError("ran")), \
             mock.patch.object(load_cli, "check_readiness", side_effect=AssertionError("network")):
            self.assertEqual(run_main("--live", "--manifest", str(manifest_path), "--output", str(out)), 2)
        self.assertEqual(self.server.state.requests_seen, [])
        self.assertEqual(json.loads(out.read_text())["status"], "BLOCKED")

    def test_live_gate_precedes_manifest_credentials_and_network(self):
        out = self.tmp / "blocked.json"
        with mock.patch.object(load_cli, "load_manifest", side_effect=AssertionError("manifest read")), \
             mock.patch.object(load_cli, "check_credentials", side_effect=AssertionError("secret read")), \
             mock.patch.object(load_cli, "check_readiness", side_effect=AssertionError("network")), \
             mock.patch.object(load_cli, "_http_exchange", side_effect=AssertionError("network")), \
             mock.patch.object(load_cli, "run_live", side_effect=AssertionError("network")):
            self.assertEqual(run_main("--live", "--manifest", str(self.tmp / "does-not-exist"), "--output", str(out)), 2)
        self.assertEqual(json.loads(out.read_text())["status"], "BLOCKED")

    def test_manifest_not_found_blocked_json_not_bare_exception(self):
        with self.assertRaises(SystemExit) as ctx:
            run_main("--fixtures", "--manifest", "/nonexistent/manifest.json", "--output", str(self.tmp / "out.json"))
        self.assertEqual(ctx.exception.code, 2)

    def test_invalid_json_manifest_blocked_json_not_bare_exception(self):
        bad = self.tmp / "manifest.json"
        bad.write_text("{not valid json")
        with self.assertRaises(SystemExit) as ctx:
            run_main("--fixtures", "--manifest", str(bad), "--output", str(self.tmp / "out.json"))
        self.assertEqual(ctx.exception.code, 2)


if __name__ == "__main__":
    unittest.main()
