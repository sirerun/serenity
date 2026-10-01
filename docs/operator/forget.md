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

This MCP procedure covers memory facts. The internal serialized source-tombstone
writer also purges the target source path from Git history, reflogs and
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

Hosted brain export at this checkout bundles all Git refs. It may therefore
include Git history; the history-free export default described in ADR 019 is
not yet reflected in this hosted export path. Treat an export as containing
historical data and handle it accordingly.

This hosted lifecycle description reports the checked-in configuration; it
does not certify a live bucket's current state or prove that every historical
version has expired. See [the threat model](../threat-model.md#right-to-forget-the-deletion-chain)
for the erasure scope and limits.
