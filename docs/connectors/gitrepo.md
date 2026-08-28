# Git-repo crawler connector

The git-repo crawler (`internal/connector/gitrepo`, plan T1.5) walks a git
working tree at the state of `HEAD` and turns its documentation into
sources. It produces `git_repo`-kind sources.

## Scope

The crawler ingests two kinds of file, at any depth in the tree:

- **README files**, matched by name regardless of case or extension:
  `README`, `README.md`, `Readme.txt` all match.
- **Doc-extensioned files under any directory named `docs`**: `.md`,
  `.mdx`, `.markdown`, `.txt`, and `.rst`.

Everything else in the repository — source code, configuration, binary
assets — is out of scope. This connector ingests documentation, not the
codebase.

`.gitignore` is respected using git's own matcher: the crawler runs `git
ls-files --cached --others --exclude-standard` rather than reimplementing
gitignore semantics, so nested files and negation patterns behave exactly
as git itself resolves them.

## The brain repo is excluded by default

Your brain repo is itself a git repository, holding the dira ledger under
`.dira/entries/`. Crawling it would re-ingest your own precepts and claims
as if they were new source material. If you set `Config.BrainRoot` to your
brain repo's path, `Poll` compares it against the crawled repo's resolved
top level and returns zero items on a match. Set
`Config.IncludeBrainRepo` to `true` to opt back in.

## Precept-draft candidates

No file this connector reads is ever a precept, and nothing it does can
write one — `Poll` only reads bytes, and `ToSource` only builds a
`domain.Source`. When a crawled file happens to decode, byte for byte, as
a well-formed dira ledger entry (frontmatter valid against
`internal/dira/ledger.Entry`, ADR 008's schema), the resulting source
carries a `precept_draft_candidate: true` metadata flag, so you can later
choose to promote it through the disposition queue. That flag lives on
the source's metadata; the connector has no code path that writes under
`.dira/`, so a document whose prose instructs "create precept X" has
nothing to act on — it either fails to parse as a ledger entry (the common
case, since prose isn't frontmatter) and ingests as an ordinary source, or
it parses and still only earns the metadata flag.

## Cursor and re-ingestion

The cursor stores the last-seen `HEAD` commit SHA. `Poll` is a no-op until
the repository's `HEAD` moves — even a forced re-poll of an unchanged
`HEAD` returns zero items, and the source store's content-address dedup
means a moved-then-reverted `HEAD` still adds no new sources.

## Setup

Configure one entry per repository under `serenity.yml`'s
`connectors.git_repo` list (see the connector guide's
[what's wired today](README.md#whats-wired-today)), then run
`serenity sync` — `sync` sets `BrainRoot` to your brain repo automatically:

```yaml
connectors:
  git_repo:
    - path: /path/to/repo-one
    - path: /path/to/repo-two
```

Crawl five repositories with five entries — each gets its own `Name()`
(derived from the repo root's base name) and so its own cursor. There's
still no CLI command to author this config; edit `serenity.yml` directly.

To use the package directly instead, construct it yourself:

```go
c := gitrepo.New(gitrepo.Config{
    RepoRoot:  "/path/to/repo", // any path inside, or at, the repo
    BrainRoot: "/path/to/brain-repo", // excluded by default; leave empty to skip this check
})
```

## Limitations

- Only documentation is ingested — README files and doc-extensioned files
  under `docs/`. There's no option to widen the scope to other file
  types.
- The crawler reads the tree at `HEAD` only; it doesn't walk commit
  history.
- It shells out to `git`, so `git` must be on `PATH`.
- There's no CLI command to author `serenity.yml`'s `connectors.git_repo`
  list; edit the file directly.
