# E17 — Decision ledger storage integrity

Real filesystem tests reproduced ledger traversal reads/deletes, symlink-directed
reads/writes, mismatched filename/payload identities and a partial entry observed
while a concurrent writer replaced it.

- [x] T17.1 Confine ledger operations and publish complete entries  Owner: pool  Est: 90m  verifies: [UC-026]  deps: [T16.1]  acc: [all reads and mutations reject invalid entry IDs and symlink boundaries without changing external bytes; filename/payload identities match on reads; create remains exclusive across independent queues; concurrent readers observe only complete old/new entries; replacement preserves permissions and incomplete staging files do not become entries; real CLI boundary and process-kill evidence plus executed test counts/skips are recorded]

This task protects individual ledger files. Multi-step decision lifecycle and
inbox publication recovery remain separate from atomic file replacement.

Verified 2026-09-08. The 92-case direction race suite passed with zero skips. The
full race suite passed 1,714 cases across 58 packages with six explicit skips;
vet passed and lint reported zero issues. Four deliberately restored defects
failed their targeted regressions and were restored byte-for-byte afterward.

Actual executable create/replace processes were stopped while an unpublished
staging file existed, then killed. Canonical entries remained complete, and
staging artifacts did not appear in List/Get. The real CLI returned error for a
symlinked ledger where the baseline returned a successful check. All data was
synthetic; no live model call was made. Evidence: `docs/evals/ledger-storage.json`.
