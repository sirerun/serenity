# E8 — Ajent agent readiness

This is a coordinated external epic for `ajent-social/ajent-social`, based on the Ora audit evidence supplied by David. Baseline: 98/100 Is Agentic readiness. Preserve Ajent's existing product behavior and visual language; every public change must be tested at the real HTTP or rendered-page boundary.

Fidelity: executable. Priority order follows the audit: failures first, then partial warnings. The accepted launch shape is multi-channel distribution (Homebrew, PyPI, and npm where each channel is technically appropriate), with both agent and human workflows and a public self-serve installation/signup path.

- [ ] A8.1 Brand discoverability  Owner: ajent-social  Est: 60m  kind: content  delivers: [canonical Ajent brand signal across homepage metadata, titles, structured data, and launch references]  deps: []
  - Acceptance: a clean `ajent.social` search has an indexed canonical Ajent result when rechecked after indexing delay; canonical, title, favicon, OpenGraph, and structured-data URLs all resolve without redirect chains. Record the exact search query and date; do not invent ranking claims.
- [ ] A8.2 Official CLI distribution  Owner: ajent-social  Est: 90m  verifies: [infrastructure]  deps: [A8.1]  acc: [official Ajent CLI packages install from Homebrew, PyPI, and npm where applicable, expose agent commands plus human login/browser handoff with documented JSON and exit behavior, and a clean-machine smoke test completes without source checkout]
  - Test: package-install smoke tests plus authenticated, unauthenticated, and public self-serve signup fixtures; never print credentials.
- [ ] A8.3 REST rate-limit headers  Owner: ajent-social  Est: 75m  verifies: [infrastructure]  deps: [A8.2]  acc: [authenticated and versioned REST responses include RFC RateLimit headers; HTTP 429 includes Retry-After and a documented body; tests assert status, headers, and response body at the real handler boundary]
  - Scope: preserve existing throttling semantics and document units, limits, reset semantics, and proxy behavior beside the API contract.
- [ ] A8.4 Developer resource discoverability  Owner: ajent-social  Est: 60m  kind: content  delivers: [searchable developer portal links, page titles/headings containing Ajent, and an llms.txt index of docs/OpenAPI/MCP resources]  deps: [A8.1]
  - Acceptance: `/docs`, `/openapi.json`, `/skill.md`, MCP setup, auth guidance, and canonical URLs are linked from the homepage or docs index; machine-readable files validate and no link redirects through unrelated hosts.
- [ ] A8.5 Homepage API/docs bridge  Owner: ajent-social  Est: 60m  verifies: [infrastructure]  deps: [A8.4]  acc: [homepage links to reachable `/docs` content that explains authentication, versioned endpoints, rate limits, examples, and MCP setup; browser test follows the link and asserts the expected headings]
- [ ] A8.6 Versioning and deprecation policy  Owner: ajent-social  Est: 75m  verifies: [infrastructure]  deps: [A8.3, A8.5]  acc: [the API policy documents URL versioning, compatibility guarantees, deprecation notice timing, and Sunset/Deprecation header behavior; a test asserts headers on a deprecated fixture route without changing current routes]
- [ ] A8.7 Readiness verification and handoff  Owner: ajent-social  Est: 60m  kind: operations  delivers: [updated Ora readiness scorecard with evidence links, remaining product decisions, and live endpoint/browser verification]  deps: [A8.2, A8.3, A8.4, A8.5, A8.6]
  - Acceptance: record the exact commands, URLs, status codes, headers, rendered headings, package versions, and any credential-dependent checks. Re-run the audit only after all prior evidence is fresh.

Resolved decisions: publish through multiple channels; support both agent automation and human login; include public self-serve installation and signup; emit rate-limit headers only on authenticated/versioned API routes. The deprecation policy is a 12-month compatibility window with at least 90 days' notice before removal, using `Deprecation` when announced and `Sunset` with an RFC 3339 timestamp for the removal date (see `docs/adr/014-ajent-api-lifecycle.md`).

Remaining credentials: authenticated search-console or press/listing access may be needed to prove indexing improvements; no credential is required to implement the API, docs, package, or browser changes.
