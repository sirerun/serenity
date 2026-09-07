# Protocol conformance (`serenity protocol conformance`)

`serenity protocol conformance --target <url>` replays the frozen
transcripts under `testdata/conformance/` (T4.13) against a live server,
comparing each recorded request/response pair to what the target actually
returns and reporting pass, fail, or skip per case with a diff on mismatch.
It is this repo's own external, wire-level conformance runner for all three
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

Exit status is nonzero when any case fails; a skipped case never fails the
run (see "Disclosed gaps" below for what earns a case "skip").

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

- **A fixed set of `list_pending`/`dispose`/`brief`/`check_plan` cases need
  a matching target, so a mismatch there reports skip, not fail.** Their
  expected response depends on server-side state a generator script seeded
  *before* its transcript's own steps ran (see
  `testdata/conformance/{disposition,direction}/gen_transcripts.go`) —
  that seeding is not itself part of the frozen transcript. DISPOSITION's
  item/group ids are `crypto/rand`, never reproducible outside that one
  generator run; DIRECTION's ledger constraints/questions are literal ids
  but still require the target's ledger to hold exactly what the generator
  seeded (or, for one `brief` case, to be an entirely fresh ledger).
  Replayed against a `--target` that was not independently seeded to
  match, these specific cases legitimately diverge — reporting that as a
  failure would be a false alarm, not a finding, since this command has no
  way to arrange that state on an arbitrary target. It reports skip
  instead, naming `go test ./internal/conformance` as the byte-exact
  authority: that suite boots and seeds its own server, so it can reproduce
  matching state exactly. See `internal/cli.httpTranscriptCasesNeedingSeededState`
  for the exact list of cases this applies to — every other case in these
  four operations (validation-only paths like `reject_requires_note`,
  `invalid_request`, or an unknown `group_id`) needs no seeded state and
  reports a genuine pass or fail like any other case.
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
