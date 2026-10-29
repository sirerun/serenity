# Native hosted MCP connection preparation

`serenity connect hosted --host codex` and
`serenity connect hosted --host claude-code` print a JSON plan. The command
does not contact the service, read credentials, execute native commands, or
change configuration. `--write-scope` explicitly adds `memory:write` to the
proposed scope set; the default is `memory:read`. Consent remains a separate
native-client action.

The plan targets the existing production Streamable HTTP MCP endpoint
`https://serenity.sire.run/mcp`, using the OAuth route described in
[hosted-mcp-oauth.md](hosted-mcp-oauth.md). It accepts no alternate endpoint.
The separate `serenity hosted` command administers the server and is not a
prerequisite for native remote-MCP login.

The native command shapes were inspected in installed Codex 0.165.0 and Claude
Code 2.1.309 help. This is CLI configuration evidence, not authentication,
installation, hook semantics, or runtime qualification. Claude Code uses its
native `/mcp` interface for authentication; no CLI login command is assumed.
If its actual requested scopes cannot match the proposed scope set, stop and
report that capability gap rather than silently granting additional access.

Before executing the printed commands, verify current service accessibility,
the intended account/project, and any existing MCP entry named `serenity`.
Preserve native trust and unrelated hooks. The printed add commands do not
resolve name conflicts or implement a rollback manifest. Operators must
prepare those configuration protections before installation. Do not pass a
token as a command-line argument. OAuth custody stays with the native client.

Acceptance requires authenticated binding verification and a bounded synthetic
recall through each actual client, with explicit unavailable or unknown status.
Configuration or consent alone does not establish this acceptance. Follow the
host's requirements for loading tools in a fresh session.

This preparation slice does not qualify RFC0003 or implement memory enforcement,
history discovery/import, lifecycle capture, or production episode enrollment.
The existing episode capture client remains synthetic-only. Real conversations
must not be uploaded to a synthetic qualification service. The plan therefore
reports binding and recall as `unverified`, hooks as `unqualified`, and automatic
capture as `disabled` until their separate delivery gates are satisfied.
