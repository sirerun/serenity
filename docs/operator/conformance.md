# Protocol conformance (`serenity protocol conformance`)

`serenity protocol conformance --target <url>` replays the frozen
transcripts under `testdata/conformance/` (T4.13) against a live server,
comparing each recorded request/response pair to what the target actually
returns and reporting pass/fail per case with a diff on mismatch. It is
this repo's own external, wire-level conformance runner for all three
protocols RFC 0001 §8 defines (MEMORY_VERBS v1, DISPOSITION v1, DIRECTION
v1) — the counterpart to `gbrain protocol conformance` (which only knows
MEMORY_VERBS) and to each protocol package's own in-process test suite.

## Usage

```
serenity protocol conformance --target http://127.0.0.1:8443 \
  --token "$SERENITY_API_TOKEN"
```

Flags:

- `--target` (required) — base URL of a live server. MEMORY_VERBS is
  spoken over MCP Streamable HTTP at `<target>/mcp` (RFC 0001 §14, the
  same endpoint `serenity serve --http` exposes); DISPOSITION and
  DIRECTION are spoken directly against `<target>` (each transcript's own
  recorded path already carries the `/disposition` or `/direction`
  prefix).
- `--token` — daemon bearer token; defaults to `$SERENITY_API_TOKEN`.
- `--protocol` — restrict the run to one or more of `memory_verbs`,
  `disposition`, `direction` (default: all three, repeatable flag).
- `--fixtures` — override the `testdata/conformance/` directory (default:
  resolved from this build's own source tree).
- `--json` — machine-readable report instead of the text summary.

Exit status is nonzero when any case fails.

## What "pass" means

A case's response is compared to the frozen transcript field by field.
Dynamic-shaped values — an opaque hex id (`internal/disposition`'s
`crypto/rand`-generated item ids among them) or an RFC 3339 timestamp — are
allowed to differ in *value* as long as both sides have that *shape*;
everything else (status codes, enum values, echoed request fields, array
lengths and ordering) must match exactly. See
`internal/conformance.CompareBodies` for the exact rule and
`testdata/conformance/README.md` for why these fields are dynamic in the
first place.

## Disclosed gaps

- **`list_pending`/`dispose` need a matching target.** Their fixture items
  are seeded by an internal `Store.Create` call the generator makes
  *before* its transcript's own steps run (see
  `testdata/conformance/disposition/gen_transcripts.go`) — that seeding
  is not itself part of the frozen transcript, and item ids are
  `crypto/rand`, never reproducible outside that one generator run.
  Replayed against a `--target` that was not independently seeded with
  matching items, these two operations' happy-path cases legitimately
  fail (the target has no such item) — this command reports that plainly
  rather than skipping it silently. `go test ./internal/conformance`
  remains this pair's byte-exact authority, since only a test that boots
  and seeds its own server can reproduce matching state.
- **`subscribe`'s SSE mode has no transcript.** Only the long-poll
  fallback envelope is recorded; an open server-sent-events stream isn't a
  single request/response pair, so it doesn't fit this transcript shape.
  `internal/server/disposition`'s own
  `TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents` remains SSE's
  authoritative coverage.
- **`memory_verbs` cases needing seeded state or schema validation.** A
  handful of pinned cases are flagged `requiresSeededEntity` or
  `requiresSynthesizeFlag` in `cases.json` — like `list_pending`/`dispose`
  above, this command does not arrange that state on an arbitrary
  `--target`, so those cases' results depend on what the target already
  has. `internal/server/memory`'s own `TestMemoryV1AllPinnedCases` (T4.20)
  is the byte-exact, schema-validated authority for the full pinned set.

## See also

- [MCP: stdio and Streamable HTTP](mcp.md) — the transport MEMORY_VERBS
  conformance runs over.
- `testdata/conformance/README.md` — fixture format, checksum pinning, and
  the dynamic-field disclosure this command implements.
- `docs/protocol/MEMORY_VERBS_v1.md`, `DISPOSITION_v1.md`,
  `DIRECTION_v1.md` — each protocol's own conformance section.
