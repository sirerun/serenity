# E21 — Approved compaction publication

The accepted compaction command rewrote uncommitted canonical shards and left its
archive/live/segment changes uncommitted. Its direct multi-file mechanics had no
receipt to finish the same pass after interruption. A failed Git commit also
exposed retry failure for already-staged deletions.

- [ ] T21.1 Publish and recover one approved compaction pass  Owner: pool  Est: 90m  verifies: [UC-032, UC-026]  deps: [T2.9, T14.1, T16.1]  acc: [uncommitted and unsafe shard inputs are refused before canonical writes; exact transitions are durable before archive/live writes and numbered-segment deletion; only owned paths commit and unrelated human staging survives; failed commits and pre/post-commit SIGKILL retry without duplicate archive rows, decisions or commits; intervening human edits and completed receipts are preserved; unapplied approvals name their retry command; ordinary publication still refuses deletion; executed verification is recorded]

Scope: the owner-facing `compact --item` workflow. The low-level shard mechanics
run only inside an isolated preview in this production path. Deletion permission
is restricted to numbered shard segments and is not added to ordinary canonical
publication. Completed receipts authorize no later sweep of new data.
