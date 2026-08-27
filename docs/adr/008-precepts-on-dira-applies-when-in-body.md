# ADR 008: Precepts are unmodified dira entries; `applies_when` lives in the entry body; Serenity owns the constraint matcher

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 section 7.3 makes the precept store a dira ledger that the dira CLI
must read unmodified (M3 AC), with constraints carrying machine-detectable
`applies_when` clauses over a closed action set. dira's entry schema
(`schema/entry.schema.json` at kazi-org/dira@15686940aa08) sets
`additionalProperties: false` on the entry and on every `$defs` object, has
five kinds (`intent`, `decision`, `question`, `constraint`, `note`), keeps
`why_not` and `revisit_if` only inside `alternatives[]`, and requires
`alternatives` (min 1) on any decision that is not `staged`. Its `check`
matcher is lexical (IDF-weighted overlap, threshold 0.38), offline by
construction (`internal/nomodel`), and takes the plan as one positional
argument with exit 0/2 for verdicts and 1 for its own errors. dira is
Sire Run, Inc. IP under Apache-2.0 (LICENSE and NOTICE name the company).

Putting `applies_when` in frontmatter would make every Serenity constraint
schema-invalid to dira, failing the M3 AC.

## Decision
- Serenity writes `.dira/entries/<id>.md` files that validate against the
  vendored `entry.schema.json` byte-for-byte: frontmatter carries only dira
  fields. Decisions leave `staged` only with at least one alternative
  (the interview wizard always collects "not doing it" as the floor).
- `applies_when` is a fenced block in the markdown body:
  ```serenity:applies_when
  action: spend_over
  params: {amount: 500, currency: usd}
  ```
  dira treats the body as prose; Serenity's `internal/direction` parser reads
  the block. The block grammar is versioned in `docs/protocol/DIRECTION_v1.md`.
- `check_plan` matching is Serenity's own two-stage matcher (RFC section 8.3):
  deterministic over structured actions, model-classified for free text,
  `unverified` without a model. `dira check` is not on the hot path; it runs
  in CI as the conformance test that the ledger is a valid dira ledger, and
  its lexical verdicts are surfaced as `notices`, never as violations.
- dira kind `note` maps to the distill queue's "decaying note" disposition;
  Serenity never mints `note` entries from ingest.
- Vendoring: `internal/dira` holds `schema/*.json`, `schema.go`, and the
  ledger reader/writer at the pinned commit, with dira's LICENSE and NOTICE
  copied verbatim; the pin is recorded in `internal/dira/PIN`. The upstream
  PR proposing an optional `applies_when` field is opened in parallel and
  never blocks.

## Consequences
- The M3 AC "dira CLI reads the same ledger unmodified" holds by
  construction; a CI job runs `dira check` and `dira why` against the fixture
  brain.
- A future dira schema field replaces the body block by migration; both
  forms are readable during the transition.
- Serenity's matcher and dira's matcher can disagree; only Serenity's counts
  for DIRECTION verdicts, and the RFC's `check` exit-code contract stays
  Serenity's.
