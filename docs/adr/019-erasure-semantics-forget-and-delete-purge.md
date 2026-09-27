# ADR 019: Erasure semantics -- forget and delete purge bytes, history, index, export and backups

## Status
Accepted

## Date
2026-09-27

## Context
Deep review 001 (PRIV-01, INF-04, FUN-07) found that `forget` writes only an expiry event: the
fact's `bytes` and `meta.yaml` stay in the working tree, in git history, in the FTS and vector
index rows, in the export bundle (`git bundle --all`) and in S3 backups that expire after about
60 days (30 days current plus 30 days noncurrent). `docs/threat-model.md` and the pricing copy
describe a deletion chain and a 30-day window. GDPR Article 17 and CCPA deletion are not met by
logical forget. On 2026-09-27 David chose full erasure in the remediation plan over
disclosure-only or a deferred purge.

Facts are stored one directory per fact (`<dir>/bytes`, `<dir>/meta.yaml`), which makes a
path-scoped history rewrite tractable. Every brain has exactly one writer (ADR 004, ADR 014), so
rewriting its history does not race another writer; a brain remote is either the person's own
remote (local product) or nonexistent (hosted brains are backed up as bundles, not pushed).

## Decision
1. `forget` (memory facts) and source tombstoning purge: the fact or source directory is removed
   from the working tree, its FTS and vector rows are deleted in the same flush, and the brain's
   history is rewritten to drop the path from every commit (`git filter-branch --index-filter`
   through `gitrun.Brain`, followed by reflog expiry and `gc --prune=now`), all under the writer
   lock. The expiry event remains as the audit record; it never carries the fact text.
2. After a rewrite the post-commit hook pushes with `--force-with-lease` and prints a one-line
   warning that history was rewritten and other clones must re-clone. The local product
   documents this in the `forget` command help and the operator docs.
3. Export defaults to history-free: the archive carries the current tree (and `.dira`) plus a
   bundle of `HEAD` only. `--with-history` is an explicit opt-in. A mid-stream export failure
   fails the response with a logged error instead of returning HTTP 200 with a partial archive.
4. Backups may not outlive the disclosed window: current objects expire at 30 days and
   noncurrent versions within 1 day, so the total retention is at most 31 days; the purge job
   verifies expiry. Hosted account and brain deletion purge through the deletion journal
   (T23.48) with the same three targets: bytes, history, index.
5. Disclosure text (threat model, README, operator docs, site pricing and privacy pages) states
   exactly this: forget purges the fact everywhere Serenity controls, backups expire within
   31 days, and a user-controlled brain remote is re-pushed with rewritten history.

## Consequences
- Positive: "forgotten" data is actually gone from every store Serenity controls within the
  disclosed window; the review's attack chain AC-1 no longer reaches "forgotten" data through
  backups.
- Negative: history rewrite invalidates earlier clones and bundles of the brain; acceptable
  because brains are single-writer and small, and the warning makes it visible. `gc` cost is
  bounded by brain size; hosted brains are capped by plan storage.
- Negative: `git filter-branch` prints a deprecation notice; it is part of git itself (no new
  dependency, ADR 003) and is silenced with `FILTER_BRANCH_SQUELCH_WARNING=1`. If a future git
  removes it, `gitrun` gains a `commit-tree` based rewrite; the contract does not change.
- Related: E24 tasks T24.21, T24.22, T24.23 and the T23.48, T23.52, T23.54, T23.56 amendments in
  `docs/plans/deep-review-001-remediation.md`.
