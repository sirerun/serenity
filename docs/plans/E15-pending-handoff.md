# E15 — Paused-write review handoff

A 2026-09-08 audit re-created the same durable pending-write record after its
human rejection. ImportPending reset the reviewed item to pending and removed its
actor and verdict. A producer then wrote a newer record under the same key while
the importer staged the previous record; the importer deleted the newer evidence.

- [ ] T15.1 Preserve paused-write evidence and review across handoff and retry  Owner: pool  Est: 90m  verifies: [UC-012, UC-026]  deps: [T14.1]  acc: [replaying identical conflict evidence preserves its original human decision and payload; a distinct conflict for the same path stages separately; importing an older record cannot delete a newer producer record; interruption or storage failure retains unconsumed evidence for retry; producer publication exposes only complete records; real filesystem/SQLite regressions and negative controls disclose named execution counts and skips]

The task moves runtime pending-write evidence into review. It does not apply a
machine version over human canonical content or automatically accept a dirty edit.
