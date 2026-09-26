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

The `Caddyfile.cutover` and `rollback-domain.sh` files preserve the historical
September cutover procedure. They refer to the retired hostname and are not
a routine release rollback. Roll back a normal release to its previous binary
and canonical Caddy configuration; retain the canonical DNS record. Restoring
the historical hostname would require a separate reviewed DNS change.
