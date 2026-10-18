# ADR 030: Zati context assembly with Serenity memory and Gist skills

Date: 2026-10-16
Status: Accepted direction for planning from the owner's Zati UX, token-efficiency
and performance requirements. Implementation and runtime qualification remain open.
Aligned to upstream source on2026-10-17; this update qualifies no runtime.

## Decision

Zati owns the user experience, conversation and task context, worker selection,
skill assignment, permission checks, execution and progress. Serenity owns durable
knowledge and admitted historical evidence under existing brain grants and
RFC0002/0003 custody and deletion rules. Gist owns skill/tool discovery, complete
immutable packages and assets, exact schemas, capability resolution and provenance.
Zati's normal conversation can select these capabilities without requiring a
user to operate separate backend catalogs. Inspection remains available on demand.

Use Zati's existing context preparation as the single assembly point. Retrieve
only applicable memory and skill candidates with bounded requests and deadlines;
parallelize independent reads. Select summaries before exact package retrieval.
Account for the whole provider request and retain required content and tool pairs.
Reuse stable context and immutable artifacts, while revalidating current scope,
permissions, revocation, deletion and freshness before reuse or execution. Required
context unavailability refuses dependent dispatch; optional enrichment can be
omitted with truthful status. Background capture/indexing follows existing durable
queue acknowledgment and erasure gates rather than extending interactive latency.

Keep Gist reusable and library-first, with Zati first. A mandatory Serenity backend
is not selected. Shared retrieval code or an optional Serenity search adapter may
be introduced only after consumer evaluation demonstrates a quality, latency or
operational benefit. Preserve authoritative package/version storage independently
of generated memory summaries and search indexes. Serenity can retain source-linked
experience about an exact skill version; an inferred lesson cannot silently mutate
a skill, install an update, grant permission or certify universal trust.

Gist RFC003 execution remains retired under its ADR016: no invocation service,
provider-token vault, execution approvals/journal or utility billing is added here.
Registry authentication and publisher provenance remain Gist concerns. The2026-10-16 hosted pause is historical: Gist subsequently authorized and
recorded scoped hosted Library/registry delivery under ADR022. Consume either
the embedded catalog or explicitly selected authenticated remote service through
its qualified contract; no mandatory remote dependency or automatic fallback
follows. Revoked remote access cannot silently switch to a different catalog.
Gist execution remains retired in both modes.

## Delivery and evidence

Refine the existing shared-memory plan with one real Zati skill containing an
asset and an admitted capability. Reuse current public Gist catalog/preparation/composition APIs, Zati
context/memory/import/effect paths and Serenity retrieval/archive foundations.
The current consumer pins Gist
`v1.2.1-0.20261017083313-d0e97da28be3`; v1.2.0 ranking is historical baseline.
The existing selected inbox skill has a template in v1 and typed bounded list/read
composition in v2. Preserve explicit version selection and schema compatibility;
controlled source evidence does not qualify Gmail or default semantic adoption.
Freeze observed baseline, representative workloads and numerical latency/token/cost
budgets before comparisons; preserve task correctness and authorization as hard
acceptance. Source, native preview, installed runtime and deployed service receipts
are separate. Do not implement cross-repository work without mapped owner tasks.

The implementation plan preserves all previous rows. Existing erasure, replay,
restore, 200-message continuity and host-plugin qualification are still required.
Embedding model experiments are optional, not a prerequisite for usable Zati UX;
no model download, provider spend or new service follows from this decision.

## Current continuation and preparation owners

Zati RFC002 and contracts revision49 own bounded incremental continuation within
its existing turn/task/effect identity. Tasks supplies complete child/dependency
snapshots; Messaging supplies ordered authorized ranges and full pending-message
snapshots; Execution validates summaries and installs checkpoints at fenced safe
boundaries through its existing out-of-transaction performer. No source summary
can omit unresolved effects, reset budgets or manufacture authority. Those are
Zati-owned runtime obligations; contract delivery alone does not enable them.

Serenity stores accepted external evidence and checkpoint bytes with source
coverage, scope, provenance and deletion fences. Its archive/context protocol
must map to the actual Zati owner contract without predicting artifact IDs,
copying mutable work state into an authoritative scheduler, or replacing current
membership and pending-work validation. Zati-local continuation can be qualified
without automatically enabling Serenity history capture. Recovering after loss
of Zati's canonical history additionally needs qualified acknowledged archive
custody and a supported receiver; neither local checkpoints nor artifact links
prove that recovery.

Gist now prepares a complete admitted bundle and deterministic derived guide.
Zati supplies current task/readiness decisions, executes typed compositions and
assembles that bundle with relevant Serenity memory. A historical recipe summary
is lower-trust evidence; current exact schemas, connection/account affinity,
positive task coverage and runtime policy govern use. Do not build a second
preparer or resolver in Serenity or infer all prerequisites from similarity.

See the [pinned alignment record](../receipts/2026-10-17-gist-zati-alignment.md)
for evidence, canonical task mapping and qualification limits.

## References

- [Implementation plan](../plans/shared-agent-memory-implementation-20261016.md)
- [Memory design](../rfc/0002-long-horizon-agent-memory.md)
- [Host integration](../rfc/0003-agent-memory-plugins.md)
- [Gist consumer contract](https://github.com/sirerun/gist/blob/7ece68b47870b89bb648ea24efdcfc146b377d2c/docs/rfc-002.md)
- [Gist execution retirement](https://github.com/sirerun/gist/blob/7ece68b47870b89bb648ea24efdcfc146b377d2c/docs/adr/016-zati-library-first-rfc003-gateway-boundary.md)
