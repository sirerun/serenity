# E22 — Final import performance investigation

The final main 10K run completed its workload but failed its unchanged 20%
regression threshold: 66.298 seconds versus the previous 50.248 seconds. Broad
stage slowdown alone does not identify a code regression or runner variation.

- [ ] T22.1 Diagnose and resolve the final import timing failure  Owner: pool  Est: 60m  deps: [T5.8, T21.1]  acc: [retain the original failing timing/test artifact and accepted baseline; compare fixed prior/current revisions on the same runner with complete counts and zero live calls; record stage-level evidence and limitations; fix any proven regression without weakening or resetting the budget; verify the final production gate and record its actual verdict]

The read-only same-runner ABBA comparison completed: prior 62.581/60.836 seconds,
current 60.992/60.728 seconds. All four workload tests passed with no skips and
complete counts. No code slowdown reproduced. The original successful run and
production replication used the same Go/Git/runner-image versions; CPU details
were absent from the original artifact, so the cause remains unproven.

PR #218 added allowlisted environment metadata without changing the benchmark or
gate. One subsequent production run failed at 68.124 seconds (+35.58%), preserving
the baseline again. The task remains open. A read-only CPU/Git profile then measured the 31.426-second source stage
with 8.583 seconds in Git add and 4.388 seconds in Git commit; automatic
maintenance/repack was observed, with nested intervals that cannot be added.
Next work should isolate durable source writes, exact-path Git staging and
automatic maintenance on a controlled runner, then demonstrate a reproducible
remedy. Similar CPU labels alone do not prove equivalent performance; do not
bootstrap or loosen the gate merely to clear this failure.

Evidence: `docs/evals/final-import-budget.json` and
`docs/evals/import-budget-profile.json`. The isolated diagnostic branch
was removed after its reports were retained; no diagnostic workflow was merged.
