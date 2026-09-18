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
| UC-052: Issue client credentials | PARTIAL | Verifier storage and account-binding tests | Full browser copy/log-redaction battery |
| UC-053: Revoke existing client access | PASS | Rotation and established-session HTTP 401 test | Local service proof |
| UC-055: Use isolated hosted MCP | PARTIAL | Four-tool discovery, two-account isolation and restart tests | Caddy and real Rakazo |
| UC-059: View usage and limits | PARTIAL | Dashboard renders metered usage and reset | Memory/storage/remaining limit display |
| UC-060: Enforce shared allowances | PARTIAL | Concurrent boundary, cross-brain/cross-window retries, HTTP rate limiting | Storage ceiling and capacity qualification |
| UC-061: Upgrade plan | PARTIAL | Persisted checkout retry and duplicate-plan rejection fixtures | Real Stripe test mode |
| UC-063: Reconcile billing | PARTIAL | Signatures, current-state fetch, subscription consistency and grace fixtures | Full lifecycle table and Stripe test clocks |
| UC-064: Export canonical memory | PARTIAL | Coordinated export implementation | Full export/import adversarial battery |
| UC-065: Delete account safely | PARTIAL | Delete/re-signup and restricted deletion retry tests | Retention purge guarantee |
| UC-066: Restore service data | PARTIAL | Canonical backup roundtrip and fail-closed credential/account restoration | Reactivation and real off-host recovery rehearsal |
| UC-071: Recover partial failures | PARTIAL | Allocation, reservation lease and derived-index recovery | Actual process-kill matrix |

Remaining remediation owners, dependencies and measurable triggers are in hosted-implementation-handoff.md. Public release acceptance remains in hosted-acceptance.md.
