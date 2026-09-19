"""Minimal MCP-over-HTTP JSON-RPC client for --live mode, speaking the
exact wire protocol internal/server/mcp implements (protocol.go's
{"jsonrpc":"2.0", ...} envelope, server.go's "initialize" / "tools/call"
dispatch, http.go's Mcp-Session-Id bootstrap). Stdlib-only (urllib): this is
a CLI harness, not a service, and shouldn't need a dependency just to speak
JSON-RPC over HTTP.

This client only ever calls the URL/credential named in an explicit,
frozen qualification manifest (see manifest.py) -- it never guesses an
endpoint or falls back to a default host, matching AGENTS.md's "never add
a public debug bypass" and the plan's "refuse production targets unless the
task explicitly authorizes them."
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass

SESSION_ID_HEADER = "Mcp-Session-Id"


class MCPError(RuntimeError):
    """A JSON-RPC error object, or a transport-level failure."""


@dataclass
class CallUsage:
    request_bytes: int
    response_bytes: int


class MCPClient:
    def __init__(self, endpoint_url: str, bearer_token: str, timeout_s: float = 30.0):
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
            with urllib.request.urlopen(request, timeout=self.timeout_s) as resp:
                raw = resp.read()
                session_header = resp.headers.get(SESSION_ID_HEADER)
                if session_header:
                    self._session_id = session_header
        except urllib.error.HTTPError as e:
            raw = e.read()
            self.total_response_bytes += len(raw)
            raise MCPError(f"HTTP {e.code} from {self.endpoint_url}: {raw[:500]!r}") from e
        except urllib.error.URLError as e:
            raise MCPError(f"transport error calling {self.endpoint_url}: {e.reason}") from e

        self.total_response_bytes += len(raw)
        try:
            envelope = json.loads(raw)
        except json.JSONDecodeError as e:
            raise MCPError(f"non-JSON response from {self.endpoint_url}") from e

        if envelope.get("error"):
            err = envelope["error"]
            raise MCPError(f"JSON-RPC error {err.get('code')}: {err.get('message')}")
        return envelope.get("result", {})

    def initialize(self) -> dict:
        return self._post("initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "t23.43-eval-embeddings", "version": "1"}})

    def call_tool(self, name: str, arguments: dict) -> dict:
        if self._session_id is None:
            self.initialize()
        return self._post("tools/call", {"name": name, "arguments": arguments})

    def usage(self) -> CallUsage:
        return CallUsage(request_bytes=self.total_request_bytes, response_bytes=self.total_response_bytes)
