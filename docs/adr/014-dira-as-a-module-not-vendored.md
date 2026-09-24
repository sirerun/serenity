# ADR 014: dira is imported as a Go module, not vendored

## Status
Accepted

## Date
2026-09-23

## Context
ADR 008 vendored dira's ledger codec, frontmatter splitter, and entry schema
into `internal/dira` at a pinned commit, with a `PIN` file, `verify-pin.sh`,
`scripts/update-dira.sh`, and an empty `patches/` directory to keep the copy
byte-identical to upstream. That was the only option at the time: every
package Serenity needed lived under dira's `internal/`, which Go forbids any
other module from importing.

Two premises behind that choice were wrong. First, kazi-org/dira is Sire Run
IP and David has admin on the repo; the "repo we don't own" framing in
`internal/dira/UPSTREAM.md` explains why PR 34 sat unreviewed for a month.
Second, the vendoring machinery was carrying about 2,500 lines and three
scripts to solve a problem dira itself could solve in one PR.

## Decision
- dira exports `ledger` (with `ledger/local`, `ledger/fixture`,
  `ledger/ledgertest`), `frontmatter`, and `schema` as public packages
  (kazi-org/dira PR 41, tagged v0.2.0). Its `internal/` stays unversioned.
- Serenity requires `github.com/kazi-org/dira` in `go.mod` and imports those
  packages directly. `internal/dira`, its `PIN`, `verify-pin.sh`, `patches/`,
  `UPSTREAM.md`, and `scripts/update-dira.sh` are deleted. The
  `dira-vendor-pin` CI job is removed.
- `scripts/verify-dira-cli.sh` and the `dira-cli-conformance` CI job install
  the dira CLI at the version `go.mod` requires, read via `go list -m`, so the
  binary the conformance job runs and the library Serenity links are always
  the same commit.
- The in-memory `ledger.Store` used only by the vendored tests is deleted
  with them; dira's own test suite is the codec's contract now.
- Bumping dira is an ordinary `go get github.com/kazi-org/dira@vX.Y.Z`.

## Consequences
- ADR 008's "Vendoring" clause is superseded by this ADR. Its other
  decisions (precepts are unmodified dira entries, `applies_when` lives in the
  body block, Serenity owns the constraint matcher) stand.
- dira v0.2.0 carries the optional `applies_when` frontmatter field from
  PR 34. Migrating Serenity's writer from the body block to that field is a
  separate task, per ADR 008's "both forms are readable during the
  transition" clause; nothing in this ADR schedules it.
- The Go directive in `go.mod` moves from 1.26 to 1.26.5 because dira
  requires it.
- A dira change that breaks Serenity now surfaces as a failed `go get` or a
  failed test on the bump PR, not as silent drift between a copy and its
  source.
