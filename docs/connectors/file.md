# File watcher connector

The file watcher connector (`internal/connector/file`, plan T1.3) ingests
plain files dropped into a directory tree you point it at — drops,
exports, PDFs, screenshots. It produces `file`-kind sources.

## Modes

The connector runs in one of two modes:

- **Watch mode** (`file.New`): a background `fsnotify` watcher covers the
  root directory and every subdirectory, adding new subdirectories as they
  appear. Use this mode on a local filesystem.
- **Poll mode** (`file.NewPoll`, the `--poll` fallback in RFC 0001's
  connector design): the connector rescans the entire directory tree on
  every `Poll` call instead of relying on OS-level change notification.
  Use this mode on filesystems where inotify-style events are unreliable
  or unavailable, such as network mounts.

Both modes share the same debounce and cursor logic; only how a changed
file is detected differs.

## Debounce

A file must go 2 seconds (`file.DefaultDebounce`) without a further
write before `Poll` treats it as stable and reads it. This absorbs the
burst of write events an editor or a download produces mid-write, so a
half-written file is never ingested. Construct with `file.WithDebounce` to
override the window, or `file.WithClock` to inject a clock in tests.

## What's skipped

- **Hidden directories.** Any directory whose name starts with `.` is
  skipped entirely, including everything under it.
- **Editor temp files.** Atomic-save scratch files matching `.*.tmp` and
  vim swap files matching `*.swp` are never emitted.
- **Non-regular files.** Symlinks, sockets, and similar are ignored.

## Cursor and re-ingestion

The cursor records, per relative path, the last-seen SHA-256, modification
time, and size. On the next `Poll`:

- An unchanged path (same size and modification time) is skipped without
  being read.
- A path whose modification time moved but whose content hash is
  unchanged (for example, a bare `touch`) is skipped too.
- A path that no longer exists is dropped from the cursor and not
  emitted.

Combined with the source store's content-address dedup, ingesting an
unchanged directory tree twice adds zero new sources.

## Setup

The file watcher has no CLI command yet (see the connector
guide's [what's wired today](README.md#whats-wired-today)). Construct it
directly:

```go
c, err := file.New(root) // watch mode
// or
c := file.NewPoll(root) // poll mode
```

Call `Close` on a watch-mode connector when you're done with it, to stop
the background goroutine. Poll mode has nothing to close.

## Limitations

- Poll mode rescans the full directory tree on every call, so its cost
  grows with the size of the tree.
- Hidden directories are always skipped; there's no option to include
  them.
- There's no CLI command to configure or run this connector — you use it
  through its Go package, and wiring it into `serenity sync` is plan task
  T1.15.
