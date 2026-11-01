# Embedded Dira lexical checks

Serenity's optional `(*Brain).CheckLexical(ctx, plan)` calls the public Dira
`engine.Check` API over the existing authorized, read-only brain ledger.
No Dira executable, second ledger, provider call or writer is created.

The producer is Dira PR42, landed commit
`cd0ec6c16d813fe395ea3c6692e3ac01d6d734c6`. Consumption uses an immutable Go
pseudo-version resolved through the public module proxy and checksum database;
it does not create or imply a Dira release tag. `go.mod` and `go.sum` own the
exact version and integrity pins. There is no local `replace` directive.

`LexicalVerdict` aliases the producer's canonical JSON result. Conflicts remain
successful evaluations with `exit_code: 2`; unreadable ledgers, invalid input
and cancellation remain errors with no verdict. Private source citations retain
Dira's redaction. The input plan is echoed and may itself be sensitive.

This is Dira's offline lexical contract. Serenity's existing structured
`CheckPlan`, DIRECTION `check_plan`, CLI checks, memory methods, HTTP/MCP tools
and human disposition remain unchanged. A compliant lexical result is neither
execution authority nor proof of complete policy coverage. Parent inheritance
is not evaluated by this producer API. Brief/why, inheritance, lifecycle hooks,
skills and hosted tool exposure remain separate full-parity work.

Only the caller's existing brain binding supplies ledger access. This addition
does not choose hosted scopes, migrate custody, disclose private data remotely,
confirm staged decisions, or install hooks. Standalone Dira remains usable.

Validation for this source slice must cover the canonical conflict and private
citation behavior, missing/read-failing/canceled inputs, unchanged brain files,
existing structured checks, the facade surface/read-only gates and affected
direction/consumer regressions. Previously passing producer CI establishes the
producer revision; it does not qualify this new consumer or a deployment.
