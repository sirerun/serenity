# Hosted MCP OAuth integration

This candidate uses `github.com/ajent-social/go/mcpoauth`; the shared library
owns protocol validation and token lifecycle. Serenity owns browser sessions,
CSRF, project consent, account state, quotas and persistent SQLite storage.
It does not turn an OAuth client identity into account authority.

## Connecting

An OAuth-capable Streamable HTTP MCP client connects to
`https://serenity.sire.run/mcp`. Discovery leads to registration and browser
sign-in. The user explicitly selects a project and read-only or read/write
access. Approval never exceeds the client's requested scopes. Manual scoped
bearer credentials remain available for clients without OAuth support.

Registered clients are unverified; consent displays their supplied name and
actual callback host. Only HTTPS callbacks are admitted except explicitly
supported numeric loopback and exact `localhost:<port>` HTTP callbacks for
native clients. Localhost is a compatibility choice for Claude Code; numeric
loopback remains preferred. Every callback must exactly match registration,
and S256 PKCE is mandatory. No client metadata URLs are fetched.

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
- Public registration and token requests are rate-limited. Each transient table
  is capped at 10,000 rows. Unused registrations live one hour; pending flows
  and grants extend retention. Capacity eviction protects referenced clients.
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
