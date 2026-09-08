# DISPOSITION v1

DISPOSITION v1 is Serenity's human-in-the-loop wire protocol (RFC 0001
§8.2): the approval queue any HITL client — mobile, desktop, or CLI —
implements to become Serenity's disposition surface. **Blink is the
reference mobile client.** Serenity's own CLI (`serenity inbox`,
`serenity capture`) is the first conformant client; the daemon has no
privileged internal path — the CLI consumes the same
`internal/disposition.Store` and `internal/events.Store` this protocol's
HTTP handlers call.

This is a protocol Serenity authors (unlike MEMORY_VERBS v1, adopted from
gbrain — `docs/protocol/MEMORY_VERBS_v1.md`), so RFC 0001 §8.2 states its
four operations but not their exact wire shapes. The shapes below are
`internal/server/disposition`'s live implementation (T4.4).

## Transport

DISPOSITION v1 is served over HTTP by `internal/server.Server`: bound to
loopback by default (`127.0.0.1`, OS-assigned port unless configured),
with a bearer token required on every route — loopback included, since any
local process is not automatically trusted (RFC 0001 §14). LAN/Tailscale
exposure is explicit opt-in config, with optional mTLS layered on top.
Send `Authorization: Bearer <daemon-token>` on every request; a missing or
mismatched token returns `401`. Not wired into a live `serenity serve`
command yet as of this writing — `internal/server/disposition.Register`
mirrors the `Registrar` interface any future daemon assembly wires onto
the same `*internal/server.Server` `MEMORY_VERBS`, `DIRECTION`, and
`/healthz` already share.

| Operation | Method | Path |
|---|---|---|
| `list_pending` | `POST` | `/disposition/list_pending` |
| `dispose` | `POST` | `/disposition/dispose` |
| `capture` | `POST` | `/disposition/capture` |
| `subscribe` | `GET` | `/disposition/subscribe` |

## The `item` object

Every operation below returns items in this shape. Shared by
`list_pending` and `dispose`'s own response objects, referenced by `$ref`
rather than redeclared.

**Schema** — [`disposition_item.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_item.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | |
| `kind` | enum: `reconcile`, `precept_draft`, `effect`, `distill`, `tombstone`, `dirty_edit`, `entity_merge`, `compact`, `decompose` | yes | |
| `state` | enum: `pending`, `deferred`, `parked`, `disposed` | yes | |
| `group_id` | string | no | |
| `payload` | opaque JSON | no | Shape depends on `kind`; not constrained by this schema. |
| `created_at` | string (date-time) | yes | |
| `updated_at` | string (date-time) | yes | |
| `defer_count` | integer | no | How many times this item has been deferred. |
| `verdict` | enum: `accept`, `edit_accept`, `reject`, `defer` | no | Set once `state == disposed`. |
| `note` | string | no | |
| `edited_payload` | opaque JSON | no | Attached by an `edit_accept` verdict; not constrained here. |
| `actor` | string | no | |
| `disposed_at` | string (date-time) | no | |
| `idempotency_key` | string | no | |
| `route` | enum: `claim-batch`, `precept-draft`, `note`, `trash` | no | Set only for a distill item disposed through the capture routing path. |
| `route_effect_pending` | boolean | no | Recovery marker for a committed precept-draft route whose follow-on staging has not completed. Absent on legacy and completed routes. |
| `resurfaced` | boolean | no | `true` once a parked item has been resurfaced back to pending — a one-time transition. |
| `applied_entry_id` | string | no | Committed ledger entry created by an accepted precept draft or child intent. |
| `ledger_effect_pending` | boolean | no | Recovery intent recorded with new ledger acceptances; cleared on publication, absent on legacy unmarked effects. |
| `applied_publication_id` | string | no | Committed dirty-edit receipt ID; a publication may contain zero or more claims. |
| `applied_claim_id` | string | no | The claim id a reconcile item's accept/edit_accept verdict wrote to the canonical brain repo, once applied. |

## Operations

### `list_pending(kinds?, expiring_before?, group?)`

Lists approval items: reconciliation pairs (A/B with provenance), precept
drafts, effect requests, distill items, tombstones, and the other kinds
above. With `group: true`, items sharing a non-empty `group_id` collapse
into one row carrying every member — one disposition still covers all of
them, but each member is recorded individually for the earned-automation
ladder (RFC 0001 §10.3). `parked` and `cursor`/`limit` are this
implementation's own wire additions, not literally named in RFC 0001's
`list_pending` signature: `parked` is the wire equivalent of `serenity
inbox --parked` (a parked item is resurfaced "only by explicit filter" —
this is that filter, mutually exclusive with the default pending/deferred
view), and `cursor`/`limit` implement the pagination the acceptance
criteria require ("a cursor walk of 120 items in pages of 50 terminates").
Pagination is a plain numeric offset over the filtered, ordered result
set — correct for a snapshot walk, not proof against a page landing
mid-mutation.

**Request** — [`disposition_list_pending_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_list_pending_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `kinds` | array of string | no | Filters to these `kind` values. |
| `expiring_before` | string (RFC3339) | no | |
| `group` | boolean | no | Collapse items sharing a non-empty `group_id` into one row carrying every member. |
| `parked` | boolean | no | List parked items instead of the default pending/deferred view — the two are mutually exclusive. |
| `cursor` | string | no | Opaque page cursor from a prior response's `next_cursor`. |
| `limit` | integer ≥ 0 | no | Default 100, capped at 500. |

**Response** — [`disposition_list_pending_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_list_pending_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `items` | array of `{item, members?}` | yes | `members` (array of `item`) is set only when `group` was requested and the row represents a non-empty `group_id`. |
| `next_cursor` | string | no | Present when more rows follow. |

### `dispose(item_id \| group_id, verdict, edited_payload?, note?, idempotency_key)`

Applies `verdict` to one item (`item_id`) or every item sharing a
`group_id` — exactly one of the two is required. A `group_id` dispose
applies the verdict to every member as its own store call, never one bulk
write ("each recorded individually for the ladder"). `reject` requires
`note`. `idempotency_key` is required at this wire layer (RFC 0001 §8.2's
own signature gives it no `?`, unlike `edited_payload`/`note`) even
though the underlying store treats an empty key as "no idempotency
check." `edit_accept` writes through the deterministic writer like any
accept — the edited payload lands in the fence or shard with human-tier
provenance, not a client-side file patch.

**Cross-client conflict:** the first successful `dispose` call wins; a
second disposition of the same item returns `item` carrying the
originally recorded verdict with `already_disposed: true`, so the losing
client reconciles its view rather than erroring. A replayed call (same
`idempotency_key` as a prior successful call) returns the identical
original result with `replayed: true`.

**Request** — [`disposition_dispose_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_dispose_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `item_id` | string | one of `item_id`/`group_id` | |
| `group_id` | string | one of `item_id`/`group_id` | |
| `verdict` | enum: `accept`, `edit_accept`, `reject`, `defer` | yes | |
| `edited_payload` | opaque JSON | no | For an `edit_accept` verdict; not constrained here. |
| `note` | string | no | Required by the store when `verdict` is `reject`. |
| `idempotency_key` | string | yes | |
| `actor` | string | no | |

**Response** — [`disposition_dispose_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_dispose_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `results` | array of `{item, already_disposed?, replayed?}` | yes | One result per disposed item — more than one only for a `group_id` request. |

### `capture(text \| audio_ref, hint?)`

Zero-friction ingress: stages a distill item and returns its id. At least
one of `text`/`audio_ref` must be non-empty (enforced by the handler,
which returns `capture_empty` otherwise).

**Request** — [`disposition_capture_request.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_capture_request.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `text` | string | at least one of `text`/`audio_ref` | |
| `audio_ref` | string | at least one of `text`/`audio_ref` | |
| `hint` | string | no | |

**Response** — [`disposition_capture_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_capture_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `item_id` | string | yes | The newly staged distill item's id. |

### `subscribe(cursor)`

Server-sent events over HTTP with a long-poll fallback, at-least-once
delivery with monotonic cursors — a disconnected client resumes from its
cursor. Send `Accept: text/event-stream` to select SSE (a browser
`EventSource` sets this automatically); any other request gets long-poll.
Either way, resume prefers the `Last-Event-ID` header (SSE's own
reconnect mechanism) over the `cursor` query parameter when both are
present, so a reconnecting `EventSource` resumes correctly with no
application code involved. Both delivery modes poll the underlying event
log on a fixed interval (200ms default) rather than an event-driven
wakeup — a subscriber may read a published event up to one poll interval
late, never sooner.

`list_pending`, `dispose`, and `capture` each publish one event on a
genuinely new state change: `disposition.item_disposed` for a dispose
that is neither `already_disposed` nor `replayed`, `disposition.item_created`
for a successful `capture`.

**Event** (one row, either replayed over SSE one frame per event, or
batched into the long-poll response below) — [`disposition_event.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_event.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `cursor` | integer | yes | Monotonic; pass back to resume. |
| `kind` | string | yes | e.g. `disposition.item_disposed`, `disposition.item_created`. |
| `payload` | opaque JSON | no | Per-kind; not constrained here. |
| `occurred_at` | string (date-time) | yes | |

**Long-poll response** — [`disposition_subscribe_longpoll_response.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_subscribe_longpoll_response.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `events` | array of `disposition_event` | yes | May be empty if the long-poll wait (25s default) elapsed with nothing new. |

The SSE arm frames the identical `disposition_event` payload one event
per `id: <cursor>\ndata: <json>\n\n` wire frame instead of batching into
this envelope.

## Semantics

- **Expiry is auto-defer, never auto-decline.** An expired item (default
  14 days, configurable per kind) drops to a low-priority aged state,
  never silently converting machine ambiguity into a claim-state change.
- **Deferral terminates.** After N defer cycles (default 3, configurable)
  an item moves to `parked` — terminal but reversible, visible only via
  the `parked` filter, excluded from queue-depth/age SLO metrics, and
  resurfaced only by explicit filter or new evidence on the same
  `(subject, predicate)`. Nothing resurfaces forever.
- Every disposition is recorded with actor, client, and timestamp;
  disposition history is the training signal for the earned-automation
  ladder (RFC 0001 §10.3).

## Error envelope

**Schema** — [`disposition_error.schema.json`](https://github.com/sirerun/serenity/docs/protocol/schemas/disposition_error.schema.json)

| Field | Type | Required | Notes |
|---|---|---|---|
| `error` | string (enum, below) | yes | |
| `message` | string | yes | |

Error codes:

| Code | HTTP status | Meaning |
|---|---|---|
| `method_not_allowed` | 405 | Wrong HTTP method for the route. |
| `invalid_request` | 400 | Malformed body, missing required field, or a mutually-exclusive-field violation (e.g. neither/both of `item_id`/`group_id`). |
| `not_found` | 404 | The referenced item or `group_id` does not exist. |
| `invalid_verdict` | 400 | `verdict` is not one of the enumerated values. |
| `reject_requires_note` | 400 | `verdict: reject` was sent without `note`. |
| `capture_empty` | 400 | Neither `text` nor `audio_ref` was non-empty. |
| `internal_error` | 500 | An unclassified internal failure. |

## Governance

- **Who arbitrates changes:** the maintainer, via a public RFC in this
  repo (`docs/rfc/`), the same process that produced RFC 0001 (RFC 0001
  §3). DISPOSITION v1 is a protocol Serenity itself authors — any
  affordance a client (Blink included) wants arrives as a public protocol
  change through this process, never as a private integration.
- **Additive-forever:** field names and semantics are frozen forever once
  shipped. Only additive-optional changes are allowed — a new optional
  field, never a renamed or removed one, never a narrowed type or a
  changed enum member's meaning. A breaking change is a new document
  (`DISPOSITION_v2.md`), expected never.
- **Schemas:** versioned JSON Schema (draft 2020-12) for every request,
  response, item, event, and error object lives in
  [`docs/protocol/schemas/`](schemas/), one file per wire object, each
  carrying its own `$id` and `protocol_version`. `docs/protocol/schemas/schemas.go`
  pairs every schema with the exact Go struct
  (`internal/server/disposition.*`, `internal/disposition.Item`,
  `internal/events.Event`) the live server marshals; a reflection test
  (`docs/protocol/schemas/schemas_test.go`) fails CI if the two drift
  apart.
- **Conformance fixtures:** RFC 0001 §3 names `testdata/conformance/` as
  the fixture location for all three protocols; `testdata/conformance/disposition/`
  (T4.13) holds real HTTP-recorded transcripts for `list_pending`,
  `dispose`, `capture`, and long-poll `subscribe`, checksum-pinned by a
  `MANIFEST`. `serenity protocol conformance --target <url>` (T4.15)
  replays them against a live server, normalizing dynamic fields (ids,
  timestamps) by shape rather than byte-comparing them
  (`internal/conformance.CompareBodies`) — see
  `testdata/conformance/README.md` for the disclosed gap this closes
  (item ids are `crypto/rand`, never reproducible run to run) and for the
  two the command still discloses: `list_pending`/`dispose` seed their
  items via an internal `Store.Create` call the generator makes before its
  transcript's own steps run, so their happy-path cases only pass against
  a `--target` independently seeded with matching items — replayed against
  a bare/fresh target, this command reports the specific cases that need
  that seeded state as skip rather than a false-alarm fail (it has no way
  to tell "not seeded to match" from "the target regressed"); and `subscribe`'s SSE mode has no
  transcript at all, only its long-poll fallback — `disposition_test.go`'s own
  `TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents` remains SSE's
  authoritative coverage. DISPOSITION v1 is also exercised by
  `internal/server/disposition`'s own test suite (fourteen tests over a
  real HTTP listener, covering pagination termination, replay
  byte-identity, reject-without-note, mid-stream SSE resume, and the
  parked-filter split) and by T4.9's CLI-vs-protocol drift tests.
- **Kill criterion:** if real-world trials show users do not repeatedly
  exercise conflict review (dispositioning reconcile/precept-draft items
  through this protocol) or plan-check (`docs/protocol/DIRECTION_v1.md`),
  the protocol surface stops expanding until they do (RFC 0001 §3).
  DISPOSITION v1's own five operations are not affected retroactively —
  the criterion gates future additions, not what is already shipped.

## See also

- RFC 0001 §8.2 (`docs/rfc/0001-serenity.md`) — the acceptance contract
  this document elaborates.
- `docs/protocol/MEMORY_VERBS_v1.md`, `docs/protocol/DIRECTION_v1.md` —
  Serenity's other two protocols, under the same governance model.
- RFC 0001 §10.3 — the earned-automation ladder that consumes disposition
  history recorded here.
