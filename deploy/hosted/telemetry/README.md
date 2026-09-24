# Hosted telemetry contract

The hosted runtime emits only fixed-cardinality metrics through `internal/hosted/telemetry`. Allowed operation names and metric units are closed lists; account IDs, emails, brain IDs, memory text, credentials, session IDs and request IDs cannot become dimensions.

The logger uses a bounded asynchronous queue. A full queue returns `ErrQueueFull` and increments a dropped counter; it never waits on a provider, disk or log sink while serving a memory request. Shutdown drains with a caller-supplied context and reports a timeout if the sink is blocked.

Structured fields are recursively redacted before JSON encoding. Authorization, Cookie, session, token, code, secret, key and magic-link query values are replaced, and the existing account/card/API-key/email redactor handles nested strings and errors. The current Caddyfile emits no access log by default, so proxy credentials are not written; enabling access logs requires a reviewed redacting sink before deployment.

Recommended operational contract:

- Metrics: one event per request outcome; readiness every 60 seconds; pool/disk/backup gauges every 60 seconds; provider cost/tokens aggregated per minute.
- Retention: logs 7 days, metrics 30 days; no customer content or high-cardinality labels.
- Missing data: emit a typed failure/deferred outcome; never invent zero-cost provider usage or deny a memory write because telemetry is unavailable.
- Cost: local JSON lines and bounded in-process buffering; no custom metric dimensions or per-request remote log writes.
