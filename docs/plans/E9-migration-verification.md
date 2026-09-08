# E9 -- Migration verification remediation (2026-09-08)

Fidelity: executable. Scope: gbrain migration CLI and retrieval of imported claims.
The end-to-end audit exercised seven local-owner flows. Six passed, including a
real CLI SIGKILL/resume of 22 pages and 148 claims. Local search failed for both
private and public imported claim text despite eight indexed claims. The index
only had two title/summary chunks, so the imported evidence was not searchable.

- [x] T9.1 Wire imported claim text into retrieval with canonical eligibility  Owner: pool  Est: 90m  verifies: [UC-032, UC-010]  deps: [T5.1, T5.3]  acc: [local CLI search finds active private and public imported claims after import; remote recall and embedding exclude private, retracted, expired, changed, and deleted evidence even against a stale index; per-claim chunks are rebuildable derived state, never a new canonical source]
- [x] T9.2 Reverify migration operator flows and retrieval privacy  Owner: pool  Est: 30m  verifies: [UC-032, UC-010]  deps: [T9.1]  acc: [all seven migration CLI audit flows pass; named retrieval/privacy suites execute their asserted cases; a negative control that removes canonical eligibility fails; full repository race checks pass with skips disclosed]

The audit's passing cases cover import/report errors, unchanged repetition,
human edits/deletions, corrupt checkpoints, invalid/overlapping sources, recovery
after index creation failure, and a real interrupted CLI migration. Naming,
real-mailbox migration, and release decisions remain separate human gates.

## Verification receipt

Completed 2026-09-08. All seven audited CLI flows now pass. The repaired search
flow was rerun with a newly built CLI; the other six had already passed and their
import/checkpoint paths are unchanged. A real MCP stdio session returned the
public imported project with its review label, excluded the private preference,
and immediately excluded a public claim changed to private without an index
rebuild. Imported rows remain claims, not fabricated raw source facts.

Full repository race suite: 1,510 passing test/subtest cases across 58 passing
packages, six explicit test skips (two external conformance controls, two
inapplicable direction cases, the subprocess-only crash helper, and the separate
10K performance run). Focused retrieval/date/CLI/MCP/composition suite: 28 passing
cases, zero skips. Removing the remote-privacy check exposed a private fixture;
removing canonical-text comparison exposed changed and forged rows. Both faults
failed their tests, were restored, and the focused suite passed again.

Changed-package lint/vet and a real CLI build passed. Retrieval checks canonical
visibility, lifecycle, validity, source policy, text, review label, entity, and
family storage tier on every request; a stale index cannot authorize evidence.
