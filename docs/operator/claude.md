# Connect Claude Code

Install Serenity in a Claude project, with a separate brain directory:

```sh
serenity -C /path/to/brain connect claude --config-dir /path/to/project
serenity -C /path/to/brain connect claude --config-dir /path/to/project --print
```

`--config-dir` defaults to the current directory. Installation writes
`.mcp.json` and `.claude/settings.local.json` in that project. The MCP stanza
runs `serenity -C /path/to/brain serve --stdio`. The local settings add a
synchronous `PreToolUse` command hook matching `^ExitPlanMode$`, running
`serenity -C /path/to/brain check --claude-hook`. Paths are shell-quoted.
See the official [MCP configuration](https://code.claude.com/docs/en/mcp)
and [hook contract](https://code.claude.com/docs/en/hooks).

`--print` emits only the generated MCP JSON document, without creating files
or directories. Neither command accesses the keychain or includes tokens,
Authorization headers, credential environment variables or permission rules.
Stdio currently advertises zero tools; installation does not add memory tools.

Installation preserves unrelated settings, MCP servers, and hooks, including
other hooks for ExitPlanMode. Existing conflicting Serenity entries or invalid
configuration fail before either file is changed. Symlink config destinations
and the `.claude` directory are rejected. Each file is replaced atomically;
an I/O failure between replacements can leave only the first file installed.
Re-running completes installation. Identical configurations retain bytes and
modification times. Existing file permissions are preserved; new files use 0600.

The hook reads the injected `tool_input.plan` from stdin, never a plan-file
path or an embedded actions fence. Only a verified `pass` exits silently with
0. Every other outcome, malformed matching input, or engine error produces
an explanation on stderr and exit 2, which blocks ExitPlanMode. Unrelated
well-formed events exit silently with 0. It never auto-approves a plan or changes
Claude permissions.

**Current limitation:** free-text classification has no configured router, so
ordinary plan text is `unverified` and the installed hook blocks it. This
installer does not enable a classifier. Structured checks remain available
separately, using the ordinary CLI's unchanged exit semantics:

```sh
serenity -C /path/to/brain check --json --actions '[{"action":"spend_over","params":{"amount":100}}]'
```

To remove the integration, remove `mcpServers.serenity` from `.mcp.json` and
the Serenity command hook from `.claude/settings.local.json`, preserving other
entries. Review the free-text limitation before enabling this gate in daily use.
