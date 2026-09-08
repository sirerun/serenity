# M5 migration and launch evidence

Checked 2026-09-08. M5 is **not exit-verified**: naming, the real-message laptop
run, the name-dependent README, and the human release gate remain open.

## T5.8: synthetic cached import budget -- complete

The nightly workflow ran the deterministic seed-1 corpus through the production
IMAP connector, source store/Git commits, chunker, cached extraction,
reconciliation/disposition engine, claim writer, SQLite rebuild, and embedding
storage. The mailbox server and model-cache fixtures are test-only. No real mail
or live model calls were used. Corpus/cache preparation is outside measured stage
time; the real-mailbox acceptance task T5.12 is not satisfied by this benchmark.

[Successful workflow run](https://github.com/sirerun/serenity/actions/runs/34224399213)
used revision `87d9a4741772680c0a0e34aa1d2123519457674f`, Ubuntu 24.04 AMD64, and
`GOMAXPROCS=2`. Exactly one `TestImportBudget10K` executed and passed in 182.05
seconds, with zero test skips. It persisted 10,000 sources, 10,000 claims, and
10,050 vectors, with 10,000 cache hits and zero model calls. The measured JSON
receipt is [m5-import-budget.json](m5-import-budget.json).

| Stage | Items | Seconds |
| --- | ---: | ---: |
| Poll | 10,000 | 0.322 |
| Store sources and commit | 10,000 | 33.571 |
| Chunk and cached extraction | 10,000 | 1.016 |
| Reconcile | 10,000 | 7.343 |
| Write claims and commit | 10,000 | 123.020 |
| Rebuild index | 10,000 | 10.465 |
| Store cached embeddings | 10,050 | 3.294 |
| **Measured total** | | **179.031** |

The job uploaded its JSON test log, report, and Markdown timing table. It then
recorded the first accepted baseline on the separate `results/import-budget`
branch at `966760c1020174f0d5f7a3864649f1a5a9c192a2`.

### Regression gate: actual failed job

A temporary test branch added a real 220-second delay inside the measured polling
stage. [The negative-control workflow](https://github.com/sirerun/serenity/actions/runs/34225007046)
completed all 10,000 messages with the same counts and zero model calls, but
measured 391.816 seconds: **118.85% slower** than the accepted baseline. The
pipeline test itself passed in 394.63 seconds; the workflow correctly failed at
**Enforce completeness and performance budget**, uploaded its timing evidence,
and skipped **Record successful baseline**. The results-branch ref was verified
byte-identical before and after this run. The fault branch was never merged and
was removed from the remote after verification.

The exact 20% boundary is also covered by the comparator's named unit cases.
Changing its threshold to 100% makes both the exact-20% and slower-regression
cases fail. Corrupting an extraction cache key makes the smoke run fail with
`live calls forbidden`; there is no spending fallback. Restored smoke/comparator
race suites pass 21 test/subtest cases, with one explicit 10K test skip in the
ordinary suite; the dedicated 10K job above executes that test separately.

### Subsequent normal comparison

[The second normal nightly run](https://github.com/sirerun/serenity/actions/runs/34225863225)
loaded the previous accepted report and passed at 180.023 measured seconds
(+0.554%), with the same complete counts and zero model calls. Artifact upload
and baseline publication both succeeded. This verifies the normal comparison
path as well as first-run bootstrap and deliberate rejection.

### Local receipt

A committed-harness run on macOS ARM64 with three Go processors measured 341.323
seconds (346.05 seconds for the whole test), with the same source/claim/vector
counts and zero model calls. The earlier development run measured 367.051
seconds. These local timings are not compared against the Linux baseline.

## T5.12: real-message laptop run -- pending human execution

No real mailbox was imported for this report. The manual receipt must still name
the laptop, record at least 10,000 real messages, demonstrate interruption/resume,
and report elapsed time and final counts. Synthetic performance evidence cannot
substitute for those observations.

## Remaining release work

- T5.6: record the naming decision and collision check, then execute any rename.
- T5.14: complete the name-dependent README and conformance presentation.
- T5.20: make the release decision, verify all release artifacts/formula, and
  decide repository visibility. No v1.0.0 release is claimed here.
