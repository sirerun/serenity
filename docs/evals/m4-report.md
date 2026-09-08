# M4 exit verification

Date: 2026-09-07  
Build: `origin/main` at `0f06c08a3eb200cdef374e27c563450a819062d5`

## External-client check

The project-local integration was installed with:

```text
serenity -C <throwaway-brain> connect claude --config-dir <throwaway-project>
Installed .mcp.json and .claude/settings.local.json; free-text checks currently block as unverified.
```

An actual external Claude Code invocation was attempted with the generated
`.mcp.json`, `--bare`, `--strict-mcp-config`, and `--dangerously-skip-permissions`.
It stopped before creating a session because the CLI reported:

```text
Not logged in · Please run /login
```

This is the remaining human action for T4.17: authenticate Claude Code, repeat
the prompt below, and append its short transcript excerpt here. No Claude
answer is claimed by this report.

For the same throwaway brain, the real Serenity stdio MCP server was exercised
by an independent JSON-RPC client. It initialized successfully and returned
the five advertised memory tools. Two Ava decisions were remembered and then
recalled together:

```text
Question: What did we decide about X?
Answer: Ava decided to use feature flags for the migration cutover and to
prioritize the Beacon redesign.
Citations: fact_id=a361bd63d4ee3436aac0c0f2239e44fd79f4286734f3959889df36666f88cc70;
fact_id=5290101744c53cdaed9cbc2027a4be7e2abf599b8785851eea812b37824e3a0b.
```

The two citation IDs are returned by Serenity's `remember`/`recall` wire
responses; the corpus content and source provenance remain in the throwaway
brain only.

## Spend ceiling

The focused acceptance test `TestCheckAndRecordOverCeilingStagesEffectItemAndDoesNotRun`
passed under `go test -race`. A separate run seeded $45 against a $50 monthly
ceiling and attempted a $10 judgment call. Serenity returned
`recorded=false`, `month_to_date=$45.00`, and staged approval item
`76fcc26e38e4dec9b302cb748d96bcff`; the blocked row was absent from the spend
ledger. The existing test then accepts that item and verifies the held-back
row is recorded only after the accept verdict.

## Authentication sweep

Against a live `serenity serve --http` process on loopback, requests without a
bearer token returned HTTP 401 for every registered endpoint:

| Endpoint | Unauthenticated result |
| --- | --- |
| `/healthz` | 401 |
| `/mcp` | 401 |

The server logs exposed no token. The temporary brain and process were stopped
after the sweep.

## Validation

```text
go test -race -count=1 ./internal/spend ./internal/server ./internal/server/mcp  PASS
```

T4.17 remains open until a human logs Claude Code in and records the real
external-session answer. The machine-side MCP, ceiling, and auth evidence
above is complete.
