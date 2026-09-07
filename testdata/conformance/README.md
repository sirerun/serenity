# Protocol conformance fixtures (T4.13)

Checksum-frozen fixtures for Serenity's three wire protocols (RFC 0001
section 8): `memory_verbs/`, `disposition/`, `direction/`. Each directory
carries a `MANIFEST` -- `<sha256>  <filename>` lines, `shasum -a 256 -c
MANIFEST`-compatible -- pinning every fixture file in it.
`internal/conformance.VerifyManifest` (called by
`internal/conformance/fixtures_test.go`, which `go test ./...` runs on
every push) fails CI if a fixture changes without its manifest entry
being re-pinned in the same change.

A `MANIFEST` never pins the `//go:build ignore` generator script that
sits beside it in the same directory -- that script is source, not a
fixture (`internal/conformance`'s own `computeManifest` excludes `*.go`
files).

## memory_verbs/

Vendored, unmodified, from `dndungu/gbrain@d35c9c9e441e`
(`docs/protocol/MEMORY_VERBS_v1.md`, the frozen memory-verbs contract):

- `cases.json` -- the 17 pinned conformance cases
  (`test/fixtures/memory-verbs/cases.json` upstream). Byte-identical to
  `internal/server/memory/testdata/memory-cases-upstream.json`, which
  T4.20's `TestMemoryV1AllPinnedCases` drives against the live MCP
  server -- this copy is this task's own broader conformance-harness
  home for the same vendored set (see that package's own
  `testdata/README.md` for the distinction).
- `LICENSE` -- gbrain's own MIT license, carried alongside its fixtures
  per the license's own attribution requirement.

To re-vendor after gbrain's pinned commit moves (a deliberate act, never
routine): copy the new `test/fixtures/memory-verbs/cases.json` and
`LICENSE` from the new pinned commit into this directory, update the
commit hash in this file and in `docs/adr/009`, then re-pin:

```
GOWORK=off go run testdata/conformance/memory_verbs/gen_manifest.go  # docs/lore.md L-0001
```

## disposition/ and direction/

Serenity's own transcripts: real HTTP request/response pairs recorded
against a live `internal/server/disposition.Handlers` /
`internal/server/direction.Handlers` listener (the identical
real-listener harness `internal/server/disposition/disposition_test.go`
and `internal/server/direction/direction_test.go` themselves use, T4.4 /
T4.6). These are recordings, not hand-authored fixtures -- every
`request_body` and `response` is exactly what a live handler sent and
received when the generator ran.

One JSON file per operation (`list_pending.json`, `dispose.json`,
`capture.json`, `subscribe_longpoll.json` under `disposition/`;
`brief.json`, `check_plan.json`, `propose.json` under `direction/`). Each
file is a `Transcript` (`internal/conformance/transcript.go`):

```json
{
  "protocol": "disposition",
  "operation": "dispose",
  "cases": [
    {
      "name": "human-readable scenario description",
      "steps": [
        {
          "method": "POST",
          "path": "/disposition/dispose",
          "request_body": { "...": "..." },
          "response": { "status": 200, "body": "{\"...\":\"...\"}\n" }
        }
      ]
    }
  ]
}
```

A `Case` may carry more than one `Step` when the scenario is a sequence
against shared server-side state (e.g. a dispose-replay case's second
step reuses the first step's `idempotency_key` against the same item; a
parked-filter case's two steps read the default view then the parked
view of the same seeded items). `response.body` is the raw response
text, not re-encoded JSON -- most bodies are JSON (decode to inspect
them), but an unauthenticated-rejection step (`"no_auth": true` on the
request) captures `net/http`'s own plain-text `401` body, and a
transcript format that can only hold JSON couldn't record that case at
all.

Item ids, dispose/created timestamps not pinned by the generator's fixed
clock, and any other genuinely dynamic field are real captured values,
not placeholders -- they will differ on the next `go run` of a
generator. That is expected and is exactly what "checksum-frozen" means
here: a fixture is captured once, then frozen; a future consumer that
replays these transcripts against a live server (T4.15) is expected to
normalize dynamic fields (ids, timestamps) before comparing bodies,
the same way gbrain's own `memory_verbs` conformance run seeds and
compares by a `{{marker}}`-templated identity rather than a byte-exact
match.

Regenerate a protocol's transcripts and re-pin its manifest after a
DELIBERATE change to that protocol's wire shapes (review the diff before
committing, same discipline as any other checksum-pinned corpus in this
repo):

```
GOWORK=off go run testdata/conformance/disposition/gen_transcripts.go  # docs/lore.md L-0001
GOWORK=off go run testdata/conformance/direction/gen_transcripts.go    # docs/lore.md L-0001
```

Coverage is deliberately representative, not exhaustive: each file's
cases were chosen to exercise T4.4/T4.6's own acc-line clauses (kind/
group/parked filtering and cursor pagination; accept/reject/replay/
group/not-found dispose outcomes; capture's happy and empty paths;
long-poll's immediate-return path and its auth requirement; brief's
empty-ledger, zero-budget, and whole-section-drop shapes; check_plan's
violated/no-applicable-constraints/unverified/malformed-request shapes;
propose's invalid-kind/invalid-payload/precept_draft/effect shapes) --
not a scan of every reachable branch. `disposition/subscribe`'s SSE mode
is not represented here: an open server-sent-events stream is not a
single request/response pair, so it does not fit this transcript shape;
`disposition_test.go`'s own
`TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents` remains the
authority for that surface. Extending coverage later (a new case in an
existing file, or a new operation file) is additive: add the case to the
generator, re-run it, re-pin the manifest.
