# Hosted launch acceptance matrix

Current execution: [post-PR234 completion plan](hosted-plan.md), with per-task receipts specified in [evidence.md](hosted-completion/evidence.md). PR234 is merged; none of the public qualification rows below becomes PASS merely from that merge. Coordinator folds accepted receipts here in T23.70.

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

## Local candidate evidence — 2026-09-18

Historical source: `feat/hosted-launch`, PR #234 (subsequently merged at `b9863824`). The matrix above remains the public
release gate; the following local evidence does not make a hosted-origin PASS.

| Area | Local evidence | Remaining limit | State |
|---|---|---|---|
| Signup and identity | Real Chrome loopback signup/save; concurrent single-use links, expiry, CSRF, logout and deletion-session tests | Resend delivery, public TLS and controlled mailbox runs | PARTIAL |
| Provisioning | Concurrent default allocation; additional allocation resumes without restart | Actual SIGKILL fault injection | PARTIAL |
| Isolation and credentials | Two-account real HTTP MCP test, cross-session 401, rotate/revoke and deletion | Complete browser/export adversarial battery | PARTIAL |
| Memory and recovery | Save/recall, restart, coordinated backup canonical facts; wiped derived index rebuilt on open | Real Rakazo and semantic paraphrase negative control | PARTIAL |
| Quotas and admission | 100 reservations admit exactly 60; cross-brain and cross-window retry tests; HTTP 429 with Retry-After | Complete storage boundary and multi-tenant capacity qualification | PARTIAL |
| Billing | Signature, duplicate/out-of-order current-state reconciliation, checkout reuse after restart, fixed grace deadline, subscription-plan consistency fixtures | Real Stripe test clocks and full lifecycle table | PARTIAL |
| Deployment | CloudFormation validate-template; shell syntax; backup units installed by deployment script | Fresh VM/bootstrap, alarms, backup upload and rollback rehearsal | PARTIAL |

Full pre-audit local race suite: 1,895 passed cases, six explicit skips, 66 tested
packages and eight packages with no tests. Focused post-repair tests, vet, lint
and build passed; CI results are attached to PR #234. Test adapters are
explicit and do not represent real email or embedding-provider qualification.

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
