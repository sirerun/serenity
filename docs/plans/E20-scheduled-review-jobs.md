# E20 — Scheduled review jobs

The `decay` and `slo` registry entries still only wrote a timestamp after their
underlying domain tasks shipped. Executable fixtures confirmed that the scheduled
entry points created no stale-claim review and computed no queue snapshot.

- [x] T20.1 Wire scheduled decay and SLO behavior  Owner: pool  Est: 60m  verifies: [UC-044, UC-013]  deps: [T2.12, T2.15, T2.19, T14.1]  acc: [cron decay reads canonical active heads and stages stale-claim and lexical-alias review; repeated runs preserve human decisions and changed evidence creates a new card; malformed or dirty inputs stop before staging; confidence and lifecycle bytes never change; cron slo records actual queue metrics and failed computation never advances success; real CLI, executed tests and negative controls are recorded]

Scope: scheduled review and reporting. Human review of a stale fact does not
assert a replacement, upgrade confidence, or merge entities. Existing canonical
publication workflows remain the mechanism for those separate decisions. This
closes the disclosed cron wiring gap without introducing live model calls.

Verified 2026-09-08: final race suite 1,752 passing cases across 58 packages,
six explicit skips; full vet clean and lint zero issues. Three injected wiring
faults were detected and restored. Real CLI created three review cards, preserved
three human rejections on retry, and recorded real SLO metrics without canonical
changes. SIGKILL after 19 cards recovered 2,001 unique cards; a second retry created
none. A canceled empty scan was separately reproduced and fixed before the final
suite. Evidence: `docs/evals/scheduled-review-jobs.json`.
