# ADR 023: Partner API for first-party apps (Blink)

Status: Accepted (founder decisions 2026-09-27)
Date: 2026-09-27

## Context

Blink, a Sire Run product, makes hosted Serenity its long-term memory. Every
Blink user gets a real Serenity account under their own email, usable from
Claude and other agents as well ("2 for 1"). Hosted Serenity today only admits
accounts through a magic link and clients through public PKCE OAuth. It has no
server-to-server surface, meters usage per account only, and revokes
credentials per brain, so revoking one client disconnects every client.

Founder decisions that this ADR implements:

1. Blink creates the Serenity account automatically for the user's email
   (disclosed in Blink's terms). If the email already has a Serenity account,
   Blink links it only after the account owner consents.
2. Blink reads and writes the account's default brain, the same one the user's
   other agents use.
3. Blink Free usage counts against the account's own plan. Blink Pro usage made
   by Blink is metered in a separate partner bucket with Builder-sized limits,
   and other clients stay on the account's plan. Blink Pro therefore never
   upgrades the account itself (no "$9 buys $19" loophole).
4. Deleting a Blink account revokes Blink's access only. The Serenity account
   and its memory remain the user's.

## Decision

### Partners

A `partners` table holds `id` (e.g. `blink`), `display_name`, `secret_hash`
(SHA-256 of a 32-byte secret), `status`, and `created_at`. The partner secret
is loaded from `/etc/serenity/secrets/partner-<id>` by the deploy script and
seeded by an admin-socket route; it is never stored in plaintext.

### Surface: `/partner/v1/*`

Every call carries `Authorization: Partner <id>:<secret>`. Comparison is
constant time. Each call writes an `audit_log` row (`actor=partner:<id>`). Rate
limit: 600 requests/min per partner. JSON in and out; errors are
`{"error":code,"message":...}`.

| Method and path | Purpose |
| --- | --- |
| `POST /partner/v1/accounts:ensure` `{email, email_verified:true}` | Find or create. **New** email: create an active account with no Stripe customer and `created_by_partner=<id>`, provision the default brain, and return `{account_id, created:true, linked:true}`. **Existing** account not created by this partner and not yet linked: return `{account_id, created:false, linked:false}` (the partner must use a link request). Emails are hashed exactly as identity does. Rejects `email_verified:false`. Exempt from `invite_only` and from `AccountCap`, but counted in a separate `PartnerAccountCap` (default 100,000). |
| `POST /partner/v1/link-requests` `{account_id, return_url}` | For an existing account. Returns `{link_request_id, consent_url, expires_at}` (15 min). `return_url` must match the partner's registered redirect prefix (`blink://serenity-linked`). |
| `GET /partner/consent?request=<id>` (browser) | Serenity page. The user signs in with the normal magic link if there is no dashboard session, sees "Allow Blink to read and write your Serenity memory?", and on Allow is redirected to `return_url?code=<one-time>&state=<request id>`. On Deny, `?error=denied`. |
| `POST /partner/v1/link-requests/{id}:redeem` `{code}` | One-time and 15 min. Marks the link active. Returns `{account_id, linked:true}`. |
| `POST /partner/v1/credentials` `{account_id}` | Requires a link (created or consented). Issues an API key bound to `(account, default brain, memory:read+write, partner_id)`. Returns `{credential_id, key}` once. Idempotent per account: an existing live partner key is rotated. |
| `DELETE /partner/v1/credentials/{credential_id}` | Revokes that key only (new per-key revoke; not `Issuer.Revoke`). |
| `DELETE /partner/v1/links/{account_id}` | Revokes all of the partner's keys for the account and ends the link. The account and brains are untouched. |
| `PUT /partner/v1/entitlements/{account_id}` `{tier:"free"\|"pro", expires_at}` | Sets the partner bucket tier. `pro` maps to the Builder limits for partner-bound traffic. `free` or expiry means partner traffic counts against the account's plan. |
| `GET /partner/v1/accounts/{account_id}/usage` | Account-plan usage and partner-bucket usage for the current window, for Blink's "memory full" state. |

The account owner sees the Blink connection on the dashboard Connections page
and can disconnect it, which revokes the partner link and keys.

### Credentials

The `client_credentials` table gains `partner_id TEXT NULL`. `credential.Binding`
gains `PartnerID`. A new `Issuer.RevokeKey(credentialID)` revokes one key
without bumping the brain epoch. Partner keys count toward
`MaxActivePerAccount`.

### Metering

A new `partner_entitlements` table holds `account_id`, `partner_id`, `tier`,
`expires_at`, and `updated_at`. The gateway picks a meter subject per call:

- The binding has a partner **and** a live `pro` entitlement: the subject is
  `partner:<id>:<account_id>` with the Builder plan limits, windowed by UTC
  calendar month.
- Otherwise: the account, as today.

The same subject selection applies in `operation.Ledger` quotas and in the
memories and storage inventory checks. Inventory (live memories and storage)
remains physically account-wide. For partner-pro calls the inventory cap is
the larger of the account plan and Builder, so a Blink Pro user's brain can
hold Builder-sized memory written by Blink. Writes by other clients are still
checked against the account plan, and that check counts memories Blink wrote.
This is accepted: storage is the user's, while the operations Blink pays for
are writes, recalls, and input tokens.

`limit_exceeded` keeps its current shape and adds `"bucket":"account"|"partner"`.

### Lifecycle

- Account deletion by the user works as today and also revokes partner links.
- Partner unlink never calls `DeleteAccount` or `DeleteBrain`.
- A partner-created account the user never signed into stays active; the user
  can claim it anytime with the magic link to their email.

## Consequences

- There is new code in identity (partner find-or-create), credential
  (partner_id and RevokeKey), meter (subject selection), service (the
  `/partner/v1` mux and consent page), and store (three migrations).
- The API goes live only through the existing tagged-release deploy.
- A leaked partner secret is high impact (create accounts, mint keys). It is
  mitigated by the secret file, rate limits, full audit, and the requirement
  that link requests for existing accounts go through owner consent. Rotate by
  replacing the file and hash.
