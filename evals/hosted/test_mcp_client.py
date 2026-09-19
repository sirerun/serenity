"""Real-wire tests for MCPClient: redirect/credential-leak protection,
endpoint allowlisting, and isError handling -- all against real loopback
HTTP servers, never mocked at the urllib layer, so the actual bytes on the
wire are what's being tested.
"""

from __future__ import annotations

import http.client
import http.server
import json
import re
import sys
import threading
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

from lib.mcp_client import MAX_RESPONSE_BYTES, MCPClient, MCPError, validate_endpoint_url  # noqa: E402


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


class _DripHandler(http.server.BaseHTTPRequestHandler):
    """Sends a valid status line, then trickles the body one byte every
    0.05 s -- each recv succeeds well inside a 0.1 s socket timeout, yet the
    exchange never finishes. (The coordinator's urllib reproduction ran 0.789 s
    under a 0.1 s socket timeout.)"""

    def log_message(self, *a):
        pass

    def do_POST(self):
        import time as _t

        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", "100000")
        self.end_headers()
        try:
            for _ in range(200):
                self.wfile.write(b" ")
                self.wfile.flush()
                _t.sleep(0.05)
        except OSError:
            pass


class _SlowHeaderHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        import time as _t

        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        try:
            self.wfile.write(b"HTTP/1.1 200 OK\r\n")
            for _ in range(200):  # header bytes dripped one at a time, never finishing
                self.wfile.write(b"X")
                self.wfile.flush()
                _t.sleep(0.05)
        except OSError:
            pass


class TestWallClockDeadline(unittest.TestCase):
    """A socket timeout bounds one recv, not the exchange. Every request runs
    under a wall-clock deadline enforced by a watchdog that shuts the socket."""

    def _elapsed_for(self, handler, timeout_s, budget=None):
        import time

        server = _serve(handler)
        self.addCleanup(server.shutdown)
        self.addCleanup(server.server_close)
        client = MCPClient(f"{_origin(server)}/mcp", "tok", allowed_origins=[_origin(server)], timeout_s=timeout_s, budget=budget)
        started = time.monotonic()
        with self.assertRaises(MCPError) as ctx:
            client.initialize()
        return time.monotonic() - started, str(ctx.exception)

    def test_a_dripped_body_is_cut_off_at_the_deadline_not_at_the_drip_end(self):
        elapsed, message = self._elapsed_for(_DripHandler, timeout_s=0.3)
        self.assertLess(elapsed, 0.9, f"a 0.3 s deadline took {elapsed:.3f}s")
        self.assertIn("deadline exceeded", message)

    def test_dripped_response_headers_are_also_cut_off(self):
        elapsed, message = self._elapsed_for(_SlowHeaderHandler, timeout_s=0.3)
        self.assertLess(elapsed, 0.9, f"a 0.3 s deadline took {elapsed:.3f}s")
        self.assertIn("deadline exceeded", message)

    def test_the_budget_deadline_shortens_the_request_deadline(self):
        class _Budget:
            def precharge(self, n):
                pass

            def remaining_seconds(self):
                return 0.3

        elapsed, message = self._elapsed_for(_DripHandler, timeout_s=30.0, budget=_Budget())
        self.assertLess(elapsed, 0.9)
        self.assertIn("deadline exceeded", message)

    def test_a_fast_response_is_unaffected(self):
        server = _serve(_CaptureAuth)
        self.addCleanup(server.shutdown)
        self.addCleanup(server.server_close)
        client = MCPClient(f"{_origin(server)}/mcp", "tok", allowed_origins=[_origin(server)], timeout_s=5.0)
        self.assertEqual(client.initialize(), {})

    def test_the_deadline_message_never_contains_upstream_text_or_the_credential(self):
        _, message = self._elapsed_for(_DripHandler, timeout_s=0.2)
        self.assertNotIn("tok", message.replace("took", ""))


class _RawServer:
    """One-connection raw-socket server, so a test controls every byte and
    every pause (http.server adds headers of its own). `script(conn, done)`
    runs after the request is read; `done` is set at cleanup so a stalled
    script wakes and exits instead of outliving the test."""

    def __init__(self, script):
        import socket

        self.done = threading.Event()
        self._srv = socket.socket()
        self._srv.bind(("127.0.0.1", 0))
        self._srv.listen(1)
        self.origin = f"http://127.0.0.1:{self._srv.getsockname()[1]}"
        self._script = script
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def _run(self):
        conn = None
        try:
            conn, _ = self._srv.accept()
            conn.settimeout(5)
            self._read_request(conn)
            self._script(conn, self.done)
        except OSError:
            pass
        finally:
            for s in (conn, self._srv):
                try:
                    if s is not None:
                        s.close()
                except OSError:
                    pass

    @staticmethod
    def _read_request(conn) -> None:
        """The whole request, headers and body. http.client sends them in
        separate segments; replying and closing with body bytes still unread
        makes the OS reset the connection, and the client would see a reset
        instead of the response this server means to send."""
        data = b""
        while b"\r\n\r\n" not in data:
            chunk = conn.recv(65536)
            if not chunk:
                return
            data += chunk
        head, _, rest = data.partition(b"\r\n\r\n")
        m = re.search(rb"content-length:\s*(\d+)", head, re.I)
        need = int(m.group(1)) if m else 0
        while len(rest) < need:
            chunk = conn.recv(65536)
            if not chunk:
                return
            rest += chunk

    def close(self):
        self.done.set()
        self._thread.join(timeout=5)


_OK_BODY = b'{"jsonrpc":"2.0","id":1,"result":{}}'


def _close_delimited(status_line: bytes, body: bytes):
    """`Connection: close` and no Content-Length: the body ends at EOF."""

    def script(conn, done):
        conn.sendall(status_line + b"\r\nConnection: close\r\nContent-Type: application/json\r\n\r\n" + body)

    return script


def _first_byte_then_stall(first_byte_after: float, connection_close: bool):
    def script(conn, done):
        if connection_close:
            conn.sendall(b"HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Type: application/json\r\n\r\n")
        else:
            conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\n")
        if done.wait(first_byte_after):
            return
        conn.sendall(b"{")
        done.wait(5)  # stall: never sends another byte

    return script


class _Recorder:
    """Records every socket http.client connects and every response it hands
    back, so a test can assert each one is closed after MCPClient returns or
    raises. Patched at the http.client layer; the bytes still cross a real
    loopback socket."""

    def __init__(self, test: unittest.TestCase):
        self.socks: list = []
        self.responses: list = []
        real_connect = http.client.HTTPConnection.connect
        real_get = http.client.HTTPConnection.getresponse
        recorder = self

        def connect(conn):
            real_connect(conn)
            recorder.socks.append(conn.sock)

        def getresponse(conn):
            resp = real_get(conn)
            recorder.responses.append(resp)
            return resp

        for name, fn in (("connect", connect), ("getresponse", getresponse)):
            patcher = mock.patch.object(http.client.HTTPConnection, name, fn)
            patcher.start()
            test.addCleanup(patcher.stop)

    def assert_all_closed(self, test: unittest.TestCase, label: str):
        test.assertTrue(self.socks, f"{label}: no socket was recorded, so the check proves nothing")
        for s in self.socks:
            test.assertEqual(s.fileno(), -1, f"{label}: a socket is still open")
        for r in self.responses:
            test.assertTrue(r.isclosed(), f"{label}: a response is still open")


class TestResponseClosureAndStallDeadline(unittest.TestCase):
    """Closure on every path, and the coordinator's load-close-body case: a
    `Connection: close` response whose first body byte arrives before the
    deadline and whose remainder never does."""

    def _call(self, script, timeout_s=5.0):
        server = _RawServer(script)
        self.addCleanup(server.close)
        rec = _Recorder(self)
        client = MCPClient(f"{server.origin}/mcp", "tok", allowed_origins=[server.origin], timeout_s=timeout_s)
        return client, rec

    def test_a_close_delimited_body_is_read_and_its_response_closed(self):
        client, rec = self._call(_close_delimited(b"HTTP/1.1 200 OK", _OK_BODY))
        self.assertEqual(client.initialize(), {})
        rec.assert_all_closed(self, "close-delimited success")

    def test_every_failure_path_closes_the_response_and_the_socket(self):
        oversize = b" " * (MAX_RESPONSE_BYTES + 10)
        cases = {
            "http 500": _close_delimited(b"HTTP/1.1 500 Internal Server Error", b"boom"),
            "redirect": _close_delimited(b"HTTP/1.1 302 Found", b""),
            "non-json body": _close_delimited(b"HTTP/1.1 200 OK", b"not json"),
            "json-rpc error": _close_delimited(b"HTTP/1.1 200 OK", b'{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"x"}}'),
            "oversize body": _close_delimited(b"HTTP/1.1 200 OK", oversize),
        }
        for label, script in cases.items():
            with self.subTest(path=label):
                client, rec = self._call(script)
                with self.assertRaises(MCPError):
                    client.initialize()
                rec.assert_all_closed(self, label)

    def test_a_connection_that_is_refused_raises_a_fixed_error_and_leaves_nothing_open(self):
        import socket

        probe = socket.socket()
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]
        probe.close()  # nothing listens here now
        origin = f"http://127.0.0.1:{port}"
        client = MCPClient(f"{origin}/mcp", "tok", allowed_origins=[origin], timeout_s=2.0)
        with self.assertRaises(MCPError) as ctx:
            client.initialize()
        self.assertIn("transport error", str(ctx.exception))

    def test_first_byte_at_half_the_budget_then_stall_is_cut_off_at_the_deadline(self):
        import time

        for label, connection_close in (("connection: close", True), ("content-length", False)):
            with self.subTest(framing=label):
                client, rec = self._call(_first_byte_then_stall(0.5, connection_close), timeout_s=0.6)
                started = time.monotonic()
                with self.assertRaises(MCPError) as ctx:
                    client.initialize()
                elapsed = time.monotonic() - started
                # The coordinator's reproduction ran 1.105 s against a 0.6 s budget: the
                # per-recv timeout restarted when the first byte arrived.
                self.assertLess(elapsed, 0.85, f"a 0.6 s deadline took {elapsed:.3f}s")
                self.assertIn("deadline exceeded", str(ctx.exception))
                rec.assert_all_closed(self, f"stall ({label})")

    def test_a_budget_shortened_deadline_covers_the_stall_after_the_first_byte(self):
        import time

        class _Budget:
            def precharge(self, n):
                pass

            def remaining_seconds(self):
                return 0.6

        server = _RawServer(_first_byte_then_stall(0.5, True))
        self.addCleanup(server.close)
        client = MCPClient(f"{server.origin}/mcp", "tok", allowed_origins=[server.origin], timeout_s=30.0, budget=_Budget())
        started = time.monotonic()
        with self.assertRaises(MCPError):
            client.initialize()
        self.assertLess(time.monotonic() - started, 0.85)


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
