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

## File representation implemented 2026-09-08

The existing seven-column Serenity claim table cannot carry visibility or full
provenance. Imported pages therefore add a `serenity:metadata` JSON block that
supplements the visible table with visibility, original provenance cells,
`review`, and confidence precision. The table remains authoritative for text,
state, validity and supersession; a changed visible confidence overrides the
stored precision. The block also retains source frontmatter, original prose,
and typed wiki-link edges. Known link targets carry the destination entity slug;
unresolved targets retain their source reference without inventing an entity.
No graph traversal endpoint is introduced by the file importer.

Facts and takes have independent row-number spaces in the pinned upstream
parser. Their human-readable SourceRef can consequently be identical, but the
claim ID includes page path, fence name, row number and original cells. The
`gbrain_fence` provenance key disambiguates references. Resumption must likewise
key rows by `(page, fence, row)` rather than collapsing facts #1 with takes #1.
Both `->` and `→` take date ranges are read; take lifecycle annotations are in
the source column, while facts use context. Original extension columns remain
verbatim in provenance rather than being silently dropped.

The initial import writes complete pages exclusively and atomically through the
writer queue, commits each page, then rebuilds the disposable index. Identical
existing pages are idempotent; different pages are conflicts, even if untracked.
Incremental replacement is not inferred from ID equality. Use metadata-aware
Serenity versions to read imported brains: older binaries do not understand
these supplementary privacy/provenance fields.
