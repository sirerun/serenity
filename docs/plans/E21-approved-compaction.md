# E21 — Approved compaction publication

The accepted compaction command rewrote uncommitted canonical shards and left its
archive/live/segment changes uncommitted. Its direct multi-file mechanics had no
receipt to finish the same pass after interruption. A failed Git commit also
exposed retry failure for already-staged deletions.

- [x] T21.1 Publish and recover one approved compaction pass  Owner: pool  Est: 90m  verifies: [UC-033, UC-026]  deps: [T2.9, T14.1, T16.1]  acc: [uncommitted and unsafe shard inputs are refused before canonical writes; exact transitions are durable before archive/live writes and numbered-segment deletion; only owned paths commit and unrelated human staging survives; failed commits and pre/post-commit SIGKILL retry without duplicate archive rows, decisions or commits; intervening human edits and completed receipts are preserved; unapplied approvals name their retry command; ordinary publication still refuses deletion; executed verification is recorded]

Scope: the owner-facing `compact --item` workflow. The low-level shard mechanics
run only inside an isolated preview in this production path. Deletion permission
is restricted to numbered shard segments and is not added to ordinary canonical
publication. Completed receipts authorize no later sweep of new data.

Verified 2026-09-08 at `3f1cdc467de5d724062fbe2bb03092ff86a1bc89`.
Full Linux CI race testing reported 58 package passes, with full vet/build/lint
and all 12 checks successful. Its non-verbose log does not provide a per-case
count. Local Darwin verification executed 18 CLI cases with global Git config
disabled and identity guessing forbidden, plus 13 writer cases; all passed with
zero skips. Four deliberately broken behaviors were detected and restored.
Actual pre/post-commit SIGKILL recovered one history entry, one live claim and one
archive row without duplicate commits; current facts stayed searchable without
rebuilding the index. The first CI run exposed a test-fixture dependency on Git
identity; fixtures now set their own local identity and the corrected run passed.
At merge time the shared build lease was occupied. The later final-main Darwin
run passed 1,779 cases across 58 packages, with six explicit test skips and four
packages without tests; full vet passed and lint reported zero issues. The lease
was released after the run. Evidence: `docs/evals/approved-compaction.json` and
`docs/evals/overnight-final-verification.json`.
