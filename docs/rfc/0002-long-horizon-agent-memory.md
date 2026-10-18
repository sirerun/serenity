# RFC 0002 — Long-horizon conversation memory

Date: 2026-10-16

Status: Proposed design, based on owner-confirmed product requirements. No
implementation, deployment, capacity, or release qualification is implied.
Extends [RFC 0001](0001-serenity.md); companion:
[RFC 0003](0003-agent-memory-plugins.md).

## 1. Outcome and confirmed requirements

Serenity is the shared memory system for the owner's AI agents across coding,
marketing and other work. Conversations and projects can continue for months
or years, across host changes and loss of a host's local history. Zati is the
first runtime integration; the public design is agent-neutral from inception.

The owner confirmed:

- Hosted Serenity is the default across computers, with a local queue during
  outages. Preserve the standalone local product and public interfaces.
- Enrolled agents automatically import existing histories and capture new
  conversations and tool evidence. A standing policy also permits saving useful
  long-term facts automatically.
- Capture full visible text and attachments where the host exposes them. Report
  omissions rather than silently claiming a complete archive.
- Retain admitted evidence until explicitly deleted, subject to visible storage
  limits. Summarization is not permission to discard originals.
- Shared personal knowledge and project/workspace organization coexist. Reuse
  existing brains/grants for restricted material; no new within-brain ACL system.
- Cross-agent continuation is a core feature. The receiving agent rechecks
  permissions, files and external state before acting.
- Exclude credentials, secret files and explicitly private sessions by default;
  provide pause controls and redact before upload.
- Extend existing Serenity incrementally; do not replace its storage or adopt
  another memory product.

“Long-horizon” means growing retained history with bounded working context.
It does not mean infinite storage, perfect recall, free processing, or the
ability to reproduce a vendor's hidden reasoning or native session state.

## 2. Current implementation and gaps

The baseline reviewed for this RFC is Serenity commit `67c67e13`. Source
inspection establishes these capabilities, not their live deployment state:

| Existing foundation | Reuse and limitation |
| --- | --- |
| Content-addressed sources and serialized canonical writer | Reuse for admitted episode records and attachment metadata; no independent plugin writer |
| Git-canonical files; derived SQLite search | Preserve; benchmark episode batching, repository growth and rebuild cost |
| Generic FTS/vector/RRF search | Reuse engine components; current governed HTTP recall is a separate lexical scan, so parity needs implementation |
| Source/observation/claim/precept distinction | Extend with typed conversation evidence; an utterance does not establish its truth |
| Temporal claims and supersession | Reuse for derived durable knowledge; ingestion date must not overwrite historical event time |
| Hosted account/brain credentials and verified binding | Reuse authorization boundary; labels within a brain do not restrict access |
| Governed command identity and status | Reuse patterns, not an assumed archive protocol; current artifact references do not copy bytes |
| Entity consolidation | Current deterministic summaries are not a conversation-summary hierarchy |

[Governed memory v1](../protocol/GOVERNED_MEMORY_v1.md) explicitly lacks
promotion and immutable revision export. Its retraction removes active recall,
retains history, and cannot substitute for archive erasure. Current hosted
backup/export limitations remain as documented in
[operator forget notes](../operator/forget.md).

The initial Zati baseline rejects histories exceeding its 200-message admission
bound. As of2026-10-17, Zati has landed a bounded continuation contract, not full
runtime qualification. Its own messaging/tasks/execution owners implement that
path; Serenity archive/context integration consumes the owner contract. Neither
contract publication nor a Serenity MCP connection removes the limit. See §14.

## 3. Responsibilities and boundaries

**Serenity** owns accepted archive copies, provenance, scoped search, derived
summaries, durable memory, retrieval budgets, archive export, erasure and their
receipts. Canonical here means authoritative evidence of what was recorded,
not authority for the content's assertions.

**The host/runtime** owns the live conversation, tool execution, current task
state, model context window, permissions and approval decisions. It compacts
its working context and uses Serenity to retrieve historical material.

**The plugin** adapts supported host lifecycle events and visible records to a
shared Serenity client. It manages enrollment, capture progress, an outage
queue and context injection. It never writes directly into brain storage or
restores old execution privileges.

For an acknowledged event, Serenity's archived copy survives loss of the
originating host. Before acknowledgment, the producer remains responsible for
its durable queue. A crash may lose data the host never exposed or the plugin
never durably queued; capture receipts must make this coverage boundary visible.

## 4. Reuse existing scope boundaries

One personal brain is shared across the enrolled agents. Stable project and
workspace identifiers focus search and link relevant work across computers;
they are organizational metadata, not security boundaries or filesystem paths.
Explicit owner mappings associate local workspace paths with those identifiers.
Use distinct field names for organizational workspace/project labels and the
existing hosted `project_id` (brain ID). A caller cannot turn an organizational
filter into a brain selector. Unmapped or conflicting workspaces pause capture
admission/upload until resolved; the hosted personal default is not permission
to capture an arbitrary newly discovered workspace.

Restricted projects use separate existing Serenity brains and credentials.
Every server request remains bound to its authenticated brain. A client can
query personal and project brains separately only with current grants for
each. There is no new cross-brain server super-token or implicit inheritance.

Copying or promoting restricted evidence into another brain is a separate,
explicitly authorized transfer. It preserves provenance and reports the new
copy's retention consequences. This RFC does not claim that current governed
promotion implements that transfer; it stays unavailable until qualified.

Legacy unrestricted agents of a brain must never be presented with a false
promise of project isolation. A restricted project must not share that brain.
See [ADR 028](../adr/028-shared-brain-scope-and-host-adapters.md).

## 5. Capture and epistemic policy

Enrollment records a versioned standing policy: selected hosts and existing
history coverage, brain/workspace mappings, automatic capture and fact-saving
rules, exclusions, permitted provider processing, retention, storage and spend
limits. Once enrolled, ordinary covered capture and fact saves do not ask for
approval per message. Changed disclosure destinations or broader scope require
current authority; content from a repository cannot enroll itself or change
policy. Pause/private mode stops future capture and removes covered unpublished
queue items. Already published history is not made private by a local toggle;
the control must show that fact and provide the separate deletion operation.

Capture includes visible user/assistant messages, tool requests and outcomes,
and exposed attachments. Hidden chain-of-thought, credentials, secret-file
contents and explicitly private sessions are excluded. Hooks must not harvest
arbitrary files merely because a path appears in a message. Use host-exposed
attachment handles or separately authorized source access. Default filtering
applies before durable plugin staging and upload. Redaction is best effort,
not a guarantee of detecting every secret; uncertain secret-file inputs are
excluded, with content-free gap reasons.

Preserve the roles and timestamps reported by the host as attributed evidence.
An agent-supplied `role=user` or quoted approval never becomes a trusted human
principal, a precept, or permission to act.

The standing policy permits selective automatic durable fact capture, not
automatic acceptance of all assistant claims. Stable owner preferences,
decisions and supported lessons can be recorded with source attribution and
uncertainty. Established reconciliation and earned-automation rules govern
activation/conflicts. Precepts retain their separate human-disposition rule.
Routine evidence capture must not flood the disposition inbox; unresolved
semantic conflicts can remain explicitly uncertain. Add an explicit episodic
extraction path: existing raw memory facts are intentionally excluded from
automatic extraction. Do not treat saved facts, injected recall or summaries as
new independent evidence and create a self-reinforcing memory loop.

## 6. Archive model and publication

Introduce a versioned episodic source kind in the existing SourceStore. An
episode is a bounded batch of visible events, not a lossy substitute for them.
Do not create a second authoritative memory database.

| Record | Required semantics |
| --- | --- |
| Producer | Stable client/installation identity and host/version; authenticated grant remains the authority |
| Conversation | Stable producer session identity, optional explicit cross-host continuation links, project metadata |
| Event | Stable producer event ID, local sequence, role/type, event time and observation time, exact admitted text, attachment references, attribution |
| Episode | Ordered event range, content digest, policy version, exclusion/gap metadata, publication receipt |
| Attachment | Admitted bytes, digest, media type/size, source-event linkage, exclusions and capture state |
| Checkpoint | Exact source ranges/revisions, pending-work description, summary/model pin, creation state and continuation links |

Per-producer order is preserved. Concurrent agents are separate streams; wall
clock order does not invent causal dependencies. Explicit handoff/fork links
record causality. Re-import and live capture use the same event identity map;
matching content alone must not collapse independently authored occurrences.
Identical retries replay the original receipt; reuse of an event/command ID
with changed normalized content conflicts. Host logs lacking stable IDs require
a documented deterministic identity algorithm and rotation/rewrite detection.

Acknowledgment requires canonical publication, not merely a queued write or
completed summary. Bound batches and attachment staging; serialize publication
through the existing writer. Provider processing stays outside the commit
guard, consistent with ADRs 017 and 025. Derived indexing can lag and report its
watermark. An event receipt distinguishes text publication from each attachment's state;
only a receipt covering every admitted component can claim a complete episode.
An attachment is complete only when its admitted bytes are durable;
an unavailable URL or original host path is not a preserved attachment.
Capture receipts bind stable logical IDs and content digests, not solely Git
commit hashes: physical history erasure can rewrite those hashes. A retained
record's logical identity survives maintenance; deleted content stays unavailable.

Keep canonical text/metadata in existing files/Git with bounded segmentation
and batched commits. Storage admission includes history, staging, indexes and
derived work. Initial attachment storage reuses qualified existing source
facilities within their measured size limits. Larger binaries are visibly
pending/unsupported until a separately specified durable blob/export/erasure
path is qualified; silently setting `index_only` does not meet recovery.

## 7. Retrieval and hierarchy

Provide direct historical search and exact authorized event/attachment reads
before depending on any hierarchy. Reuse lexical and optional pinned-vector
retrieval; do not mix embedding versions. Distinguish recall of what was said,
current accepted knowledge and the state known at a historical time.

Add linked episode/topic/time summaries as derived retrieval aids. A summary
records source ranges/digests, policy and model versions, and observed coverage.
Its provenance must reach original evidence, even if higher levels use lower
summaries. Never treat a summary of summaries as the only remaining evidence.
Missing or invalid summaries do not hide eligible original records.

Compare direct hybrid retrieval, flat episode summaries and hierarchical
navigation on the same held-out workloads. Enable hierarchical routing only
when it improves the selected quality/cost/latency tradeoff without unacceptable
historical-detail loss. Always retain a direct-search route for omitted details.
Tree depth alone is not a bound on total CPU, index memory or maintenance cost.

Bound retrieval by requested bytes/tokens, result count, elapsed work and
provider spend. Return citations, actual selected ranges, archive/index
watermarks, estimator identity, omissions and degradation. “No match” does not
prove no such event exists. Coverage means known captured/selected ranges, not
an unsupported claim of exhaustive semantic recall.

Recheck current authorization and deletion state before returning sources,
summaries or cached context. A derived artifact cannot outlive its source
eligibility. Query expansion and model synthesis also obey provider egress
policy; a summary is not automatically safe to disclose.

## 8. Public extension contract

Add an agent-neutral, versioned episode/context protocol beside existing
MEMORY_VERBS and governed memory. Preserve their frozen request/response
semantics. The names below are conceptual operation families, not claims of
implemented tools; implementation must freeze bounded schemas and conformance
fixtures before host packages depend on them.

- **Append/status:** idempotent bounded event batches and attachments; current
  brain grant, stable request identity, publication receipt and explicit gaps.
- **Search/read:** authorized historical search and exact original reads with
  pagination, date/project/conversation filters and no hidden model requirement.
- **Context/checkpoint:** budgeted retrieval and persisted continuation evidence,
  with source references, current-state qualifications and unsupported fields.
- **Export:** paginated portable archive with manifest, bytes/digests, policy
  exclusions and consistency boundary. Distinct from native host-session replay
  and from existing account export's unqualified historical behavior.
- **Fence synchronization:** current deletion/policy generation and bounded
  changes or a reset-required response for an expired cursor; authenticated to
  the exact brain. No upload can declare itself exempt from server fences.
- **Delete/status:** explicit target selection, immediate retrieval exclusion,
  queued-copy fences, derived invalidation and separately reported physical
  purge progress/retained-copy limits.

Use current authenticated brain read/write grants initially. Granting a legacy
client brain read access must be understood as trust in that brain, not enforced
project restriction. Advertise extension capabilities explicitly. Do not expose
transcripts by repurposing existing fact responses or silently altering old
`recall` results. Each transport uses the same owner logic and authority checks.

Missing status is unknown, not proof of nonexecution. Retry reconciliation must
never silently repeat a charged mutation or mint a second event identity.

## 9. Context compaction and continuation

Zati must budget the entire provider request, reserve output space, and compact
before admission failure. Preserve recent exchanges, active instructions and
intact tool-call/result pairs. Pin continuation summaries to acknowledged
archive ranges before relying on them to replace working context. Do not
rewrite or discard the original conversation to achieve compaction.

Other hosts retain control of their own context windows. Plugins checkpoint
and retrieve at supported lifecycle events; they cannot promise to override a
host's compaction algorithm. Where the host cannot checkpoint before eviction,
report that limitation and rely on previously captured events, not an invented
complete transcript.

A handoff exports goals, constraints, decisions, progress, unresolved questions,
pending work and source links. The receiving agent must verify current files,
environment, credentials and approvals. Pending tool calls are historical
intent, never runnable replay. A saved claim that a deployment succeeded is
evidence to check, not proof of current deployment state.

## 10. Retention, deletion and outage recovery

No automatic age-based loss of admitted originals. Reaching storage or provider
limits stops the affected admission/processing with visible progress and a
recoverable backlog; never silently evict old evidence or promise unbounded
local buffering. Capture and summary budgets are separate so unavailable
summarization does not falsely make successfully archived evidence unavailable.

The local queue is access-restricted, protected at rest, bounded and retry-safe.
It retains only policy-admitted content. Every replay rechecks grant and policy
version. Revocation, brain remapping, private-session changes and deletion must
prevent stale queued/imported events from reappearing. Reconnect first verifies
binding and downloads/applies current fences, then drains the queue. If that
step fails, uploads stay paused. Server admission and canonical publication
independently recheck the current deletion generation and each event's fence,
closing the race where deletion occurs after client synchronization. Expired
fence cursors require a complete reset/synchronization, never a bypass.
No automatic rerouting
to the personal brain when a restricted brain becomes unavailable.

Deleting an episode/conversation immediately excludes its records and affected
derived nodes from reads, including stale indexes and in-flight publication.
Physical erasure must cover canonical bytes, derived summaries, embeddings,
history, managed exports/backups and controlled queues according to the
disclosed erasure contract. Removal from search is not a purge receipt.
Batch physical Git-history cleanup
in serialized, fenced maintenance, not an unbounded rewrite on every interactive
delete. Publish its schedule/deadline and backlog. Canonical exclusion applies
throughout maintenance; paused writes use bounded queues. Restore and export
must apply current fences, so old revisions cannot resurrect deleted evidence.

Dependent claims lose withdrawn provenance; only independently supported claims
may remain usable. Shared attachment references require explicit accounting:
conversation deletion removes its references; content-wide deletion identifies
all controlled references. The deletion UI/receipt must state any independently
retained copies. No derivative summary may preserve the deleted content under
another name. Offline devices receive deletion fences before upload/retrieval;
unreachable devices and user-held exports are reported, never claimed erased.

Governed command history needs a new compatible erasure design before archive
APIs can offer physical deletion: retain only minimal content-free replay
fences/receipts, not original text or secret-bearing request envelopes. This
refines rather than weakens ADR 019 and does not change governed v1 retraction.

## 11. Incremental delivery and acceptance

These are extension gates, not replacements for RFC 0001's existing gates.
Targets below are proposed qualification workloads, not measured capacity.

1. Freeze episodic schemas and policy semantics; reuse brain/source/writer
   components. Qualify retry/restart, exact read/export, exclusions, attachments
   within declared limits, deletion and bounded offline backlog.
2. Integrate Zati context/checkpoint consumption and demonstrate continuity
   across the current 200-message cliff and model input limits. A restart and
   loss of the originating runtime must not lose acknowledged eligible history.
3. Qualify the four host plugins in RFC 0003: historical import, continuous
   capture and cross-agent handoff are required outcomes, not optional extras.
4. Evaluate flat versus hierarchical retrieval; ship the simplest qualified
   strategy. Large-scale optimizations require measured need and separate ADRs
   if they change canonical storage or authorization.

The test matrix includes concurrent producers, duplicates, log rotation,
interrupted imports/uploads, redaction, attachments, quota exhaustion, cold
rebuild, temporal corrections, conflicting agents, rare old details, malicious
quoted instructions, restricted-brain leakage, revoked credentials, deletion
during capture/search, stale summaries and offline-device resurrection.

Evaluate at 10K, 100K and 1M visible events with text/attachment byte volumes
reported separately, including simulated multi-year conversations. Publish
recall precision/recall, answer provenance, continuation correctness, p50/p95
latency, CPU/RAM, repository growth, rebuild/purge time and cost per workload.
Large synthetic runs do not prove years of reliability; add real soak evidence.
Capacity and latency ceilings must be declared before qualification runs rather
than selected afterward to fit results. Any failed workload bounds the support
claim; it must not be hidden by a successful smaller run.

## 12. Decisions and references

- [ADR 027](../adr/027-episodic-memory-and-recoverable-history.md): archive
  custody, epistemic separation, derived hierarchy and erasure consequences.
- [ADR 028](../adr/028-shared-brain-scope-and-host-adapters.md): existing scope
  boundaries and a shared client with host-specific plugin adapters.
- [IHMT](https://github.com/gonzaroman/IHMT-MEMORY): inspiration for navigable
  hierarchies, not a dependency or evidence that tree routing beats Serenity.
- [ADR 029](../adr/029-episodic-erasure-and-replay-fences.md): deletion and
  non-resurrection across derived records, imports and offline queues.
- [Design review record](../reviews/shared-agent-memory-20261016.md): owner
  decisions, Quorum findings and validation boundaries.

## 13. Zati context and skill integration refinement — 2026-10-16

[ADR 030](../adr/030-zati-context-and-gist-boundaries.md) records the owner's UX,
token-efficiency and performance priorities. Zati owns one bounded context
assembly path combining relevant Serenity knowledge with exact Gist skills and
capability requirements. Skills remain authoritative packages rather than memory
snippets; experience links to their versions. Gist stays reusable and library-first
without a required Serenity backend. Context reuse must preserve current brain,
permission, deletion, revocation and freshness checks. Evaluate task correctness,
full-request tokens/cost and interactive latency together before adopting more
retrieval infrastructure. E4 in the existing implementation plan adds this
acceptance without replacing archive, erasure or host qualification gates.

## 14. Current upstream ownership and preparation — 2026-10-17

[ADR030](../adr/030-zati-context-and-gist-boundaries.md) and the
[pinned alignment record](../receipts/2026-10-17-gist-zati-alignment.md) supersede
older first-consumer discovery assumptions. Zati's focused-worker organization
RFC and revision49 continuation contract own incremental history ranges, complete
work and pending snapshots, bounded checkpoint-model calls, validated source
coverage and atomically fenced segment transitions. These preserve the same
turn/task/run and effect identities, cumulative budgets and unknown outcomes.
Source reauthorization and current owner snapshots precede summary disclosure
and installation. Canonical live messages stay with Zati; Serenity archive
copies become durable only after its own canonical acknowledgment.

Keep local continuation and archive recovery separately usable and qualified.
A runtime may compact its authorized local context while unqualified Serenity
capture stays off. Loss-of-host recovery, cross-agent continuation and retained
originals still require archive/import/export and receiver evidence. T2.2 now
owns that bridge and consumes Zati's eventual E-FO1 runtime receipts. No second
checkpoint builder or task scheduler is implemented in Serenity. Host summaries
remain attributed evidence and cannot become current approved knowledge simply
because a hierarchy or worker title says so.

Gist's public preparation/composition packages already provide selected exact
skills, required tool contracts, admitted bounded dataflow and derived guides.
Zati verifies current task fit, schemas, account/connection affinity and effects;
Serenity contributes relevant knowledge and source-linked experience about those
exact versions. Catalog similarity never establishes task fit or memory access.
Do not archive provider secrets or use old mailbox output to fake current tool
readiness. Mail/content retention follows the actual producer consent and archive
policy; using Gist does not itself authorize capture.

Gist's hosted Library has separately scoped deployment receipts. It remains
optional for this architecture and contributes no execution authority. Reuse its
existing bounded catalog/index evaluation only within its published workload;
Serenity episodic hybrid quality, full-request tokens, native latency and
cross-agent safety require their own measurements. Optional embedding adapters
stay caller-selected and evaluated, with no new adoption or launch from this RFC.
