# E10 — Inbox reconciliation verification

Scoped audit: local-owner CLI review of staged reconciliation proposals. The audit uses the installed public binary against real Git repositories, SQLite disposition state and canonical entity pages; all data is invented. Protocol approval identity, capture routing and other item-kind application are separate surfaces.

Two production failures were reproduced on 2026-09-08: space/accept exits zero and removes a reconcile item while leaving the old canonical claim active; an edit blocked by a human file change becomes disposed and disappears from the next inbox even though its write never happened. Reject and defer preserve the canonical claim, and an ordinary edit writes and commits correctly.

- [x] T10.1 Apply accepted reconciliation through the inbox  Owner: pool  Est: 90m  verifies: [UC-012, UC-015]  deps: []  acc: [plain accept and edit_accept write the accepted claim through the canonical writer and commit it; reject/defer do not alter canonical claims; grouped independent proposals apply without misclassifying this session's own writes as human edits; stale or human-modified targets are not overwritten]
- [x] T10.2 Recover accepted reconciliation writes without duplicate effects  Owner: pool  Est: 90m  verifies: [UC-012, UC-015, infrastructure]  deps: [T10.1]  acc: [failed or interrupted applications remain discoverable and have an explicit CLI retry; retry preserves the recorded verdict and actor, writes no duplicate claim/history and finishes a failed commit safely; intervening canonical edits or retractions are preserved; real CLI failure/retry and negative-control evidence is recorded]

Implementation must distinguish recording a human decision from successfully publishing its canonical effect. An accepted item is not reported fully applied merely because its queue state is terminal. Existing human release gates are unchanged.


## Verification receipt

Completed 2026-09-08. All five scoped real-CLI workflows pass: accept, edit,
reject, defer, and recovery after a human edit blocks publication. The repaired
CLI preserves the original reviewer/time/verdict and committed human prose on
retry. Two actual SIGKILL probes stopped before and after Git commit; both
remained visible and recovered without duplicate claims or repeat commits. The
pre-commit fixture required resolving its killed Git process's orphaned
`index.lock`; the post-commit fixture did not.

Full repository race suite: 1,536 passing test/subtest cases across 58 passing
packages, six explicit test skips (two external conformance controls, two
inapplicable direction cases, the subprocess-only crash helper, and the separate
10K performance run). Focused inbox/publication/managed-fence race suite: 36
passing cases across three packages, zero skips. Checks include fence and shard
publication, rotated shards, failed commits, exact-history retry, stale/retracted
or deleted targets, unrelated staged files, symlinks, same-page grouped approvals,
human prose, and truthful reporting when canonical commit succeeds but index
rebuilding fails. Vet, changed-package lint, CLI build and strict docs build pass.

Negative controls that disable plain-accept publication or replace the whole page
instead of managed blocks each fail their dedicated test. Both changes were
restored before the final passing suites and CLI probes. Sanitized machine-readable
receipts are in [inbox-recovery.json](../evals/inbox-recovery.json); operational
limits and recovery steps are in the [inbox guide](../operator/inbox.md).
