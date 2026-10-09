# Hosted OAuth binding v1

`GET /oauth/binding` returns the server-observed account and project binding
for the exact OAuth access token in the `Authorization: Bearer` header. The
endpoint accepts no query parameters, account selectors, project selectors,
or grant selectors. Callers must use TLS in production and must not put a
token in a URL.

The response is generated only after the OAuth server verifies the access
token, its resource, effective scopes, grant family and expiry. Serenity then
reads the account, project and current OAuth revocation epoch from the hosted
control database. It returns `401` with a Bearer challenge when the token is
missing, expired, revoked, issued for another resource, bound to a disabled
account or non-ready project, or carries an old project epoch. A query string
is rejected with `400`; methods other than GET return `405`.

Successful responses are JSON and carry `Cache-Control: no-store` and
`Pragma: no-cache`:

```json
{
  "schema": "serenity.oauth-binding/v1",
  "issuer": "https://service.example",
  "resource": "https://service.example/mcp",
  "account_id": "opaque-account-id",
  "account_state": "active",
  "project_id": "opaque-project-id",
  "scopes": ["memory:read", "memory:write"],
  "grant_id": "opaque-grant-id",
  "project_state": "ready",
  "revocation_epoch": 0,
  "observed_at": "2026-09-26T00:00:00Z"
}
```

All identity, scope, and state fields are derived from server-side records,
not request parameters. `project_id` is the hosted brain identifier and is
opaque to clients. `revocation_epoch` is the value bound into the OAuth grant;
the response is refused if it differs from the current project epoch. This
read does not reserve the grant for a later operation: every subsequent MCP
request still authenticates and checks current authorization independently.

This endpoint proves account/project/scope/grant binding only. It does not
attest the running binary's source commit or claim that the hosted memory
protocol provides bounded per-call cost, authoritative command reconciliation,
freshness/index revision, lineage, immutable export, or restore. Those remain
separate release requirements and must not be inferred from this response.

The schema name is `serenity.oauth-binding/v1`; there is no client-supplied
nonce or bearer token in the body. `observed_at` is diagnostic snapshot time,
not a lease or freshness guarantee.
