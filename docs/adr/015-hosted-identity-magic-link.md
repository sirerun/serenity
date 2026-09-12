# ADR 015: Hosted identity is an email magic link sent through Resend

## Status
Accepted (David, 2026-09-11)

## Date
2026-09-11

## Context

The hosted launch needs one established identity method with session
security, logout and a usable recovery path, chosen from configuration that
actually exists. No OAuth client for Serenity exists in any repository secret
store, and creating one (GitHub, Google) needs a founder console action and,
for Google, consent-screen review. Rakazo users have an email address by
definition. Browser sessions must stay distinct from MCP client credentials
(ADR 014).

Options put to David on 2026-09-11: email magic link, GitHub OAuth, Google
OAuth, password plus email verification. He chose an email magic link and
named https://resend.com as the sender.

## Decision

- Signup and login are the same action: the person enters an email address,
  receives a single-use link, and clicking it creates the account on first use
  or opens the existing one. No password is stored anywhere.
- Login tokens: 256 random bits, stored as a SHA-256 hash with a 15-minute
  expiry and single-use consumption; the link carries the raw token once.
  Issuance is rate limited per email and per IP; the response never reveals
  whether an address is known.
- Sessions: 256 random bits, stored as a SHA-256 hash with a 30-day sliding
  expiry, delivered as a cookie that is `HttpOnly`, `Secure`, `SameSite=Lax`,
  host-only on `app.serenity.sire.run`. Logout deletes the row. Every
  state-changing dashboard request checks a per-session CSRF token and the
  `Sec-Fetch-Site`/`Origin` headers.
- Recovery is requesting a new link. Changing the account email is out of
  scope for launch.
- Email sending goes through the Resend HTTP API with `RESEND_API_KEY` read
  from the hosted secrets directory. The sender is `login@serenity.sire.run`,
  which requires Resend domain verification records in the `sire.run` Google
  Cloud DNS zone (a `sirerun/foundation` PR). Tests use an in-memory sender; a
  development mode prints the link to the server log only when
  `SERENITY_HOSTED_DEV=1` and the bind address is loopback.
- Missing items to name by name, never by value: `RESEND_API_KEY` and the
  Resend domain verification DNS records. Nothing else is needed for identity.

## Consequences

Positive: one external dependency, no password policy or breach handling,
recovery is inherent, no OAuth app to register, and the flow fits the
"Start free" journey with no card, company name or install.

Negative: sign-in latency includes email delivery; an attacker with mailbox
access owns the account (same as password reset everywhere); Resend outage
blocks new logins but not existing sessions or MCP access. If Resend domain
verification stalls, the fallback is a Resend sender on a verified `sire.run`
address, not a different identity method.
