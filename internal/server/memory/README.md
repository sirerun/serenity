# MEMORY_VERBS v1 implementation

This package serves `recall`, `remember`, `entity`, `synthesize`, and `forget`
over the shared MCP registry. The adopted contract is gbrain revision
`d35c9c9e441e`; the pinned cases, response schemas, and upstream license are in
[testdata](testdata/README.md). Exported aliases in [wire.go](wire.go) refer to
these live request and response types.

A `remember` request saves attributed source material. It does not create an
accepted claim, change a precept, or promote a conflicting statement to a belief.
The original fact and provenance strings remain verbatim. Exact duplicate
matching includes entity type and slug, kind, visibility, attribution, and the
resolved expiry instant. Semantic duplicate detection is unavailable and is
reported through `degraded_dedup: true`.

Each fact is a versioned JSON payload inside the existing content-addressed
`brain/sources` store. The payload contains its semantic metadata and a persistent,
random positive 53-bit numeric ID. Its full SHA-256 is the opaque protocol ID.
Allocation checks existing IDs; projection also rejects collisions after Git
merges. `forget` adds an immutable expiry source targeting that SHA. It preserves
the original bytes and returns `expired: false` on a repeated request. Generic
imports cannot create the reserved fact or expiry kinds.

The writer queue serializes allocation, duplicate checks, publication, and expiry
within one writer process. Complete source directories are published atomically;
malformed canonical records fail closed. Queue flushes commit exact touched files
and retain retry bookkeeping after a Git failure. Stdio shutdown joins tool calls,
closes its queue, flushes writes, and closes the index. Independent writer
processes do not share this queue, so cross-process semantic deduplication is not
guaranteed.

The source projection remains authoritative when the search index is stale.
Local CLI search can read active private facts; remote tools cannot read or
forget them. Shared eligibility checks run before retrieval caps and before
embedding, extraction, or composition. Raw memory facts and expiry events are
excluded from automatic extraction. `index_only` remains a separate egress policy.
A successful remember refreshes its derived chunk; if that cache update fails,
`status_text` reports the durable write and gives `serenity sync` rebuild guidance.

Entity cards use current public canonical evidence, omitting cached summaries
and timeline entries whose privacy attribution cannot be established. Synthesis
uses the shared composer with separate source citations and claim citations. Date
bounds filter evidence before the prompt. Missing providers produce
`unavailable`; unknown usage is null, and a reported provider model takes
precedence over the configured model. Legacy fence files do not retain all
visibility and provenance metadata; this repair cannot reconstruct metadata
absent from those files.

Tool-domain errors are versioned JSON text with MCP `isError: true`, including
input-schema rejection and internal execution failure. Invalid JSON-RPC framing
or a non-object MCP `arguments` value remains a transport error. The protocol
version belongs to the domain response, not the request or JSON-RPC envelope.

The tests execute all 17 pinned cases through real stdio MCP sessions, including
seeded entity cases and unavailable synthesis. Independent tests cover corruption,
privacy, stale indexes, persistence, CLI parity, and negative mutations. These
checks do not establish an HTTP MCP endpoint, full upstream HTTP conformance,
provider integration, or a latency SLA.
