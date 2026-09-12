# Hosted launch acceptance matrix

One row per requirement. States: `not-run`, `PASS`, `FAIL`, `PARTIAL`. Every PASS cites the source revision, the command or test, the observed result and the remaining limit. Real-process and protocol checks, not mocks. No secrets, no customer data.

| Area | Requirement | Source rev | Command / test | Observed | Remaining limit | State |
|---|---|---|---|---|---|---|
| Signup | Fresh account gets exactly one ready brain automatically; refresh, retry and crash recovery cannot duplicate it; no operator action | | T23.5 tests, T23.32 runs | | | not-run |
| Identity | Login, logout, recovery; forged and expired session denied; CSRF denied; account binding on every route | | T23.9, T23.35 | | | not-run |
| Isolation | Two accounts: cross read, write, export, delete, session and credential attacks denied | | T23.12 harness, T23.35 | | | not-run |
| Credentials | Issue, one-time display, rotate, revoke, old-session rejection, headless restart, log redaction | | T23.6, T23.7, T23.11, T23.29 | | | not-run |
| Memory | Real Rakazo save; paraphrase recall with lexical negative control; fresh write visible without rebuild; restart; retry dedup; scope isolation; citations; forget absent from every route | | T23.34 | | | not-run |
| Quotas | Concurrent boundary; durable reservation recovery; period rollover; account-wide limits; direct-MCP bypass denied | | T23.19, T23.20, T23.35 | | | not-run |
| Billing | Test Checkout to verified entitlement; forged, replayed and out-of-order events; failures; renewal; cancellation; upgrade and downgrade; portal isolation | | T23.21, T23.22 | | | not-run |
| Durability | Kill and restart at boundaries; backup restore into an isolated environment; no revoked-key or ended-plan resurrection; export and import sanity; delete and retention | | T23.26, T23.27 | | | not-run |
| Performance | Ten account-to-ready-endpoint runs with p95; landing-to-first-recall human walkthroughs; cold start; full-limit load; per-plan cost; initial capacity | | T23.32, T23.33, T23.36 | | | not-run |
| Release | Immutable artifact; CI; migration rehearsal; readiness; rollback; deployed smoke | | T23.14, T23.17, T23.37 | | | not-run |

## Timing detail

### Account-to-ready-endpoint (T23.32)

| Run | Start (UTC) | Duration (s) | Result | Environment |
|---|---|---|---|---|
| 1-10 | | | not-run | |

Small-sample p95: not computed.

### Landing-to-first-Rakazo-recall (T23.33)

| Participant | Duration | Notes |
|---|---|---|
| none yet | | not-run; do not fabricate participants |
