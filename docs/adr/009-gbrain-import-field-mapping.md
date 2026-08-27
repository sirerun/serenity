# ADR 009: gbrain import reads the markdown fences; field mapping and the flagged semantic translations

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 section 15 requires `serenity import --from-gbrain` to be lossless
at the representation level, with a field-level round-trip test, and to flag
every semantic translation for review. gbrain (dndungu/gbrain@d35c9c9e441e,
branch `master`) keeps facts and takes in markdown fences on entity pages;
its DB tables are derived (`src/core/facts-fence.ts`, `takes-fence.ts`,
`fence-shared.ts`). An earlier research pass claimed no fence syntax existed;
reading the source refuted that. The grammars:

- Facts: `<!--- gbrain:facts:begin -->` ... `<!--- gbrain:facts:end -->`,
  columns `# | claim | kind | confidence | visibility | notability |
  valid_from | valid_until | source | context`; kinds `event | preference |
  commitment | belief | fact`; `~~claim~~` with context `superseded by #N`
  or `forgotten: <reason>`; row numbers are append-only and never shift.
- Takes: `<!--- gbrain:takes:begin -->` ... `<!--- gbrain:takes:end -->`,
  columns `# | claim | kind | who | weight | since | source`; `since` may be a
  range `A -> B`; kind is an open string seeded `fact | take | bet | hunch`.
- Pages: YAML frontmatter with `type`, `aliases`, external ids; `## Timeline`
  section; wiki links.

## Decision
- The importer parses pages and fences directly from the gbrain repo
  checkout (no DB, no running gbrain). Each fence row maps to exactly one
  Serenity claim with `SourceRef = "gbrain:<slug>#<row>"` so the round-trip
  test can address rows by their stable number.
- Representation-level mapping (asserted equal by the round-trip test):
  claim text -> `Object`; `valid_from`/`valid_until` -> `ValidFrom`/`ValidTo`
  (takes: `since` range); `visibility` -> `Visibility` (`world` -> shared,
  `private` -> private; takes default private); strikethrough +
  `superseded by #N` -> `State superseded`, `SupersededBy` = the mapped id
  of row N; `forgotten:` -> `State retracted` with the reason in provenance;
  `source`/`context`/`notability`/`who` -> provenance `Meta` keys verbatim.
- Semantic translations, each flagged `review: true` on the produced claim:
  gbrain `kind` -> Serenity predicate (`preference` -> `prefers`,
  `commitment` -> `committed_to`, `event` -> `said` with the date, `belief`
  and `fact` -> `relates_to` unless a controlled predicate is inferable);
  `confidence` and take `weight` -> initial `Confidence` capped at 0.90;
  take `who` -> `said` predicate with the holder as provenance actor.
- The importer is resumable: progress is keyed by `(page path, row number)`
  in `.serenity/import/gbrain.json`; re-running skips completed rows and
  never duplicates (derived ids make this idempotent).
- The public fixture brain for the round-trip test is a small synthetic
  gbrain repo under `testdata/gbrain-fixture/` covering every column, both
  strikethrough contexts, a range `since`, and a timeline.

## Consequences
- "Lossless" is testable: for every fence row the test asserts the mapped
  claim's fields equal the row's fields, then counts and spot checks.
- Predicate inference is the lossy step and is visibly flagged, matching
  the RFC's asymmetry rule.
- gbrain's DB-only facts (if any exist without a fence row) are out of
  scope for the file importer; the operator's manual says to run gbrain's
  own fence backfill first.
