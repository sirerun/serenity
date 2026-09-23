# Hosted deployment

The hosted binary embeds `site/`: `/` is the existing website, `/login`
requests a sign-in link, `/dashboard` is the account, and `/mcp` is the agent
endpoint. Website updates require a hosted binary release. Pages remains a
mirror and DNS rollback target.

## First canonical-domain cutover

Run these steps through SSM as root, using deployment files pinned to the
reviewed release commit. DNS changes use foundation's isolated
`serenity-site-dns` workflow, never a direct DNS API mutation.

1. Run `deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET cutover`. This validates
   Caddy, saves the prior configuration and binary path under
   `/root/serenity-domain-rollback`, and installs the release. The legacy host
   remains the login origin. The canonical host temporarily redirects there
   with HTTP 307, so forms always use the configured origin.
2. Apply foundation's reviewed one-record CNAME update. Wait for DNS and
   verify the canonical HTTPS connection and temporary redirect. Caddy obtains
   its certificate once DNS reaches the host. Test login on the legacy host
   during this phase.
3. Run `deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET final`. This changes
   the old default public origin to the canonical origin and activates browser
   redirects. Verify canonical landing, docs, login, dashboard and readiness.

The mode is an explicit fourth argument, so sudo cannot discard it. Subsequent
normal releases default to `final`. The migration preserves custom origins and
senders and moves only the obsolete default sender to `login@mail.sire.run`,
which was verified in Resend. Config ownership and private permissions persist.

Old GET links retain paths and queries; old browser form submissions lead to
fresh login. Host-only sessions require signing in again. Existing agent clients
continue through the legacy `/mcp` proxy without losing bearer authorization;
new dashboard connections advertise `https://serenity.sire.run/mcp`.

## Rollback

Run the pinned `rollback-domain.sh` through SSM. It restores the saved public
origin, Caddyfile and binary together, then checks readiness. Revert the CNAME
to `sirerun.github.io.` through foundation's isolated workflow. Keep the Pages
site and its custom-domain TLS certificate available throughout the cutover;
verify HTTPS on that target before reverting DNS. Do not overwrite the saved
snapshot during final activation. This rollback restores the pre-cutover
application configuration; it does not alter memory data or backup retention.
