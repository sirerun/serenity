# Hosted verification report — 2026-09-18

Scope: hosted Serenity candidate, draft PR #234. Verdict: **DEGRADED**; local
functional paths work, production release qualification is incomplete.

Architecture: server-rendered dashboard, opaque-token identity, SQLite control
store, per-brain Git/SQLite memory, bounded runtime pool, authenticated MCP gateway,
Stripe/Resend/embedding adapters, systemd/Caddy and AWS templates.

Read-only audit agents inspected identity/lifecycle and billing/quota/durability.
Confirmed defects fixed in this lane include cross-brain quota replay, duplicate
checkout creation, missing derived-index recovery, orphaned backup units,
non-default allocation recovery, unbounded credentials/session bindings, missing
usage/reset visibility, fixed browser-cookie expiry and deletion retry lockout.
A regression test caught migration bookkeeping still expecting one schema version;
it was updated to assert all applied versions after reopening the database.

Local checks are real Go HTTP/SQLite/Git operations with explicit test provider
adapters; browser checks use a loopback service. No provider test is represented
as live email, payment or semantic-quality evidence.

| Use case | State | Evidence | Remaining |
|---|---|---|---|
| UC-049: Sign up with email | PARTIAL | Real loopback Chrome signup; single-use/expiry tests | Real Resend delivery and public origin |
| UC-051: Provision private brain | PARTIAL | Concurrent allocation and retry recovery tests | SIGKILL rehearsal |
| UC-052: Issue client credentials | PARTIAL | Verifier storage, account-binding and real browser copy tests | Full log-redaction battery |
| UC-053: Revoke existing client access | PASS | Rotation and established-session HTTP 401 test | Local service proof |
| UC-055: Use isolated hosted MCP | PARTIAL | Four-tool discovery, two-account isolation and restart tests | Caddy and real Rakazo |
| UC-059: View usage and limits | PARTIAL | Dashboard renders all six allowances, remaining and reset; two-brain inventory test | Production capacity qualification |
| UC-060: Enforce shared allowances | PARTIAL | Concurrent boundary, cross-brain/cross-window retries, HTTP rate limiting | Storage ceiling and capacity qualification |
| UC-061: Upgrade plan | PARTIAL | Persisted checkout retry and duplicate-plan rejection fixtures | Real Stripe test mode |
| UC-063: Reconcile billing | PARTIAL | Signatures, current-state fetch, subscription consistency and grace fixtures | Full lifecycle table and Stripe test clocks |
| UC-064: Export canonical memory | PARTIAL | Browser download verifies canonical facts and Git bundle | Full export/import adversarial battery |
| UC-065: Delete account safely | PARTIAL | Delete/re-signup and restricted deletion retry tests | Retention purge guarantee |
| UC-066: Restore service data | PARTIAL | Canonical backup roundtrip and fail-closed credential/account restoration | Reactivation and real off-host recovery rehearsal |
| UC-071: Recover partial failures | PARTIAL | Allocation, reservation lease and derived-index recovery | Actual process-kill matrix |

Remaining remediation owners, dependencies and measurable triggers are in hosted-implementation-handoff.md. Public release acceptance remains in hosted-acceptance.md.

## Follow-up verification

The failed CI test in run 35388713465 was the file-first architecture gate:
hosted pool recovery called `InsertChunk` and `UpsertVector` directly. Commit
fe26d30 moves recovery into `index.RecoverMemorySearch` in the canonical rebuild
layer; the gate and its allowlist are unchanged. A regression test proves
canonical text repairs a tampered derived chunk and unchanged recovery reuses
existing vectors.

A fresh local full race suite passed 1,902 cases with six explicit skips;
full vet, lint and CLI build passed. The subsequently added recovery regression
also passed independently. Actual CLI browser checks passed at mobile and desktop
sizes (two cases, zero skips), covering signup, single-use login, token copy,
save, all six usage rows, ZIP export contents, revocation and logout. CI now
runs this browser battery using an explicit local embedding fixture. These tests
do not qualify live email, payments or semantic quality. Published pricing JSON
is checked byte-for-byte against the plans used by metering.
