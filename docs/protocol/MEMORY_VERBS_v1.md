# MEMORY_VERBS v1

MEMORY_VERBS v1 is Serenity's memory-read/write wire protocol: five MCP
tools — `recall`, `remember`, `entity`, `synthesize`, `forget` — that let an
agent runtime query and update the brain. Serenity does not invent this
protocol. It adopts MEMORY_VERBS v1 from
[gbrain](https://github.com/dndungu/gbrain) (`upstream-verbs.ts`, pinned at
commit `d35c9c9e441e`) and implements it as a **conformant MEMORY_VERBS v1
server**: every field name, envelope shape, and error code below is the
pinned gbrain contract, not a Serenity redesign. `gbrain protocol
conformance --target <serenity>` passing is a release gate (RFC 0001 §8.1).
Serenity-specific additions ride only as optional fields (per-fact
`confidence`, `claim_id`, `superseded_by` on future claim-aware facts) —
legal under the additive-forever rule below, and none are load-bearing in
the shapes this document describes.

**Credit.** MEMORY_VERBS v1 is gbrain's protocol, proven in production
before Serenity existed. Serenity's lineage runs through it explicitly (RFC
0001 §2): the same file-first system of record, the same hybrid
lexical+vector retrieval pattern, and this wire protocol, kept
byte-for-byte compatible rather than re-specified. `serenity import
--from-gbrain` migrates a gbrain brain losslessly (RFC 0001 §15), and any
existing gbrain-conformant client keeps working against a Serenity brain
without modification.

## Transport

MEMORY_VERBS v1 is served over MCP's newline-delimited JSON-RPC stdio
transport (`internal/server/mcp`), not HTTP. `serenity serve --stdio`
starts the server; `serenity connect claude` wires it into a Claude Code
project's `.mcp.json` in one command (`docs/operator/claude.md`). Against
an initialized brain, the server exposes exactly five tools: `recall`,
`remember`, `entity`, `synthesize`, `forget`. There is no separate
authentication layer at this transport — the stdio channel is the trust
boundary, inherited from whatever process spawns the server (the MCP
client's own subprocess model). A Streamable HTTP MCP endpoint is planned
(RFC 0001 §13.1: "serve (MCP stdio + HTTP)") but has not shipped; when it
does, it is bound by the same bearer-token requirement DISPOSITION and
DIRECTION already enforce (`docs/protocol/DISPOSITION_v1.md`,
`docs/protocol/DIRECTION_v1.md`).

## Envelope

Every response carries `protocol_version: 1` (an integer constant). A
failed call returns the shared error envelope (below) in place of its own
success shape — never a partial or malformed success object. Field names
and semantics are frozen forever per the governance section; see that
section for what "frozen" permits.

## Operations

### `recall`

Hybrid-search the brain: an entity-scoped facts arm and a free-text search
arm, both budget-packed. Every field on the request is optional — an empty
object returns every active public fact. Facts pack first; the search arm
packs into whatever budget remains (`budget_tokens`, a char/4 estimator).

**Request** — [`memory_verbs_recall_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_recall_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `query` | string | no | Hybrid-searches the brain's pages; omit to skip the search arm. |
| `entity` | string | no | Scopes the facts arm to one entity (name or `type/slug`). |
| `budget_tokens` | integer ≥ 0 | no | Server-side char/4 packing budget; facts pack first. |
| `since` | string | no | ISO 8601 date/datetime — filters the facts arm only. |
| `session_id` | string | no | |
| `limit` | integer ≥ 0 | no | Per-arm cap on candidates. |

**Response** — [`memory_verbs_recall_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_recall_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `facts` | array of `fact` | yes | See below. |
| `total` | integer | yes | |
| `results` | array of `result` | no | Search arm — present only when `query` was passed. |
| `search_degraded` | string | no | Present when the search arm fell back to keyword-only (no embedding provider). |
| `budget_tokens` | integer | no | Present when `budget_tokens` was passed. |
| `budget_used` | integer | no | |
| `dropped_count` | integer | no | |

`fact`: `id` (integer, legacy — use `fact_id`), `fact_id` (string, the
opaque id `forget` accepts), `fact` (string), `kind` (enum: `event`,
`preference`, `commitment`, `belief`, `fact`), `entity_slug` (string or
null), `provenance` (string), `valid_until` (string or null),
`visibility` (enum: `world`, `private`) — all required.

`result`: `slug`, `title` (string or null), `chunk` (string or null),
`evidence` (enum: `alias_hit`, `exact_title_match`, `high_vector_match`,
`keyword_exact`, `weak_semantic`), `create_safety` (enum: `exists`,
`probable`, `unknown`), `provenance` (string, origin page slug) — all
required.

### `remember`

Insert one fact, one claim per call.

**Request** — [`memory_verbs_remember_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_remember_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `fact` | string | yes | The fact to remember, one claim per call. |
| `provenance` | string | yes | Where this fact came from (free text, max 500 chars). |
| `ttl` | string | no | Duration shorthand (`"30d"`, `"12h"`, `"45m"`) or an absolute ISO 8601 timestamp. Omit = never expires. |
| `entity` | string | no | Person/company/project this fact is about. |
| `kind` | string (enum, see `recall`'s `fact.kind`) | no | |
| `visibility` | string (enum: `world`, `private`) | no | |

**Response** — [`memory_verbs_remember_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_remember_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `id` | string | yes | Opaque fact id. On `status: duplicate` this is the **existing** fact's id. |
| `status` | enum: `inserted`, `duplicate`, `superseded` | yes | Branch on this, never on `status_text`. |
| `status_text` | string | yes | Human rendering of `status`. Display only. |
| `entity_slug` | string or null | yes | |
| `valid_until` | string or null | yes | ISO 8601, or null (never expires). |
| `degraded_dedup` | boolean | no | Present (`true`) when no embedding provider is configured — near-duplicates may insert. |

### `entity`

Look up one entity page by name, alias, or slug. Never errors on a miss —
returns `found: false` with suggestions instead.

**Request** — [`memory_verbs_entity_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_entity_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Free-text name, alias, or slug (e.g. `"Alice Example"`, `"people/alice-example"`). |

**Response** — [`memory_verbs_entity_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_entity_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `found` | boolean | yes | |
| `latency_ms` | integer | yes | |
| `card` | object | no | Present when `found: true`. See below. |
| `suggestions` | array of `suggestion` | yes | Populated on a miss; may be empty on a hit. |

`card`: `entity` (`slug`, `title`, `type` string-or-null — all required),
`aka` (array of string), `summary` (string), `last_touched`
(`updated_at`, `last_retrieved_at`, `last_timeline_date` — each string or
null, all required), `open_threads` (array of `{kind: "commitment" |
"recent_event", text, date}`), `edges` (array of `{type, direction: "out"
| "in", slug, context?}`), `backlink_count` (integer),
`active_fact_count` (integer) — all top-level `card` fields required.

`suggestion`: `slug`, `title`, `create_safety` (enum: `exists`,
`probable`, `unknown`) — all required.

### `synthesize`

Answer a question over the brain: a cited answer plus an explicit gap
statement and a best-effort cost aggregate.

**Request** — [`memory_verbs_synthesize_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_synthesize_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `question` | string | yes | The question to answer. |
| `since` | string | no | Optional temporal window start (ISO 8601 date or datetime). |
| `until` | string | no | Optional temporal window end (ISO 8601 date or datetime). |

**Response** — [`memory_verbs_synthesize_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_synthesize_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `answer` | string | yes | |
| `sources` | array of string | yes | |
| `gaps` | array of string | no | What the brain doesn't know. |
| `cost` | object | yes | `model` (string), `input_tokens`/`output_tokens` (integer or null), `usd_estimate` (number or null) — all required within `cost`. An honest signal, not an invoice: retries and multi-call flows sum, cache hits may undercount. |

### `forget`

Expire one fact by its opaque id. Idempotent — re-forgetting an
already-expired fact succeeds with `expired: false`.

**Request** — [`memory_verbs_forget_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_forget_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | Opaque fact id from `remember`/`recall` (`facts[].fact_id`). Never a page slug. |
| `reason` | string | no | Written to the fact's audit trail. |

**Response** — [`memory_verbs_forget_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_forget_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `id` | string | yes | |
| `expired` | boolean | yes | `true` = this call expired the fact; `false` = it was already expired. |
| `reason` | string or null | yes | |

## Error envelope

Every verb returns this object in place of its own success response
whenever the call fails at the protocol layer. `suggestion` is always
populated: problem, cause, fix.

**Schema** — [`memory_verbs_error.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/memory_verbs_error.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `protocol_version` | integer (const `1`) | yes | |
| `error` | string (enum, below) | yes | Versioned enumerated error code, verbatim from gbrain's `upstream-verbs.ts` `ERROR_SCHEMA`. |
| `message` | string | yes | |
| `suggestion` | string | yes | Populated on every verb error: problem + cause + fix. |
| `detail` | string | no | Freeform specifics (e.g. which dependency failed). |

Error codes:

| Code | Meaning |
|---|---|
| `invalid_params` | The request failed validation. |
| `provenance_required` | `remember` was called without `provenance`. |
| `not_found` | The referenced resource does not exist. |
| `scope_denied` | The call is outside the caller's granted scope. |
| `unavailable` | A dependency (e.g. the index) is unavailable. |
| `budget_unsatisfiable` | Schema-listed by the pinned gbrain contract but **reserved** — Serenity's current implementation never emits it. |
| `internal` | An unclassified internal failure. |

## Governance

- **Who arbitrates changes:** the maintainer, via a public RFC in this
  repo (`docs/rfc/`), the same process that produced RFC 0001 (RFC 0001
  §3). MEMORY_VERBS v1's own field names and semantics are pinned to
  gbrain's upstream contract; a Serenity-side RFC can extend it with
  additive-optional fields but cannot rename or redefine what gbrain
  already specified without breaking conformance.
- **Additive-forever:** field names and semantics are frozen forever.
  Only additive-optional changes are allowed (a new optional field, never
  a renamed or removed one, never a narrowed type). Every response
  carries `protocol_version`. A breaking change is a new document
  (`MEMORY_VERBS_v2.md`), expected never for a protocol this project does
  not itself author.
- **Schemas:** versioned JSON Schema (draft 2020-12) for every request,
  response, and error object lives in
  [`docs/protocol/schemas/`](schemas/), one file per wire object, each
  carrying its own `$id` and `protocol_version`. `docs/protocol/schemas/schemas.go`
  pairs every schema with the exact Go struct the live server marshals; a
  reflection test (`docs/protocol/schemas/schemas_test.go`) fails CI if
  the two drift apart.
- **Conformance fixtures:** RFC 0001 §3 names `testdata/conformance/` as
  the fixture location for all three protocols; `testdata/conformance/memory_verbs/`
  (T4.13) vendors gbrain's own 17 pinned `cases.json` request/response
  scenarios verbatim, checksum-pinned by a `MANIFEST`
  (`internal/conformance.VerifyManifest`, run on every `go test ./...`).
  `serenity protocol conformance --target <url>` (T4.15) replays them over
  MCP Streamable HTTP against a live server, resolving `{{marker}}`/
  `{{id:key}}` templating and evaluating each case's `expect`/
  `expectErrorCode` assertions (`internal/conformance`); it is this
  package's own external, wire-level counterpart to `internal/server/memory`'s
  in-process pinned suite (17 pinned MCP request/response cases plus 14
  schema-mutation tests against the gbrain contract, T4.20) and to
  `gbrain protocol conformance` itself, run against a live Serenity
  endpoint as a release gate (T4.14).
- **Kill criterion:** if real-world trials show users do not repeatedly
  exercise plan-check (DIRECTION) or conflict review (DISPOSITION), the
  protocol surface stops expanding until they do (RFC 0001 §3). This
  applies to the two protocols Serenity authors, not to MEMORY_VERBS
  itself — MEMORY_VERBS v1 is adopted, not invented, and conformance with
  it is a release gate independent of usage trials.

## See also

- RFC 0001 §8.1 (`docs/rfc/0001-serenity.md`) — the acceptance contract
  this document elaborates.
- `docs/protocol/DISPOSITION_v1.md`, `docs/protocol/DIRECTION_v1.md` — the
  two protocols Serenity does author, under the same governance model.
- `docs/operator/claude.md` — `serenity connect claude`, the one-command
  MCP install path for this protocol.
