# Hosted deployment

The hosted binary embeds `site/`: `/` is the existing website, `/login`
requests a sign-in link, `/dashboard` is the account, and `/mcp` is the agent
endpoint. Website updates require a hosted binary release. Pages remains a
mirror and DNS rollback target.

## Current production domain

The website, dashboard, login, OAuth discovery and MCP endpoint all use
`https://serenity.sire.run`. Cloudflare is authoritative for `sire.run`.
The canonical hostname is a proxied A record to the existing hosted server;
it no longer depends on `app.serenity.sire.run`, whose Cloudflare record was
removed on 26 September 2026 at the owner's request. Clients using the retired
hostname must update their server URL to `https://serenity.sire.run/mcp`.

Run `deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET final` through SSM as root,
using deployment files pinned to the reviewed release commit. The release
embeds the public website and dashboard together. Verify readiness, canonical
HTTPS, sign-in, dashboard sections and OAuth discovery after deployment.

The explicit `final` argument is required. The deployment preserves custom
origins and senders, migrates only the obsolete default origin, and retains
private configuration permissions. Billing activation is separate.

Roll back a normal release to its previous binary and canonical Caddy
configuration; retain the canonical DNS record. Restoring the historical
hostname would require a separate reviewed DNS change; the retired cutover
configuration was removed (founder ruling, 2026-09-27).

## Blink partner provisioning

The hosted stack creates `serenity/hosted/BLINK_PARTNER_SECRET` with a generated
64-character value and grants the instance read access to that secret alone.
After deploying the ADR 023 tagged release, run `seed-blink-partner.sh` on the
instance through SSM as root. It installs the secret as a service-owned 0600
file and sends only its path to the private admin socket. Repeating this step
with the same secret preserves partner authentication; do not rotate a live
secret without coordinating Blink's SSM value and service restart.

Blink's operator copies this value securely to
`/blink/prod/SERENITY_PARTNER_SECRET`, sets partner ID `blink`, canonical base
URL `https://serenity.sire.run`, and a base64-encoded 32-byte encryption key in
Blink SSM. Secret values must stay out of shell traces, command bodies, logs,
PRs, and project channels. Configure all values before restarting Blink's
server and worker. Verify the partnership with test accounts separately.

The stack requires an explicit ARM64 AMI ID for `ImageId`. On an existing
stack update, pass the current instance's AMI ID. The previous SSM "latest"
AMI parameter could resolve to a different AMI during an unrelated update,
causing an unintended instance and data-attachment replacement. Inspect the
CloudFormation change set and reject replacements during partner-secret setup.
