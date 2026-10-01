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
