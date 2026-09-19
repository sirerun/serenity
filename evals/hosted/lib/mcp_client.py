"""Minimal MCP-over-HTTP JSON-RPC client for --live mode, speaking the
exact wire protocol internal/server/mcp implements (protocol.go's
{"jsonrpc":"2.0", ...} envelope, server.go's "initialize" / "tools/call"
dispatch, http.go's Mcp-Session-Id bootstrap, tools.go's Result.IsError).
Stdlib-only (urllib): this is a CLI harness, not a service, and shouldn't
need a dependency just to speak JSON-RPC over HTTP.

This client only ever calls the URL/credential named in an explicit,
frozen qualification manifest (see manifest.py) -- it never guesses an
endpoint or falls back to a default host, matching AGENTS.md's "never add
a public debug bypass" and the plan's "refuse production targets unless the
task explicitly authorizes them."

Security: Python's urllib default opener follows HTTP redirects and
forwards every request header -- including Authorization -- to whatever
host the redirect names, even a different origin (confirmed empirically:
a 302 from server A to server B on a different port arrives at B carrying
A's bearer token). This client never uses the default opener. It builds
its own OpenerDirector with all redirect handling disabled, so any 3xx
response surfaces as an HTTPError instead of being followed -- a
credential is only ever sent to manifest.hosted_mcp.endpoint_url itself,
never to a location that URL's server names afterward.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from urllib.parse import urlsplit

SESSION_ID_HEADER = "Mcp-Session-Id"
_LOOPBACK_HOSTS = frozenset({"127.0.0.1", "localhost", "::1"})


class MCPError(RuntimeError):
    """A JSON-RPC error object, a tool-level isError result, or a
    transport-level failure. Never carries a raw provider response body or
    a credential value -- only a status code / error code and a short,
    literal, non-input-derived description."""


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuses every redirect: the response arrives at the caller as the
    original 3xx HTTPError instead of the credential silently following a
    Location header to a different origin."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


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


@dataclass
class CallUsage:
    request_bytes: int
    response_bytes: int


class MCPClient:
    def __init__(self, endpoint_url: str, bearer_token: str, allowed_origins: list[str], timeout_s: float = 30.0):
        validate_endpoint_url(endpoint_url, allowed_origins)
        self.endpoint_url = endpoint_url
        self._bearer_token = bearer_token
        self.timeout_s = timeout_s
        self._session_id: str | None = None
        self._next_id = 1
        self.calls_made = 0
        self.total_request_bytes = 0
        self.total_response_bytes = 0
        self._opener = urllib.request.build_opener(_NoRedirect())

    def _post(self, method: str, params: dict) -> dict:
        req_id = self._next_id
        self._next_id += 1
        body = json.dumps({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params}).encode("utf-8")

        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": f"Bearer {self._bearer_token}",
        }
        if self._session_id is not None:
            headers[SESSION_ID_HEADER] = self._session_id

        request = urllib.request.Request(self.endpoint_url, data=body, headers=headers, method="POST")
        self.calls_made += 1
        self.total_request_bytes += len(body)
        try:
            with self._opener.open(request, timeout=self.timeout_s) as resp:
                raw = resp.read()
                final_url = resp.geturl()
                session_header = resp.headers.get(SESSION_ID_HEADER)
                if session_header:
                    self._session_id = session_header
        except urllib.error.HTTPError as e:
            body_len = len(e.read())
            self.total_response_bytes += body_len
            if 300 <= e.code < 400:
                raise MCPError(
                    f"refused: HTTP {e.code} redirect from {self.endpoint_url} -- "
                    f"redirects are never followed (credential-leak protection)"
                ) from e
            raise MCPError(f"HTTP {e.code} from the hosted endpoint ({body_len} byte body, not logged)") from e
        except urllib.error.URLError as e:
            raise MCPError(f"transport error calling the hosted endpoint: {e.reason}") from e

        if final_url != self.endpoint_url:
            raise MCPError("refused: final response URL diverged from the configured endpoint_url")

        self.total_response_bytes += len(raw)
        try:
            envelope = json.loads(raw)
        except json.JSONDecodeError as e:
            raise MCPError("non-JSON response from the hosted endpoint") from e

        if envelope.get("error"):
            err = envelope["error"]
            raise MCPError(f"JSON-RPC error {err.get('code')}: {str(err.get('message'))[:200]}")
        result = envelope.get("result", {})
        if isinstance(result, dict) and result.get("isError"):
            raise MCPError(f"tool call reported isError=true (method={method})")
        return result

    def initialize(self) -> dict:
        return self._post(
            "initialize",
            {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "t23.43-eval-embeddings", "version": "1"}},
        )

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
        return self._post("tools/call", {"name": name, "arguments": arguments})

    def usage(self) -> CallUsage:
        return CallUsage(request_bytes=self.total_request_bytes, response_bytes=self.total_response_bytes)
