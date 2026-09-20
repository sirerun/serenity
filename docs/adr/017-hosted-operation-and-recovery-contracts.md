# ADR 017: Hosted operation accounting and recovery contracts

## Status

Accepted as an architecture ruling by chief-architect on 2026-09-20, against
PR236 revision `218d7234d9abea5964f9d4640d1bfffe5c9f8087`. This ADR records
design approval only; it does not mark T23.41 accepted, approve implementation
or merge, or authorize infrastructure changes. See the [review comment](https://github.com/sirerun/serenity/pull/236#issuecomment-5746461189)
and `docs/launch/evidence/T23.41/architecture-review-request.md`.

## Date

2026-09-20

## Context

Hosted Serenity needs crash-safe operation accounting, bounded storage
admission, independently complete deletion history, and a recovery fence that
prevents an old writer from returning after restore. PR236 proposed executable
contracts for those seams. Chief-architect reviewed the exact revision named
above and found one contradiction: the review packet recommended holding the
brain's commit fence across the entire `remember` handler, while
`internal/hosted/contracts/commitfence.go` says provider calls must stay outside
the section.

## Decision

1. **Storage admission:** approve staged writes with a global stage budget
   reserved before stage bytes exist, an aborting stager, and admission based
   on measured growth. The core-writer seam (Git quarantine objects or
   equivalent) is approved. In-place admission is approved only as a fallback
   if that seam is rejected. A physical bound may be called hard only after
   allocation-based accounting includes external Git/index writes and the
   staging area has an OS-enforced size limit. The statistical growth
   multiplier remains withdrawn. **Storage admission stays BLOCKED** until
   task44 supplies the OS enforcement, allocation accounting, and measured
   `MaxMutationStageBytes`.
2. **Operation accounting:** approve the `pending_review` holding phase,
   fingerprint-bound retry keys, and absence-based release only by a
   reconciler holding the exclusive brain fence across canonical inspection
   and ledger transition. The commit section covers only the canonical write,
   from its first canonical byte through `writer.Flush` returning. Embedding
   and all other provider calls happen before the section. The previously
   recommended wide section was rejected because slow provider I/O could defer
   reconciliation. Proposed schema and migration remain unapplied pending
   implementation and ordinary review.
3. **Deletion journal:** approve journal-only completeness based on a hash
   chain, gapless sequence and generation seal. Approve the existing
   versioned-bucket substrate and its specified conditional-write, IAM,
   watermark and purge obligations. The substrate remains conditional on
   task48 qualifying conditional create against the actual bucket, policy and
   CLI/SDK build on an authorized disposable resource. Retention beyond bucket
   versioning, including Object Lock/compliance mode, is not decided here.
4. **Restore fencing:** approve requiring all three facts before unfreeze:
   sealed journal generation, provider-verified old-instance stop, and
   revocation of every old credential and session. The distributed generation
   barrier alternative is rejected because it adds a coordination service the
   single-VM deployment does not otherwise need. The local writer lock is not
   accepted as cross-host fencing.

## Consequences

- Task44 may build against the approved narrow commit-section scope, but the
  storage-admission guarantee remains blocked on its explicit physical-bound
  prerequisites and measurement.
- Task48 still needs live conditional-write qualification before the approved
  bucket adapter is operationally accepted.
- The three-fact recovery receipt is a design requirement; implementation,
  tests against provider state, and ordinary code review remain necessary.
- No SQL migration, production code path, cloud resource, provider call, or
  launch action is approved by this ADR.

## Alternatives

- **Hold the fence across the whole `remember` handler:** rejected because
  provider latency under the fence can delay reconciliation.
- **Use a statistical growth multiplier as a hard storage bound:** remains
  withdrawn; only OS enforcement plus allocation-based accounting can support
  a hard-bound claim.
- **Add a distributed generation barrier:** rejected because this deployment
  does not otherwise need a coordination service.
- **Use a local writer lock to fence another host:** rejected; a host-local
  lock cannot establish cross-host stop.

## References

- `docs/launch/evidence/T23.41/architecture-review-request.md`
- `docs/launch/hosted-completion/interfaces.md`
- `internal/hosted/contracts/commitfence.go`
- Chief-architect ruling: https://github.com/sirerun/serenity/pull/236#issuecomment-5746461189
