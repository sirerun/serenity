# ADR 020: Edge and host trust posture -- unprivileged Caddy, trusted proxies, IMDS deny, origin lock-down

## Status
Accepted

## Date
2026-09-27

## Context
Deep review 001 found the hosted edge and host trusting more than they should. Caddy runs as
root because `deploy/hosted/caddy.service` sets no `User=`, and its default admin API listens
unauthenticated on `localhost:2019` because the Caddyfile has no global `admin` directive
(SEC-H01, CVSS 7.8); `deploy.sh` depends on that API for `systemctl reload caddy`. The app unit
does not deny the instance metadata service although the instance role can read the Stripe
secret and every backup (INF-03). Limiters key on `{remote_host}`, which behind Cloudflare's
proxy is a shared edge address (SEC-M03), and the origin security group accepts 80/443 from
`0.0.0.0/0`, so Cloudflare can be bypassed entirely (INF-02). `ajent.social` records that the
hosted candidate `v0.1.12-hosted-candidate` is deployed and serving as of 2026-09-26, so these
are live conditions. T23.54's contract said to keep inbound 80/443 unchanged.

On 2026-09-27 David chose: trusted-proxy fix now, security-group lock-down after the ACME
challenge path is confirmed.

## Decision
1. Caddy runs as a dedicated `caddy` user with `AmbientCapabilities=CAP_NET_BIND_SERVICE`,
   `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, and state under
   `/var/lib/caddy` (`XDG_DATA_HOME`, `XDG_CONFIG_HOME`). The admin API moves to
   `unix//run/caddy/admin.sock|0600`; `ExecReload` and `deploy.sh` pass
   `--address unix//run/caddy/admin.sock`. Bootstrap creates the user and directories and
   migrates existing certificates from `/root/.local/share/caddy` before the first start under
   the new unit, so no certificate is re-issued.
2. The Caddyfile declares `trusted_proxies static <Cloudflare IPv4 and IPv6 ranges>` and
   `client_ip_headers CF-Connecting-IP`, and forwards `X-Serenity-Client-IP {client_ip}`. The
   application keeps its loopback-only header trust; no application change is needed for the
   limiters to see real client addresses.
3. `serenity-hosted.service` sets `IPAddressDeny=169.254.169.254` (the app has no AWS SDK use).
   The backup unit keeps IMDS access because the AWS CLI needs it.
4. The origin security group is restricted to Cloudflare's published ranges only after
   `docs/launch/evidence/E24/acme-path.md` records which ACME challenge Caddy used for the last
   issuance and that renewal succeeds with the origin closed to other sources (HTTP-01 through
   the proxy, DNS-01, or a Cloudflare origin certificate). T23.54 carries that change; its
   "keep inbound 80/443" clause is replaced.
5. Alarms deliver: an SNS topic with `AlarmActions`, an `OnFailure` handler on the backup unit,
   and `deploy.sh` rolls back to the previous binary when readiness does not pass within the
   retry window.

## Consequences
- Positive: an application compromise no longer escalates to root or to a wiretap of tenant
  traffic; the review's attack chain AC-1 is broken at two links (SEC-H01 and INF-03).
- Positive: per-IP limits and the login rate limit key on the real client, so one noisy user
  behind a Cloudflare egress address cannot lock out others.
- Negative: certificate storage moves; the migration step must run before the first start under
  the new unit or issuance repeats against Let's Encrypt rate limits. The task's live
  verification checks `ss -ltnp` shows nothing on 2019 and that `caddy reload` succeeds.
- Negative: the Cloudflare range list is a static allowlist that must be refreshed when
  Cloudflare changes it; the deploy test pins the source URL and the review date.
- Related: E24 tasks T24.1, T24.2, T24.42 and the T23.54 amendment.
