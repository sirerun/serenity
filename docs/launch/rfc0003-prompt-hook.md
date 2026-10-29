# Prompt-hook adapter seam

`internal/agentmemory.Handle` accepts a native `UserPromptSubmit` JSON input,
an explicit host (`codex` or `claude-code`), a trusted expected account/brain/
workspace binding, and an injected recall client. It emits structured native
hook output for bounded relevant context, or a fixed `decision: block` reason
when verification or recall fails. A verified complete no-match is distinct
from unavailable recall. Returned evidence keeps its source citations and is
encoded as untrusted reference data with no effect authority.

The client must perform authenticated current binding verification and governed
recall as one trusted operation. Results must match the caller's exact expected
binding and explicitly attest verification. Input hook JSON cannot supply this
authority; `cwd`, session and turn identifiers are not enrollment or a trusted
workspace mapping. The package neither opens transcript paths nor interprets
recalled text as instructions. Common extra native event fields are ignored;
duplicate top-level keys, an explicit unsupported schema, a different event,
invalid UTF-8 and malformed/trailing input are refused.

The current prompt becomes the client's query, which is sensitive outbound
data even without capture. The injected client must enforce current enrollment,
private-session exclusions, redaction and standing policy before any network
request. This package does not implement those controls. It provides no default
network client, token accessor, configuration loader, enrollment, executable,
installation or history-capture path. Passing arbitrary caller-provided data as
`Verified` is not authenticated acceptance.

Limits are 64 KiB input, 8 KiB prompt, 32 evidence entries, 12 KiB final context
and 32 KiB serialized output. Budgets count UTF-8 bytes and JSON expansion;
oversized evidence is refused rather than silently truncating citations.
The two-second recall deadline is passed to the injected client, which must
cooperate with context cancellation and bound its own I/O and allocations.
Late results are refused. No leak-prone goroutine attempts to kill a client
that ignores cancellation. The caller owns bounded input-stream delivery.

A future executable must emit the generated JSON and **exit zero when Handle
returns ErrBlocked**, allowing native clients to process `decision: block`.
Mapping that sentinel directly to a nonzero exit may discard the structured
decision. An output-writer failure means valid blocking JSON is not guaranteed;
that integration failure needs native-specific handling with fixed diagnostics.
No raw prompt, backend error, credential or transcript fragment belongs in a
failure diagnostic.

Codex 0.165.0 and Claude Code 2.1.309 are observed integration inputs. Official
contracts inspected by the coordinator describe `UserPromptSubmit` context
output and a top-level blocking decision:
[Codex hooks](https://learn.chatgpt.com/docs/hooks) and
[Claude Code hooks](https://code.claude.com/docs/en/hooks).
These contracts and package tests do not establish runtime enforcement in an
installed client. Actual secure client custody, hosted authentication/recall,
private/redaction checks, native exit behavior, trust, coexistence with existing
hooks, and synthetic end-to-end qualification remain separate gates. Existing
synthetic-only episode capture stays disabled for real histories.
