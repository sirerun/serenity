# T23.3 bounded benchmark receipt

Run on 2026-09-12 against final T23.1 main `799d6747c7bf90d7590ed1470ce43b30e257ae02`
on Darwin arm64 with Go 1.27.1. The command ran in an isolated offload
worktree with the required build lease held for the run and released
immediately afterward.

Command:

```text
/usr/bin/time -l go run ./.claude/scratch/scale-bench/main.go
```

The throwaway harness opened 1, 10, and 100 independent SQLite indexes, wrote
100 synthetic 64-dimensional vectors per index, and issued 20 exact cosine
searches per index. It measures retrieval and local database setup only. It
does not exercise hosted provisioning, HTTP/MCP admission, writer queues, real
embedding calls, CPU saturation, or recovery after interruption.

| brains | open ms | populate ms | queries | p50 ms | p95 ms | max ms | database MiB |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 7.36 | 24.61 | 20 | 0.324 | 0.364 | 0.770 | 0.13 |
| 10 | 22.36 | 264.78 | 200 | 0.184 | 0.260 | 0.461 | 1.34 |
| 100 | 155.24 | 1748.41 | 2,000 | 0.187 | 0.293 | 0.585 | 13.46 |

The process peak resident set reported by `/usr/bin/time -l` for the complete
1/10/100 sequence was 142,475,264 bytes; this is a combined benchmark-process
measurement, not per-brain RSS. The result is a useful baseline for the
existing exact-scan SQLite path, but it is not a hosted capacity ceiling and
does not satisfy the full economics acceptance criteria. Real embedding cost,
hosted RSS/CPU, provisioning time, HTTP load, writer contention, and recovery
remain open measurements before any plan-limit claim.
