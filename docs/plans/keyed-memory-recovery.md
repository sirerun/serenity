# Keyed MEMORY_VERBS recovery

Status: implementation under local qualification; not released.
Owner: Codex, supporting Ajent SK-03. Baseline source: 2d024726.

A client can lose a remember response and retry after the fact has expired or
been withdrawn. Active-content deduplication can then create another fact. Add
an optional brain-scoped `operation_key` to the existing remember boundary and
immutable memory-fact source. No new canonical database or writer API.

Contract:

- 1–128 ASCII letters/digits or `_.:-`; keys contain no secrets.
- Same key and normalized durable input return the original ID, including its
  current expired state, across restart. Recovery never removes an expiry.
- Changed fact, attribution, entity, kind, visibility or absolute expiry with
  the same key returns `operation_conflict`, without canonical mutation.
- Keyed TTL must be absolute or omitted. Different keys own distinct facts,
  even when their content matches. Unkeyed calls retain active exact dedup.
- Duplicate keys in merged canonical state fail projection closed.
- The response exposes `expired`; keyed recovery is available only in binaries
  advertising `operation_key` in tools/list. Older writers do not enforce it.

Acceptance: persisted restart/withdrawal recovery, concurrent same-queue retries,
conflicting payload rejection, merged-key collision rejection, wire validation,
and a real CLI/MCP replay. Existing memory/store/writer checks must stay green.

This does not claim a standalone operation-status API, document revisions,
semantic retrieval, distributed idempotency, or process ownership. All writes
must still use one queue. Follow-up owner Codex: fence canonical writers across
processes before running a shared Ajent brain. Trigger: keyed recovery proof is
qualified; include non-MCP CLI writers rather than protecting only serve.
