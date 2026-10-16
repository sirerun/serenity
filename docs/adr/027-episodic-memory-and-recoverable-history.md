# ADR 027: Extend Serenity with recoverable episodic memory

Date: 2026-10-16

Status: Proposed architecture. Product requirements confirmed by the owner;
implementation and qualification are pending. No deployment or spending
authorization follows from this record.

## Context

The owner wants all coding, marketing and other agents to share Serenity memory,
continue conversations for months or years, import existing histories, and
recover work in another agent after losing host-local history. Hosted Serenity
is the default. Automatic capture and useful fact saving operate under a
standing policy. Retention lasts until explicit deletion within visible limits.

The existing content-addressed source store, single writer, Git publication,
derived indexes and epistemic layers are valuable foundations. Existing memory
facts and governed artifact references do not constitute a full conversation
archive. Repeatedly replacing older history with a summary loses details and
cannot meet the recovery requirement.

## Decision

1. Add typed episodic source records to existing canonical storage through the
   existing writer. Preserve admitted visible text and exposed attachment bytes
   with stable identities, provenance, policy versions and coverage gaps.
   Serenity assumes custody after canonical acknowledgment; producers retain
   unacknowledged content in a protected bounded queue.
2. Treat captured statements and continuation checkpoints as attributed
   evidence, not accepted beliefs, precepts or execution authority. Add explicit
   policy-gated extraction for episodic sources. Do not recursively extract
   saved memory statements, retrieved context or summary output as independent
   evidence. Current precept approval and reconciliation rules remain.
3. Use direct historical retrieval and exact reads as the baseline. Derived
   episode/topic/time summaries may improve navigation but cannot replace the
   originals or become the sole route to them. Measure hierarchy against flat
   summaries and existing retrieval before enabling it.
4. Publish an additive agent-neutral episodic/context protocol. Preserve
   MEMORY_VERBS v1 and governed-memory v1 semantics. Capturing attachment bytes,
   exporting a recoverable archive and physically deleting it are new qualified
   capabilities, not inferred from old operations.
5. The runtime owns live execution and model context budgeting. Serenity supplies
   bounded historical context and recoverable checkpoints. Zati is the first
   integration; other hosts use their supported lifecycle surfaces.
6. Preserve Git/files as canonical and SQLite as derived. Use bounded source
   segmentation and publication batching; do not adopt another memory product
   or introduce a replacement storage engine without measured need and a new
   decision. Attachment support is explicitly limited to qualified storage,
   export and purge paths.

## Alternatives considered

- **Install IHMT alongside Serenity:** creates another memory authority and
  synchronization problem. Borrow hierarchical navigation ideas, not a second
  product dependency.
- **Only store summaries or links to host logs:** cannot recover missing host
  history or omitted details; rejected for the confirmed recovery requirement.
- **Replace the substrate immediately:** a large rewrite without capacity
  evidence; rejected. Benchmark writer, search, history and purge costs first.
- **Make the tree the only retrieval path:** summary omissions can make retained
  details unreachable in practice; rejected.

## Consequences

Storage, ingestion and erasure load grow. Published capacity limits and visible
backlogs are essential; “infinite” is not a service guarantee. Capture can remain
available without model-backed extraction. Logical records remain immutable
until a governed purge; this does not exempt them from deletion.

Existing source eligibility does not automatically implement episode extraction,
host import, attachment custody or export. All require implementation and
acceptance evidence. Source-linked summaries can be rebuilt, but stochastic
model regeneration must not promise byte-identical output: preserve exact
checkpoint bytes and model/source provenance when replaying a saved checkpoint.

## Related decisions

[RFC 0002](../rfc/0002-long-horizon-agent-memory.md),
[ADR 028](028-shared-brain-scope-and-host-adapters.md),
[ADR 029](029-episodic-erasure-and-replay-fences.md), ADRs 004, 012, 017, 025.
The read-only embedded facade stays read-only; new writes still use the writer
service. Provider work stays outside canonical commit sections.
