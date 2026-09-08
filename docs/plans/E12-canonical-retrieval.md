# E12 — Native canonical claim retrieval

On 2026-09-08, a real CLI confirmed and committed a human assertion through the
extraction inbox. `serenity search "Human Company"` then returned no results.
Rebuilding indexes canonical claim rows but only indexes title/summary text for
native entity pages; the edited assertion has no matching raw source text. Imported
claims already have derived per-claim chunks with canonical eligibility checks.

- [x] T12.1 Make native canonical facts retrievable with current eligibility  Owner: pool  Est: 90m  verifies: [UC-005, UC-010]  deps: [T11.3]  acc: [a confirmed human assertion and active fence/shard facts are found by real local CLI search and eligible MCP recall; remote recall and provider embedding exclude private, index-only, retracted, superseded, expired, changed and deleted evidence even with a stale index; derived chunks rebuild deterministically and preserve attribution; meaningful negative controls fail and named suites disclose executed counts/skips]

Scope is derived retrieval wiring for canonical facts, not new canonical storage,
live model-quality claims or automatic resolution of existing contradictions.

## Verification — 2026-09-08

Native fence facts and resolved shard heads now produce rebuildable claim chunks.
The complete canonical claim fingerprints the cache reference, so changed text,
confidence, attribution, validity or privacy cannot reuse an old entry. Imported
claim references and review qualifiers retain their existing contract. Native
legacy visibility keeps the existing single-principal shared default. Claim
privacy/source disclosure is shared by search, embedding and direct composition.

Real CLI search now finds the previously missing confirmed human assertion. A
real MCP stdio session returned it with human attribution, then immediately
excluded it after a privacy change and after deletion, without an index rebuild.
The session correctly disclosed keyword-only search and used zero model calls.
Fifty-four native/imported policy cases passed, including fence/shard privacy,
index-only and missing source attribution, lifecycle, deterministic rebuilds and
forged cache entries. The broader repaired focused suite passed 69 cases. Final
full race suite: 1,611 passing cases in 58 packages, six explicit skips. Lint:
zero issues. See [receipt](../evals/native-claim-retrieval.json).

The first full run exposed unrealistic old composer source hashes and an import
benchmark database outside the brain root. Fixtures now store real attributed
sources, and the benchmark uses the production `.serenity/index.db` layout so
canonical embedding eligibility actually executes. Numbered shard segments are
also deduplicated into their original family, including when only numbered
segments remain. The original failing search and segment-enumeration regressions
were reproduced before repair; three visibility/text/source-policy negative
controls were detected.

The matching Darwin/arm64 Go 1.27.1, three-processor 10K run took 176.927 seconds
versus 171.649 seconds (+3.07%, within the 20% budget). It produced 10,000 claims,
20,050 vectors and 10,000 cache hits with zero live model calls. The previous run
had 10,050 vectors: the extra claim projection work is disclosed, and the baseline
was not reset. [Raw timing](../evals/native-claim-budget.json) records the code
revision, corpus digest, cache identity and stages.

A separate raw-source probe found that physically deleting a source leaves its
old generic source chunk searchable until rebuild. E13 records that distinct gap;
these canonical-claim receipts do not claim stale raw-source deletion is fixed.
