# HTTP/MCP queue simulation receipt (not writer contention)

Run on 2026-09-12 against `799d6747c7bf90d7590ed1470ce43b30e257ae02` in an
isolated offload worktree, with the required build lease held only for the
run. The throwaway harness used `httptest.NewServer` around the real
`internal/server/mcp.HTTPHandler` and a real `net/http.Client`.

Command:

```text
/usr/bin/time -l go run ./.claude/scratch/http-bench/main.go
```

It attempted 100 MCP session initializations, then issued 32 concurrent
`tools/call` requests from each accepted session. The synthetic `remember`
tool held a shared mutex for 2 ms to simulate queueing behind serialized work.
This is a transport/queue simulation, not a test of Serenity's real writer,
repository renders, or SQLite contention.

| measure | result |
|---|---:|
| session attempts | 100 |
| sessions accepted / rejected | 64 / 36 (HTTP 200 / 503) |
| concurrent calls | 2,048 |
| call statuses | 2,048 HTTP 200, 0 errors |
| handler peak active writers | 1 |
| call p50 / p95 / p99 / max | 2,540 / 4,848 / 5,047 / 5,099 ms |
| process peak RSS | 164,446,208 bytes |
| wall time | 5.65 s |

This demonstrates the existing per-process session guard and the latency cost
of simulated serialization under concurrent load. It is not a sustainable
throughput or hosted capacity claim: the tool is synthetic, the server is
in-process, and
the run excludes real repository renders, SQLite contention, embedding calls,
network proxies, provisioning, CPU saturation, and recovery. Provider
economics and the hosted 1/10/100-brain acceptance measurements remain open.
