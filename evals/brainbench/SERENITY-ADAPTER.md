# Serenity's BrainBench adapter (T1.21)

`README.md`, `fixtures/`, `gold/`, `schema/`, `_ledger.json`, and `LICENSE` in
this directory are vendored **verbatim** from
[`dndungu/gbrain`](https://github.com/dndungu/gbrain) at the commit pinned in
`PIN` — do not hand-edit them; `verify-pin.sh` diffs them against upstream in
CI, and `../../scripts/update-brainbench.sh <new-pin>` is the only sanctioned
way to move the pin. This file, `gen_trend.go`, and
`../../internal/eval/brainbench/` are Serenity's own code, not part of the
vendored corpus.

## What this adapter actually measures

BrainBench's full harness (see the vendored `README.md`) scores four things an
autonomous memory agent does: decide when to retrieve unprompted
(`know-to-ask`), decide what to inject when it does (`push`), persist
conversational facts into storage (`write-back`), and recall a decision across
harness sessions (`continuity`). Serenity does not have an autonomous
retrieval-decision layer or a write-back pipeline wired to this eval (that
lands with T1.9 and T1.12) — what exists today is `serenity search <query>`,
invoked on demand.

`internal/eval/brainbench.Evaluate` maps the one part of BrainBench that
comparison is honest about onto that surface: for every gold turn marked
`should_retrieve: true`, it builds a fresh, fixture-scoped index from that
fixture's own `seed_pages`, runs the turn's text through Serenity's real
hybrid search (`internal/search.Search`, T1.11 — FTS-only, since no live
embedder is wired; zero model calls), and scores the retrieved entity slugs
against `gold_slugs`/`acceptable_slugs`: precision hits count a retrieved slug
in `gold_slugs ∪ acceptable_slugs`; recall hits count a `gold_slugs` entry
that was actually retrieved (an `acceptable_slugs` hit never counts toward
recall — see the BrainBench README's own "injected = fine, missed = no
penalty" rule).

**Skipped, not silently scored** (`Report.FixturesSkipped` names every one and
why):

- Fixtures with no gold file.
- Fixtures with zero `seed_pages` — write-back fixtures and continuity-reader
  fixtures seed their content by having a *writer* fixture persist a fact
  first; without T1.9's write path there is nothing to index.
- Fixtures whose gold turns are all `should_retrieve: false` — `kta-neg` and
  `write-back` fixtures test suppression or fact persistence, not retrieval
  quality.

**Not modeled**: multi-source fixtures (`ms-*`) are scored on the union of all
`seed_pages` regardless of `source_id` — Serenity's search surface has no
per-source visibility concept yet, so a `should_retrieve:false` turn whose
point is cross-source suppression is skipped at the per-turn level (its gold
turn just isn't `should_retrieve:true`) rather than scored as a pass.

## A defect this adapter works around, not fixes

BrainBench turns are full conversational sentences
(`"What did Alice Example say about the Widget Co deal?"`). Passing one
verbatim into `internal/index.SQLite.SearchFTS`'s `chunks MATCH ?` throws a
SQL logic error the moment the text contains an FTS5 syntax character — most
commonly `?`, since these are questions — rather than degrading to a
no-match. `internal/cli/search.go` and `internal/search.Search` (T1.11) pass a
caller's query straight through with no escaping, so a real
`serenity search "what's the deal?"` invocation hits this exact crash today.
`internal/eval/brainbench.ftsQuery` sanitizes the query locally (tokenize,
double-quote each token per FTS5's documented literal-text technique, OR them
together) rather than patching `internal/search`/`internal/index`, which are
T1.11's already-merged, concurrently-touched (T1.15) surface — out of this
task's scope. `internal/eval/brainbench/evaluate.go`'s `ftsQuery` doc comment
carries the full detail; see also `docs/lore.md`.

## Running it

```
go test ./internal/eval/brainbench/...      # unit tests + the real vendored corpus, no artifact written
go run evals/brainbench/gen_trend.go        # writes evals/brainbench-trend.json (gitignored, CI-only)
```

CI runs both on every push (`ci.yml`'s `test` and `brainbench-trend` jobs);
neither needs network access — the corpus is vendored, not fetched.
`evals/brainbench-trend.json` is this run's score row only; T5.10 is what
persists rows like it across runs on a results branch and renders the trend
chart into the docs site.
