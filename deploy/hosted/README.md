# Hosted deployment

## Unified public domain

The hosted binary embeds `site/`: `/` is the existing marketing website,
`/login` requests a sign-in link, `/dashboard` is the signed-in account, and
`/mcp` is the agent endpoint. Website changes require a new hosted binary
release; the Pages workflow remains a mirror and rollback target.

For the first cutover, deploy the unified release and Caddyfile before applying
foundation's isolated `serenity-site-dns` workflow. `deploy.sh` migrates only
an existing `https://app.serenity.sire.run` public origin; it preserves all other
operator configuration. Then update the website CNAME through foundation IaC
and verify HTTPS landing, docs, login, dashboard and readiness. Caddy obtains
the canonical certificate after DNS reaches the host. Old browser links retain
their path and query through permanent redirects; existing host-only sessions
require signing in again. Agent clients should update their configured endpoint
to `https://serenity.sire.run/mcp` (cross-host redirects may drop authorization).

Rollback requires reverting both the reviewed binary/Caddy configuration and
public origin, and reverting the isolated CNAME to GitHub Pages. The website is
embedded so assets always match the deployed application version.
