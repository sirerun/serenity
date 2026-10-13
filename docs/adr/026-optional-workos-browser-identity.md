# ADR 026: Optional WorkOS browser identity with existing account authority

Status: Accepted scope by the owner on 2026-10-12; source implementation and
provider qualification remain subject to the paid-auth delivery gates.

## Decision

Use free WorkOS AuthKit as an optional browser sign-in provider. Keep Serenity's
clickable magic-link login and existing server-side browser sessions. The service
continues to own account IDs, projects, memory, MCP grants, metering and Stripe
customer/subscription state. WorkOS does not become the MCP authorization server
or the billing/entitlement authority in this change.

A server-side exchange with the fixed WorkOS API verifies the authorization code.
Use single-use durable state, a browser binding, PKCE and exact callback routing.
Bind the verified WorkOS subject and configured client ID to a Serenity account;
never use a client-supplied email or account ID as binding authority. Preserve
provider environment separation even if subjects appear identical.

For an existing account, linking requires a current Serenity session (the existing magic-link path provides recovery),
CSRF-protected explicit linking intent,
a verified matching provider email and the same session at callback completion.
Do not silently merge an existing account by email. New identities may create a
Free account only under the existing registration and account-cap policy. A
binding to an unavailable account never grants login. Normal returning sign-in
resolves the durable provider subject rather than moving memories when provider
profile fields change.

Explicit replacement through the same authenticated linking flow can recover a
deleted/recreated provider subject. A replacement revokes all local browser sessions
and pending linked sign-ins before issuing a new session; MCP grants remain intact.
A subject already bound to another account cannot be adopted.

Restore clears external identity bindings and pending provider login requests,
alongside existing session/OAuth invalidation. Users re-establish binding through
their existing magic links. Account deletion removes external bindings and
pending linked logins. This conservative recovery path avoids resurrecting
external identity authority from an older snapshot.

Use the WorkOS-provided AuthKit domain and free methods only. Passkeys stay bound
to that domain. No custom-domain subscription, enterprise SSO, organization/seat
billing or WorkOS Stripe add-on is part of this decision. The existing Resend
magic-link path remains necessary, including recovery.

## Browser handoff

A form response that redirects into a provider's redirect chain remains subject
to Chrome's form-action checks. Synthetic tests reproduced a successful OAuth
code exchange followed by blocked navigation. Complete approved/denied consent
and billing form submissions with a no-store continuation document. Only the
exact server-generated validated provider destination is rendered. It navigates
as a document, with an escaped fallback link, strict CSP and no referrer. Do not
add callback wildcards, arbitrary next URLs or weaken project authorization.

## Consequences

This retains existing customer identities and billing code with a small provider
boundary. Existing customers take an explicit linking step once. WorkOS failure
does not remove the magic-link recovery path. Users can still have an upstream
WorkOS session after local Serenity logout; Serenity access requires a new local
login/session and its own current account checks.

Provider setup is separate from source delivery. Real WorkOS, Stripe and named
MCP-client acceptance are required before calling the hosted launch complete.
See [delivery plan](../plans/paid-auth-2026-10-12.md) and
[configuration and acceptance guide](../launch/workos-onboarding.md).
