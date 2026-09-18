# Hosted implementation handoff

Status: implemented candidate, under verification; not deployed.
Owner: Codex hosted implementation lane on `feat/hosted-launch`.

## Running locally

Build `./cmd/serenity`, copy `deploy/hosted/config.example.json`, and configure
absolute data/secrets directories. Each secret file must be private and nonempty.
Run `serenity hosted serve --config CONFIG`. Production requires HTTPS public
origin, a loopback listener, Resend and embedding credentials. Development mode
requires a loopback HTTP origin and prints disposable login links to the local log.
Never enable development mode on a public listener.

`serenity hosted backup --data-dir DIR --snapshot DEST` coordinates with the
running service over its private Unix socket, flushes acknowledged writes and
snapshots SQLite plus canonical Git bundles. Offline backup acquires the service
writer lock. `serenity hosted restore --snapshot SNAPSHOT --data-dir EMPTY_DIR`
restores canonical data, revokes credentials/sessions and freezes accounts pending
billing reconciliation. Reactivation tooling is still an open release gate.

## Verification and limitations

The end-to-end Go test uses real HTTP, SQLite, Git and memory handlers with
explicit test mail/embedding adapters. It exercises two-account isolation,
credential revocation, CSRF, MCP session ownership, memory writes/recalls,
backup/restore, restart and deletion. Focused tests cover transactional admission,
operation replay, interrupted allocation and Stripe signature/reconciliation.
Successful memory writes now flush their canonical commit before acknowledgment.
Recovery scans the canonical projection once and reuses existing vectors.
The browser smoke exercises rendered forms; it does not prove real email delivery
or the quality of embeddings.

Infrastructure templates have only been syntax-validated. Deploy requires a
reviewed release version, archive SHA256 and stack-owned backup bucket; it
requires an already mounted data volume and installed Caddy/AWS/Git. It starts
an initial backup before enabling the hourly timer. No live resources or charges
have been created by this implementation lane.

## Remaining work with owners and acceptance triggers

All rows below are owned by the Codex hosted implementation lane. Plan contracts
remain authoritative; these rows record concrete follow-ups, not task acceptance.

| Follow-up | Dependencies | Measurable completion trigger |
|---|---|---|
| Complete billing lifecycle and entitlement consistency | Stripe fixtures, T23.22 | Grace/downgrade/duplicate-subscription tests plus Stripe test-clock evidence |
| Complete fair admission and rate-limit qualification | Gateway, T23.20 | Account/IP 429/Retry-After and multi-tenant capacity tests |
| Complete deletion retry and restore reactivation | Lifecycle, T23.26–28 | Failure/retry and billing-reconciled restore exercises |
| Bootstrap and observability | AWS config, T23.13–15/T23.29 | Fresh host deploy, disk/readiness/error/backup-age alarm evidence |
| Finish usage/storage admission qualification | Metering, T23.21–23 | Full plan limits and exact remaining-capacity tests |
| Actual client and public onboarding | Hosted origin, T23.17/T23.30–35 | Rakazo live connection and human walkthrough receipts |
| Capacity, cost and release rehearsal | Provider credentials, T23.36–38 | Fixed-cap load results, restore/rollback evidence and release checklist |
| Deployment and release | All release gates plus required credentials | Public HTTPS signup, real email, isolated memory and verified backups |

No external prerequisite substitutes for completing the remaining code and
fixture-based verification that can run without it.
