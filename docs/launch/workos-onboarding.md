# Compatible WorkOS sign-in and paid onboarding

This delivery adds an optional browser identity provider to the existing hosted
service. Existing clickable email links remain available. Serenity remains the
owner of account IDs, projects, memory, MCP grants and Stripe customers.

## Customer journey

Sign in, receive a default personal project, connect an MCP client, remember and
recall, inspect usage, then upgrade when more capacity is needed. No card is
required for the Free plan. Existing project selection and read/write consent
remain explicit. Connecting an agent does not itself grant paid allowances.

The production MCP endpoint remains `https://serenity.sire.run/mcp`. The website,
MCP endpoint and sign-in provider URL are different destinations, not interchangeable
configuration values. Public copyable configuration contains no credentials.

## Free WorkOS boundary

Use AuthKit on the provided WorkOS domain. Use only included authentication
methods: Google/social login, email/password and optional passkeys. Keep Serenity's
clickable magic links; WorkOS Magic Auth is an emailed six-digit code and is not
a replacement for that existing experience. Do not enable enterprise SSO, custom
domains, Directory Sync, metered Radar or paid add-ons for this launch.

WorkOS currently lists AuthKit free through one million monthly active users.
A custom domain is listed at $99/month. Passkeys are bound to their enrollment
domain; changing the AuthKit domain requires users to enroll new passkeys. WorkOS
recommends a custom domain before production passkey enrollment; this launch's
free-only constraint instead retains the provided domain. Keep another working
login method available. These are provider terms observed on 2026-10-12, not a
promise about future provider pricing.

Sources: [WorkOS pricing](https://workos.com/pricing),
[Magic Auth](https://workos.com/docs/authkit/magic-auth),
[passkeys](https://workos.com/docs/authkit/passkeys).

WorkOS's Stripe add-on synchronizes organization entitlements and seats. Serenity
already owns account-level billing and quota reconciliation, so this integration
does not enable that add-on or introduce WorkOS organizations as billing authority.
See [WorkOS Stripe integration](https://workos.com/docs/authkit/add-ons/stripe).

## Identity continuity and recovery

A WorkOS subject must be durably bound to the intended Serenity account through
a verified provider response. An email string supplied by a browser is never
binding authority. Existing accounts use their current magic link to establish
ownership before adding WorkOS sign-in. New sign-ins must not create a second
account for an email that already owns memories or a subscription.

If an upstream identity is deleted or recreated, sign in with the existing magic
link and choose **Connect or replace alternate sign-in** in Settings. Replacement
requires verified matching email, revokes existing browser sessions and pending
link attempts, and preserves MCP grants.

After backup restore, require renewed identity binding through the existing
magic-link path rather than resurrecting stale external identity authority.
Deletion and disabled accounts must deny both login paths. Existing MCP grants
retain their own authorization and revocation checks; browser sign-in is not an
MCP access token.

## Provider configuration custody

The owner supplies WorkOS and Stripe credentials only in the private operator
`.env`. Never source that file as shell code, commit it, print it, copy it to
workers or add secrets to callback URLs. Test and production values are separate:

| Operator input | Intended use |
| --- | --- |
| WORKOS_TEST_API_KEY / WORKOS_TEST_CLIENT_ID / WORKOS_TEST_AUTHKIT_DOMAIN | WorkOS test environment only |
| WORKOS_PRODUCTION_API_KEY / WORKOS_PRODUCTION_CLIENT_ID / WORKOS_PRODUCTION_AUTHKIT_DOMAIN | Production WorkOS environment only |
| STRIPE_TEST_API_KEY | Stripe test operations only |
| STRIPE_API_KEY | Production Stripe configuration only |

The selected WorkOS API key belongs in the existing service secret custody under
`WORKOS_API_KEY`, not in public configuration. Provider setup must verify the
selected environment and exact callbacks before enabling login. Existing Stripe
runtime secrets remain `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET`; existing
server configuration selects `builder_price` and `scale_price`. A Checkout return
URL never activates a plan; verified reconciliation does.

Set runtime JSON `workos_enabled: true` and `workos_client_id` from the selected
environment. Register the exact callback
`https://serenity.sire.run/login/workos/callback`. Sign-in begins at
`/login/workos`; existing users add WorkOS from Settings through the authenticated
`POST /login/workos/link` flow. Authorization uses WorkOS official API endpoints,
which redirect to the hosted AuthKit UI; the operator AuthKit domain is used for
provider configuration verification, not as an arbitrary runtime endpoint.

Register only Serenity-specific Stripe products, prices and signed webhooks.
Reuse `/billing/webhook`, server-created Checkout and Customer Portal sessions.
Never modify another product's Stripe objects. No real-card test charge is
included in acceptance. The operator's current new-spend ceiling is zero.

## Pricing decision

Keep the published Free, Builder $19/month and Scale $49/month subscriptions
while completing onboarding. This avoids a new billing model and preserves
existing subscription liabilities. This is a competitive entry price, not a
claim of universally better value or measured profitability.

On 2026-10-12, [Supermemory](https://supermemory.ai/pricing/) listed Free with
$5 monthly credits, Pro $19 with $20 credits, Max $100 and Scale $399.
[Mem0](https://mem0.ai/pricing) listed Free with 10,000 adds and 1,000 retrievals,
and Starter $19 with 50,000 adds and 5,000 retrievals per month.
Serenity Builder has 5,000 successful writes and 100,000 recall calls, alongside
stored-memory, input-token and durable-storage caps. The meters and product
features differ, so these counts are not interchangeable benchmarks.

The proposed $9 personal entry point remains an experiment candidate, not a
published commitment. `hosted-economics.md` and T23.60 still distinguish estimated
costs from qualified full-limit capacity. Lower prices or higher allowances require
fresh evidence and a recorded commercial decision; no hidden allowance reductions
or automatic overages compensate for missing cost evidence.

## Required acceptance evidence

- Existing magic-link login, one-time use, logout and OAuth resume.
- Optional WorkOS signup/login, callback state and replay protection, safe existing
  account linking, disabled/deleted-account refusal and restore recovery.
- Actual Chromium consent navigation across a callback's additional redirect;
  approval and denial with exact destinations and no broadened security policy.
- Stripe Checkout and Portal browser handoff, signed webhook reconciliation,
  failed payment, renewal, upgrade/downgrade and cancellation in test mode.
- Real Gumloop, Zati, claude.ai and Codex connections with nonempty remember/recall,
  correct account/project, scope limits and revocation.
- Reviewed/merged source, signed deployed artifact, readiness and recovery.

Synthetic provider and local browser tests do not establish real WorkOS, Stripe
or MCP-client acceptance. Provider configuration and live gates remain pending
until their actual receipts are recorded. General MCP compatibility is a target;
only clients exercised end to end are called qualified.
