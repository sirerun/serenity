# MCP over standard input and output

Run `serenity serve --stdio` as an MCP client subprocess. The command reads
one UTF-8 JSON-RPC message per line and writes only protocol messages to stdout.
Command failures go to stderr. This transport requires no brain initialization,
network listener, token, background scheduler, or writer queue.

The transport implements the [MCP 2025-11-25 lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
and [stdio framing](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports).
Send `initialize` with `protocolVersion`, `capabilities`, and `clientInfo`
(name and version), then `notifications/initialized` before using tools.
The server negotiates `2025-11-25`, including when the requested version is
unsupported. A client that cannot use this version should disconnect.
`ping` is available throughout the handshake.

The production tool registry is currently empty: `tools/list` returns
`{"tools":[]}`. Memory and direction tool registration are separate tasks;
this command alone does not provide persistent agent memory. Internal consumers
can construct an immutable registry with `mcp.New(version, tools)`. Each tool
supplies an object input schema and a handler accepting context and raw JSON
arguments. The transport validates arguments against the compiled schema before
calling the handler. This does not start another daemon or grant a private path
to the brain; registered tools must call the same domain services as the CLI.

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
