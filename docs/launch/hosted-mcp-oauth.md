# Hosted MCP OAuth integration

This candidate uses `github.com/ajent-social/go/mcpoauth`; the shared library
owns protocol validation and token lifecycle. Serenity owns browser sessions,
CSRF, project consent, account state, quotas and persistent SQLite storage.
It does not turn an OAuth client identity into account authority.

## Connecting

An OAuth-capable Streamable HTTP MCP client connects to
`https://serenity.sire.run/mcp`. Discovery supports dynamic registration or HTTPS Client ID Metadata Documents
(CIMD), followed by browser sign-in. The user explicitly selects a project and read-only or read/write
access. Approval never exceeds the client's requested scopes. Manual scoped
bearer credentials remain available for clients without OAuth support.

Registered clients are unverified; consent displays their supplied name and
actual callback host. Only HTTPS callbacks are admitted except explicitly
supported numeric loopback and exact `localhost:<port>` HTTP callbacks for
native clients. Localhost is a compatibility choice for Claude Code; numeric
loopback remains preferred. Every callback must exactly match registration,
and S256 PKCE is mandatory. For CIMD, the HTTPS metadata URL is the client ID;
the consent page also displays its publisher host. Client names remain unverified.

### Client ID Metadata Documents

The candidate advertises `client_id_metadata_document_supported: true` alongside
`registration_endpoint`. Existing dynamically registered clients keep their IDs.
A public PKCE client may instead publish an HTTPS JSON document such as:

```json
{
  "client_id": "https://client.example/oauth.json",
  "client_name": "Example client",
  "redirect_uris": ["https://client.example/callback"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"]
}
```

The document's exact `client_id` must match the requested URL. Metadata scope
claims never grant access: the authorization request and consent remain bounded
to `memory:read` and `memory:write`. Secret-based client authentication is not
supported. The resolver blocks redirects and non-public network targets, pins
connections to checked DNS results, limits responses to 64 KiB and fetches to
five seconds, and caps its cache at 128 entries for at most five minutes. Expired
cache entries are not used when a fetch fails. URL clients do not create DCR rows.
This implements the public-client subset of the [CIMD draft](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02).

### Consent troubleshooting

Each consent request expires after 20 minutes and can be used once. Restart
connection from the client after expiry, reuse or changed connection details.
Do not reload or resubmit a completed approval form. A permission mismatch is
reported separately from an expired/used request; storage or service failures
return a temporary-unavailability response. Logs contain only a bounded reason
category, never consent handles, tokens or account identifiers.

The 2026-10-10 Gumloop investigation verified a fresh request containing both
memory scopes, S256 PKCE, the correct MCP resource and its registered callback.
Read-only service inspection found a prior successful grant, an expired pending
request and another unexpired request. Those observations do not identify the
reported failed attempt. An approved, correlated live reproduction remains
required before claiming that Gumloop's failure is fixed. Candidate source tests
and discovery support do not establish production deployment.

## Security and lifecycle

- Access tokens last one hour. Refresh grants have a 30-day absolute lifetime,
  strict single-use rotation, and family-wide revocation on refresh reuse.
  Lost committed refresh responses can require signing in again. A grant is
  limited to 1,024 refresh generations; used hashes remain until grant expiry.
- Consent lasts 20 minutes to cover the 15-minute email sign-in link. The
  resume cookie contains only an opaque handle; there is no arbitrary next URL.
- Every request checks current account/project state and the project's durable
  revocation epoch. Issuance and refresh repeat live policy inside their write
  transactions. Project-wide revoke/delete invalidates outstanding codes and
  grants. Previously admitted work may finish after revocation.
- Refresh retains the MCP session's grant identity, while tool authorization
  uses the current request's token. Revoked sessions are evicted from the cache.
- Reauthorizing the same client for the same project replaces its old grant
  atomically. Active OAuth and manual credentials share the 16-connection cap.
- Every `/oauth` request is rate-limited per source prefix first (/24 for
  IPv4, /56 for IPv6; 120 per minute, 10 for registration, 30 for token and
  revoke). A global ceiling of 5,000 per minute covers only the unauthenticated
  state-creating paths, registration and authorize; token refresh and revoke
  are never refused by it, so a flood from a few prefixes cannot disconnect
  every tenant's agents. Pending consents are capped at 500 live per client,
  not per table; the other transient tables are capped at 10,000 rows. Unused
  registrations live one hour; pending flows and grants extend retention.
  Capacity eviction protects referenced clients.
- Restore deletes OAuth grants (cascading tokens), authorization codes and
  consent records before serving. A backup must never resurrect old tokens.
- Production proxy identity is trusted only from a loopback peer whose
  `X-Serenity-Client-IP` header is overwritten by the deployed proxy.

## Verification on 2026-09-24

Local fixtures use synthetic accounts, loopback email sign-in links and a
synthetic embedding provider. They do not exercise production email or prove
semantic retrieval, billing or cloud connector availability.

| Check | Result |
| --- | --- |
| AMSL `go vet ./...`, `go test -race ./...` | Passed, including callback policy and refresh concurrency |
| Hosted packages, MCP transport and site `go test -race` | Passed |
| Hosted/MCP/site lint | Passed |
| Playwright signup and OAuth consent | Six passed: desktop, mobile, dark |
| Native Codex 0.155.1 | OAuth login and actual `recall` completed; empty synthetic results |
| Native Claude Code 2.1.281 | OAuth login and actual `recall` completed; empty synthetic results |
| Claude cloud, Claude.ai, ChatGPT | Not qualified; requires the actual account/workspace connector path |

Regression tests cover one-time redemption across database reopen, concurrent
refresh across separate Store instances, account disablement, project revocation
races, restore invalidation, reference-preserving registration eviction, and
atomic replacement rollback. Browser tests exercise email login resumption,
CSRF, read-only default consent, real loopback callback navigation and disconnect.

Four initial headless Claude source reviews and follow-ups informed the fixes.
These are automated reviews, not an independent security audit. AMSL remains a
CANDIDATE pending human maintainer review. This document does not claim deployment.

Both native probes returned the actual MCP tool result
`{"protocol_version":1,"facts":[],"total":0,"results":[]}`. This verifies
authenticated transport against an empty fixture, not useful retrieval.
The Codex probe explicitly enabled and required its MCP server and enabled
its recall tool; exit status alone was not counted as a successful tool call.
Claude Code used its local loopback callback listener even in no-browser mode.
Test registrations and credentials were logged out after each probe.

## Token-bound identity snapshot

`GET /oauth/binding` reports the account, project, grant and granted scopes for
an OAuth access token after the same live checks used by MCP authorization.
See [Hosted binding v1](../protocol/HOSTED_BINDING_v1.md). This read-only,
non-cacheable snapshot does not attest operation status, spend ceilings,
index freshness, immutable exports or future authorization.
