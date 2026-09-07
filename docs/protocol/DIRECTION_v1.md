# DIRECTION v1

DIRECTION v1 is Serenity's agent-governance wire protocol (RFC 0001 §8.3)
— **the wedge** (RFC 0001 §3). Memory is the substrate; "agents that know
what you've decided — and are stopped before violating it" is the
differentiated product behavior. Three operations: `brief` (attention-
budgeted context), `check_plan` (schema-primary plan-vs-precept
verdicts), and `propose` (agents stage precept drafts and effects into
the DISPOSITION queue — never mutate a precept directly). **Sire is the
reference consumer**: a governed agent runtime calls `brief` at session
start and `check_plan` before executing any plan. Claude Code integrates
via hook + MCP in one command (`serenity connect claude`,
`docs/operator/claude.md`).

Like `docs/protocol/DISPOSITION_v1.md`, this is a protocol Serenity
authors — RFC 0001 §8.3 states the three operations but not their exact
wire shapes. The shapes below are `internal/server/direction`'s live
implementation (T4.6).

## Transport

DIRECTION v1 is served over HTTP by `internal/server.Server`: bound to
loopback by default, with a bearer token required on every route —
loopback included (RFC 0001 §14). Send `Authorization: Bearer
<daemon-token>` on every request; a missing or mismatched token returns
`401`. Not wired into a live `serenity serve` command yet as of this
writing — `internal/server/direction.Register` mirrors
`internal/server/disposition.Registrar` exactly, so a future daemon
assembly wires both protocol packages onto the same
`*internal/server.Server`.

| Operation | Method | Path |
|---|---|---|
| `brief` | `POST` | `/direction/brief` |
| `check_plan` | `POST` | `/direction/check_plan` |
| `propose` | `POST` | `/direction/propose` |

## Operations

### `brief(task_hint?, token_budget)`

The attention-budgeted context packet (RFC 0001 §12): active precepts,
current intents, relevant entities, open blocking questions. Server-side
packing; whole-section drop-not-truncate; omission counts always stated.
**The caller's token budget always governs** — the per-section caps
(12/8/8/5, in priority order: precepts, intents, entities, questions) are
maxima *within* that budget, not independent limits. A `token_budget` of
`0` still returns a structurally valid response: every section with zero
candidate items is "included, 0 items," never reported as omitted.

`budget_estimator` names the unit governing packing — currently
`"words"`, a disclosed word-count approximation of tokens
(`internal/briefing.WordEstimator`), never left for a caller to guess. A
real tokenizer-backed estimator is future work.

**Request** — [`direction_brief_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_brief_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `task_hint` | string | no | Ranks the entities section; falls back to recency when omitted. |
| `token_budget` | integer ≥ 0 | yes | The caller's token budget always governs — per-section caps are maxima within it. |

**Response** — [`direction_brief_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_brief_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `budget_estimator` | string | yes | e.g. `"words"`. |
| `sections` | array of `section` | yes | The four fixed sections, in priority order. |

`section`: `name` (enum: `precepts`, `intents`, `entities`, `questions`),
`items` (array of string), `omitted` (integer) — all required.
`omitted > 0` implies `items` is empty and vice versa: a section is
included whole or dropped whole, never truncated mid-item.

### `check_plan(plan_text, actions?)`

The **schema verdict is primary**. Exactly one of `plan_text`/`actions` is
required (the same mutual-exclusivity rule `serenity check` enforces for
its own two input forms). Two matching stages:

1. **Structured actions** (`actions[]`, the closed action set of RFC 0001
   §7.3, with parameters — e.g. `{action: "spend_over", params: {amount:
   500}}`) match against `applies_when` clauses **deterministically and
   fully offline**.
2. **Free text** (`plan_text`) is classified into the closed action set by
   the local-cheap tier first; classifier output rides in the response as
   `matched_actions` (with spans), so the caller can audit the mapping.
   **With no model available, free-text checking returns `unverified`** —
   an explicit verdict, never a silent pass.

A plan matching zero active constraints returns
`no_applicable_constraints`, never a bare `pass` — so a caller can
distinguish "checked and clean" from "nothing checked." This response is
produced by `internal/direction/check.ToWire`, the identical converter
`serenity check --json` calls: the CLI and this endpoint cannot drift on
field names or omission rules by construction.

**Request** — [`direction_check_plan_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_check_plan_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `plan_text` | string | one of `plan_text`/`actions` | |
| `actions` | array of `{action, params?}` | one of `plan_text`/`actions` | `action` is a string naming a member of the closed action set; `params` is an object. |

**Response** — [`direction_check_plan_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_check_plan_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `status` | enum: `pass`, `violated`, `unverified`, `no_applicable_constraints` | yes | |
| `considered_count` | integer | yes | |
| `constraints` | array of `{precept_id, outcome, why_not?, revisit_if?}` | no | `outcome` is `pass` or `violated`, required per entry; `precept_id` required. |
| `warnings` | array of `{precept_id, title, action}` | no | Unanswered blocking questions relevant to the plan; all three fields required per entry. |
| `matched_actions` | array of `{action, params?, span}` | no | Present only when `plan_text` was supplied (stage 2 ran). `span`: `{start, end, text}`, all required. |
| `confidence` | number | no | Present only alongside `matched_actions` — a structured `actions` call never classifies, so it never has one. |

### `propose(kind, payload)`

Agents propose precept drafts and effects; everything lands in the
DISPOSITION queue. **No model call ever mutates a precept** — `propose`
never calls a ledger write method; only a later, separate human accept
(through DISPOSITION v1's own `dispose`) writes `.dira/` or the brain
repo. `kind` is deliberately narrower than DISPOSITION's own full kind
vocabulary: an agent may only propose `precept_draft` or `effect`, not
`reconcile`/`distill`/`tombstone`/`entity_merge`/`compact`/`decompose`,
each of which has its own non-agent-facing origin elsewhere.

Payload is validated before anything is staged: a `precept_draft` payload
requires a non-empty `title` and at least one `alternatives` entry (the
"do not adopt this" floor, mirroring the same check the accept-time path
applies) — a malformed call fails loudly at propose-time rather than
staging garbage. An `effect` payload need only decode as a well-formed
effect payload; its business validation runs at accept-time.

**Request** — [`direction_propose_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_propose_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `kind` | enum: `precept_draft`, `effect` | yes | Narrower than DISPOSITION's full kind vocabulary — deliberately. |
| `payload` | opaque JSON | yes | Shape depends on `kind`; validated server-side before staging. |

**Response** — [`direction_propose_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_propose_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `item_id` | string | yes | The newly staged DISPOSITION queue item. |

## Error envelope

**Schema** — [`direction_error.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/direction_error.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `error` | string (enum, below) | yes | |
| `message` | string | yes | |

Error codes:

| Code | HTTP status | Meaning |
|---|---|---|
| `method_not_allowed` | 405 | Wrong HTTP method for the route. |
| `invalid_request` | 400 | Malformed body, or a mutually-exclusive-field violation (neither/both of `plan_text`/`actions`). |
| `invalid_action` | 400 | An `actions[]` entry names an action outside the closed action set. |
| `invalid_kind` | 400 | `propose`'s `kind` is not `precept_draft` or `effect`. |
| `invalid_payload` | 400 | `propose`'s `payload` is missing, empty, or fails kind-specific validation. |
| `internal_error` | 500 | An unclassified internal failure (e.g. a ledger read failure, a malformed `applies_when` clause on an active constraint). |

## Consumer surfaces

`pkg/serenity` (T4.18/T4.19) is a read-only, in-process embedding of this
protocol for consumers that run Serenity's read paths without going over
the wire — `docs/adr/012-embedded-read-facade-single-writer.md`.
`(*Brain).CheckPlan` and `(*Brain).Brief` are thin wrappers that call
exactly the functions this document's `check_plan` and `brief` handlers
call (`internal/direction/check.ToWire`, the same converter `serenity
check --json` uses; `internal/server/direction.Handlers.BuildBrief`, the
same function `handleBrief` calls) — no separate implementation exists
for either operation to drift from. `pkg/serenity` is a consumer surface
bound by ADR 012 §5's protocol_version policy: it may only track this
document's own additive-forever evolution, never diverge from it or move
ahead of it. `pkg/serenity/drift_test.go` proves the binding directly: it
replays every case in `testdata/conformance/direction/brief.json` and
`check_plan.json` (the frozen T4.13 transcript corpus this document's own
Governance section names below) through the facade and asserts a
byte-for-byte match, on normalized JSON, against this wire's own recorded
output.

## Governance

- **Who arbitrates changes:** the maintainer, via a public RFC in this
  repo (`docs/rfc/`), the same process that produced RFC 0001 (RFC 0001
  §3). DIRECTION v1 is a protocol Serenity itself authors — any
  affordance a consumer (Sire included) wants arrives as a public
  protocol change through this process, never as a private integration:
  "Serenity's roadmap is set by its standalone user, never by Blink/Sire
  needs" (RFC 0001 §3).
- **Additive-forever:** field names and semantics are frozen forever once
  shipped. Only additive-optional changes are allowed — a new optional
  field, never a renamed or removed one, never a narrowed type or a
  changed enum member's meaning. A breaking change is a new document
  (`DIRECTION_v2.md`), expected never.
- **Schemas:** versioned JSON Schema (draft 2020-12) for every request,
  response, and error object lives in
  [`docs/protocol/schemas/`](schemas/), one file per wire object, each
  carrying its own `$id` and `protocol_version`. `docs/protocol/schemas/schemas.go`
  pairs every schema with the exact Go struct
  (`internal/server/direction.*`, `internal/direction/check.WireResult`)
  the live server marshals — the same converter `serenity check --json`
  uses for `check_plan`'s response — and a reflection test
  (`docs/protocol/schemas/schemas_test.go`) fails CI if a schema and its
  struct drift apart.
- **Conformance fixtures:** RFC 0001 §3 names `testdata/conformance/` as
  the fixture location for all three protocols; `testdata/conformance/direction/`
  (T4.13) holds real HTTP-recorded transcripts for `brief`, `check_plan`,
  and `propose`, checksum-pinned by a `MANIFEST`. `serenity protocol
  conformance --target <url>` (T4.15) replays them against a live server.
  Unlike DISPOSITION's item ids, DIRECTION's ledger entry ids
  (`cst-0001`, `qst-0001`, …) are caller-chosen literals, not
  server-random, so a target seeded with the same fixture entries
  reproduces byte-identical responses; `internal/conformance.CompareBodies`'s
  dynamic-field normalization exists for the other two protocols, not
  because DIRECTION needs it. A handful of `brief` and `check_plan` cases
  still need a target whose ledger holds exactly those fixture entries (or,
  for one `brief` case, an entirely fresh ledger) — against an arbitrary
  `--target` this command did not seed to match, it reports those specific
  cases as skip rather than a false-alarm fail (`docs/operator/conformance.md`'s
  disclosed gaps); `go test ./internal/conformance` boots and seeds its own
  server and is the byte-exact authority for them. DIRECTION v1 is also
  exercised by
  `internal/server/direction`'s own test suite (nineteen tests over a
  real HTTP listener, including a CLI-binary comparison proving
  `check_plan` deep-equals `serenity check --json` and a `.dira/`
  content-hash test proving `propose` never mutates the ledger) and by
  T4.9's CLI-vs-protocol drift tests.
- **Kill criterion:** if real-world trials show users do not repeatedly
  exercise plan-check (this protocol) or conflict review
  (`docs/protocol/DISPOSITION_v1.md`), the protocol surface stops
  expanding until they do (RFC 0001 §3) — DIRECTION is the wedge; this is
  the criterion that would say the wedge is not landing. It gates future
  additions to `brief`/`check_plan`/`propose`, not the three operations
  already shipped.

## See also

- RFC 0001 §8.3 and §12 (`docs/rfc/0001-serenity.md`) — the acceptance
  contract this document elaborates.
- `docs/protocol/MEMORY_VERBS_v1.md`, `docs/protocol/DISPOSITION_v1.md` —
  Serenity's other two protocols, under the same governance model.
- `docs/adr/012-embedded-read-facade-single-writer.md` — the decision
  behind `pkg/serenity`, this document's "Consumer surfaces" section
  above.
