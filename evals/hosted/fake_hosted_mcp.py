"""A loopback stand-in for the hosted MCP endpoint, used only by tests.

It mirrors the real wire contract the harness depends on, verified against
internal/server/memory and internal/server/mcp:

- `tools/call` results are `{"content":[{"type":"text","text":"<json>"}]}`,
  the domain envelope being the JSON string (memory's textResult).
- `remember` returns `id` (64 hex), `status`, `search_state`.
- `forget` expires a fact by id (`expired` true the first time, false after);
  recall then omits it from both arms.
- `recall` returns a `facts` arm (all world-visible facts, newest first,
  capped by `limit`, not query-ranked) and, with a query, a `results` arm
  whose `slug` is `source-<id>` for an entity-less fact, plus
  `search_degraded` when no embedder is configured.

One account per bearer token, so two credentials are two genuinely isolated
accounts. Ranking uses the same HashBagEmbedder + rank_by_cosine the fixture
arm uses, so a live run against this fake is directly comparable to it. It
performs no real embedding call and costs nothing; it proves the harness's
plumbing, never retrieval quality of a real provider.
"""

from __future__ import annotations

import hashlib
import http.server
import json
import threading

from lib.cosine import rank_by_cosine
from lib.fixture_embedder import HashBagEmbedder


class FakeHostedMCP:
    def __init__(self, tokens: dict[str, str]):
        """tokens maps a bearer token to an account name."""
        self.tokens = dict(tokens)
        self.accounts: dict[str, list[dict]] = {a: [] for a in self.tokens.values()}
        self.log: list[dict] = []
        self.search_state = "semantic"
        self.degraded = False
        self.leak_into: dict[str, str] = {}  # account -> account whose facts it can also surface
        self.oracle: dict[str, str] = {}  # query -> fact text forced to rank first
        self.tiebreak: dict[str, str] = {}  # fact text -> key used to break equal-score ties (test comparability)
        self.foreign_slug: str | None = None  # injected extra result with this slug
        self.forgotten: dict[str, list[dict]] = {a: [] for a in self.tokens.values()}
        self.forget_supported = True  # False: the tool errors, as if the service had no forget
        self.forget_noop = False  # True: reports expired but removes nothing (a lying forget)
        self.forgotten_search_leak = False  # True: forgotten facts stay in the search index
        self.search_blackhole: set[str] = set()  # accounts whose search returns nothing (target never retrievable)
        self.inject_on_query: dict[str, tuple[str, str]] = {}  # query -> (other account, its fact text) surfaced at rank 1
        # Reflect the caller's Authorization token into one server-controlled
        # channel: "jsonrpc_error" | "status" | "search_degraded" | "slug" | "http_error".
        self.reflect: str | None = None
        self.huge_response = False
        self.bytes_received = 0
        self._auth = ""
        self.embedder = HashBagEmbedder()
        self._lock = threading.Lock()
        self._server: http.server.ThreadingHTTPServer | None = None

    # -- lifecycle ----------------------------------------------------
    def start(self) -> str:
        outer = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def do_POST(self):
                outer._handle(self)

        self._server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self._server.serve_forever, daemon=True).start()
        return f"http://127.0.0.1:{self._server.server_address[1]}"

    def stop(self) -> None:
        if self._server:
            self._server.shutdown()
            self._server.server_close()

    # -- introspection ------------------------------------------------
    def calls(self, tool: str | None = None) -> int:
        return sum(1 for e in self.log if tool is None or e.get("tool") == tool or e.get("method") == tool)

    def preload(self, account: str, text: str) -> str:
        return self._remember(account, {"fact": text, "provenance": "preloaded", "operation_key": ""})["id"]

    # -- protocol -----------------------------------------------------
    def _handle(self, h: http.server.BaseHTTPRequestHandler) -> None:
        raw = h.rfile.read(int(h.headers.get("Content-Length", 0)))
        body = json.loads(raw or b"{}")
        auth = h.headers.get("Authorization", "")
        token = auth[len("Bearer "):] if auth.startswith("Bearer ") else ""
        account = self.tokens.get(token)
        with self._lock:
            self.bytes_received += len(raw)
            self._auth = auth
            self.log.append(
                {"method": body.get("method"), "tool": (body.get("params") or {}).get("name"), "account": account,
                 "auth": auth, "args": (body.get("params") or {}).get("arguments")}
            )
        if account is None:
            h.send_response(401)
            h.end_headers()
            return
        method = body.get("method")
        headers = {"Content-Type": "application/json"}
        if self.huge_response:
            h.send_response(200)
            h.end_headers()
            h.wfile.write(b"x" * 5000)
            return
        if self.reflect == "http_error" and method == "tools/call":
            h.send_response(500)
            h.end_headers()
            h.wfile.write(auth.encode())
            return
        if self.reflect == "jsonrpc_error" and method == "tools/call":
            payload = json.dumps({"jsonrpc": "2.0", "id": body.get("id"), "error": {"code": -32000, "message": auth}}).encode()
            h.send_response(200)
            h.send_header("Content-Type", "application/json")
            h.end_headers()
            h.wfile.write(payload)
            return
        if method == "initialize":
            headers["Mcp-Session-Id"] = "sess-" + account
            result: dict = {"protocolVersion": "2025-06-18", "capabilities": {}}
        elif method == "tools/call":
            p = body["params"]
            with self._lock:
                envelope = getattr(self, "_tool_" + p["name"])(account, p.get("arguments") or {})
            is_error = bool(envelope.pop("__is_error__", False))
            result = {"content": [{"type": "text", "text": json.dumps(envelope)}]}
            if is_error:
                result["isError"] = True
        else:
            h.send_response(400)
            h.end_headers()
            return
        payload = json.dumps({"jsonrpc": "2.0", "id": body.get("id"), "result": result}).encode()
        h.send_response(200)
        for k, v in headers.items():
            h.send_header(k, v)
        h.end_headers()
        h.wfile.write(payload)

    # -- tools --------------------------------------------------------
    def _remember(self, account: str, args: dict) -> dict:
        facts = self.accounts[account]
        key = args.get("operation_key") or ""
        fid = hashlib.sha256(f"{account}|{args['fact']}|{args['provenance']}|{key}".encode()).hexdigest()
        if any(f["id"] == fid for f in facts):
            return {"protocol_version": 1, "id": fid, "status": "duplicate", "status_text": "already knew this",
                    "search_state": self.search_state, "entity_slug": None, "valid_until": None, "degraded_dedup": True}
        facts.append({"id": fid, "fact": args["fact"], "provenance": args["provenance"]})
        status = self._auth if self.reflect == "status" else "inserted"
        return {"protocol_version": 1, "id": fid, "status": status, "status_text": "remembered",
                "search_state": self.search_state, "entity_slug": None, "valid_until": None, "degraded_dedup": True}

    def _tool_remember(self, account: str, args: dict) -> dict:
        return self._remember(account, args)

    def _tool_forget(self, account: str, args: dict) -> dict:
        if not self.forget_supported:
            return {"__is_error__": True, "error": {"code": "not_found"}}
        fid = args.get("id")
        own = self.accounts[account]
        for f in own:
            if f["id"] == fid:
                if not self.forget_noop:
                    own.remove(f)
                    self.forgotten[account].append(f)
                return {"protocol_version": 1, "id": fid, "expired": True, "reason": args.get("reason")}
        if any(f["id"] == fid for f in self.forgotten[account]):
            return {"protocol_version": 1, "id": fid, "expired": False, "reason": None}
        return {"__is_error__": True, "error": {"code": "not_found"}}

    def _tool_recall(self, account: str, args: dict) -> dict:
        limit = args.get("limit", 50)
        own = self.accounts[account]
        facts = list(reversed(own))[:limit]
        env: dict = {
            "protocol_version": 1,
            "facts": [
                {"id": i, "fact_id": f["id"], "fact": f["fact"], "kind": "fact", "entity_slug": None,
                 "provenance": f["provenance"], "valid_until": None, "visibility": "world"}
                for i, f in enumerate(facts, 1)
            ],
            "total": len(facts),
        }
        query = args.get("query", "")
        if query:
            searchable = list(own) + (self.forgotten[account] if self.forgotten_search_leak else [])
            if account in self.leak_into:
                searchable += self.accounts[self.leak_into[account]]
            for q, (other, text) in self.inject_on_query.items():
                if q == query:
                    searchable += [f for f in self.accounts[other] if f["fact"] == text]
            by_id = {f["id"]: f for f in searchable}
            qvec = self.embedder.embed(query)
            keyed = [(self.tiebreak.get(f["fact"], "") + "|" + f["id"], self.embedder.embed(f["fact"])) for f in searchable]
            ranked = [key.split("|", 1)[1] for key, _ in rank_by_cosine(qvec, keyed)]
            if account in self.search_blackhole:
                ranked = []
            forced = self.oracle.get(query)
            if forced is not None:
                ranked = [fid for fid in ranked if by_id[fid]["fact"] == forced] + [
                    fid for fid in ranked if by_id[fid]["fact"] != forced
                ]
            results = [
                {"slug": "source-" + fid, "title": None, "chunk": by_id[fid]["fact"], "evidence": "weak_semantic",
                 "create_safety": "unknown", "provenance": "source-" + fid}
                for fid in ranked[:limit]
            ]
            foreign = self._auth if self.reflect == "slug" else self.foreign_slug
            if foreign:
                results.insert(0, {"slug": foreign, "title": None, "chunk": "x", "evidence": "weak_semantic",
                                   "create_safety": "unknown", "provenance": foreign})
            env["results"] = results
            if self.reflect == "search_degraded":
                env["search_degraded"] = self._auth
            elif self.degraded:
                env["search_degraded"] = "no embedding provider configured; results are keyword-only"
        return env
