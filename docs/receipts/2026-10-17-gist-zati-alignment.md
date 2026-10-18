# Gist and Zati alignment inspection — 2026-10-17

Documentation scope only. Inspected fetched remote main objects rather than the
older primary working trees; their authored/untracked material was preserved.
Serenity baseline `7b2901cb38777f0c5432e292f4e451dc60aa1992`; Gist `b847a18abd5fa53df83a7facf3ccf44bd9998570`; Zati `7ec4ec724bf4a90c37135cd883e525052d817c7d`.
These are source and repository-receipt observations, not fresh live qualification.

## Material changes and consequences

| Evidence at the inspected revision | Consequence for Serenity |
| --- | --- |
| Gist public catalog packages now include discovery, packages, resolution, preparation, provenance and composition; Zati pins `v1.2.1-0.20261017083313-d0e97da28be3` | Reuse existing public APIs and consumer pins; do not plan a fresh extraction from hosted/internal or assume v1.2.0 is the complete current baseline |
| Gist ADR017/020 prepare maintained skills plus derived task-specific guides with exact schemas and positive current task/readiness admission | Zati composes memory with the prepared bundle; Serenity does not synthesize a competing tool-use guide or capability resolver |
| Gist ADR021 and composition producer/consumer source preserve exact v1/v2 packages and explicit typed bounded dataflow | Link memories to exact skill/composition/contract versions; historical success is not current readiness, schema compatibility or an execution grant |
| Gist T-COMPOSE.8-.14 and Zati ADR019/receipt record controlled v2 list/read admission and runtime coverage | The selected skill is now known: zati/inbox-briefing v1.0.0 and explicit v2.0.0; use current owner receipts rather than rediscovering its identity; real Gmail/provider/general-model acceptance stays separate |
| Gist composition qualification records 19 synthetic outcomes and 10k/100k summary-index workloads | Reuse scoped evidence; it does not measure Serenity episodic retrieval, Zati UI, token savings, native inference or arbitrary provider compatibility |
| Gist ADR022 and web/hosted deployment receipts record a separately authorized Library release at gist.sire.run | The old absolute hosted-expansion pause is a historical decision, superseded for those scoped slices; neither a mandatory hosted dependency nor authority to expand Serenity follows |
| Zati RFC002, focused-worker-organizations E-FO0 and contracts revision49 define owner-private history ranges, full current work/pending snapshots, bounded summary builder and fenced segment transition | Zati owns continuation and canonical live messages; Serenity owns acknowledged archive copies, derived retrieval and exact externally stored checkpoint bytes/provenance |
| Zati T-FO1 runtime owner rows remain open at the inspected revision | Contract/specification delivery does not close the 200-message problem or Serenity T2.2; bridge and joined runtime qualification stay open |

## Canonical owner mapping

- Serenity T2.2: archive/context bridge consuming Zati E-FO0 contracts and eventual
  T-FO1.2-.6 runtime receipts. Do not implement Zati messaging/tasks/execution here.
- Serenity T4.1: refresh current interface/evidence handoff, including complete
  task-to-recipe fit and current readiness, not a new catalog design exercise.
- Serenity T4.3: consume Gist T-COMPOSE.8-.14 and the owning Zati source receipt;
  identify only uncovered Serenity integration deltas. No upstream status edits.
- Serenity T4.6: compare the actually called memory retrieval path, using Gist
  measurements only for their named catalog/preparation workload. Semantic models
  remain optional caller-selected adapters; no adoption from source availability.
- Serenity T4.7: joined qualification still requires archive-bridge and continuation
  runtime acceptance. General context efficiency work can proceed using current
  governed-memory paths while archive capture remains disabled.
- Zati-owned work/task/effect state is read from its current authoritative owner;
  archived records never schedule work, replay actions, approve changes or widen
  organization/worker memory grants.

## Limits and next gate

No endpoint probes, installs, model downloads, provider calls, deployment or real
history import were made. Existing erasure/replay/restore, real-host, native UX,
provider and performance gates remain open. Gist hosting acceptance is scoped to
its receipts; named external clients and broader operational/provider gates are
not completed by the public website. Revalidate pins, source owners and unresolved
receipts at T4.1 before implementation. Canonical portfolio dependencies remain
with their existing owners, not this crosswalk.

## Pinned source references

- [Gist catalog API](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/catalog/README.md)
- [Task preparation](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/adr/017-task-preparation-and-semantic-retrieval.md)
- [Preparation admission contract](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/adr/020-preparation-contract-and-offline-freeze.md)
- [Composition contract](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/contracts/composition-v1.md)
- [Composition/index qualification](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/receipts/2026-10-17-composition-qualification.md)
- [Hosted Library decision](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/adr/022-hosted-library-web-app.md)
- [Web release receipt](https://github.com/sirerun/gist/blob/b847a18abd5fa53df83a7facf3ccf44bd9998570/docs/receipts/2026-10-17-web-app.md)
- [Zati composition owner](https://github.com/zati-run/zati/blob/7ec4ec724bf4a90c37135cd883e525052d817c7d/docs/adr/019-gist-composition-consumer.md)
- [Zati consumer source receipt](https://github.com/zati-run/zati/blob/7ec4ec724bf4a90c37135cd883e525052d817c7d/docs/receipts/2026-10-17-gist-composition.md)
- [Focused-worker product RFC](https://github.com/zati-run/zati/blob/7ec4ec724bf4a90c37135cd883e525052d817c7d/docs/rfc/002-focused-worker-organizations.md)
- [Continuation owner contract](https://github.com/zati-run/zati/blob/7ec4ec724bf4a90c37135cd883e525052d817c7d/docs/implementation/contracts.md)
- [Continuation task state](https://github.com/zati-run/zati/blob/7ec4ec724bf4a90c37135cd883e525052d817c7d/docs/plans/focused-worker-organizations.md)

Plan preservation: all91 task IDs, statuses and owners survive. The six amended
rows and their full previous lines are retained in the sibling preservation JSON;
all85 other rows remain byte-identical. T4.2 drops its old local T2.2 prerequisite
to permit current-memory context work while archival bridging is unavailable;
T4.7 adds T2.2, so joined continuity acceptance retains that prerequisite. No other
local dependency edges change. Wazi validation and exact review/landing evidence
are recorded in the PR; this record does not claim runtime completion.
