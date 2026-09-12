# Serenity scalability plan

Updated 2026-09-12 from `origin/main` at
`799d6747c7bf90d7590ed1470ce43b30e257ae02`. This is a bounded design and
measurement plan for a possible hosted-user surge. It contains no capacity
claim and makes no infrastructure or provider change.

## What the current code proves

| Surface | Evidence | Scaling implication |
|---|---|---|
| HTTP listener | `internal/server/server.go:170-172` constructs `http.Server{Handler: s.mux}` without explicit `ReadHeaderTimeout`, read, idle, or header-size settings. Go supplies a default maximum header size when `MaxHeaderBytes` is zero; the missing explicit settings still leave slow-client and idle-connection policy implicit. | Slow clients can occupy connections and memory during a surge. A write timeout needs care because tool calls may be long-running. |
| MCP sessions | `internal/server/mcp/http.go:46-56` caps one handler at 64 sessions and evicts idle sessions after 30 minutes. | This is a per-process session cap, not a service-wide capacity limit. |
| MCP calls | `internal/server/mcp/server.go:14-16,215-231` caps each session at 32 in-flight calls; `HTTPHandler` starts one goroutine per accepted pending call. | The configured guards have a nominal product of 64 × 32 = 2,048 calls per handler, subject to session eviction and call lifetime; this is not a hard process capacity or measured sustainable throughput. There is no global semaphore or queue. |
| Canonical writes | `internal/writer/queue.go:34-40,81-96` drains all jobs for a brain through one goroutine and serializes complete renders/publication with `runMu`. | Per-brain writes are intentionally serialized. Surge handling needs admission/backpressure and a clear single-owner rule; adding writers would risk canonical corruption. |
| SQLite index | `internal/index/sqlite.go:124-125` opens WAL SQLite with a 5-second busy timeout; `database/sql` pool limits are not set. | Concurrent readers/writers can contend unpredictably; lock waits and connection count are unmeasured. |
| Vector search | `internal/index/vectors.go:96-140` reads every vector for a model, decodes and scores all vectors, sorts them, then hydrates hits with one query each (`:142-152`). | Search is O(V log V) per query with an N+1 hydration pattern. This is the clearest data-size bottleneck. The source comment in `internal/index/sqlite.go:23-25` already identifies Postgres+pgvector as the scale profile. |

The bounded synthetic receipt in
[`scalability-benchmark-20260912.md`](scalability-benchmark-20260912.md)
measured the existing SQLite retrieval path at 1/10/100 indexes. It does not
measure hosted provisioning, HTTP load, writer contention, real embedding
latency or cost, or per-brain RSS, so it is a baseline rather than a capacity
claim. Existing green CI proves correctness for the tested suite, not surge
capacity.

The companion
[`scalability-http-benchmark-20260912.md`](scalability-http-benchmark-20260912.md)
exercised the real MCP HTTP handler with 100 session attempts and 2,048
concurrent calls. The existing 64-session guard rejected 36 attempts, while a
2 ms serialized synthetic writer produced p95 call latency of 4.85 seconds.
This is evidence for prioritizing admission control and writer queue metrics,
not a hosted throughput limit.

## Priority order

1. **Measure before changing limits (T23.3).** Run the existing hosted spike
   at 1, 10, and 100 brains with fixed synthetic workloads. Record request
   rate, p50/p95/p99 latency, active calls, writer wait, SQLite busy errors,
   vector count/search time, embedding latency, RSS, CPU, disk growth, and
   recovery time. Repeat at the planned Free/Builder/Scale limits. Publish
   ceilings and the first failing resource in `hosted-economics.md`.
2. **Add demand control at the hosted boundary.** Introduce a global,
   configurable in-flight-call budget and per-account admission quotas. Return
   typed overload responses with retry guidance; keep the existing per-session
   32-call protocol guard. Add queue depth and rejection metrics. Verify with
   a deterministic overload test and a two-account isolation test.
3. **Harden connection resource use.** Configure `ReadHeaderTimeout`, a
   bounded `IdleTimeout`, and `MaxHeaderBytes` on the HTTP server. Do not add a
   blanket `WriteTimeout` until long-tool behavior is measured; cancellation
   and shutdown semantics must remain intact. Add slow-header and idle-client
   tests.
4. **Keep one writer owner per brain.** The hosted runtime pool must ensure a
   brain is open for writes in one process at a time, and deployment must not
   route the same brain to two writable processes. Expose writer queue wait and
   drain duration before considering batching or sharding. Never parallelize
   canonical writes as a first response.
5. **Replace full-scan vector search at the measured threshold.** Keep the
   `Engine` interface and SQLite profile for small/self-hosted brains. Add a
   Postgres+pgvector or ANN-backed hosted profile only after the benchmark
   identifies a vector-count/latency threshold. Batch hit hydration and retain
   the model-pin invariant. Verify exact-result parity on a fixed corpus before
   switching a hosted tier.
6. **Bound SQLite pools and lock behavior.** After measurement, set explicit
   connection limits and instrument busy/timeout errors. Keep WAL and the
   existing 5-second busy timeout unless evidence supports a change. A pool
   setting without a workload test is not a capacity fix.

## Capacity policy

Until T23.3 produces measurements, the only safe capacity statement is that
the current implementation has protocol guards of 64 sessions and 32 calls
per session per process. Those constants must not be presented as supported
user or request capacity. A surge policy should shed work before embedding or
canonical-write queues exhaust memory, preserve per-account isolation, and
surface an observable retryable overload state.

## Reversible implementation slices

- **S1:** metrics and benchmark harness; no production behavior change.
- **S2:** hosted global admission semaphore and overload response, guarded by
  configuration and disabled for the self-hosted default.
- **S3:** HTTP header/idle limits with focused transport tests.
- **S4:** measured SQLite pool limits and batched vector-hit hydration.
- **S5:** hosted vector backend experiment behind the existing `Engine`
  interface; migration requires exact-result and recovery evidence.

S1 must precede S2–S5. No cloud resources, replicas, paid provider tier, or
database migration should be introduced until the measured bottleneck and
rollback path are recorded in the hosted plan.
