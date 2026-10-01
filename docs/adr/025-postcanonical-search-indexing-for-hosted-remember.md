# ADR 025: Postcanonical search indexing for hosted remember

Date: 2026-10-01

Status: Accepted for the locally reviewed recovery implementation by the
current recovery coordinator, within the founder-authorized development scope.
This is not a claim that ADR 017's original architecture reviewer approved the
new ordering, or that storage, recovery, provider or launch gates are accepted.

## Context

[ADR 017](017-hosted-operation-and-recovery-contracts.md) requires a narrow
canonical commit section and rejects holding it through slow provider I/O.
Its operation-accounting decision also says embedding and provider calls
happen before that section. Hosted remember's fact payload contains the
validated fact and provenance, not an embedding. MEMORY_VERBS already permits
remember to succeed with a degraded or unavailable search index, and optional
embedding failures are recoverable cache failures.

The old sequence wrote the source, attempted optional indexing, then flushed
Git. That made the durability boundary depend on a later cache step. The
reviewed sequence commits the source before optional indexing and preserves
its trusted durable identity even if subsequent work fails. It meets the
provider-exclusion rule, but needs this explicit refinement of ADR 017's
literal ordering.

## Decision

For hosted remember, validation, request identity and canonical fact rendering
remain required publication work. The shared commit section spans the first
canonical source byte through the successful inline Git flush. The trusted
post-flush notification records that settled outcome after the guard releases.

Optional search-index embedding is a derived postcanonical projection. It may
run after source publication, always outside the commit section, and may fail
without undoing the durable fact or its committed write accounting. The
response continues to disclose degraded indexing. A retry of a committed
operation must replay the same fact rather than charge or publish it again.

This supersedes ADR 017's "before the section" wording only for this optional
derived indexing step. Any provider work required to determine canonical
bytes must finish before canonical mutation. No provider call may execute
while the commit guard is held.

## Evidence and limits

Independent review verified protocol compatibility and this ordering. A real
service regression checks the matching fact bytes in Git HEAD before an
injected embedding failure, then verifies committed ledger state and retry
with one source, one provider attempt and one charged write. Full local race,
vet and lint pass on source `7ab139359a16f3e60d4d6d1ebfaef4cf7af51f8c`.

Ordinary forget/cancel/publication routes and recovery checker activation
remain unfinished. Missing evidence after erasure must be Unknown. No extra
retained identifiers are introduced. Physical allocation and OS-enforced
staging limits remain required; moving indexing after publication does not
qualify those limits or remove cache growth from storage accounting.
