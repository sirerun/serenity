# Authority contract independent runtime review

Verdict: CLEAR FOR SOURCE CONTRACT FREEZE ONLY.
Reviewed SHA256: 55f99e3a1ea2012a40e578198edb9cc1f7cdd119615d1b6e652cddbcf7485895 (`authority-contract.md`). Reviewer: runtime lane; no authority source authored. Date: 2026-10-10.

Initial HOLD findings were cross-table publication fencing, restored META bypass of independent witness checks, cut witness scope, runtime seal capability, incompatible manifest-v2 extension, and nonexistent journal constructor. Revised contract resolves these with independent TIP CAS, mandatory Current anchor comparison, witnessed BeginCut/FinalizeCut, recovery-only sealing, authenticated cut sidecar retaining exact manifest v2, and NewJournalReader/NewWriterJournal. Follow-up orphan witness gap is resolved by content-addressed immutable candidates plus exact TIP ancestor traversal; unreferenced pre-CAS candidates remain preserved but unauthoritative. Owner-only explicit TIP bootstrap and stable observation rechecks complete source design.

This approves implementing the documented contract, not its implementation correctness, provider behavior, trust-policy activation, disaster-recovery qualification or deployment. Exact-head independent source review and stated adversarial tests remain required. No live provider/resource change was performed by reviewer.
