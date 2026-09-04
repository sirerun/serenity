# HTTP transport: loopback, LAN, and the daemon token

Serenity's protocol servers (MCP, DISPOSITION, DIRECTION — plan epic E4)
share one HTTP transport, `internal/server`. This page covers what an
operator configures: where the daemon binds, how it authenticates, and how
to rotate the token. RFC 0001 §14 ("Security and privacy") is the design
contract this implements.

## Default: loopback only, always authenticated

With no `server:` section in `serenity.yml`, the daemon binds
`127.0.0.1` on an OS-assigned port. Every route — including the health
check — requires a bearer token, because loopback is authenticated too:
any local process is not automatically trusted.

```
Authorization: Bearer <token>
```

A missing or incorrect token gets `401` on every route. The comparison is
constant-time, so a wrong token can't be brute-forced faster by timing the
response.

## The daemon token

The token is minted once by `serenity init` and stored only in the OS
keychain (service `serenity`) — never in `serenity.yml`, never in
`.serenity/`, never in any tracked or untracked file. `serenity doctor`
reports whether it's present.

Rotate it at any time:

```
serenity connect --rotate-token
```

The previous token stops authenticating immediately — the server reads
the current token from the keychain on every request rather than caching
it, so no restart is required. Rotation has no expiry model in v1: this
is an accepted single-principal, loopback trade-off (ADR 010). If a token
leaks, rotate it.

## Exposing beyond loopback (LAN / Tailscale)

Binding to anything other than `127.0.0.1` or `::1` is refused by default
with a named error — not a generic bind failure — unless you opt in
explicitly:

```yaml
# serenity.yml
server:
  bind: "0.0.0.0:8443"     # or a Tailscale interface address
  allow_lan: true           # required for a non-loopback bind to succeed
```

Without `allow_lan: true`, the daemon refuses to start rather than
silently exposing itself.

### Optional mTLS

When exposing beyond loopback, you can additionally require client
certificates:

```yaml
server:
  bind: "0.0.0.0:8443"
  allow_lan: true
  client_ca_file: /path/to/client-ca.pem
  server_cert_file: /path/to/server-cert.pem
  server_key_file: /path/to/server-key.pem
```

With all three files set, the daemon only accepts connections presenting
a client certificate signed by `client_ca_file`, on top of the bearer
token check above — both apply together, neither replaces the other.
`client_ca_file` alone (without a server cert/key pair) is a
configuration error, not a silently-skipped feature.

## Verifying

```
curl -i http://127.0.0.1:<port>/healthz                              # 401, no token
curl -i http://127.0.0.1:<port>/healthz -H "Authorization: Bearer wrong"  # 401
curl -i http://127.0.0.1:<port>/healthz -H "Authorization: Bearer $(serenity connect | ...)"  # 200
```

(`serenity connect` prints the daemon token's presence, not its value —
the value never leaves the keychain except into the `Authorization`
header a client sends.)
