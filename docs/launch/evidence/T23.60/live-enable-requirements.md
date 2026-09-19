# Live-load enablement requirements

The CLI returns BLOCKED/exit 2 with a zero-call receipt before it reads a manifest or credential or opens a socket. `main()` reaches neither `prepare_live` nor `run_live`, and a test proves it with a fully valid manifest, credential directory and running server (zero requests seen). No approval or spending is implied by this document.

The candidate client in `scripts/hosted/load.py` is local evidence, tested against a real loopback HTTP server. It is not a supported live-run entry point, and it never reports better than `PARTIAL`.

## Implemented and tested locally

Tests are in `evals/hosted-load/test_load_cli.py`.

| Requirement | State | Tests |
|---|---|---|
| Failure criteria over all offered requests | Every offered request ends in exactly one of nine outcomes. Completion, 5xx and admission rates use all offered steady-state requests, so skipped forgets and never-dispatched requests stay in the denominator. Status is `BLOCKED`, `FAIL` or `PARTIAL`, never `COMPLETE`. A threshold the client cannot measure is `pass: null`, not `true`. | `OfferedOutcomeTests` |
| Wall-clock deadline on each exchange | The exchange connects, owns every socket it creates (plain and TLS) and shuts them all down at the deadline. It does not rely on `conn.sock`, which `getresponse()` clears on `Connection: close`. Measured on loopback with a 0.6 s budget and a first byte at 0.5 s then a stall: 0.604 s (1.105 s before the fix). The response, connection and sockets close on every exit path. | `ExchangeWallDeadlineTests` |
| Deadline and cap check immediately before every socket operation | `RunBudget.reserve` runs before readiness, initialize, notification, tools/call and close, in the worker thread, so queued work is cancelled instead of launched late. Timeout is the remaining budget with no floor above it. | `PreSocketGuardTests`, `QueuedCancellationTests`, `RunBudgetLedgerTests` |
| Strict finite budget | Rejects missing, boolean, NaN, infinite, negative, zero-where-meaningless and malformed values, including `NaN`/`Infinity` literals that `json.loads` accepts. The ledger re-validates a hand-built budget and its `tokens` and timeout arguments. | `BudgetGuardTests`, `RunBudgetLedgerTests` |
| Conservative token precharge | Each tools/call charges the encoded bytes of its arguments, an upper bound because a tokenizer emits at most one token per byte. The synthetic per-arrival word count understates input several-fold. Readiness and every authenticated operation also charge an operator-declared `budget.provider_work_bound`; an undeclared bound blocks the run. | `TokenPrechargeTests`, `BudgetGuardTests` |
| Canonical exact origins, redirects, proxies | Origin must be byte-for-byte `scheme://host[:port]`: lowercase, no trailing dot, userinfo, path, query or fragment, no numeric-looking hosts. Production hosts are refused in any case or trailing-dot form, including in `allowed_hosts`. The client never follows a redirect and honors no environment proxy. | `CanonicalOriginTests`, `EnvironmentGuardTests`, `RedirectRejectionTests` |
| Credentials never in output | Failures carry an HTTP status number or a fixed exception class, never response or exception text. A long credential echoed by the server across a truncation boundary leaks no prefix. | `ErrorSanitizationTests` |
| Truthful cleanup and call counts | A session counts as closed only on a 200/204 reply to `DELETE`. Other statuses, redirects, drops and budget refusals are recorded per class. `attempted`, `sent` and `completed` exchanges are counted separately. | `CleanupStatusTests` |
| All repetitions and whole-workload budget | `prepare_live` replays the same arrivals for all three repetitions and blocks before any spend when the caps cannot cover the whole frozen workload. The committed manifest budget fails that check on calls, tokens and elapsed time. | `PrepareLiveTests`, `OfferedOutcomeTests` |

## Still open

The unconditional CLI gate stays until every item below is closed.

- Seeded account, brain, cardinality and storage state is not verified or created. Offered forgets with no fact id are skipped and reported as failures, not hidden.
- The cold flag is a schedule label. No cold-open workload runs and no cold-ready time is measured.
- Quota-boundary saturation as a separate run is not implemented.
- CPU, RSS and disk telemetry are not collected, and no cross-tenant isolation probe exists. Durability covers only a forget of an id this run remembered.
- Hostname origins are unqualified for the deadline: `getaddrinfo()` cannot be interrupted, and no bounded resolver is implemented. Only IP-literal origins have an end-to-end deadline. Every result states its scope in `exchange_deadline_scope` and lists the gap.
- `provider_work_bound` values are operator declarations the client cannot observe, and the token charge is a byte bound, not a provider-billed count.
- `prepare_live` checks that the reviewer freeze, provider pin, privacy review, source SHA and binary SHA are present. It cannot check that they are authorized.
- A separately authorized real run against an authorized candidate.

Owner: T23.60 follow-up, with T23.41 review and T23.64/T23.68 qualification inputs.
