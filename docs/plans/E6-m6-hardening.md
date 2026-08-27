# E6 -- M6: hardening soak (post-code-complete, ADR 002)
Acceptance: RFC section 17 M6 AC, all measured not asserted: 7 consecutive days unattended on both profiles (laptop, 128GB box with `profile: local-heavy`); chaos test (kill any worker, no lost jobs; WAL-backed queue and idempotent workers documented); trust-ladder promotion demonstrated on real dispositions with sampled false-acceptance reported; p95 warm search < 400ms; repo-growth and rebuild-time metrics collected over the soak.
fidelity: outline

Intent: M6 measures the finished binary. It produces numbers and defect reports, not features; any miss reopens the owning epic (E1 search p95, E2 ladder, E4 job durability). It starts only after the code-complete gate (every E0-E5 AC line in docs/plan.md checked). The 128GB-box profile is the DGX via Spark (load the /spark skill; never raw ssh).

Exit criteria: docs/evals/m6-soak-report.md with the seven measurements, each with the command, the window, and the observed value; zero lost jobs across the chaos matrix; ladder report shows >= 1 cell promoted on real dispositions with its sampled false-acceptance rate.

- [ ] T6.0 PLAN: expand E6 to executable fidelity (informed by E0-E5 learnings and the shipped metrics surface)  Owner: pool  Est: 1h  kind: plan  delivers: [docs/plans/E6-m6-hardening.md at fidelity: executable]  deps: [T5.20]  acc: [parse_plan.py sees E6 with >= 8 tasks covering soak harness, chaos matrix, p95 benchmark, growth/rebuild metrics, DGX profile run, and report; every task carries acceptance criteria; fidelity flipped to executable]
