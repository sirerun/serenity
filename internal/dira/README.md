# internal/dira — vendored dira, pinned

This directory vendors a subset of [kazi-org/dira](https://github.com/kazi-org/dira)
at the commit recorded in [`PIN`](PIN): dira's entry contract, the JSON Schema
validator, and the ledger codec (the reader/writer for `.dira/entries/*.md`
files). It exists so Serenity's precept store is a dira ledger the real `dira`
CLI can read unmodified, per RFC 0001 §7.3 and
[docs/adr/008-precepts-on-dira-applies-when-in-body.md](../../docs/adr/008-precepts-on-dira-applies-when-in-body.md).

## What is vendored, and what is not

| Path | Upstream source | Purpose |
|---|---|---|
| `schema/entry.schema.json` | `schema/entry.schema.json` | The entry contract (draft 2020-12 JSON Schema). |
| `schema/check.schema.json` | `schema/check.schema.json` | The `dira check --json` verdict contract. |
| `schema/schema.go` | `schema/schema.go` | Compiles and runs the entry schema against a file. |
| `ledger/*.go` | `internal/ledger/*.go` | The `Entry` type, the YAML frontmatter codec (`Encode`/`Decode`), the `Store` interface, `Add`/`AddEdge`/`AddTag`, and the `ReadOnly` wrapper (cst-0003 rule 1: a parent ledger is never written to by a child). |
| `frontmatter/frontmatter.go` | `internal/frontmatter/frontmatter.go` | Splits a file into YAML frontmatter and markdown body; depended on by both packages above. |
| `LICENSE`, `NOTICE` | repo root | Apache-2.0 license and required notice, copied verbatim per §4. |

**Not vendored:** dira's filesystem-backed `Store` implementation
(`internal/ledger/local`) and everything else in the upstream repo (the CLI,
the enforcer/check engine, the index, the site, fixtures, tests). Serenity
never reuses dira's own storage backend or its `check_plan` matcher — ADR 008
gives Serenity its own writer-queue-backed `Store` (`internal/direction`,
T3.3) and its own two-stage matcher (T3.5/T3.6), and reserves `dira check`
for CI conformance testing only (T3.14).

## No fork-and-edit

Every vendored `.go` file carries a header comment naming the pinned commit
(`vendor:pin=<sha>`, matching `PIN`) and is otherwise byte-identical to
upstream, except for two import-path lines rewritten to this module's path
(`github.com/kazi-org/dira/internal/frontmatter` and
`github.com/kazi-org/dira/schema` become
`github.com/sirerun/serenity/internal/dira/frontmatter` and
`github.com/sirerun/serenity/internal/dira/schema`) — required because the
files move from dira's module into ours, and because dira's own
`internal/ledger` and `internal/frontmatter` packages cannot be imported from
outside dira's module tree under Go's internal-package visibility rule.
`LICENSE`, `NOTICE`, and both `schema/*.json` files are untouched.

A local behavior change is never made by hand-editing a vendored file. It
lands as a `.patch` under [`patches/`](patches/) (see that directory's
README) and is applied by `scripts/update-dira.sh`, which also re-fetches
every vendored file from the pin. [`verify-pin.sh`](verify-pin.sh) is the
predicate that keeps this honest — it fetches the real upstream repo at
`PIN` and diffs every vendored file against it (reversing the two documented
import rewrites first), and runs as a CI job (`.github/workflows/ci.yml`,
`dira-vendor-pin`) on every push and PR.

## Updating the pin

```
scripts/update-dira.sh <new-commit-sha>
```

This re-fetches every file listed above at the new commit, reapplies any
patches under `patches/`, rewrites `PIN`, and updates each vendored file's
header. Run `go build ./...`, `go test ./internal/dira/...`, and
`internal/dira/verify-pin.sh` before committing the bump.

## Upstreaming

An `applies_when` field on `constraint` entries is proposed upstream in
parallel (T3.15, non-blocking) so a future dira release can carry it
natively; until then, Serenity reads `applies_when` from a fenced code block
in the entry body (`internal/direction`), which is invisible to dira's own
schema and CLI.
