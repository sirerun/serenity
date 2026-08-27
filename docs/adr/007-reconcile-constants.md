# ADR 007: Reconciliation candidate threshold, corroboration update, and parked-item resurfacing

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 section 10.2 says conflicts are detected against claims sharing
`(subject, predicate)` "plus high-similarity neighbors", corroboration gives a
"weighted confidence bump", and section 8.2 says parked items resurface on
"new evidence arriving on the same (subject, predicate)". None of the three is
numeric, and each directly gates the false-conflict rate the ladder measures.

## Decision
- Candidate neighbors are claims with cosine similarity >= 0.85 on the object
  embedding within the same subject, matching section 11's dedup threshold.
  Embedding similarity only surfaces candidates: a conflict verdict requires a
  structural match on `(subject, predicate)`. The constant lives in
  `internal/reconcile` and is pinned by a named test.
- Corroboration update: `c' = min(cap, (n * mean(c_1..c_n) + c_new) / (n + 1))`
  where `cap` is the tier cap from section 9 (0.90 local-cheap, 0.95
  judgment); only a human disposition can exceed 0.95. Exposed as a pure
  function with table tests.
- A parked item resurfaces at most once when any new claim (any state) lands
  on its `(subject, predicate)`; repeated evidence does not re-ping.
- Decay is computed at read and rank time from `observed_at` and the family
  half-life; it is never written back as a new confidence value.

## Consequences
- The reconcile eval's numbers are reproducible and the constants are
  reviewable in one file.
- A future change to any constant is a visible diff against a named test,
  which is what "measured, not assumed" needs.
