"""Minimal MCP-over-HTTP JSON-RPC client for --live mode, speaking the
exact wire protocol internal/server/mcp implements (protocol.go's
{"jsonrpc":"2.0", ...} envelope, server.go's "initialize" / "tools/call"
dispatch, http.go's Mcp-Session-Id bootstrap, tools.go's Result.IsError).
Stdlib-only (http.client): this is a CLI harness, not a service, and shouldn't
need a dependency just to speak JSON-RPC over HTTP.

This client only ever calls the URL/credential named in an explicit,
frozen qualification manifest (see manifest.py) -- it never guesses an
endpoint or falls back to a default host, matching AGENTS.md's "never add
a public debug bypass" and the plan's "refuse production targets unless the
task explicitly authorizes them."

Security: the transport is http.client, not urllib. urllib's default opener
follows redirects and forwards every header -- including Authorization -- to
whatever host the redirect names (confirmed empirically: a 302 from server A
to server B on another port arrives at B carrying A's bearer token), and it
honors proxy environment variables. http.client follows no redirect and reads
no proxy setting: any 3xx is an error, and the credential goes only to
manifest.hosted_mcp.endpoint_url.

Deadline: a socket timeout bounds each individual recv, not the exchange. A
server that sends one byte every few tenths of a second never trips it (the
coordinator's urllib-drip reproduction ran 0.789 s under a 0.1 s socket
timeout). Every request therefore runs under a WALL-CLOCK deadline: a
watchdog thread shuts the socket down when the deadline passes, which
interrupts a blocked recv wherever it is (headers or body), and the elapsed
time is re-checked after each phase. Not covered: DNS resolution
(getaddrinfo is not interruptible), so manifests should name literal IPs or
already-resolvable hosts.
"""

from __future__ import annotations

import http.client
import json
import socket
import threading
import time
from dataclasses import dataclass
from urllib.parse import urlsplit

SESSION_ID_HEADER = "Mcp-Session-Id"
_LOOPBACK_HOSTS = frozenset({"127.0.0.1", "localhost", "::1"})


class MCPError(RuntimeError):
    """A JSON-RPC error, a tool-level isError result, or a transport
    failure. Messages are built only from fixed text, exception class names
    and validated integers (an HTTP status, a JSON-RPC code). Nothing the
    upstream sent -- an error message, a URLError reason -- is ever echoed,
    because a server can reflect the Authorization header into any field it
    controls."""


class BudgetExceeded(MCPError):
    """Raised by a budget guard before any bytes leave the process. The
    message is a guard-authored reason, never upstream text."""


# A hostile or broken upstream must not be able to make the harness buffer
# an unbounded body. The largest legitimate response is the inventory probe
# (a few hundred short facts).
MAX_RESPONSE_BYTES = 4 * 1024 * 1024


def validate_endpoint_url(url: str, allowed_origins: list[str]) -> None:
    """Raises ValueError if url is not exactly one of allowed_origins'
    schemes+hosts, carries userinfo (user:pass@host), or carries a query
    string (both are places credential-shaped material could hide).
    allowed_origins entries are "scheme://host[:port]" strings (no path).
    A bare loopback host (127.0.0.1/localhost/::1) is permitted over http
    only when it is itself listed in allowed_origins -- this function
    grants no implicit exception, so a manifest must say so explicitly
    even for local test fixtures.
    """
    parts = urlsplit(url)
    if parts.scheme not in ("http", "https"):
        raise ValueError(f"endpoint_url scheme must be http or https, got {parts.scheme!r}")
    if parts.scheme == "http" and parts.hostname not in _LOOPBACK_HOSTS:
        raise ValueError("endpoint_url must use https unless the host is a literal loopback address")
    if parts.username or parts.password:
        raise ValueError("endpoint_url must not carry userinfo (user:pass@host)")
    if parts.query:
        raise ValueError("endpoint_url must not carry a query string")
    origin = f"{parts.scheme}://{parts.netloc}"
    if origin not in allowed_origins:
        raise ValueError(f"endpoint_url origin {origin!r} is not in the manifest's allowed_origins {allowed_origins!r}")


def request_body(req_id: int, method: str, params: dict) -> bytes:
    return json.dumps({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params}).encode("utf-8")


def tool_call_params(name: str, arguments: dict) -> dict:
    return {"name": name, "arguments": arguments}


INITIALIZE_PARAMS = {
    "protocolVersion": "2025-06-18",
    "capabilities": {},
    "clientInfo": {"name": "t23.43-eval-embeddings", "version": "1"},
}


def decode_tool_payload(result: dict) -> dict:
    """The domain envelope inside a tools/call result. The hosted memory
    verbs return it as a JSON string in content[0].text (internal/server/
    memory's textResult) and the protocol also allows structuredContent;
    accept either, refuse anything else. Handing the raw wrapper to a caller
    would make every field lookup ('results', 'facts', 'search_degraded')
    silently miss."""
    structured = result.get("structuredContent") if isinstance(result, dict) else None
    if isinstance(structured, dict):
        return structured
    content = result.get("content") if isinstance(result, dict) else None
    if isinstance(content, list) and content and isinstance(content[0], dict) and content[0].get("type") == "text":
        try:
            payload = json.loads(content[0].get("text", ""))
        except (TypeError, json.JSONDecodeError) as e:
            raise MCPError("tool result text content is not JSON") from e
        if isinstance(payload, dict):
            return payload
    raise MCPError("tool result carries neither structuredContent nor a JSON object in text content")


@dataclass
class CallUsage:
    request_bytes: int
    response_bytes: int


class MCPClient:
    def __init__(
        self,
        endpoint_url: str,
        bearer_token: str,
        allowed_origins: list[str],
        timeout_s: float = 30.0,
        budget=None,
    ):
        """`budget`, when given, must offer precharge(request_bytes) and
        remaining_seconds(); see lib/budget.py. precharge runs on the exact
        serialized request before any counter moves or socket opens, so no
        request can start that the caps cannot cover."""
        validate_endpoint_url(endpoint_url, allowed_origins)
        self._budget = budget
        self.endpoint_url = endpoint_url
        self._bearer_token = bearer_token
        self.timeout_s = timeout_s
        self._session_id: str | None = None
        self._next_id = 1
        self.calls_made = 0
        self.total_request_bytes = 0
        self.total_response_bytes = 0

    def _post(self, method: str, params: dict) -> dict:
        req_id = self._next_id
        self._next_id += 1
        body = request_body(req_id, method, params)

        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": f"Bearer {self._bearer_token}",
        }
        if self._session_id is not None:
            headers[SESSION_ID_HEADER] = self._session_id

        deadline_s = self.timeout_s
        if self._budget is not None:
            self._budget.precharge(len(body))  # raises BudgetExceeded before anything is sent
            deadline_s = min(deadline_s, self._budget.remaining_seconds())
        self.calls_made += 1
        self.total_request_bytes += len(body)
        status, session_header, raw = self._exchange(body, headers, deadline_s)

        if 300 <= status < 400:
            raise MCPError(f"refused: HTTP {status} redirect -- redirects are never followed (credential-leak protection)")
        if status >= 400 or status < 200:
            raise MCPError(f"HTTP {status} from the hosted endpoint (body not logged)")
        if session_header:
            self._session_id = session_header

        try:
            envelope = json.loads(raw)
        except json.JSONDecodeError as e:
            raise MCPError("non-JSON response from the hosted endpoint") from e

        if not isinstance(envelope, dict):
            raise MCPError("non-object JSON-RPC response from the hosted endpoint")
        if envelope.get("error"):
            err = envelope["error"]
            code = err.get("code") if isinstance(err, dict) else None
            shown = code if isinstance(code, int) and not isinstance(code, bool) and -10**9 < code < 10**9 else "invalid"
            raise MCPError(f"JSON-RPC error code={shown} (message not logged)")
        result = envelope.get("result", {})
        if isinstance(result, dict) and result.get("isError"):
            raise MCPError(f"tool call reported isError=true (method={method})")
        return result

    def _exchange(self, body: bytes, headers: dict, deadline_s: float) -> tuple[int, str | None, bytes]:
        """One POST under a hard wall-clock deadline. Returns (status,
        session header, bounded body). Every failure is a fixed-text MCPError."""
        parts = urlsplit(self.endpoint_url)
        https = parts.scheme == "https"
        port = parts.port or (443 if https else 80)
        path = parts.path or "/"
        conn_cls = http.client.HTTPSConnection if https else http.client.HTTPConnection
        deadline = time.monotonic() + deadline_s
        conn = conn_cls(parts.hostname, port, timeout=deadline_s)
        fired = threading.Event()
        # http.client hands the socket to the response object (and clears
        # conn.sock) once headers arrive on a connection it will close, so the
        # watchdog keeps its own reference, taken right after connect().
        socks: list = []

        def abort() -> None:
            fired.set()
            for sock in socks:
                try:
                    sock.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                try:
                    sock.close()
                except OSError:
                    pass

        watchdog = threading.Timer(deadline_s, abort)
        watchdog.daemon = True
        watchdog.start()
        try:
            conn.connect()  # bounded by timeout=deadline_s; DNS is the documented exception
            socks.append(conn.sock)
            if fired.is_set() or time.monotonic() >= deadline:
                raise TimeoutError
            conn.request("POST", path, body=body, headers=headers)
            resp = conn.getresponse()
            raw = resp.read(MAX_RESPONSE_BYTES + 1)
            status = resp.status
            session_header = resp.getheader(SESSION_ID_HEADER)
        except (OSError, http.client.HTTPException) as e:
            if fired.is_set() or time.monotonic() >= deadline:
                raise MCPError(f"deadline exceeded: the request took longer than {deadline_s:.3f}s") from e
            # Class name only: an exception's text can carry server-influenced strings.
            raise MCPError(f"transport error calling the hosted endpoint: {type(e).__name__}") from e
        finally:
            watchdog.cancel()
            conn.close()
        if fired.is_set() or time.monotonic() >= deadline:
            raise MCPError(f"deadline exceeded: the request took longer than {deadline_s:.3f}s")
        if len(raw) > MAX_RESPONSE_BYTES:
            self.total_response_bytes += MAX_RESPONSE_BYTES
            raise MCPError(f"response exceeds {MAX_RESPONSE_BYTES} bytes; refused")
        self.total_response_bytes += len(raw)
        return status, session_header, raw

    def initialize(self) -> dict:
        return self._post("initialize", INITIALIZE_PARAMS)

    def call_tool(self, name: str, arguments: dict) -> dict:
        """Requires an active session (call initialize() first). This is
        deliberate, not an oversight: a caller enforcing a per-call budget
        must be able to charge every real HTTP request against that budget
        BEFORE it happens, including the handshake -- a call_tool that
        silently initializes on first use would let the very first budgeted
        call cost two real requests with only one accounted for.
        """
        if self._session_id is None:
            raise MCPError("call_tool: no active session -- call initialize() first (and budget for it)")
        return decode_tool_payload(self._post("tools/call", tool_call_params(name, arguments)))

    def usage(self) -> CallUsage:
        return CallUsage(request_bytes=self.total_request_bytes, response_bytes=self.total_response_bytes)
