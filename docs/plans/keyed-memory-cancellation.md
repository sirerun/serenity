# Durable cancellation of ambiguous memory writes

Status: implementation under qualification; not released.
Owner: Codex. Trigger: Ajent gateway ingestion recovery analysis after #222.

A source can be withdrawn after its remember response was lost. Retrying its
body merely to discover the fact ID could create revoked content and cause a
new provider call. An absent status lookup would not fence a delayed write.

Add cancel_memory_operation alongside the unchanged five MEMORY_VERBS v1 tools. The existing writer
queue persists a cancellation fence even if the fact does not exist. A later
remember of that absent key fails with operation_canceled before indexing.
If the fact exists, cancellation expires it and an exact retry still recovers
its original expired identity. Changed payloads still conflict. Canceling an
existing private fact through the remote interface is refused.

Canonical storage remains SourceStore: a memory_expiry v2 payload targets the
operation key; v1 fact-ID expiries retain their original encoding. Readers that
only understand v1 reject the v2 source instead of ignoring its cancellation.
The complete projection applies a cancellation to a matching fact even when
independent histories are merged. Lifecycle records never become knowledge.
A canceled key is never reusable; cancellation retention cannot be pruned while
late delivery/recovery remains possible. This is logical withdrawal, not physical
Git/history erasure or authority to republish under a new key.

Acceptance: cancel-before-write/restart/late retry, cancel-after-commit with lost
response, repeated cancellation, concurrent queue order, private scope, codec
version/target validation and merged-history retrieval exclusion. Real MCP proof
must cross process restart, followed by full race/lint/vet/build qualification.

Gateway dependency: only enable cancellation-dependent ingestion when tools/list
advertises cancel_memory_operation and the pinned writer is qualified. Old clients
using forget by id remain supported; old writer downgrades cannot open a v2-cancellation
brain. Ajent must immediately suppress source reads on revocation independently
of cleanup completion. Canonical bodies and model credentials stay outside logs.
