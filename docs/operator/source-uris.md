# Source URIs and local paths

Every source Serenity ingests gets a `meta.yaml` record under
`brain/sources/<sha256[0:2]>/<sha256>/`. That record is committed to the brain
repo and pushed with it, so it must not reveal where files live on your
machine (deep review finding PRIV-03, plan task T24.24).

## Record shape

File-backed connectors write a location relative to the connector's own root,
plus a hash of the absolute path:

| Connector  | `uri`                                   | `meta.path`       | `meta.path_hash`            |
|------------|-----------------------------------------|-------------------|-----------------------------|
| `file`     | `file:<path relative to the watched directory>` | same relative path | sha256 of the absolute path |
| `git_repo` | `git-repo://<repo dir name>/<path relative to the repo>@<commit>` | same relative path | sha256 of the absolute path |

For example, a file at `notes/plan.txt` inside the watched directory records:

```yaml
kind: file
uri: file:notes/plan.txt
occurred_at: "2026-01-02T03:04:05Z"
meta:
    path: notes/plan.txt
    path_hash: 5b1f...e09c
    size: "12"
```

`path_hash` is the lowercase hex SHA-256 of the absolute path in cleaned,
forward-slash form (`connector.PathHash`). The same file always gets the same
hash, so re-syncing a tree yields the same `uri` and `path_hash`. Source
identity and dedup stay content-addressed on the bytes' SHA-256, as before.

## Where the absolute path lives

`serenity sync` records `path_hash -> absolute path` in the `source_paths`
table of the local derived index (`.serenity/index.db`). The index is never
committed or pushed. It's runtime state: deleting the index loses the table
along with the connector cursors, and the next `sync` re-polls from scratch
and fills it in again. A brain cloned onto another machine has the hashes but
not the paths, which is intended.

## Existing brains

The source-URI migration does not rewrite history. Sources written before
this change keep their
original `meta.yaml`, including an absolute `file://` URI and no `path_hash`,
and they still read, index, and cite exactly as before. Source records are
immutable and content-addressed, so re-syncing an unchanged file does not
replace the old record. Only new content gets the new shape.

If you need old absolute paths out of a pushed brain's history, you have to
migrate retained records and their history separately. Explicit erasure through
the serialized source-tombstone writer removes the whole target source path
from history; it does not replace a retained record's URI in place. To find
the affected records:

```sh
grep -rl '^uri: file:///' brain/sources
```

## Limits

- `path_hash` is an unsalted SHA-256. It doesn't reveal your directory
  layout, but someone who already suspects a specific full path can hash it
  and confirm a match.
- The `git_repo` URI and its `meta.repo` key still carry the repository
  directory's base name (for example `serenity`). They never include its parent
  directories.
- The voice-note connector (`internal/connector/voice`) still builds absolute
  `file://` URIs. `serenity sync` doesn't wire it, so no synced source uses
  that shape today.
