# Connector guide

A connector is the code that turns something you already have — files on
disk, a git repository, an email mailbox — into `domain.Source` rows the
brain repo ingests. Every connector implements the same interface
(`internal/connector.Connector`, RFC 0001 §10.1): `Poll` fetches new items
since a resume cursor, and `ToSource` converts one fetched item into the
immutable, content-addressed source the store dedupes on. Nothing
downstream of `Poll` branches on which connector produced an item.

Run `serenity connectors status` to print this table from the command
line, with the path to each connector's page.

## Support matrix

| Connector | Ingests | Source kind | Auth | Doc page |
|---|---|---|---|---|
| File watcher | Plain files dropped into a watched directory tree (drops, exports, PDFs, screenshots) | `file` | None | [file.md](file.md) |
| Git-repo crawler | README files and doc-extensioned files under any `docs/` directory in a git working tree | `git_repo` | None | [gitrepo.md](gitrepo.md) |
| IMAP | Email from a Gmail mailbox over IMAP | `email` | Gmail app password, stored in the OS keychain | [imap.md](imap.md) |

## What's wired today

`serenity connectors auth imap` authenticates the IMAP connector and is the
only connector-facing CLI command beyond `status`. `serenity sync` polls
every connector configured under `serenity.yml`'s `connectors:` map and
ingests what it finds (plan T1.15):

```yaml
connectors:
  imap:
    account: you@gmail.com        # written by `serenity connectors auth imap`
  file:
    path: /path/to/watched/dir     # a single directory, polled (not watched) each sync
  git_repo:
    - path: /path/to/repo-one      # one entry per repository -- crawl 5 repos with 5 entries
    - path: /path/to/repo-two
```

The file watcher always runs in poll mode from `sync` (`file.NewPoll`), not
watch mode: `sync` is a one-shot CLI invocation, and watch mode's
background change-notification only accumulates events while something
keeps it running, which a one-shot process never does long enough to see
anything. Only one `file` entry is supported today — the connector's own
`Name()` doesn't vary by path, so two configured directories would share
one cursor. Configure as many `git_repo` entries as you have repositories;
each gets its own cursor from its own repo root.

A connector with no entry is simply not polled — a brain repo with nothing
configured yet runs `sync` as a no-op. `serenity extract` (or
`serenity extract all` — the same full pass today) then runs extraction and
embedding over every source `sync` has ever ingested; see
[the extraction and embedding section](#extraction-and-embedding) below.

## Extraction and embedding

`serenity extract` needs a pinned model plus a credential to do anything —
otherwise it reports why it skipped and exits cleanly, the same posture
`sync` takes for an unconfigured connector:

- `serenity.yml`'s `models.extraction` and `models.embedding` pins select
  the model (`none@v0`, the install default, skips that stage entirely).
- The provider is inferred from the model name: a name containing
  `claude` uses the Anthropic Messages API (`ANTHROPIC_API_KEY`); anything
  else uses an OpenAI-compatible chat-completions endpoint
  (`OPENAI_API_KEY`, or `OPENAI_BASE_URL` to point at a local
  Ollama-class server instead). Embedding always uses a real
  OpenAI-compatible `/embeddings` endpoint (`OPENAI_API_KEY`, or
  `OPENAI_EMBEDDINGS_BASE_URL` for a local server).
- Credentials are read from the environment, not the OS keychain — unlike
  the IMAP connector's app password, there is no keychain entry for these
  yet. This is a deliberate, minimal scope, not a final design.

## Re-auth path

If a connector needs a credential, re-running its `auth` command replaces
the stored credential — it's always one command, never a multi-step reset.
See [imap.md](imap.md#re-authenticating) for the IMAP walkthrough. The file
watcher and git-repo crawler have no credentials to re-authenticate.

## Shared guarantees

Every connector honors two invariants regardless of what it ingests:

- **Idempotent polling.** Replaying the same cursor returns the same
  items, and advancing the cursor never skips or duplicates a source.
  Content-address dedup in the source store (T1.2) means even a forced
  re-poll of unchanged content adds nothing.
- **No precept writes.** A connector only ever produces a `domain.Source`.
  None of the three can write to `.dira/`, the precept ledger — the
  git-repo crawler's precept-draft-candidate flag (see
  [gitrepo.md](gitrepo.md#precept-draft-candidates)) is metadata on a
  source, not a ledger write.
