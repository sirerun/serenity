# Semantic readiness for fresh memory writes

Status: implementation under qualification; not released.
Owner: Codex, after keyed recovery and CLI ownership.

The real two-client proof needs a stop/extract/restart cycle because remember
only refreshes lexical chunks. A hosted writer cannot run a competing CLI index
job while holding ownership. Index the newly remembered eligible fact through
the supported remember path, using the configured embedding provider.

Keep readiness fields optional for older v1 responses; absence means unknown.
Return `search_state` separately from canonical success: `unavailable`,
`not_eligible`, `lexical`, or `semantic`. A provider/index failure does not erase
the canonical write or turn it into an ambiguous failed mutation. Replaying an
operation key recovers the same fact and retries its missing projection. Reuse
existing vectors under the same pin; do not scan/re-embed the whole brain.

The complete eligibility/provider/index decision runs inside the existing
writer queue, with a ten-second context deadline. This orders it with withdrawal
and other canonical writes. Private, expired and index-only sources are refused
before provider egress, using the existing canonical retrieval eligibility check.
Recheck TTL immediately before egress, bound the provider context by remaining
TTL, and withhold readiness/vector insertion if the fact expires during embedding.
Only a configured provider is used; no model pin, provider or credential is
installed implicitly. Without it, valid world facts remain lexical-searchable.

Acceptance: actual two-client semantic retrieval before restart/index CLI work,
lexical miss control, fresh vector readiness, exact-retry vector reuse, durable
provider-failure recovery, private-source negative plus eligible positive control,
and concurrent withdrawal ordering. Full tests, schemas, lint, vet and build.

This adds bounded synchronous provider latency to configured writes; canonical
mutations wait behind that queue work. It does not establish distributed jobs,
provider cost accuracy, ranking effectiveness, public eligibility or team ACLs.
