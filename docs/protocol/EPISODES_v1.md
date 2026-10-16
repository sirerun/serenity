# Episodic archive v1 — synthetic qualification contract

Schema: `serenity.episodes/v1`. Endpoint: `POST /episodes/v1`, JSON with
`Authorization: Bearer …`. This is an additive extension; MEMORY_VERBS v1,
governed-memory v1, existing backup/export and recovery semantics are unchanged.

This implementation is **default off and synthetic only**. Set hosted JSON
configuration `episode_archive_enabled: true` and `episode_synthetic_brains`
to an explicit list of at most 16 dedicated synthetic brain IDs to qualify it.
The default empty allowlist admits no brain. Every request must contain
`qualification: "synthetic"`. Neither this declaration nor the allowlist detects
real data automatically: operators must use disposable synthetic brains and
must not enroll or upload real history. Physical history purge, managed restore,
portable export, native session restore and production readiness are unsupported.
The extension does not change the existing backup/restore subsystem.

## Authority and publication

`project_id` must exactly equal the current credential's opaque hosted brain ID.
No additional brain selector is accepted. Existing credential verification,
revocation, ready-brain/active-account checks, IP/account rate limits and
maintenance → account → canonical runtime → mutation locks apply. Mutations
require `memory:write`; all other operations require `memory:read`. The writer
rechecks current credential authority inside its serialized commit section.
Workspace/conversation labels are filters, never ACLs. Other authorized readers
of that brain can read all its eligible archive evidence.

All text, roles, timestamps, attribution and checkpoints are **producer-reported
historical evidence**, never authenticated human instructions, accepted beliefs,
precepts, or transferable tool/deployment approval. The receiver rechecks current
files, permissions and external state. There are no model or embedding calls,
automatic extraction, URL downloads or host-file reads.

Canonical records use existing SourceStore `memory_episode` sources and the
existing writer queue. JSON originals and inline attachment bytes are Git
published together. Source metadata preserves the first newly stored event's
source date, or a checkpoint's creation date; publication time is separate.
A references-only re-import record uses its receipt time and does not rewrite
the original event's source date. No second authoritative database or duplicate
plaintext request journal is introduced. Derived projections are rebuilt from
canonical files. Ordinary source writes reject the reserved kind. Legacy index,
source inspector, memory disclosure and ordinary extraction exclude it.

Successful mutation acknowledgment requires canonical publication of both bytes
and metadata, plus settlement of its existing hosted write quota reservation.
Receipts bind logical hashes and a SHA256 source identity, not a rewrite-sensitive
Git commit ID. Unpublished/dirty records are not read as acknowledged history.
A timed-out request can have committed: query status or retry the same identity.
Missing or unsettled status means `unknown`, never “did not execute.”

## Envelope and operations

The request schema is [episode_request.schema.json](schemas/episodes/episode_request.schema.json).
Unknown fields and trailing JSON are rejected. See the executable
[append fixture](../../testdata/conformance/episodes/append.json).

Required envelope fields are `schema`, `project_id`, `operation`, `qualification`.
Mutation operations also require `generation` and `mutation`. `mutation.operation`
must equal the envelope operation. Read selectors and filters are rejected on
mutations. Generation is rejected on reads: read operations always use current
server eligibility.

| Operation | Additional fields | Result |
| --- | --- | --- |
| `capabilities` | None | Explicit supported/unsupported flags, numerical limits and archive usage |
| `append` | `generation`, `mutation` with ordered visible events | Stable content-free receipt after publication |
| `checkpoint` | `generation`, `mutation` with checkpoint | Exact client-supplied continuation evidence and validated source refs |
| `exclude` | `generation`, minimal `mutation` identity only | Immediate conversation exclusion, new generation; no physical purge claim |
| `status` | `command_key` | Current `completed`, `excluded` or `unknown`; receipt only when eligible and settled |
| `read` | Exactly one `event_key` or `command_key`; optional `max_bytes` | Exact event(s), inline bytes, or checkpoint; no partial exact read |
| `search` / `context` | Optional filters and budgets below | Bounded direct lexical selection with exact event citations |
| `fences` | None | Complete bounded conversation-hash → generation map and current generation |

Read/search/context consume the existing recall quota. New append/checkpoint
commands consume one existing write quota unit, independent of their event count.
Identical retries never reserve/charge again. Account storage admission includes
existing files and Git history with a conservative growth allowance. Exclusion,
status, fences and capabilities do not consume write/recall units; exclusion has
a separately reserved bounded record allowance. No price or production capacity
qualification follows from these implementation limits.

## Identity, events and exact bytes

Producer, conversation, command and event IDs are nonempty opaque ASCII strings
of at most 128 characters from `[A-Za-z0-9._:-]`. Use stable host IDs or a
separately qualified deterministic import identity algorithm. This server does
not infer identity from text or paths.

Keys are lowercase SHA256 over UTF-8 JSON string arrays, without whitespace:

- Command: `[producer_id, command_id]` (command namespace spans conversations).
- Conversation: `[producer_id, conversation_id]`.
- Event: `[producer_id, conversation_id, event_id]`.

The semantic request digest covers the Go JSON encoding of typed `mutation`,
including exact text, dates, IDs, attribution, gaps and policy. It excludes the
envelope's generation, allowing an identical retry after synchronizing a newer
generation. Different optional empty/null representations may normalize through
the typed schema; byte-identical transport JSON is not required. Changing typed
content under a command or event identity conflicts. UTC timestamps are
recommended; a changed timestamp representation can change the typed digest.

An append mutation requires `host`, `policy_version` and 1–64 `events`, in
addition to its identity fields. `host_version`, `workspace_label` and `gaps`
are optional and preserved when supplied.
Each event has `event_id`, nonnegative `sequence`, `role`, `type`, `occurred_at`,
`observed_at`, `attribution`, and exact UTF-8 `text`; `attachments` and `gaps`
are optional (empty/null means none reported).
Roles are user/assistant/tool/system/developer. Types are
message/tool_call/tool_result; hidden reasoning is not an admitted type.
Sequence must increase within a batch and be unique per conversation stream.
Historical backfill across batches is allowed without inventing wall-clock
causality. Independently authored equal text with different IDs is retained.

An attachment contains `attachment_id`, `media_type`, base64 `data`, and the
lowercase SHA256 of decoded bytes. Empty exposed bytes are allowed; null/missing
bytes, invalid digest and over-limit bytes are refused. URLs or paths cannot
stand in for custody. Unsupported or unavailable attachments belong in explicit
producer gap codes, not fabricated complete attachments. Binary bytes are
preserved exactly. Text is not trimmed or Unicode-normalized.

An event repeated identically in another command is referenced from its original
source rather than stored twice. The new batch retains its own host/policy/gap
metadata and content-free event refs; original event attribution and source date
remain available. Read-by-command resolves these refs in the original batch
order. Event refs are `{event_key, digest}`; the event digest is SHA256 of its
canonical typed JSON. Read responses include the original source SHA256,
producer/conversation IDs, original host/version, workspace/policy, batch
`capture_gaps`, and exact event. Read-by-command also returns `capture` metadata
for that command, so a deduplicated import retains its own coverage gaps.

## Context, checkpoints and coverage

Search/context filters: `query` (case-insensitive substring of event text),
`producer_id`, `conversation_id`, `workspace_label`, inclusive `from`, exclusive
`until`; filters are ANDed. Empty query selects all otherwise matching events.
Attachment bytes are not searched. Ordering is producer ID, conversation ID,
then sequence, not inferred causal order. There is no ranking, semantic search,
synthesis or automatic summary hierarchy in this increment.

`limit` defaults to 20, maximum 64. `max_bytes` defaults to 64 KiB for search/context
and 480 KiB for exact reads, maximum 480 KiB. It counts serialized selected JSON
records (exact reads include their selection container), not model tokens; final
response JSON is independently capped at 512 KiB. A result/byte-limited page has
`next_offset` and an omission code. To continue, send that offset plus the exact
returned `revision`. Revision covers the current archive source set, including
fences. A changed revision returns 409; restart pagination. If the first event
exceeds the budget, `next_offset` remains unchanged: increase the budget or use
an exact event read. A full batch can exceed an exact-read response budget; its receipt exposes event
keys so each admitted event can be read individually. No event is silently truncated. No selected result is not
proof that no relevant or uncaptured history exists.

A checkpoint mutation replaces `events` with
`checkpoint: {text, sources: [{event_key,digest}], model, created_at}`.
It requires 1–64 exact eligible source references from the current brain. `model`
is producer-reported provenance, not a server provider call. Text is preserved
exactly, not generated. Read the checkpoint by its returned `command_key`;
context selection independently retrieves original events. No pending tool call
is executable replay. Deleting any dependency excludes that checkpoint, even
when it belongs to a different conversation.

Responses always disclose unsupported physical purge and managed restore,
`authority: "attributed_evidence_not_execution_authority"`, current generation,
archive revision and omission codes. Original event/batch gaps remain producer
assertions. `host_coverage_not_verified` is always present; publication does not
prove that a host exposed every event. There is no lagging archive index:
search/context scan eligible originals directly. No model-token estimator or
exhaustive semantic recall claim is made.

## Deletion and replay fences

`exclude` accepts only operation, producer/conversation/command IDs; host labels,
text, reason strings, events and checkpoints are rejected. Empty host/policy
metadata and empty/null gaps emitted by the typed envelope are harmless and
normalize into the content-free fence; nonempty values are rejected. The persisted fence
contains hashed command/conversation identities, request digest, brain binding,
generation and time, with no original IDs, paths, transcript or attachment bytes.
Each new exclusion increments the brain's archive generation, including exclusion
of a conversation not yet uploaded. A repeated same command returns its stable
receipt after the caller refreshes generation. A different command for an already
excluded conversation is rejected. Intentional recapture is unsupported.

`fences` is always a **complete snapshot**, never an incremental cursor. Its empty
map is `{}`. Apply it before queue upload, then use its generation on mutations.
The server independently rechecks generation and fences in the serialized writer.
A stale generation yields 409; an excluded identity yields 410. An identical old
append cannot resurrect excluded events. Reads and checkpoint dependencies are
checked against current fences, never a stale search index or cached response.

Exclusion is conversation-scoped and immediately stops this API disclosing those
originals and dependent checkpoints. Physical original bytes, Git history,
managed backups, offline queues and external copies may remain. The response
explicitly says `physical_purge: "unsupported"`, not completed or erased.
Fence rollback through an old managed restore is not qualified: keep real history
capture disabled. Full history purge, backup admission/fence recovery, controlled
queue propagation and other real-history acceptance are parent followup gates.

## Hard bounds and failures

| Bound | Value |
| --- | --- |
| HTTP request | 1 MiB |
| Canonical record | 512 KiB (input also reserves metadata overhead) |
| Event text | 64 KiB UTF-8 bytes |
| Entire encoded event | 448 KiB, ensuring an admitted event fits an exact read |
| Events / batch | 64 |
| Attachment decoded bytes | 32 KiB each |
| Attachments | 8/event, 16/batch |
| Append/checkpoint records / brain | 256, including deduplicated receipts |
| Archive originals/metadata JSON | 32 MiB, plus reserved fence allowance |
| Exclusion records / brain | 256, no silent fence eviction |
| Source-tree scan entries | 65,536; excess fails closed |
| Final response | 512 KiB |
| Query | 512 UTF-8 bytes |
| Gaps | 32 codes per event/batch, 128 bytes each |
| Request processing context | 30 seconds; existing shared lock waits may delay admission |

400 invalid parameters; 401 unauthorized/revoked; 403 scope/binding/synthetic-brain
restriction; 404 disabled or exact record unavailable; 409 identity/generation/
revision conflict; 410 excluded; 413 HTTP or exact-read budget exceeded; 415 content
type; 429 rate/quota/archive bound; 503 capacity/publication/outcome unknown.
Error responses use the same schema plus `error`, never input text. Responses
set `Cache-Control: no-store` and `Pragma: no-cache`. Browser Origin is refused.

Qualification requires executing focused source/writer/HTTP tests, race checks,
legacy regressions and independent review on the exact candidate. Source and
fixture delivery alone are not test, provider, deployment or production evidence.
