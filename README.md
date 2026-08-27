# Serenity

> "Serenity" is a working title (it collides with SerenityOS); the rename
> decision gates the launch milestone. See RFC 0001.

Serenity is a claim-based personal memory and direction system — a
protocol-compatible successor to [gbrain](https://github.com/dndungu/gbrain),
whose substrate decisions (markdown repo canonical, derived index, frozen
wire protocol) it keeps verbatim and whose lineage it credits explicitly.
It ingests everything one person produces, reconciles what is true in a
**git-canonical brain repository**, serves that truth plus the person's
standing judgments to AI agents over open protocols, and gates every
consequential change through human disposition.

Full design: [docs/rfc/0001-serenity.md](docs/rfc/0001-serenity.md).

## Status: M0 (skeleton + substrate invariants)

What exists today:

- `serenity init` — scaffolds a brain repo (entities, sources, claims,
  `.dira/entries`), seeds `serenity.yml` with the pinned model set and the
  predicate-family vocabulary, initializes git with a post-commit push
  durability hook, and stores the daemon auth token in the OS keychain.
- Deterministic **fence writer** (entity pages) and **shard store**
  (append-only JSONL for high-volume families) — both byte-identical
  round-trip, property-tested, with the §7.2a authority rule enforced
  (shard beats a diverged fence head).
- **SQLite derived index** with the wipe-and-rebuild invariant:
  `rm -rf .serenity/ && serenity sync` reconstructs identical state from
  repo bytes. There are no database backups by design.
- `serenity sync`, `extract`, `doctor`, `status`.

Not yet here (see RFC §17 for the milestone order): connectors and
extraction (M1), reconciliation + the disposition queue + the
earned-automation ladder (M2), precepts and plan-check (M3), the protocol
servers (M4), gbrain migration (M5).

## Install

```sh
go install github.com/sirerun/serenity/cmd/serenity@latest
```

Releases ship single static binaries for macOS (arm64) and Linux
(amd64/arm64) via goreleaser + a brew tap.

## Quickstart

```sh
mkdir my-brain && cd my-brain
serenity init      # scaffold + git + keychain token; warns loudly if no remote
serenity doctor    # health: config, durability, keychain, index
serenity sync      # (re)build the derived index from repo bytes
serenity status
```

## Development

```sh
make build   # CGO_ENABLED=0 go build ./cmd/serenity
make test    # go test -race ./...
make vet
```

The load-bearing invariants are enforced by tests, not convention:

- `internal/store` — fence round-trip byte-identity (property test), the
  10,000-claim shard property test (append / resolve / rebuild / merge
  with deterministic, order-independent head resolution).
- `internal/index` — wipe-and-rebuild identity, including the
  fence/shard-disagreement fixture (the shard is canonical, §7.2a).

## License

Apache-2.0.
