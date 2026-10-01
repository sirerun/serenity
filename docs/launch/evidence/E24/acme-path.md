# E24 ACME path inspection — 2026-10-01

Read-only SSM inspection `ea4421a6-cf4b-46cf-b8d3-eaae7de77ba8` succeeded.
The hosted binary reports `0.1.13-hosted-partner-candidate`.

Caddy's journal records the last issuance using **HTTP-01** for
`serenity.sire.run`, followed by valid authorization and successful certificate
acquisition. Renewal-information polling is present; it is not a completed
renewal rehearsal with the origin restricted to Cloudflare.

Current live Caddy runs as `caddy:caddy`, is active, and uses
`unix//run/caddy/admin.sock|0600`; the TCP listener query found no port 2019
listener. Its trusted-proxy ranges match the static Cloudflare list in the
reviewed Caddyfile. Authoritative range source: https://www.cloudflare.com/ips/
(the list has not been freshly fetched in this inspection).

Live app `IPAddressDeny` and backup `OnFailure` are empty. Therefore PR310's
unit hardening is merged but **not live-qualified**. No configuration change or
service restart was performed by this inspection.

T24.42 remains partial: the challenge type is observed, but renewal behind
origin lock-down and a fresh published-range comparison remain unverified.
ADR020 requires that evidence before T23.54 changes origin ingress. Do not
infer that renewal-information polling proves renewal or authorize an ingress
change from this receipt.
