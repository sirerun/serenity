# ADR 030: Zati context assembly with Serenity memory and Gist skills

Date: 2026-10-16
Status: Accepted direction for planning from the owner's Zati UX, token-efficiency
and performance requirements. Implementation and runtime qualification remain open.

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
Registry authentication and publisher provenance remain Gist concerns. Hosted
expansion remains paused except repairs and existing obligations; no shutdown.

## Delivery and evidence

Refine the existing shared-memory plan with one real Zati skill containing an
asset and an admitted capability. Reuse the existing Gist v1.2.0 ranking, Zati
context/memory/import/effect paths and Serenity retrieval/archive foundations.
Freeze observed baseline, representative workloads and numerical latency/token/cost
budgets before comparisons; preserve task correctness and authorization as hard
acceptance. Source, native preview, installed runtime and deployed service receipts
are separate. Do not implement cross-repository work without mapped owner tasks.

The implementation plan preserves all previous rows. Existing erasure, replay,
restore, 200-message continuity and host-plugin qualification are still required.
Embedding model experiments are optional, not a prerequisite for usable Zati UX;
no model download, provider spend or new service follows from this decision.

## References

- [Implementation plan](../plans/shared-agent-memory-implementation-20261016.md)
- [Memory design](../rfc/0002-long-horizon-agent-memory.md)
- [Host integration](../rfc/0003-agent-memory-plugins.md)
- [Gist consumer contract](https://github.com/sirerun/gist/blob/7ece68b47870b89bb648ea24efdcfc146b377d2c/docs/rfc-002.md)
- [Gist execution retirement](https://github.com/sirerun/gist/blob/7ece68b47870b89bb648ea24efdcfc146b377d2c/docs/adr/016-zati-library-first-rfc003-gateway-boundary.md)
