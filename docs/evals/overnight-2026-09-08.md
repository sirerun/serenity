# Migration and recovery verification — 2026-09-08

Application work through `dce0ddd502490132b43c88893c69db7e9766eb74` (PR #217),
plus benchmark environment capture in PR #218 (`12548bdb19e51da4471a8ea50504f91cefd4fe88`).
[The merge receipt](overnight-2026-09-08-merges.json) records 32 merged work PRs,
#186–#217. Each merged head passed its required CI checks; individual feature
reports below preserve executed counts, deliberate faults and limitations.

## Completed work

| Area | Outcome | Evidence |
| --- | --- | --- |
| M4 external-agent exit | A real Claude Code session recalled two planted facts; E4 is 21/21 | [M4 report](m4-report.md), [external session](m4-external-session.json) |
| gbrain migration | Provenance-preserving import, 74-cell round-trip audit, interruption recovery and pinned public fixture | [M5 report](m5-report.md), [migration operator guide](../operator/gbrain-import.md) |
| Import budget | Deterministic 10K synthetic mailbox, cached pipeline timing and real slowdown rejection; no live-model fallback | [M5 report](m5-report.md) |
| Canonical retrieval | Imported and native facts are searchable; source deletion, privacy and integrity apply across CLI, MCP and the embedded facade | [Native claims](native-claim-retrieval.json), [source retrieval](source-retrieval.json) |
| Extraction and human review | Batches preserve prose, contradictions stage persistent review, and low-confidence evidence requires an explicit human assertion before canonical publication | [Extraction guide](../operator/extraction.md), [inbox recovery](inbox-recovery.json) |
| Durable decisions | Atomic decision/history updates, durable paused-write handoff, reviewed human edits and exact ledger publication recovery | [Atomicity](disposition-atomicity.json), [handoff](pending-handoff.json), [human edits](dirty-edit-publication.json), [ledger publication](ledger-publication.json) |
| Storage and scheduled jobs | Confined atomic ledger files, readable SQLite TEXT updates, real decay review and queue SLO snapshots | [Ledger storage](ledger-storage.json), [SQLite compatibility](disposition-storage-types.json), [scheduled jobs](scheduled-review-jobs.json) |
| Approved compaction | One durable approved pass, exact-path commits, protected human edits and safe interrupted retries | [Compaction evidence](approved-compaction.json), [operator guide](../operator/compaction.md) |
| Launch preparation | Infrastructure inventory, HTTPS, chat operations, browser-local adoption events and launch copy | [Launch checklist](../launch/checklist.md) |

## Verification limits

The final full Darwin race run on `12548bdb19e51da4471a8ea50504f91cefd4fe88`
passed **1,779 test/subtest cases across 58 packages**, with six explicit test
skips and four packages without tests. Package membership matched all 62 packages
from `go list ./...`. Full vet passed and lint reported zero issues. The build
lease was acquired after the other project released it and was released after
verification. [Counted final receipt](overnight-final-verification.json).

Compaction also passed full Linux CI race testing across 58 packages, 18 focused
Darwin CLI cases and 13 writer cases with zero skips, four fault controls and
actual pre/post-commit SIGKILL checks. The non-verbose Linux log does not provide
an individual case count; the local counted result above is stated separately.

Actual SIGKILL checks exercised pre/post-commit recovery and partial scheduled
staging. These demonstrate the recorded crash boundaries; they do not claim a
seven-day soak. Cross-build checks compile their target platforms, not runtime
execution on those platforms. Cached synthetic import timing does not certify
live model quality or the required real-mailbox laptop migration. The M4 external
session report also preserves its keyword-only recall and tool-permission limits.

## Latest performance gate

The [final main run](https://github.com/sirerun/serenity/actions/runs/34270989376)
completed all 10,000 messages and claims, 20,050 vectors and 10,000 cache hits
with zero model calls, but **failed the performance gate**: 66.298 measured
seconds versus the prior 50.248 seconds (+31.94%). Its failure artifact was
preserved and the accepted baseline ref did not advance. The 20% threshold was
not changed. A same-runner ABBA comparison then measured the prior revision at 62.581 and
60.836 seconds, and current code at 60.992 and 60.728 seconds. All four workload
tests passed with zero skips and complete counts. It did not reproduce a code
slowdown; the prior revision also exceeded its original accepted timing. This
supports variation between runs, but does not prove the original cause or pass
the production gate.

After [PR #218](https://github.com/sirerun/serenity/pull/218) added allowlisted
runner/tool metadata to artifacts, one [production verification run](https://github.com/sirerun/serenity/actions/runs/34273146565)
also failed: **68.124 seconds (+35.58%)**. Its single workload test passed with
zero skips and all counts complete; the baseline again remained unchanged.
The environment artifact was captured successfully. **T22.1 remains open**;
no additional retries were used to select a passing sample.
[Exact receipt](final-import-budget.json).

## Remaining owner gates

E5 remains **11/15** and E7 remains **4/6**. No naming, personal-mailbox or release
decision was made on the owner's behalf.

1. **T5.6 — Name decision:** record the choice and collision check in `docs/NAME.md`.
   The module/binary/docs rename and name-dependent **T5.14** follow that decision.
2. **T5.12 — Real migration:** import at least 10K real messages on a laptop, record
   interruption/resume evidence, elapsed time and final counts.
3. **T7.4/T7.6 — Launch review:** review the prepared content and the still-draft
   [ndungu.dev installation-link PR](https://github.com/dndungu/dndungu.github.io/pull/6),
   then record the launch go/no-go decision.
4. **T5.20 — v1.0.0 gate:** review all M0–M5 acceptance evidence and complete the
   release and Homebrew publication gate after its dependencies are satisfied.

The E6 hardening soak remains gated on code completion and requires seven days.
The original checkout and its existing drafts were preserved; implementation work
used isolated worktrees. The live plan remains in [docs/plan.md](../plan.md).
