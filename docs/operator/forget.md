# Forgetting a memory fact

Call the MEMORY_VERBS v1 `forget` tool from an authenticated Serenity MCP
client. The tool removes the fact bytes and sidecar, purges derived index rows,
and rewrites the brain repository history to remove that source path from every
ref. It then expires reflogs and prunes unreachable Git objects. An expiry
record remains as the audit trail; it contains the forgotten source identity
and reason, not the fact text.

The rewrite runs inside the serialized writer job. Its marker tells the
post-commit hook to push with `--force-with-lease` after the forget commit.
After that push succeeds, the hook prints a warning that other clones must be
re-cloned. If the push fails, the marker remains and the next commit retries
the history rewrite push.

Use `serenity forget --help` for the command-line summary. The CLI command
explains the MCP path and does not directly perform forget; use
`serenity serve --stdio` or `serenity serve --http` to expose the tool.

This MCP procedure covers memory facts. Source tombstones through the internal
serialized writer also purge the target source path from Git history, reflogs and
unreachable objects, alongside removal of current bytes and index rows. It
preserves unrelated history and supports retries. No source-delete CLI command
is added by that internal API change.

## Hosted backups and exports

Hosted account or brain deletion removes the active brain directory. Backups
are separate copies and follow the configured versioned-bucket lifecycle.
The current configuration expires current versions after 30 days and
noncurrent versions 30 days after they become noncurrent. A version may spend
time in both states, so this permits retention for nearly two 30-day
intervals; it does not establish a 31-day maximum or a fixed purge deadline.

Hosted brain export at this checkout bundles all Git refs for its qualified
ordinary-source cohort. It refuses archived episode history, pending episode
exclusions and unsupported history before emitting a bundle; disabling capture
does not bypass that refusal. A history-free portable episode export is still
unimplemented. Successful ordinary-source exports contain Git history and must
be handled as historical copies.

This hosted lifecycle description reports the checked-in configuration; it
does not certify a live bucket's current state or prove that every historical
version has expired. See [the threat model](../threat-model.md#right-to-forget-the-deletion-chain)
for the erasure scope and limits.

## Episodic maintenance boundary

Episode exclusion immediately fences eligible reads and replay; it does not
mean historical copies have been erased. Internal authority-bound preparation
constructs an inactive candidate only. Active installation, crash recovery,
complete derivative closure and controlled-copy disposal remain separate gates.
Automatic capture remains disabled pending their qualification.

Cooperating canonical readers use a permanent shared view lock; maintenance
requires its exclusive ownership plus the existing writer lock and durable
marker. A surviving maintenance marker blocks ordinary readers and writers
until its exact operation is reconciled. Never remove marker or lock files to
clear an error. Matching local pins alone do not authorize recovery or erasure.

Only ordinary writer ownership initializes a missing view lock. Admitted
recovery may explicitly obtain that ownership before reading restored brains,
whose runtime directories are intentionally omitted. Read-only commands and cold
inspectors never initialize or repair it. Older binaries and direct filesystem
readers are outside this protocol and must be drained or qualified before any
active installation is enabled. The reader lock excludes maintenance, not
ordinary concurrent writes; existing queue/runtime snapshot rules still apply.
