# Hosted journal transport bound evidence

Baseline: main `6ba1fb184db249a347d3b80f8e3995fcbec9eacf`, including PR368. Execution: isolated Linux ARM64 DGX task, Go module-selected toolchain, task-owned caches and temporary directory, GOMAXPROCS=2 and package parallelism 1. No production or provider mutation.

The S3 adapter previously used unbounded `io.ReadAll` before the observer applied its 64-KiB object limit. The correction reads no more than 65,537 bytes, rejects oversized objects without returning partial data, and closes the response. Header length is not trusted. Writes above 65,536 bytes are rejected before calling S3 so the adapter cannot create objects it cannot read. Exactly 65,536 bytes remain allowed.

Compatibility: the 64-KiB ceiling already applies to complete journal observation, but broad S3 `Get`/`ReadThrough` paths previously accepted larger objects. Those now fail closed. No data is truncated or rewritten; an existing oversized object requires operator reconciliation before startup/recovery. This is not a live object inventory or a global subject-ID size contract. Filesystem fake behavior and startup/recovery authority are unchanged.

## Regression and validation

- New `TestS3GetObjectBound` and `TestS3PutObjectBound` failed against unchanged baseline: oversized responses exposed data; unknown/underreported/negative lengths read 524,288 bytes; oversized put called the SDK.
- `go test -p 1 -race ./internal/hosted/deletion -count=1`: PASS after correction, including existing ambiguous-write readback and conflict cases.
- `go vet -p 1 ./internal/hosted/deletion`: PASS.
- `golangci-lint run --concurrency 2 ./internal/hosted/deletion`: PASS, zero issues.
- `git diff --check`: PASS.
- Wazi plan schema, owning semantic validator and actual Markdown reader: PASS, contract0.0.1, ten tasks. Plan status and exact digest are revalidated after review changes.

The package race test is a local synthetic source check, not AWS integration or hosted CI. Full hosted startup, release signing and deployment remain unavailable. Actions on baseline PR368 failed before execution with an account-billing annotation.

## Source input digests

| File | SHA-256 |
| --- | --- |
| `internal/hosted/deletion/store.go` | `7f62e0e066bcc1d2aab9a10dded665b6b3cf7350556b5a969e41c9b8c7c44010` |
| `internal/hosted/deletion/store_bounds_test.go` | `a243b7c1c511fc2410a22bcda1a7c29e3daef04b5c7c55d7ce395a87a24f23c0` |

Independent exact-head source review is recorded in `hosted-unblock-review-2026-10-10.md`; final-head and landed provenance remain separate delivery gates.
