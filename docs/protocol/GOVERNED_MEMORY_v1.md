# Governed memory v1

`serenity.memory/1` is an additive hosted protocol. It serves one authenticated
physical request per command, without an MCP handshake. Existing MEMORY_VERBS
and OAuth routes remain available. This is source implementation, not a hosted
release or live deployment qualification.

## HTTP boundary

POST `/memory/v1/commands` accepts:

```json
{"schema":"serenity.memory/v1","project_id":"HOSTED_PROJECT","action":{"schema":"zati.serenity.action/v1","brain_id":"LOCAL_UUID","adapter_command_id":"COMMAND_UUID","kind":"remember","text":"An attributed statement","sources":[],"writer_owner":"AUTHENTICATED_ACCOUNT","maximum_cost":{"currency":"USD","micro_units":0},"allowed_provider_destinations":[]}}
```

The bearer credential must currently authorize that exact hosted brain and
`memory:write` for remember/retract or `memory:read` for recall/inspect/status.
Account suspension, brain readiness and credential revocation are checked on
all calls and again after waiting for the account lock. First admission binds
the local UUID to this canonical brain; a later request cannot relabel it.
`writer_owner` must be the authenticated account ID. The physical writer remains
the runtime's exclusive brain writer and serialized queue.

GET `/memory/v1/commands/COMMAND_UUID?project_id=HOSTED_PROJECT&brain_id=LOCAL_UUID`
recovers a stored command under current read authority. It does not open a
runtime, flush, call a model, or repeat a mutation. Missing commands return
`unknown` and `lookup_authoritative:false`; absence is not proof of nonadmission.
A repeated POST compares normalized request bytes and conflicts on changed
input. Completed commands retain their original producer-minted claim identity.

## Evidence

Responses carry `schema`, `project_id`, `brain_id`, `adapter_command_id`, `kind`,
`command_status`, `claims`, `brain_revisions`, `cost`, `provider_destinations`,
`lookup_authoritative`, and write-specific `writer_owner` and retraction flags.
Claims contain UUID `id`, positive `version`, local `brain_id`, attributed
`text`, opaque caller-supplied artifact `sources`, producer observation time
`freshness`, and `active`. Confidence is omitted: a remembered statement has
not been independently assessed. No accepted belief or source verification is
implied by preserving an artifact reference.

Recall performs a bounded, deterministic lexical scan of current canonical
statements. `minimum_freshness` constrains the read snapshot, not the age of the
statement. Revision and index revision are the SHA-256 of sorted canonical
command-source identities; observed_at is the actual scan time. There is no
lagging derived index in this path. Cold opening explicitly skips provider-backed
index recovery. Cost is zero USD micro-units and provider destinations are empty.
Hosted request limits, account/partner write/recall quotas, memory inventory and
storage caps still apply.

Retraction increments an active claim's version, excludes it from active recall,
and preserves original attribution and historical bytes. `historical_erasure`
is false. Recovering an older command returns current claim state, preventing
an old acknowledged remember from resurrecting an inactive statement.

## Shared Serenity memory

Records are immutable, typed sources in the existing canonical SourceStore,
written and published through the existing writer queue. They are projected
into normal MEMORY_VERBS recall/read, dashboard inventory and export; there is
no second authoritative database. The legacy SHA fact identity is the original
remember command's source SHA, and provenance includes the governed UUID and
current version. Existing unversioned legacy memories are not fabricated into
new UUID claims.

MCP exposes `retract_governed_memory` with `adapter_command_id`, `claim_id`,
`version`, and `reason`. Destructive legacy `forget` refuses governed records
and points to this tool: removing a command source would destroy reconciliation
and binding evidence. Governed retraction is removal from active recall, not
physical erasure. The extension retains ordinary hosted write scope checks.

Promotion and immutable revision export actions are not implemented by this
protocol and remain explicitly unsupported. Existing account export continues
to include canonical source history. A production deployment, native Zati
connection flow and live OAuth acceptance are separate qualification gates.
