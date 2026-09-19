# Live-load enablement requirements

The CLI returns BLOCKED/exit 2 with a zero-call receipt before it reads a manifest or credential or opens a socket. `main()` reaches neither `prepare_live` nor `run_live`, and a test proves it with a fully valid manifest, credential directory and running server (zero requests seen). No approval or spending is implied by this document.

The candidate client in `scripts/hosted/load.py` is local evidence, tested against a real loopback HTTP server. It is not a supported live-run entry point, and it never reports better than `PARTIAL`.

## Implemented and tested locally

Tests are in `evals/hosted-load/test_load_cli.py`.

| Requirement | State | Tests |
|---|---|---|
| Failure criteria over all offered requests | Every offered request ends in exactly one of nine outcomes. Completion, 5xx and admission rates use all offered steady-state requests, so skipped forgets and never-dispatched requests stay in the denominator. Status is `BLOCKED`, `FAIL` or `PARTIAL`, never `COMPLETE`. A threshold the client cannot measure is `pass: null`, not `true`. | `OfferedOutcomeTests` |
| Wall-clock deadline on each exchange | The exchange connects, owns every socket it creates (plain and TLS) and shuts them all down at the deadline. It does not rely on `conn.sock`, which `getresponse()` clears on `Connection: close`. Measured on loopback with a 0.6 s budget and a first byte at 0.5 s then a stall: 0.604 s (1.105 s before the fix). The response, connection and sockets close on every exit path. A completed TLS exchange, default certificate verification (a throwaway self-signed certificate is rejected) and a TLS stall are also covered. | `ExchangeWallDeadlineTests`, `TlsExchangeTests` |
| Bounded name resolution | An IP-literal origin skips resolution and starts no child. A hostname is resolved in a disposable child (`sys.executable -I -S` running a fixed source; argv is the hostname, the port and a self-destruct limit; empty environment; stdin and stderr discarded; stdout capped at 64 KiB and validated) that the parent waits on for the time left. At the deadline, or on any error, the child gets SIGTERM, SIGKILL after 0.2 s if it ignores that, and is reaped. Resolution, each connect attempt, the TLS handshake, the request, the headers and the body share one monotonic deadline. HTTPS connects to the resolved IP but sends the origin hostname as SNI and verifies the certificate against it, and the `Host` header stays the origin. Plain http filters the resolved list to its loopback addresses, connects only to those, and refuses only when none remain; a mixed list is not refused, and its non-loopback answers are never dialed. Measured with a 0.4 s budget and a stalled lookup: 2.008 s before (the whole stall), 0.401 s after. See [Deadline guarantee](#deadline-guarantee). | `BoundedResolverTests`, `TlsHostnameTests` |
| Deadline and cap check immediately before every socket operation | `RunBudget.reserve` runs before readiness, initialize, notification, tools/call and close, in the worker thread, so queued work is cancelled instead of launched late. Timeout is the remaining budget with no floor above it. | `PreSocketGuardTests`, `QueuedCancellationTests`, `RunBudgetLedgerTests` |
| Strict finite budget | Rejects missing, boolean, NaN, infinite, negative, zero-where-meaningless and malformed values, including `NaN`/`Infinity` literals that `json.loads` accepts. The ledger re-validates a hand-built budget and its `tokens` and timeout arguments. | `BudgetGuardTests`, `RunBudgetLedgerTests` |
| Conservative token precharge | Each tools/call charges the encoded bytes of its arguments, an upper bound because a tokenizer emits at most one token per byte. The synthetic per-arrival word count understates input several-fold. Readiness and every authenticated operation also charge an operator-declared `budget.provider_work_bound`; an undeclared bound blocks the run. | `TokenPrechargeTests`, `BudgetGuardTests` |
| Canonical exact origins, redirects, proxies | Origin must be byte-for-byte `scheme://host[:port]`: lowercase, no trailing dot, userinfo, path, query or fragment, no numeric-looking hosts. Production hosts are refused in any case or trailing-dot form, including in `allowed_hosts`. The client never follows a redirect and honors no environment proxy. | `CanonicalOriginTests`, `EnvironmentGuardTests`, `RedirectRejectionTests` |
| Credentials never in output | Failures carry an HTTP status number or a fixed exception class, never response or exception text. A long credential echoed by the server across a truncation boundary leaks no prefix. | `ErrorSanitizationTests` |
| Truthful cleanup and call counts | A session counts as closed only on a 200/204 reply to `DELETE`. Other statuses, redirects, drops and budget refusals are recorded per class. `attempted`, `sent` and `completed` exchanges are counted separately. | `CleanupStatusTests` |
| All repetitions and whole-workload budget | `prepare_live` replays the same arrivals for all three repetitions and blocks before any spend when the caps cannot cover the whole frozen workload. The committed manifest budget fails that check on calls, tokens and elapsed time. | `PrepareLiveTests`, `OfferedOutcomeTests` |

## Deadline guarantee

Each exchange has one monotonic deadline, `min(default timeout, remaining run budget)`. It covers:

- Name resolution through the resolver child (none for an IP literal).
- Every connect attempt, in address order, each bounded by the time left.
- The TLS handshake, the request, the response headers and the body.

It does not cover, and no result claims otherwise:

- **Scheduling tolerance.** Timer wake-up and thread scheduling can add up to `DEADLINE_JITTER_S` (0.25 s). The tests assert deadline plus that tolerance, and the run-level overrun check uses it.
- **Child start.** `Popen` (fork and exec) runs on the worker thread before the wait begins and is not itself interruptible. Its duration is not measured separately; a whole resolver child run (start, lookup, exit) took 20-37 ms locally.
- **Cleanup after the deadline.** A child that dies on SIGTERM, which is the case for a stuck lookup, adds nothing. A child that ignores SIGTERM adds up to 0.2 s. A child that survives SIGKILL (uninterruptible sleep) adds up to 1.0 s more before it is handed to a daemon thread that reaps it later.
- **The resolver itself.** The child runs the host's operating-system resolver, unmodified. The client validates the answer's shape, port and (for plain http) loopback, not its correctness.
- **Address order.** Attempts are sequential with no parallel fallback, so one unreachable address can spend the rest of the budget.
- **Latency.** Each hostname exchange spawns one child, and its time is inside the recorded latency (20-37 ms measured locally). Latency samples for a hostname origin are not comparable with IP-literal samples.
- **Platform.** Tested on macOS with Python 3.14. The child's `signal.alarm` backstop is skipped where `signal.alarm` does not exist.

This is a client-side guarantee checked against local sockets and a scripted resolver. It is not evidence about any live target, DNS provider or network path, and it does not close any item below.

## Still open

The unconditional CLI gate stays until every item below is closed.

- Seeded account, brain, cardinality and storage state is not verified or created. Offered forgets with no fact id are skipped and reported as failures, not hidden.
- The cold flag is a schedule label. No cold-open workload runs and no cold-ready time is measured.
- Quota-boundary saturation as a separate run is not implemented.
- CPU, RSS and disk telemetry are not collected, and no cross-tenant isolation probe exists. Durability covers only a forget of an id this run remembered.
- `provider_work_bound` values are operator declarations the client cannot observe, and the token charge is a byte bound, not a provider-billed count.
- `prepare_live` checks that the reviewer freeze, provider pin, privacy review, source SHA and binary SHA are present. It cannot check that they are authorized.
- A separately authorized real run against an authorized candidate.

Owner: T23.60 follow-up, with T23.41 review and T23.64/T23.68 qualification inputs.
