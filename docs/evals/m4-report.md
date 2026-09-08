# M4 exit verification

**T4.17 completed on 2026-09-08.** The [external-session completion](#external-session-completion-2026-09-08)
below records the successful follow-up. The initial 2026-09-07 attempt and its
then-open authentication blocker are retained as historical evidence.

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

At the end of the 2026-09-07 run, T4.17 remained open pending the external
session. The 2026-09-08 follow-up below completes that remaining clause.

## External-session completion — 2026-09-08

T4.17 is complete. David requested this verification; his existing Claude Code
login was available. A fresh CLI was built from `a258f1da5d9e42cf453bd177eb04757487f1810f`
(the unrelated uncommitted website changes do not affect the Go binary).
Claude Code 2.1.263 ran a real external session against `serenity serve --stdio`
in a fresh throwaway brain. Two original Ava corpus spans (`ava-said-05.yaml`
and `ava-said-07.yaml`) were seeded verbatim through `remember`, with their
repository paths as provenance. Neither the facts nor their IDs were supplied
in the question.

Invocation (paths abbreviated):

```sh
claude -p 'What did we decide about the sprint-planning cutover approach involving Ava? Use Serenity recall to retrieve the evidence. Answer only what the sources support; distinguish a stated preference from a formally approved decision. Include two source citations with their exact fact_id values and provenance paths from the tool response.' \
  --strict-mcp-config --mcp-config <client>/mcp.json \
  --setting-sources '' --tools '' --allowedTools mcp__serenity__recall \
  --output-format json
```

The MCP configuration names the freshly built Serenity executable with
`-C <throwaway-brain> serve --stdio`. Built-in tools were disabled and only
Serenity recall was preapproved. Session `cca129c5-afd4-4b3f-8fb7-270bb6ccc9be`
completed with three successful recall calls, each returning the two seeded
facts. One attempted `entity` call was permission-denied; it was not needed
for the answer. Keyword queries reported degraded search because no embedding
provider was configured; entity recall returned both facts directly.

Transcript excerpt:

> No decision was recorded — only a stated preference.
>
> What the memory supports: Ava expressed a preference for feature flags over
> a hard cutover, twice — once in the sprint planning session itself, and
> again in a follow-up email afterward.

Citations in the actual answer:

- `7f041ab903e036d7fa8597f3824a7be12c608876f803377d7a9810853f6d98c1` —
  `evals/corpora/ava/labels/ava-said-05.yaml`
- `d7ce240edfeb9933e18c984ee9c417c28ef56531be50d12119a41a5aa276fdc7` —
  `evals/corpora/ava/labels/ava-said-07.yaml`

The answer also explicitly limits the absence-of-approval conclusion to the
retrieved evidence. These are two source entries restating one preference,
not evidence of two independent decisions.

Verification: one external session completed; three successful recall tool
responses inspected; two of two cited IDs and provenance paths matched those
responses; two of two returned texts matched the original corpus spans.
The actual tool transcript and full answer are preserved in
[m4-external-session.json](m4-external-session.json), without authentication
material or model reasoning. This verifies external MCP retrieval and cited
answering over a two-entry Ava fixture, not whole-corpus extraction or semantic
search. The spending-ceiling item and full registered-endpoint authentication
sweep remain the separately dated evidence above; they were not rerun here.
