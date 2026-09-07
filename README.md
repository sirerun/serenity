# Serenity

Serenity turns your notes, repository documentation, and email into searchable
personal memory. It extracts claims from your sources, answers questions with
citations, and checks structured plans against your recorded constraints.

Your memory lives in a Git repository you own. Serenity builds on
[gbrain](https://github.com/dndungu/gbrain)'s approach to personal memory and uses
[Dira](https://github.com/kazi-org/dira) for the ledger of standing judgments,
called *precepts*.

Serenity is under active development. The CLI workflows below are implemented;
the full reconciliation and agent-protocol workflows are still being built.
“Serenity” remains a working title.

## Install

Building from source requires **Go 1.26 or later**. Running Serenity requires
**Git** and an accessible **OS keychain**; `init` stores its authentication token
there and fails if the keychain is unavailable.

```sh
GOWORK=off go install github.com/sirerun/serenity/cmd/serenity@latest
serenity --help
```

Ensure your Go binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when
unset) is on `PATH`.

To build this checkout:

```sh
make build
./serenity --help
```

## Start with local files

Create a brain repository outside your source-code checkout:

```sh
mkdir -p ~/my-brain
cd ~/my-brain
serenity init
serenity doctor
```

`init` creates the directory layout and `serenity.yml`, initializes Git, and
installs a post-commit push hook. Add a private Git remote and configure its
upstream if you want that hook to push your brain's commits. Without a remote,
the repository is your only copy.

Point `connectors.file.path` in the generated `serenity.yml` at a directory
containing your notes. Use an absolute path outside the brain repository:

```yaml
connectors:
  file:
    path: /absolute/path/to/notes
```

Then ingest and search:

```sh
serenity sync
serenity search "project priorities"
serenity status
```

`sync` polls configured connectors, commits new sources, and rebuilds the search
index. It runs once; it does not keep watching for changes. Files must be
unchanged for at least two seconds before the file connector ingests them.
Full-text search works without a model or API key.

You can run commands from any directory with `serenity -C ~/my-brain <command>`.

## Extract claims and ask questions

New brains start with all model pins set to `none@v0`. Extraction, embeddings,
and answer composition remain disabled until configured. `sync` imports sources;
`extract` is a separate step that turns those sources into claims.

The default provider for extraction and composition is OpenRouter. Set
`OPENROUTER_API_KEY` in your environment, then pin models using
`<model-id>@<version>` strings:

```sh
serenity config set-model extraction '<model-id>@<version>'
serenity config set-model composer '<model-id>@<version>'
serenity extract
serenity ask "What are my current project priorities?"
```

Replace the placeholders with the model pins you intend to use. Model calls send
source or query context to the configured provider. Credentials come from the
process environment, not `serenity.yml`.

Embedding configuration is separate and uses an OpenAI-compatible endpoint.
See [model providers](docs/providers.md) for credentials, model pins, and local
endpoint configuration.

## Connect other sources

Edit the `connectors` section of `serenity.yml` to add sources, then run
`serenity sync`.

| Source | Configuration and scope |
| --- | --- |
| [Local files](docs/connectors/file.md) | One directory tree under `connectors.file.path`; polled on each sync. |
| [Git repositories](docs/connectors/gitrepo.md) | Multiple `connectors.git_repo` entries; ingests README files and documentation under `docs/` at `HEAD`. |
| [Gmail via IMAP](docs/connectors/imap.md) | Authenticate with `serenity connectors auth imap`; the app password is stored in the OS keychain. |

Run `serenity connectors status` for the supported connector list. The
[connector guide](docs/connectors/README.md) covers setup and polling behavior.

## Check a plan

For a brain with active constraint precepts, check structured actions before
executing a plan:

```sh
serenity check --actions '[{"action":"spend_over","params":{"amount":500}}]' --json
```

The result reports the applicable constraints and the recorded reasons for any
violation. An empty or unrelated ledger returns `no_applicable_constraints`.

| Exit code | Meaning |
| --- | --- |
| `0` | `pass` or `no_applicable_constraints`; inspect the verdict to distinguish them. |
| `2` | `violated`. |
| `1` | `unverified` or an error. |

Free-text plan checks currently return `unverified` because the CLI has no
classification provider wired in. See the
[plan-check contract](docs/adr/010-plan-check-exit-codes-transport-and-docs-toolchain.md)
for details.

## Current limits

- `serenity search` currently uses full-text search, even with an embedding model
  pinned. Answer composition can use query embeddings when configured.
- There is no CLI command to launch a daemon. `serenity connect` reports token
  status; automatic MCP configuration and hook installation are not implemented.
  Token rotation is available through `serenity connect --rotate-token`.
- `serenity migrate --models` handles model-pin changes; it does not import a
  gbrain repository. Re-extraction through reconciliation remains unfinished.
- The full human-review queue and earned-automation workflow described in the
  design are not yet available end to end.

Go applications can use the [read-only package](pkg/serenity/serenity.go) to
access a brain in-process. See the
[embedding contract](docs/adr/012-embedded-read-facade-single-writer.md) for its
scope and ownership rules.

## Documentation

- [Design RFC](docs/rfc/0001-serenity.md) — intended behavior and data contracts.
- [Model providers](docs/providers.md) — configuration and credentials.
- [Connector guide](docs/connectors/README.md) — supported sources and setup.
- [Decision reviews](docs/operator/revisit.md) — scheduled reminders when a
  recorded revisit condition becomes due.
- [HTTP transport](docs/operator/server.md) — authentication and bind configuration
  for the server implementation.
- [Threat model](docs/threat-model.md) — trust boundaries and security assumptions.
- [Work plan](docs/plan.md) — implementation progress and remaining work.

## Development

```sh
make build   # Build ./serenity with CGO disabled
make test    # Run tests with the race detector
make vet     # Run go vet
make lint    # Requires golangci-lint
```

The Makefile sets `GOWORK=off` so a parent Go workspace does not affect this
module. CI also runs cross-builds for macOS arm64 and Linux amd64/arm64,
Dira compatibility checks, and cached evaluation fixtures.

## License

[Apache License 2.0](LICENSE).
