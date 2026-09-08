# E11 — Extraction-to-review verification

Scoped audit: local-owner CLI extraction over canonical source files. A real CLI,
Git repositories, SQLite index and HTTP provider adapter are exercised with
invented source text and a deterministic local provider fixture. Fixture responses
are controlled test data, not a model-quality evaluation or live-provider claim.

On 2026-09-08, a first extraction succeeds, but four failures were reproduced:
multiple sources updating an existing page stop on this run's own dirty write;
conflicting observations become active without a review item; low-confidence
observations are counted and dropped; an otherwise successful update drops
committed prose outside managed fences.

- [x] T11.1 Publish extraction batches without losing canonical content  Owner: pool  Est: 90m  verifies: [UC-005, UC-016]  deps: []  acc: [multiple observations and sources update existing fence/shard files successfully and commit all actual segment paths; exact repeats add no claims; committed human prose and metadata survive; dirty, corrupt, ambiguous or unsafe targets fail without being replaced]
- [ ] T11.2 Route extracted contradictions through human review  Owner: pool  Est: 90m  verifies: [UC-005, UC-014, UC-015]  deps: [T11.1]  acc: [real extraction stages a conflicting proposal instead of activating it; unchanged repeated extraction does not duplicate pending or rejected decisions; acceptance through the real inbox supersedes the canonical prior claim; candidate selection uses current canonical state and preserves unrelated claims]
- [ ] T11.3 Retain and review low-confidence extraction observations  Owner: pool  Est: 90m  verifies: [UC-005, UC-012]  deps: [T11.1]  acc: [low-confidence observations remain discoverable with their source provenance; repeats do not duplicate queue records; review actions have explicit effects and never silently promote a low-confidence observation or discard an accepted effect; rejection/defer preserve canonical state and operator guidance documents the supported approval path]

Each task requires real CLI evidence, meaningful failure cases and a detected
negative control before completion. Existing human naming, mailbox and release
gates remain open. Cross-process decision arbitration and general capture routing
are separate audit surfaces unless a remediation directly requires them.

## T11.1 verification — 2026-09-08

Extraction now prepares one batch across sources, validates every existing target,
preserves content outside claim-managed fences, and commits every actual shard
segment. The same preview planner supports inbox publication. A page is rendered
once per batch rather than once per observation.

The final full race run passed 1,544 cases in 58 packages, with six explicit skips;
the changed-package lint run reported zero issues. A real CLI with local HTTP
provider fixtures verified first extraction, multiple sources updating one existing
page, and human-prose preservation. Both deliberate regressions (dropping prose
and publishing each source separately) failed their dedicated tests. Conflict
routing and low-confidence retention still fail at this revision and remain the
next two tasks. See [batch receipt](../evals/extraction-batch.json).

The unchanged synthetic 10K import benchmark took 171.649 seconds on the same
Darwin/arm64, Go 1.27.1, GOMAXPROCS=3 environment as its 341.323-second prior run
(49.71% faster). Source storage stayed near 163 seconds; claim writing fell from
172.402 to 1.819 seconds. All 10,000 claims, 10,050 vectors and 10,000 cache hits
were counted, with zero live model calls. This measures cached pipeline overhead,
not live extraction quality or the later conflict-review orchestration. The
[raw timing receipt](../evals/extraction-batch-budget.json) records the code revision,
corpus digest and cache version. An earlier run used two processors while another
suite ran; it is excluded from the matched-settings comparison.

Publication preflights the whole batch, but individual file replacements are not
a multi-file filesystem transaction. Extraction does not yet persist an inbox-style
crash-recovery receipt. Existing dirty files are preserved and reported for operator
resolution; no automatic cleanup or force-overwrite is performed.
