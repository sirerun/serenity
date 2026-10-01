# E24 continuation — 2026-10-01

Base: main `7b0baad`. Coordinator owns contracts, integration, review, aggregate
status, and final validation. Local merge gates follow ADR 024.

| Lane | Owner | Worktree suffix | Ownership and deliverable |
|---|---|---|---|
| Local acceptance audit | GPT-6-Luna audit_local | serenity-e24-audit-local-20261001 | Read-only source audit; `docs/plans/e24-local-audit-2026-10-01.md` only |
| Router and hosted readiness audit | GPT-6-Luna audit_router | serenity-e24-audit-router-20261001 | Read-only source/receipt audit; `docs/plans/e24-router-hosted-audit-2026-10-01.md` only |
| T24.23 disclosures | GPT-6-Luna disclosure | serenity-e24-disclosure-20261001 | README, threat model, operator erasure/export docs and disclosure contract test |
| Coordination and live evidence | Coordinator | serenity-e24-continuation-20261001 | Plan/roadmap/decision records; inspect hosted hardening and ACME read-only |

Workers do not edit shared status or hosted-owned implementation. Multi-package
builds are serialized with the shared lease. Current registry entries marked
planned/ready and partial receipts are not accepted hosted completion.

## Follow-up assignment and live result

The local audit found source tombstones retain history contrary to ADR019.
Worker audit_local additionally owns `internal/writer/tombstone.go`, a new
`tombstone_history_test.go`, and the narrow existing erasure assertion that
must change after history purge. It must prove red/green, retry safety and
unrelated-history preservation. Task claim: T24.21-followup. Coordinator
independently reviews and integrates; hosted files remain untouched.

Reviewed app/backup unit hardening is now installed on the existing host;
readiness and a cgroup IMDS refusal probe passed. The binary was not replaced.
See `docs/launch/evidence/E24/unit-hardening-2026-10-01.md`. T24.42 remains
partial until renewal behind origin lock-down is established.

## Docs-chat live inspection

A read-only Lambda configuration check found the existing docs-chat function
still uses plaintext `RATE_SALT` and no secret-reference variable. Its last
reported update is 2026-09-08. Merged T24.27 local code/tests therefore do not
establish live docs-chat hardening. The secret value was never printed or
stored in evidence. A scoped, reviewed deployment/migration remains needed;
no Lambda or secret was changed by this inspection.

## Source and audit follow-ups

Source-history regression independently reproduced on unchanged main7b0baad:
reachable source history, its old reflog entry, and its old blob survived.
The writer fix removes all three, preserves unrelated history and supports
idempotent retries. Author's full writer race suite and coordinator's focused
race tests pass. The existing supersession cascade must use this serialized
entry point as well; that integration is in progress. This does not expose a
new source-delete CLI or MCP verb.

The new partner best-effort audit failure now logs a fixed, sanitized error
while preserving the successful mutation response. Its regression injects a
missing audit table and checks that partner/account/secret/database details
are absent from the diagnostic. It does not invent a successful audit row.

The scoped docs-chat change set was subsequently executed: existing Lambda
and role updated without replacement; generated RateSaltSecret added. The
read-only environment check and four live checks passed. The secret adds
$0.40/month plus retrieval-call charges at published AWS rates. No GitHub
billing purchase was made. See E24/docs-chat-hardening-2026-10-01.md.
