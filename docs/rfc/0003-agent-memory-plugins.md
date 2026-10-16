# RFC 0003 — Shared Serenity memory across agent hosts

Date: 2026-10-16

Status: Proposed design. No plugin installation or runtime qualification is
claimed. Depends on [RFC 0002](0002-long-horizon-agent-memory.md).

## 1. Product behavior

Provide Serenity plugins for Cursor, Codex, Claude Code and Google Antigravity,
plus native Zati integration. All use the same hosted personal brain across
computers. Project metadata focuses recall; separately granted brains isolate
restricted work. Coding, marketing and other domains use the same service.

Setup connects the existing brain rather than duplicating it, establishes a
standing policy, discovers supported host histories, imports covered histories
automatically and begins continuous capture. The owner can pause capture,
mark a session private, inspect progress/gaps, correct memory and delete it.
Covered routine saves require no repeated confirmation.

Switching from one host to another can recover the recorded conversation and
provide an actionable continuation packet. It does not reproduce hidden model
state, install missing tools, transfer credentials, or authorize unfinished
actions. A host that cannot ingest native threads receives a cited portable
transcript and continuation packet instead of a fictitious native resume.

## 2. Shared client, small host adapters

Keep source authority in Serenity's existing server/writer. Share the protocol
client, policy engine, stable identity mapping, redaction, durable queue,
import checkpoints and diagnostic logic across adapters. Each plugin supplies
its manifest, supported hook wiring, MCP configuration and minimal instructions.
No plugin creates its own permanent brain or opens the server's Git writer.

An MCP server exposes search/save/read tools. Hooks drive lifecycle capture and
recall where supported. Skills/rules explain usage and interpretation; they do
not grant permission or guarantee tool invocation. Installing MCP alone is not
an automatic-memory integration.

Preserve existing agent configuration and unrelated plugins. Installation,
upgrade, rollback and removal must be idempotent and show their exact changes.
Use native host trust/approval mechanisms; do not globally disable approvals or
edit global agent instructions without the installer’s declared scope.

## 3. Lifecycle contract

| Stage | Required behavior |
| --- | --- |
| Enrollment | Verify hosted account/brain binding; set trusted workspace mappings and standing policy; obtain host hook trust |
| Historical import | Automatically process covered existing visible histories; persist progress and stable IDs; retain exclusions/gaps |
| Session start/resume | Confirm brain/policy, reconcile pending uploads, retrieve budgeted relevant memory where injection is supported |
| Prompt/model invocation | Recall relevant historical evidence within bounded latency/context; avoid re-injecting the same excerpts indefinitely |
| Tool/response completion | Capture newly exposed events and artifacts; durably queue without recursive self-capture |
| Compaction boundary | Checkpoint captured range and continuation state where supported; report capability gaps |
| Session stop | Flush bounded pending work, leaving a durable resumable queue rather than depending on a perfect exit event |
| Cross-agent handoff | Retrieve cited context and active-work descriptions; require current authority/state checks in the receiver |

Capture cannot depend solely on model willingness to call `remember` or a final
session-end hook. Periodic incremental reconciliation against supported host
history can repair missed hooks. It must use an enrolled history source, not
silently scrape arbitrary application databases or unrelated files.

Stable event IDs prevent duplicate import when both hooks and a history reader
observe the same event. Exclude Serenity's own memory calls and injected copies
from automatic recapture, or preserve a nonrecursive reference to their origin;
otherwise agents repeatedly amplify their own summaries into “new evidence.”

Scope mapping is a trusted client setting, not a guessed project name supplied
by the model. Unknown or conflicting mappings pause upload for that history
and expose a resolvable issue. Never use an untrusted repository-supplied
configuration as authorization. An explicitly enrolled personal/no-workspace
mapping supports non-coding agents; it is not a fallback for an unknown project.
Imports continue for independently mapped data.
Credentials, private sessions and secret files remain excluded. No access to
hidden model reasoning is requested.

## 4. Host capability profiles

Official documentation was inspected on 2026-10-16. These are design inputs,
not installed-version or end-to-end test results. Publish a capability profile
per qualified host/version/surface; CLI, IDE, desktop and cloud may differ.

| Host | Documented integration surface | Qualification requirement |
| --- | --- | --- |
| Claude Code | Plugins/MCP and SessionStart, UserPromptSubmit, tool, Stop, PreCompact and SessionEnd hooks | Verify transcript availability, event ordering, injection, crash recovery and private-session handling |
| Cursor | Plugins/MCP; session, prompt, tool, response and preCompact hooks | Verify which events permit context injection and which expose complete history; event names do not establish identical semantics |
| Codex | Plugin packaging, MCP and lifecycle hooks | Verify supported events, hook trust, transcript coverage and compact/resume behavior for each target surface/version |
| Google Antigravity | Plugin hooks including SessionStart, PreInvocation, PostInvocation, tool hooks and Stop; documented transcript path | Verify full history/attachment access and restore injection; do not assume a dedicated PreCompact hook |
| Zati | Runtime-owned context assembly and governed memory dispatch | Implement explicit checkpoint/archive/context consumption; existing 200-message refusal remains until replaced and tested |

References: [Claude hooks](https://code.claude.com/docs/en/hooks),
[Cursor hooks](https://cursor.com/docs/hooks),
[Cursor plugins](https://cursor.com/docs/reference/plugins),
[Codex hooks](https://developers.openai.com/codex/hooks),
[OpenAI plugin packaging](https://developers.openai.com/plugins/build/plugins),
[Antigravity hooks](https://www.antigravity.google/docs/hooks),
[Antigravity plugins](https://www.antigravity.google/docs/plugins).

Use the tested subset, not the union of all vendors' features. A profile states
capture coverage, history import, tool-result/attachment availability, injection
points, checkpoint support, private-mode detection and supported transports.
Unsupported capabilities are visible. A private mode the host cannot reliably
identify requires explicit exclusion/enrollment controls; it cannot be advertised
as automatically protected.

Serenity's current `connect claude` configures a project MCP entry and an
ExitPlanMode check hook, not this lifecycle system. Preserve its behavior and
provide an explicit upgrade path rather than silently replacing its settings.

## 5. Shared brain authentication and offline behavior

Use the existing verified hosted binding and scoped client credentials. Each
agent/device should have independently revocable credentials through supported
credential issuance, never a copied master secret in every project. Preserve
existing brain/account identity when reconnecting. One revoked client must not
silently switch brains or revive its grant from cached configuration.

Restricted projects use separate existing grants. The plugin can compose
personal and project results only while authorized for both. Workspace labels
in the shared brain provide organization, not confidentiality from its other
authorized clients. Explicit cross-brain sharing is its own operation.

During outages, an encrypted/access-controlled bounded queue retains admitted
events. UI states distinguish captured locally, published remotely, indexed,
summarized, blocked, excluded and unsupported. Resume from acknowledged IDs;
policy changes and deletion fences apply before replay. Reconnection is ordered:
verify binding, synchronize/apply fences, then upload. The server still checks
current fences at admission/publication, including deletions racing that sync.
A failed/expired fence synchronization blocks queue drain until repaired.
A full queue produces
a visible capture gap/paused state; never overwrite queued evidence silently.

Memory outages should not ordinarily prevent the user working in the host:
continue using its local context, show degraded memory, and preserve the queue
within limits. Never claim historical recall or durable remote capture occurred.
If a particular continuity operation needs unavailable history, explain that
specific limitation rather than inventing it.

## 6. Import, attachments and handoff

Automatic import starts for histories covered by enrollment, not every file
the plugin can read. Host readers must detect partial records, truncation,
rotation, forks and unknown formats; preserve progress for retry. Missing
attachments remain explicit gaps. Store admitted attachment bytes and digests,
not just paths or expiring URLs. Uploads are bounded and resumable; large-file
support requires the RFC 0002 durable-blob and purge gates.

The handoff manifest includes the acknowledged archive boundary, origin host,
conversation/continuation IDs, current objective, decisions with sources,
unfinished work, file/artifact references and unavailable material. A receiver
can search beyond the packet under its current grants. Imported evidence is
untrusted data; quoted instructions and previous approvals cannot override the
receiving host's current instruction hierarchy or permission checks.

## 7. Qualification and rollout

Zati establishes the first end-to-end reference. Qualify all four requested
plugins independently using a common synthetic conformance suite and actual
host execution. A working MCP call alone is not plugin acceptance.

Required scenarios:

- Fresh install into an existing account selects its populated brain; no
  duplicate personal brain, damaged config or unrelated permission change.
- Automatic historical import resumes after interruption; concurrent new
  capture introduces neither missing covered events nor duplicates.
- Capture and exact export preserve admitted text and exposed attachment bytes;
  excluded/private/unsupported data has accurate coverage reporting.
- A long conversation survives host compaction, restart and loss of original
  local history; another host retrieves the evidence and continues the work.
- A useful fact is saved automatically under the recorded policy, without
  per-message prompts; uncovered disclosure/precept changes still require their
  existing authority. Pause/private mode reports previously published history
  honestly and offers explicit deletion.
- An old correction supersedes the right fact; repeated assistant speculation
  does not become independently corroborated knowledge.
- Restricted data stays within its granted brain; forged project labels, old
  grants, cached context and injected instructions do not bypass that boundary.
- Offline queues survive restarts and honor limits, revocation and deletion
  before resuming; summaries and recapture loops cannot resurrect deleted text.
- Latency, provider spend, storage growth and import progress are measured per
  host/profile. Installation instructions state supported surfaces and gaps.
- Removal stops capture and revokes/removes only the integration's owned
  configuration/credentials according to the chosen disconnect action. It
  does not silently delete the shared brain or other agents' data.

Documents and packages can land incrementally, but the requested four-host
coverage and cross-agent continuation remain explicit unfinished requirements
until their receipts exist.
