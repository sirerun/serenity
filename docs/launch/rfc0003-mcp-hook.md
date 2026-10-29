# MCP prompt-context tool factory

`agentmemory.NewPromptContextTool` constructs the proposed MCP tool
`recall_prompt_context`. It is a source factory only: this change does not
register it in the hosted gateway, expose a server route, deploy a service or
install a native hook. No hosted credential or endpoint is selected here.

Arguments are the explicit new schema `{query, session_id?}`. Query is nonempty
and limited to 4096 UTF-8 bytes before callback dispatch; JSON Schema character
limits alone are insufficient. Session ID is an optional bounded recall tag,
not authentication. Arguments cannot supply account, brain or workspace
authority. Input duplicates, malformed JSON and invalid UTF-8 are refused.
Prompts are never silently clipped.

The constructor requires a trusted expected binding and a `GovernedRecall`
callback. Integration must recheck the **current request token**, account/brain
authorization and workspace mapping, and invoke the existing metered governed
`recall` path. Calling a raw memory handler or reusing stale MCP-session
authority would bypass this contract. The callback must enforce private-session,
redaction and enrollment policy before transmitting the sensitive query to any
embedding provider. The factory does not implement or heuristically claim that
all secrets have been redacted. Its cooperative two-second context deadline
does not terminate a callback that ignores cancellation.

The callback receives fixed `budget_tokens: 512` and per-arm `limit: 8`; callers
cannot increase them. The result is the existing single-text MEMORY_VERBS v1
recall envelope, attested with the exact verified binding. Raw envelope bytes
are capped at 32 KiB; final context/output byte limits also apply, including
JSON expansion. Fact IDs/provenance and search slugs/provenance remain paired
with text. Packing metadata is retained; no text is silently truncated.

Each ordinary MCP text result contains valid native `UserPromptSubmit` hook
JSON. Handler-reachable domain failures emit `decision: block` without
`IsError`, allowing a native hook parser to consume the decision. No raw
operational error is copied into the reason. The shared MCP server forces
`IsError: true` on results from the tool's `Failure` renderer for schema
rejections, execution failures and panics. That remains true even when their
text contains valid blocking JSON. A native host may discard such an MCP error
and fail open. Authentication, network, missing-tool and unsupported-integration
errors may also fail open. This factory cannot enforce those transport/native
boundaries; an actual server-dispatch fixture records that distinction without
changing the shared MCP contract.

Successful context wraps recall data as untrusted evidence with no authority.
Status distinguishes `recalled`, `no_returned_results`, and their `_degraded`
forms. Search degradation becomes a controlled `search_status` and
`search_degraded: degraded` marker rather than a raw diagnostic. Positive
`dropped_count` remains visible. `total` counts packed facts, not all matching
candidates: an empty bounded result never establishes exhaustive no-match.
Degraded context is explicitly partial; installation policy may require the
native caller to block it rather than continue with incomplete memory.

Source review, meaningful checks, gateway ownership handoff, current-token
wiring, server deployment, authenticated synthetic acceptance, native prompt
hook parsing/exit semantics and real policy enforcement remain separate gates.
Automatic history capture and production episode enrollment are unaffected.
