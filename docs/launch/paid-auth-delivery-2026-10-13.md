# Hosted sign-in and browser handoff delivery — 2026-10-13

The browser handoff repair is deployed. Optional free WorkOS sign-in is reviewed
and merged, but is not deployed or enabled. Billing remains disabled pending
Serenity-specific provider credentials and acceptance. This is not a completed
paid launch.

## Source and deployment

| Delivery | Reviewed head | Landed source | State |
| --- | --- | --- | --- |
| [PR379: browser handoff](https://github.com/sirerun/serenity/pull/379) | `8e8b03a47e06df80af99e0d195179a14b6876a11` | `1319479700f2feb5df036bdada8565d13b8329be` | Deployed as signed sequence4 |
| [PR380: optional WorkOS](https://github.com/sirerun/serenity/pull/380) | `f8789d57bc55a73f34384d0ce3343f1431c45920` | `c93a416a734c9598a04dc56284bdc12c8b584a1e` | Source only; provider activation pending |

Both landed trees exactly matched their independently reviewed candidates.
Focused landed behavior checks passed. GitHub Actions did not start because the
account was locked for billing; the authorized local fallback was used. No
hosted CI success is claimed. Ajent tooling was unavailable; repository
coordination and trusted holds were checked. Existing unrelated work was preserved.

Initial local worktree audit found97 registered worktrees,17 dirty trees,
15 heads ancestral to main,23 with patch-equivalent work and59 with unmatched
patches. Unmatched patches are not a count of unfinished features. No worktrees
or unique changes were deleted.

## Verified behavior

A real Chromium fixture reproduced the original failure: consent and code
exchange could succeed before Chrome blocked a later cross-origin form redirect.
Consent approval/denial and Stripe Checkout/Portal now complete the form with a
no-store continuation document. Exact validated destinations, PKCE, CSRF, project
scope, escaping and single-use consent remain enforced.

Local validation passed full build/vet, full race-suite coverage, full lint with
zero issues,22 deployment contract tests, immutable workflow-pin checks and zero
reachable vulnerabilities from govulncheck. Nine filesystem/browser/service
packages were rechecked after using a private short temporary directory and
updating the lifecycle test for the new response. All six signup/OAuth browser
cases passed across desktop, mobile and dark viewports. The browser harness now
uses a test-only admitted service; no development bypass was added to production.

WorkOS passed full race suites for identity, store, dashboard, service, gateway
and backup, full build/vet, and lint for those six package trees. Independent
review checked PKCE, browser/session binding, explicit same-account linking,
subject replacement, account deletion and restore invalidation. Replacement
revokes browser sessions and pending linked attempts while retaining MCP grants.
Existing clickable magic links remain available.

## Signed production receipt

| Binding | Value |
| --- | --- |
| Release | `v0.1.14-browser-handoff`, sequence4 |
| Source tree | `d69b2dd75603860b932bbdc7a67d06bb34304bc8` |
| Manifest SHA256 | `c29e0811beecabf9cf4b18a6ff8ff18196b8215c75df3b1d439f3f6d3c1f7567` |
| Archive SHA256 | `e503d143492550a8f4f4d2b26874655f3bae3d7be00876fb7bf059bb6fff3edb` |
| Running executable SHA256 | `65906632de255875beb53bf40ac9cda510f804734609ba7f76ab8b8602fbb68e` |

Existing KMS signing, root verification, exact previous-state approval and the
locked prepare/install/readiness/measurement/commit transaction were used.
Independent transaction review passed before activation. Existing approved builds,
keys, data, journal and resources were retained; authority configuration version6
adds the new exact build. Concurrency stayed0 until an independent live policy
comparison confirmed the complete new-version policy and unchanged base deny
policy. Direct Lambda invocation was IAM-denied. Concurrency1 was then restored.

Fresh backup succeeded at04:10:50 UTC before quiescing. Independent postflight
confirmed sequence4, no pending transaction, exact process and broker bindings,
active service/broker/backup timer, and another successful backup at04:13:43 UTC.
Public health, readiness and OAuth discovery returned200; unauthenticated MCP
returned401. WorkOS and billing remained disabled.

No production restore or older-binary rollback is claimed. The source delta also
contains the previously reviewed quota recovery change that retains unsettled
governed reservations; schema compatibility alone does not make the older binary
a safe rollback. WorkOS's separate migration10 cannot be reopened by schema9
binaries and remains outside this deployed release.

## Actual Chrome/Gumloop observations

The existing Serenity connection appeared connected, listed its tools, and
refreshed successfully at04:14:26 UTC. A fresh standard OAuth request reached
Serenity with the expected Gumloop metadata publisher and Gumstack callback.
One Cancel click followed the actual callback chain into Gumloop with
`access_denied`, without becoming stuck on Serenity's consent page. The existing
connection remained present afterward.

A new approval was not submitted. Browser tooling requires action-time
confirmation for a new memory-access grant; autonomous task approval does not
replace that check. Thus fresh production approval and nonempty client
remember/recall remain unverified. Synthetic approval, exchange and replay tests
passed. No claim of universal client compatibility is made.

## Resume when credentials are supplied

The owner will populate existing private `.env` WorkOS test/production API key,
client ID and AuthKit domain fields, and Stripe test/live API keys. Never put
values in chat or source and never reuse Saka's application objects. The working
Saka source was consulted for the AuthKit wire format; Serenity retains its own
session and MCP authority.

Then the delivery agent should verify the selected environments and permissions,
configure Serenity-specific free AuthKit methods and exact callback, create or
reuse only Serenity Stripe products/prices/portal/webhook, and qualify actual
sandbox sign-in and billing lifecycle. The owner should only handle credential
entry, provider-required human verification and the fresh browser consent.
Ordinary API configuration and signed deployment remain agent work under the
existing authorization.

Use [the configuration guide](workos-onboarding.md) and
[ADR026](../adr/026-optional-workos-browser-identity.md). Preserve Free,
Builder $19/month and Scale $49/month. Measured economics and semantic-quality
launch gates still require qualification; this work supplies no new paid-provider
load evidence and does not silently reduce allowances. No paid WorkOS feature,
custom domain, enterprise SSO, new stack or real-card test charge was introduced.
