# E15 — Paused-write review handoff

A 2026-09-08 audit re-created the same durable pending-write record after its
human rejection. ImportPending reset the reviewed item to pending and removed its
actor and verdict. A producer then wrote a newer record under the same key while
the importer staged the previous record; the importer deleted the newer evidence.

- [x] T15.1 Preserve paused-write evidence and review across handoff and retry  Owner: pool  Est: 90m  verifies: [UC-012, UC-026]  deps: [T14.1]  acc: [replaying identical conflict evidence preserves its original human decision and payload; a distinct conflict for the same path stages separately; importing an older record cannot delete a newer producer record; interruption or storage failure retains unconsumed evidence for retry; producer publication exposes only complete records; real filesystem/SQLite regressions and negative controls disclose named execution counts and skips]

The task moves runtime pending-write evidence into review. It does not apply a
machine version over human canonical content or automatically accept a dirty edit.

Verified 2026-09-08. The producer publishes complete pending JSON atomically;
the importer renames inputs into recoverable claimed directories before staging.
Content-derived identities preserve original reviews across retries and separate
changed conflict evidence. Legacy filename-keyed decisions remain intact. Target
paths are retained as evidence and are never dereferenced by the importer.

The real CLI audit also found that interactive inbox never called ImportPending.
It now recovers paused writes before displaying review items; read-only modes
remain read-only. The final race suite passed 1,667 cases across 58 packages with
six explicit skipped cases, and lint reported zero issues. Five deliberate faults
were detected and restored. Concurrent producer/read and two-importer tests passed.
Actual SIGKILL before and after SQLite staging recovered through `serenity inbox`,
retaining the older claimed record, a newer producer record, an existing human
rejection, and unchanged human bytes/Git status. A second recovery was a no-op.

Evidence: `docs/evals/pending-handoff.json`. This closes staging and handoff;
dirty-edit canonical publication remains separate from importing evidence.
