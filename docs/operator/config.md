# serenity.yml reference

`serenity.yml` sits at the root of a brain repository. `serenity init`
writes it, `serenity config set-model` and `serenity connectors auth imap`
edit single keys in it, and everything else reads it. It is committed
into the brain and synced through the brain remote like any other brain
file, so it is treated as synced content rather than as first-party input
(ADR 018): keys are strict, connector paths are contained, and the daemon
bind is loopback unless you opt in.

## Strict keys

Loading fails on any key the schema does not know, at any nesting level.
The error names the first unknown key by its dotted path and line:

```
parse serenity.yml: unknown key server.allow_lann (line 9)
```

A key is unknown when it is misspelled, when it belongs to a newer
Serenity release than the binary reading it, or when it was added to a
shared brain by something other than this binary. Fix the file and rerun;
`serenity check` and `serenity doctor` report the same error. There is no
lenient mode.

## Top-level keys

| Key | Type | Purpose |
|---|---|---|
| `version` | int | Schema version. `1`. |
| `models` | map | Pinned model set: `embedding`, `extraction`, `composer`, optional `provider`, optional `disable_thinking`. See [providers](../providers.md). |
| `index` | map | Derived-index engine: `engine: sqlite`. |
| `server` | map | Daemon HTTP transport. See [server](#server) below and [HTTP transport](server.md). |
| `families` | map | Predicate-family vocabulary: each name maps to `tier` and `half_life_days`. |
| `connectors` | map | Source connectors and the path allowlist. See [connectors](#connectors) below. |
| `ladder` | map | Earned-automation ladder policy (`default`, `per_cell`, `correlation_guards`, `never_automate`). |

Any other top-level key fails the load.

## server

| Key | Type | Default | Purpose |
|---|---|---|---|
| `bind` | string | `127.0.0.1:0` | Listen address as `host:port`. |
| `allow_lan` | bool | `false` | Required before `bind` may name anything other than a loopback address (`127.0.0.1`, `::1`, `localhost`). Without it, `serve --http` refuses to start, logs the refusal to stderr naming the bind and this key, and exits with `non-loopback bind refused`. A synced `serenity.yml` therefore cannot expose the daemon to the network on its own. |
| `client_ca_file`, `server_cert_file`, `server_key_file` | string | unset | mTLS; all three together, only meaningful with `allow_lan: true`. |
| `max_in_flight_calls` | int | `2048` | Concurrent MCP `tools/call` budget. |

```yaml
server:
  bind: "0.0.0.0:8443"
  allow_lan: true
```

## connectors

| Key | Type | Purpose |
|---|---|---|
| `roots` | list of absolute paths | Extends the directory allowlist that every connector `path` must resolve under. The home directory of the user running Serenity is always allowed. |
| `imap` | `{account}` | One Gmail mailbox. Written by `serenity connectors auth imap`; the app password lives in the OS keychain, never here. |
| `file` | `{path}` | One watched directory, polled each `serenity sync`. |
| `git_repo` | list of `{path}` | Repositories to crawl for documentation, one entry each. |

Every `path` is resolved to an absolute, cleaned path (a relative path is
resolved against the working directory of the process; use absolute
paths) and must lie under the home directory or one of `roots`. A path
outside every root fails `serenity sync` at connector build with the
resolved path named:

```
connectors.git_repo[0].path: "/srv/shared/repo" is outside every allowed root (/home/alice); add its root under connectors.roots to allow it
```

Each `roots` entry must itself be absolute; a relative entry fails the
same way. `..` segments are cleaned before the check, so a path cannot
climb out of a root.

```yaml
connectors:
  roots:
    - /srv/shared
  file:
    path: /home/alice/notes
  git_repo:
    - path: /srv/shared/repo-one
    - path: /home/alice/src/repo-two
```

A connector kind that is not one of `imap`, `file`, `git_repo` is an
unknown key and fails the load.

## Compatibility notes

- Unknown keys now fail the load with the key named. A brain whose
  `serenity.yml` carried an unrecognized key loaded silently before;
  it now stops every command that reads the config until the key is
  removed or corrected. `serenity check` reports the same error.
- Connector paths outside the home directory now need their root listed
  under `connectors.roots`. Brains that crawl repositories on another
  volume add that volume's directory there once.
- `serve --http` with a non-loopback `server.bind` and no
  `server.allow_lan: true` already refused to listen; it now also logs
  the refusal to stderr before building any listener.
