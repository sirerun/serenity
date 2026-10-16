# ADR 029: Episodic erasure must prevent replay and derived-content resurrection

Date: 2026-10-16

Status: Proposed. Extends the target erasure contract to new episodic records;
does not claim current backup/export guarantees meet that target.

## Context

The owner chose indefinite retention until explicit deletion, automatic import
of existing histories, continuous multi-agent capture and offline queues.
Deleting a stored episode without fencing later imports would let an old
laptop recreate it. Summaries, checkpoints and extracted memories can retain
deleted text even after the raw source disappears.

ADR 019 defines physical-erasure goals. Current governed-memory v1 instead
distinguishes retraction from erasure and refuses destructive legacy forget on
governed command records. Those records can contain original request text.
Neither behavior can be silently relabeled as full episode deletion.

## Decision

1. Define episode deletion as a resumable operation with distinct immediate
   retrieval exclusion and physical-purge completion. Recheck eligibility at
   read/derivation/publication time, not solely when indexing started.
2. Track dependencies from originals to attachments, summaries, checkpoints,
   extracted statements and cached context. Exclude affected derivatives at
   once; rebuild only from still-eligible evidence. A claim can remain usable
   only with independent surviving support and accurate provenance.
3. Reconcile deletion fences before retrying uploads or importing old histories.
   Synchronize fences down before draining the client queue up; server admission
   and publication also check the current generation to close concurrent-delete
   races. Expired fence cursors require reset, not blind upload. Retain minimal
   content-free identity needed to reject resurrection, protected
   as metadata under the same brain authority. No deleted text, credentials,
   raw host paths or attachment bytes may survive in a replay receipt.
4. Propagate fences to controlled device queues. Offline/unreachable devices,
   host-owned originals and uncontrolled exports are explicit residual copies,
   never part of a false completed-purge guarantee. A returning device applies
   current fences before admission; policy changes cannot implicitly resurrect
   forgotten events. Intentional recapture requires a distinct explicit owner
   action whose semantics are frozen in the extension contract.
5. Purge controlled original bytes, derivative text, vectors/indexes, relevant
   Git history and managed exports/backups. Reuse the existing writer and
   recovery fences; large purge/rewrite work must be bounded and resumable.
   Batch physical history cleanup in fenced maintenance rather than performing
   an unbounded rewrite on every delete. Declare its schedule/deadline and
   backpressure. Use stable logical IDs/digests for retained-record receipts
   because Git commit hashes may change. Old indexes/snapshots cannot become
   a read fallback for deleted material.
6. Distinguish deleting a conversation's references from deleting content
   everywhere under control. Shared attachment references and explicitly shared
   cross-brain copies have a traceable disposition. State retained independent
   references and authorization limits; do not cross a brain boundary merely
   because a deletion request contains its ID.
7. Preserve governed-memory v1 retraction semantics. Add explicit episodic
   purge/reconciliation records rather than retroactively treating old status
   receipts as safe to erase. Export, restore and backup handling must consult
   deletion state so an older backup cannot revive forgotten records.

## Consequences

Deletion requires dependency tracking and a durable metadata fence. It is more
work than tombstoning a search row, but it is necessary for the chosen automatic
import/retention model. Queue contents and metadata consume visible storage;
quotas must leave bounded capacity for exclusion/fencing operations so a full
brain can still stop disclosure and begin deletion.

The protocol must freeze which minimal identity remains, its privacy treatment,
and how absent receipts are represented. Absence after purge is not affirmative
proof that an earlier command never executed. Exact numerical purge and backup
deadlines need qualified evidence; do not inherit an unproven deadline from an
older ADR or describe active-search removal as historical erasure.

## Alternatives

- Retraction alone: insufficient because original and derivative bytes remain.
- Delete bytes and all replay metadata immediately: unsafe with automatic
  imports and delayed uploads, which can republish the same event.
- Permanently retain original command payloads for audit: contradicts erasure.
- Drop the offline/import features: contradicts confirmed product requirements.

## References

[RFC 0002](../rfc/0002-long-horizon-agent-memory.md),
[ADR 019](019-erasure-semantics-forget-and-delete-purge.md),
[ADR 017](017-hosted-operation-and-recovery-contracts.md),
[ADR 025](025-postcanonical-search-indexing-for-hosted-remember.md),
[current erasure limits](../operator/forget.md).
