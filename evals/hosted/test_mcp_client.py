"""Real-wire tests for MCPClient: redirect/credential-leak protection,
endpoint allowlisting, and isError handling -- all against real loopback
HTTP servers, never mocked at the urllib layer, so the actual bytes on the
wire are what's being tested.
"""

from __future__ import annotations

import http.server
import json
import sys
import threading
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from lib.mcp_client import MCPClient, MCPError, validate_endpoint_url  # noqa: E402


def _serve(handler_cls) -> http.server.HTTPServer:
    server = http.server.HTTPServer(("127.0.0.1", 0), handler_cls)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


def _origin(server: http.server.HTTPServer) -> str:
    return f"http://127.0.0.1:{server.server_address[1]}"


class TestEndpointAllowlist(unittest.TestCase):
    def test_rejects_non_loopback_http(self):
        with self.assertRaises(ValueError):
            validate_endpoint_url("http://example.com/mcp", ["http://example.com"])

    def test_accepts_https_in_allowlist(self):
        validate_endpoint_url("https://app.serenity.sire.run/mcp", ["https://app.serenity.sire.run"])

    def test_rejects_origin_not_in_allowlist(self):
        with self.assertRaises(ValueError):
            validate_endpoint_url("https://app.serenity.sire.run/mcp", ["https://other.example"])

    def test_rejects_userinfo(self):
        with self.assertRaises(ValueError):
            validate_endpoint_url("https://user:pass@app.serenity.sire.run/mcp", ["https://app.serenity.sire.run"])

    def test_rejects_query_string(self):
        with self.assertRaises(ValueError):
            validate_endpoint_url("https://app.serenity.sire.run/mcp?token=x", ["https://app.serenity.sire.run"])

    def test_accepts_loopback_http_when_allowlisted(self):
        validate_endpoint_url("http://127.0.0.1:9999/mcp", ["http://127.0.0.1:9999"])


class _CaptureAuth(http.server.BaseHTTPRequestHandler):
    seen_auth: str | None = None

    def log_message(self, *a):
        pass

    def do_POST(self):
        _CaptureAuth.seen_auth = self.headers.get("Authorization")
        length = int(self.headers.get("Content-Length", 0))
        self.rfile.read(length)
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"jsonrpc": "2.0", "id": 1, "result": {}}).encode())


def _redirect_handler_factory(target_url: str, code: int):
    class _Redirect(http.server.BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            self.rfile.read(length)
            self.send_response(code)
            self.send_header("Location", target_url)
            self.end_headers()

    return _Redirect


class TestRedirectNeverLeaksCredential(unittest.TestCase):
    def setUp(self):
        _CaptureAuth.seen_auth = None
        self.server_b = _serve(_CaptureAuth)
        self.addCleanup(self.server_b.shutdown)
        self.addCleanup(self.server_b.server_close)
        self.b_url = f"{_origin(self.server_b)}/mcp"

    def _run_redirect_case(self, code: int):
        handler = _redirect_handler_factory(self.b_url, code)
        server_a = _serve(handler)
        self.addCleanup(server_a.shutdown)
        self.addCleanup(server_a.server_close)
        a_url = f"{_origin(server_a)}/mcp"

        client = MCPClient(a_url, "SECRET-TOKEN", allowed_origins=[_origin(server_a)])
        with self.assertRaises(MCPError):
            client.initialize()
        self.assertIsNone(_CaptureAuth.seen_auth, f"server B must never receive a request for a {code} redirect")

    def test_301_redirect_is_refused_and_credential_not_forwarded(self):
        self._run_redirect_case(301)

    def test_302_redirect_is_refused_and_credential_not_forwarded(self):
        self._run_redirect_case(302)

    def test_303_redirect_is_refused_and_credential_not_forwarded(self):
        self._run_redirect_case(303)

    def test_error_message_never_contains_the_credential(self):
        handler = _redirect_handler_factory(self.b_url, 302)
        server_a = _serve(handler)
        self.addCleanup(server_a.shutdown)
        self.addCleanup(server_a.server_close)
        a_url = f"{_origin(server_a)}/mcp"

        client = MCPClient(a_url, "SECRET-TOKEN-XYZ", allowed_origins=[_origin(server_a)])
        try:
            client.initialize()
            self.fail("expected MCPError")
        except MCPError as e:
            self.assertNotIn("SECRET-TOKEN-XYZ", str(e))


class _IsErrorTool(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        if body.get("method") == "initialize":
            payload = {"jsonrpc": "2.0", "id": body["id"], "result": {}}
        else:
            payload = {
                "jsonrpc": "2.0",
                "id": body["id"],
                "result": {"isError": True, "content": [{"type": "text", "text": "invalid_argument"}]},
            }
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        if body.get("method") == "initialize":
            self.send_header("Mcp-Session-Id", "sess-1")
        self.end_headers()
        self.wfile.write(json.dumps(payload).encode())


class TestIsErrorRaisesRatherThanScoringEmpty(unittest.TestCase):
    def test_tool_level_is_error_raises_mcperror(self):
        server = _serve(_IsErrorTool)
        self.addCleanup(server.shutdown)
        self.addCleanup(server.server_close)
        url = f"{_origin(server)}/mcp"
        client = MCPClient(url, "tok", allowed_origins=[_origin(server)])
        client.initialize()
        with self.assertRaises(MCPError):
            client.call_tool("recall", {"query": "x", "limit": 5})


class TestCallToolRequiresExplicitInitialize(unittest.TestCase):
    def test_call_tool_before_initialize_raises_without_any_http_call(self):
        # No server is even started: if call_tool silently auto-initialized,
        # this would attempt a real connection and fail with a transport
        # error instead of the intended "no active session" refusal.
        client = MCPClient("https://example.invalid/mcp", "tok", allowed_origins=["https://example.invalid"])
        with self.assertRaises(MCPError) as ctx:
            client.call_tool("recall", {"query": "x", "limit": 5})
        self.assertIn("initialize", str(ctx.exception))
        self.assertEqual(client.calls_made, 0)


if __name__ == "__main__":
    unittest.main()
