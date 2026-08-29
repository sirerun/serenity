# Upstream proposal: optional `applies_when` field (T3.15)

**Status: DRAFTED, NOT YET SUBMITTED.** A real PR against `kazi-org/dira`
requires review before it goes out under David's GitHub identity against a
repo we don't own; that review has not happened yet. This file is the
record of what's ready to submit and where to find it. When it's approved
and actually opened, this file gets a follow-up edit adding the real PR URL
-- the acc line this task satisfies ("records the PR URL and the decision
log") is only half-true until that edit lands.

## The decision this proposes

[ADR 008](../../docs/adr/008-precepts-on-dira-applies-when-in-body.md) put
`applies_when` in a fenced markdown body block instead of dira frontmatter,
because dira's vendored `entry.schema.json` (`schema/entry.schema.json`,
pinned at `PIN`) sets `additionalProperties: false` on the entry root and
every `$defs` object -- any structured field not already in the schema
fails validation. ADR 008 recorded that "the upstream PR proposing an
optional `applies_when` field is opened in parallel and never blocks." This
is that PR, drafted.

Nothing in Serenity depends on this PR merging, being reviewed, or existing
at all. `internal/direction` (T3.2) reads `applies_when` from the body
block regardless of what dira does upstream. If dira ships this field
natively someday, ADR 008 already names the consequence: "a future dira
schema field replaces the body block by migration; both forms are readable
during the transition." That migration is not scheduled and has no task.

## What was verified before drafting

- Cloned the real `kazi-org/dira` repo (not guessed at) to ground the
  proposal in its actual schema and validation code.
- Confirmed the vendored `internal/dira/schema/entry.schema.json` is
  **byte-for-byte identical** to upstream at the pinned commit
  (`15686940aa08a87244934e55247735febebee7cf`), which is also currently the
  tip of `kazi-org/dira`'s default branch -- no drift.
- Read `internal/ledger/entry.go`, `decode.go`, `encode.go`, and
  `enforcer.go` (dira's real Go sources, not this repo's vendored subset)
  to find exactly where an optional field slots into dira's hand-rolled
  YAML codec and its `Entry.Validate()` runtime gate, and to confirm
  `dira check`'s lexical matcher never needs to change (a constraint's body
  is already tokenized as prose today; this proposal doesn't touch that
  path).
- Searched `kazi-org/dira` issues and PRs for prior `applies_when`
  discussion -- none exists. Not a duplicate.

## What's ready to submit

- **Fork:** `dndungu/dira` (created via `gh repo fork kazi-org/dira
  --clone=false`, not yet used for anything else).
- **Branch:** `propose/optional-applies-when-field`, pushed to that fork.
- **Diff:** `schema/entry.schema.json` (new optional `applies_when`
  property + `appliesWhen` $def, required `action` / optional `params`,
  `additionalProperties: false` matching the schema's existing style;
  one added sentence on the `constraint` kind's description noting the
  field), `internal/ledger/entry.go` (`AppliesWhen` type, `Entry` field,
  a `Validate()` rule requiring `action` when the clause is present),
  `internal/ledger/decode.go` / `encode.go` (codec support, following the
  file's existing conventions), `schema/testdata/valid/canonical.md`
  (the fixture that exists specifically to exercise every optional field,
  now exercising this one too), and a new
  `internal/ledger/applies_when_test.go` (decode, round trip, the
  omitted-by-default case, the required-`action` rule,
  `additionalProperties` enforcement inside the clause, and schema/codec
  agreement -- six tests, all passing).
- **Verified in the real dira module** (not this vendored subset):
  `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test
  ./internal/ledger/... ./schema/...` green; full `go test ./...` green
  (one transient timeout on `internal/relbuild`'s goreleaser-snapshot test
  on first run, unrelated to this diff -- it passed clean on a second run
  once the network was less slow).
- **Disclosed open question, not papered over:** `params` is a free-form
  `map[string]any`, so its encode/decode goes through `yaml.v3`'s own
  marshaling rather than this codec's style-memo machinery that gives
  every other optional field byte-exact round-tripping of a human's
  original formatting. Values survive a round trip; a hand-edited
  `params` map's layout doesn't. Flagged in both the commit message and
  the drafted PR body as a question for the maintainer rather than
  silently forced in either direction.

## Drafted PR (title + body, exact text)

**Title:**
```
schema: add optional applies_when field to entries
```

**Body:**

> ## Why
>
> A downstream project (mine) puts a machine-checkable trigger clause on
> constraint entries -- an action name plus parameters, e.g. `{action:
> spend_over, params: {amount: 500, currency: usd}}` -- so a caller can ask
> "is this constraint live for this action?" without parsing prose. Today
> that clause has nowhere to live in `entry.schema.json`:
> `additionalProperties: false` is set on the entry root and on every
> `$defs` object, so any structured field not already in the schema fails
> validation outright.
>
> The workaround (a fenced `serenity:applies_when` block in the markdown
> body, invisible to dira's own schema and CLI) works, but it means a
> constraint's most useful machine-checkable fact is prose as far as dira is
> concerned, and every consumer of the ledger who isn't my own tooling has
> no way to discover or validate it.
>
> ## What
>
> Adds an optional `applies_when` property to the entry schema: an object
> with a required `action` (string) and an optional `params` (free-form
> object). dira takes no position on what either means -- the action
> vocabulary and param shape belong entirely to whatever tool consumes the
> ledger. It's most useful on `constraint` entries (noted in that kind's
> schema description) but isn't hard-restricted to them, matching how
> `alternatives` is already handled for `decision`.
>
> Mirrors the existing Go-side implementation:
> - `internal/ledger`: `AppliesWhen` type, `Entry.AppliesWhen` field,
>   `Validate()` rule (`action` required when the clause is present),
>   encode/decode support following this codec's existing conventions
>   (unknown keys inside the clause are rejected the same way every other
>   mapping in this schema rejects them).
> - `schema/testdata/valid/canonical.md` picks it up too, since that fixture
>   exists specifically to exercise every optional field.
> - New tests: decode, round trip, the omitted-by-default case (every entry
>   written before this change is untouched), the required-action rule,
>   `additionalProperties` enforcement inside the clause, and schema/codec
>   agreement (mirrors `TestValidateAgreesWithTheSchema`'s shape, narrowed to
>   this field).
>
> ## Why it's safe
>
> - **Fully optional and additive.** No existing required field, enum, or
>   per-kind rule changes. `TestAppliesWhenOmittedByDefault` proves an entry
>   with no clause encodes with no `applies_when` line at all -- nothing
>   built against the current schema needs to change.
> - **`go build`/`go vet`/`gofmt -l` clean; `go test ./internal/ledger/...
>   ./schema/...` green**, including the new tests. I ran the full suite too
>   -- everything passed except `internal/relbuild`, which timed out
>   fetching goreleaser in a network-restricted sandbox and touches nothing
>   this PR changes.
>
> ## Open question for you
>
> `params` is a free-form map, so its encode/decode goes through yaml.v3's
> own marshaling rather than this codec's style-memo machinery -- every
> other optional field in `internal/ledger` preserves a hand-edited value's
> exact source formatting on round trip; `params` currently doesn't (values
> survive, layout doesn't). I didn't want to guess at how much effort that's
> worth for a first proposal, so I left it as-is and called it out in the
> commit message and here. Happy to take a pass at byte-exact params
> preservation if that's a blocker for you, or leave it as a documented gap
> if it isn't.
>
> No behavior of `dira check`/the enforcer changes here -- this is schema +
> codec only.

## To actually submit it

```
gh pr create --repo kazi-org/dira \
  --head dndungu:propose/optional-applies-when-field \
  --base main \
  --title "schema: add optional applies_when field to entries" \
  --body-file <the body above>
```

Submitted 2026-08-29: https://github.com/kazi-org/dira/pull/34 -- David
authorized submission directly; a second review pass (leak/quality check
of the fork diff and PR body, run by a peer session) also cleared it with
no findings before it went out. Status: open, awaiting maintainer response.

## Follow-up required once submitted

- Edit this file with the real PR URL and its outcome (merged / open /
  changes requested / closed).
- If dira ships the field, open a follow-up task to migrate Serenity's
  writer from the body-block form to native frontmatter, per ADR 008's
  "both forms are readable during the transition" clause -- not scheduled
  today.
