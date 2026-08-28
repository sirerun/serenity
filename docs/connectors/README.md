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
only connector-facing CLI command beyond `status`. The file watcher and
git-repo crawler need no authentication step and have no CLI command yet —
you construct and poll them directly through their Go packages
(`internal/connector/file`, `internal/connector/gitrepo`). Wiring all three
connectors into `serenity sync` and `serenity extract` for continuous,
scheduled ingestion is plan task T1.15, not yet built. Until then, treat
each connector's own package tests as the reference for its exact
behavior.

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
