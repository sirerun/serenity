# MCP: stdio and Streamable HTTP

Serenity serves MCP over two transports, chosen with mutually exclusive
flags on one command: `serenity serve --stdio` or `serenity serve --http`.
Both share the same tool registry (MEMORY_VERBS v1's five verbs: `recall`,
`remember`, `entity`, `synthesize`, `forget`) and the same protocol dispatch
logic (`internal/server/mcp`), so a client sees identical tool behavior
regardless of which transport it connects over.

## stdio (`serve --stdio`)

Run `serenity serve --stdio` as an MCP client subprocess. The command reads
one UTF-8 JSON-RPC message per line and writes only protocol messages to stdout.
Command failures go to stderr. This transport needs no network listener,
token, or background scheduler; it opens the writer queue and derived index
only when `-C` names a real brain repo (pointed elsewhere, it still serves
the bare protocol -- initialize, ping, shutdown -- with an empty tool
registry, noted on stderr).

The transport implements the [MCP 2025-11-25 lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
and [stdio framing](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports).
Send `initialize` with `protocolVersion`, `capabilities`, and `clientInfo`
(name and version), then `notifications/initialized` before using tools.
The server negotiates `2025-11-25`, including when the requested version is
unsupported. A client that cannot use this version should disconnect.
`ping` is available throughout the handshake.

Against a real brain repo, `tools/list` reports all five MEMORY_VERBS v1
verbs (`internal/server/memory`); see
[MEMORY_VERBS v1 implementation](../../internal/server/memory/README.md)
for their request/response shapes and persistence model. Internal consumers
can construct an immutable registry with `mcp.New(version, tools)` directly.
Each tool supplies an object input schema and a handler accepting context
and raw JSON arguments; the transport validates arguments against the
compiled schema before calling the handler. This does not start another
daemon or grant a private path to the brain; registered tools call the same
domain services as the CLI.

Unknown tools return JSON-RPC `-32602`; unknown methods return `-32601`.
Malformed JSON returns `-32700`; invalid envelopes and batches return `-32600`.
Invalid request parameters return `-32602`. A well-formed arguments object that
fails the tool schema produces a tool result with `isError: true`, as do handler
errors and invalid handler results. Responses preserve string IDs and arbitrarily
large integer IDs exactly. Valid notifications never receive a response.

Each input frame is limited to 1 MiB and a session allows at most 32 concurrent
tool calls. Excess calls return `-32000` without blocking cancellation or ping.
An oversized frame or duplicate in-flight ID closes the session. Numeric and
string IDs are distinct. `notifications/cancelled` cancels a matching active
call and suppresses its response. The handler must honor its context.

EOF, SIGINT, and SIGTERM cancel active work and join session workers. Closeable
input and output are owned by the session and closed on shutdown, including to
interrupt blocked pipe I/O. Embedded callers supplying other output writers must
ensure writes return promptly; custom input Close must unblock Read. Output
failures terminate the session and propagate to the caller. There is no MCP
`shutdown` method; clients close stdin to end the session.

## Streamable HTTP (`serve --http`)

Run `serenity serve --http` to serve the same tool registry over MCP's
[Streamable HTTP transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
at a single endpoint, `/mcp`. It reuses
[the daemon's existing HTTP transport](server.md) unchanged: loopback by
default, a bearer token required on `/mcp` exactly as on every other route,
explicit `allow_lan`/mTLS config to expose it beyond loopback. `serenity
init` must have already minted the daemon token (`serenity doctor` reports
whether it's present); `--http` fails fast if it hasn't.

```
$ serenity -C ~/brain serve --http
serenity MCP HTTP listening on http://127.0.0.1:54217/mcp
```

The printed line names the bound host and port only -- the daemon token
never appears in this or any other command output; it lives only in the OS
keychain and the `Authorization` header a client sends.

### Scope

This transport implements MEMORY_VERBS v1 only. The `--http` flag does not
serve DISPOSITION or DIRECTION over HTTP -- their own live daemon assembly
is separate, still-outstanding work, tracked in the E4 plan.

### Upstream conformance

Every push and PR runs the `gbrain-protocol-conformance` CI job
(`.github/workflows/ci.yml`): gbrain's own `protocol conformance` certifier
(github.com/dndungu/gbrain, pinned commit
`d35c9c9e441e6cfc86dd5e84b0b168c6b18ee775`) against a live `serve --http`
endpoint on a throwaway fixture brain, over gbrain's real TypeScript SDK
`StreamableHTTPClientTransport` client -- not a Go-side simulation of it.
`internal/cli/gbrain_conformance_test.go`'s `TestGbrainProtocolConformance`
drives it: `serve --http` runs in-process against a real TCP loopback
listener (the same harness `TestServeHTTPEndToEnd` uses), so gbrain's CLI
is the only genuine external process. The run passes gbrain's SHAPE,
CONTRACT BEHAVIOR, and ROUND-TRIP case categories with zero failures; the
two entity-hit cases skip honestly (`requiresSeededEntity`) since this
transport advertises no gbrain-specific `put_page` tool to seed an entity
page with. The run's marker-suffixed synthetic data
(`people/conformance-<marker>`, plus any `remember`ed facts scoped to that
entity) lives only in the run's own throwaway fixture brain, discarded with
the test's temp directory when the job ends -- there is nothing durable
left to clean up separately.

### Session lifecycle

The first request on a new connection must be `initialize`, with no
`Mcp-Session-Id` header. A successful response carries a fresh
`Mcp-Session-Id` header (a 128-bit random value) and an `MCP-Protocol-Version:
2025-11-25` header; a failed `initialize` (bad params) hands out no session
-- retry the same way. Every request after that must carry both headers
back: `Mcp-Session-Id` naming the session, `MCP-Protocol-Version` matching
what the server negotiated. A request with neither header, on a connection
that hasn't initialized yet, gets `400`; an unrecognized or expired session
id gets `404`; a missing or mismatched protocol version header on an
otherwise-valid session gets `400` -- none of these reach a tool.

`DELETE` with `Mcp-Session-Id` ends a session immediately, freeing its slot.
A session nobody touches (no POST or DELETE naming it) for 30 minutes is
evicted on its own. Up to 64 sessions and, within each, up to 32 concurrent
tool calls are tracked at once -- the same per-session ceiling stdio
enforces, `-32000` beyond it. Each request body is capped at 1 MiB, the same
bound stdio's own per-frame limit uses.

### Responses

Every POST gets exactly one JSON response (`Content-Type: application/json`)
or, for a notification, `202 Accepted` with no body -- this transport never
upgrades a response to a `text/event-stream`, and there is no stream to
resume after a drop. `GET /mcp` is `405`; only `POST` and `DELETE` are
implemented.

A request carrying an `Origin` header is refused with `403` before it
reaches a session or a tool. No legitimate MCP client -- this repo's own
CLI, an external agent host, gbrain's `StreamableHTTPClientTransport` --
runs as a browser page and none sets `Origin`; only a hostile web page
acting through a victim's browser would (the DNS-rebinding-shaped attack
the MCP transport spec's own Origin-validation requirement defends
against). There is no allowlist to configure because there is no browser-
based client to allow yet.

### Cancellation and shutdown

A tool call keeps running once accepted, independent of the HTTP request
that started it: a client disconnecting mid-call, or its request body
hitting EOF, does not stop the call -- its result is simply undeliverable on
that now-dead connection. Only an explicit `notifications/cancelled`,
naming the request id on the same session, cancels a call early and
suppresses its (now nonexistent) response -- identical semantics to stdio's
own cancellation. `DELETE`ing a session likewise never cancels a call
already running under it.

SIGINT and SIGTERM stop the listener from accepting new connections, then
cancel every still-running tool call and wait for each one to actually
return before the writer queue flushes and the derived index closes -- the
same shutdown order stdio's own `Serve` guarantees for a single connection,
extended here across every concurrent HTTP session.

### Three different "protocol version" fields

Three separate values carry the word "version" across this stack; none of
them is interchangeable with another:

- The JSON-RPC envelope's own `"jsonrpc":"2.0"` field -- fixed, never
  negotiated.
- MCP's own transport-level protocol version, negotiated during
  `initialize` and carried afterward in the `MCP-Protocol-Version` HTTP
  header (`2025-11-25`).
- MEMORY_VERBS v1's own domain `protocol_version` integer (currently `1`),
  a field inside a tool's JSON *content* (see
  [MEMORY_VERBS v1 implementation](../../internal/server/memory/README.md)),
  unrelated to either of the above.

### Verifying

```
curl -i -X POST http://127.0.0.1:<port>/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
# 401 with no Authorization header; 200 with -H "Authorization: Bearer <token>",
# carrying Mcp-Session-Id and MCP-Protocol-Version response headers.

curl -i http://127.0.0.1:<port>/mcp
# 405 -- GET is not implemented.
```
