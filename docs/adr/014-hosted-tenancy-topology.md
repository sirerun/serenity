# ADR 014: Hosted tenancy topology, binding chain and runtime pool

## Status
Accepted (topology and binding chain: David, 2026-09-11; capacity numbers are provisional until T23.3 measures them)

## Date
2026-09-11

## Context

Serenity ships as a single-user, local-first Go binary. `serenity serve --http`
binds loopback, authenticates every route with one daemon token from the OS
keychain, opens exactly one brain (`-C <root>`), and registers all five
MEMORY_VERBS tools plus the DISPOSITION and DIRECTION routes. There is no
account, tenant, quota or billing model anywhere in the repository, and the
only deployed infrastructure is the public docs-chat Lambda in `deploy/chat`.

The hosted launch (docs/launch/hosted-plan.md, epic E23) needs a person to sign
up, receive a private brain, connect the Rakazo adapter over HTTPS and buy a
plan. Rakazo (elie222/rakazo PR 835, merge `ef63e354`) speaks Streamable HTTP
MCP with the official SDK client, sends `Authorization: Bearer <token>` and
`Accept: application/json, text/event-stream`, forbids redirects, requires
`recall` on `tools/list` (plus `remember` and `forget` when writes are enabled),
and passes bot and space identity as an `entity` argument
(`rakazo-bot/<label>/<botId>`, `rakazo-space/<label>/<spaceId>`). A brain label
is therefore a namespace inside one brain, never a tenant boundary.

Candidate topologies considered:

1. One container or VM per customer. Rejected: idle cost per free account
   makes a no-card free plan uneconomic, and a 100-account free cap would
   need 100 always-on runtimes.
2. A distributed control plane (ECS Fargate services, EFS, ALB, a separate
   auth service). Rejected for launch: EFS latency is poor for git and SQLite,
   the idle cost is roughly twice the authorized ceiling, and nothing in the
   product needs more than one node yet.
3. One always-on process on one VM serving every tenant from per-account
   brain directories behind a bounded pool of open brains. Chosen.

David authorized (2026-09-11) AWS us-west-2 in the account that already runs
the docs-chat Lambda, one ARM VM with an EBS volume and S3 backups, at a fixed
hosting ceiling of USD 60 per month.

## Decision

### Topology

- One Go process, `serenity hosted serve`, on one Graviton VM (initial size
  t4g.small, Amazon Linux 2023 arm64) with an encrypted gp3 EBS volume mounted
  at `/var/lib/serenity`. Infrastructure is a CloudFormation template under
  `deploy/hosted/`, deployed the same way as `deploy/chat` (change sets from a
  reviewed template). No SSH keys; operator actions go through SSM Run Command.
- Caddy terminates TLS for `app.serenity.sire.run` (Let's Encrypt) and reverse
  proxies to the process on loopback. The public site stays on GitHub Pages at
  `serenity.sire.run`; the app origin is separate because Pages cannot proxy.
  Caddy serves `/mcp` at its final URL with no redirects, because the Rakazo
  client rejects redirects.
- Per-account state lives under `/var/lib/serenity/brains/<brain_id>/` as an
  ordinary Serenity brain repo (git, `serenity.yml`, `.serenity/` index), opened
  through the existing internal packages. The control database is one SQLite
  file, `/var/lib/serenity/control.db`, owned by `internal/hosted/store`.
- Backups: hourly, encrypted at rest (S3 SSE-KMS) and in transit, consisting of
  a `VACUUM INTO` snapshot of the control DB and one git bundle per brain.
  Retention is 30 days and is disclosed to customers as the deletion window.

### Binding chain

Every hosted operation resolves `authenticated account -> owned brain ->
scoped client credential -> isolated runtime and storage` server-side:

- A browser session identifies an account (ADR 015). Every dashboard route,
  export, deletion, billing session and usage view authorizes against the
  account in the session, never against an id in the URL.
- A brain row belongs to exactly one account. The on-disk path is derived from
  the brain id by the store; no request supplies a path, entity tag, label,
  session id or filesystem location that selects storage.
- A client credential (`sk_live_<prefix>_<secret>`) belongs to exactly one
  (account, brain) pair, carries a scope set (`memory:read`, `memory:write`),
  and a generation number. Only a one-way verifier is stored. Rotation creates
  a new generation and invalidates the old one on the next request, without
  ever pointing at a different brain.
- MCP sessions (`Mcp-Session-Id`) are namespaced per brain and record the
  credential generation that created them. A request whose credential is
  revoked, rotated or out of scope is rejected even on an established session.
- The public gateway exposes only `recall`, `remember`, `forget` and
  `read_memory_fact`. `entity`, `synthesize`, the DISPOSITION routes and the
  DIRECTION routes are not registered in hosted mode. `tools/list` reports
  exactly what works.
- The single-user daemon token and the OS keychain are not used in hosted
  mode. Hosted secrets (Resend, Stripe, embeddings, session signing key) are
  read from files under a root-only directory populated from AWS Secrets
  Manager at boot; nothing is passed on the command line.

### Runtime pool

- A bounded LRU pool holds open brains (writer queue, index handle). Idle
  brains close after a timeout; the pool caps concurrently open brains and
  in-flight tool calls per brain. Admission beyond the cap returns a typed
  `capacity` error, never a silent drop.
- One canonical writer per brain is enforced by the process-owner lock from
  PR 221 plus the pool holding at most one handle per brain.
- Provisioning is a state machine (`allocating -> ready`) persisted in the
  control DB before the directory exists. A crash between allocation and
  readiness is resumed, never duplicated. Deleted brains are purged, never
  reused as a template.
- Readiness (`/readyz`) checks the control DB migration level, the presence of
  required secrets, a real bounded embeddings call, and that the brains
  directory is writable. Liveness (`/healthz`) is separate and unauthenticated.

### Provisional capacity (to be replaced by T23.3 measurements)

- Free-account cap 100, total-account cap 300, both configuration values.
- Expected fixed cost: instance about USD 12, EBS 30 GB about USD 3, Elastic IP
  about USD 4, S3 and transfer under USD 5. Total under USD 25 per month,
  within the USD 60 ceiling.

## Consequences

Positive: one binary, one VM, one database and one backup job; idle cost is
independent of account count; the existing brain format, writer queue, index
and MCP handler are reused unchanged; isolation is structural (path derived
from the brain row, credentials bound to a brain).

Negative: one node is a single point of failure, so launch recovery objectives
come from measured restore time, not an SLA; vertical scaling is the only
lever until a later epic adds a second node; a bug in the control DB write
path affects every tenant. A `PLAN` task re-grooms the later waves once the
slice has deployed and the spike numbers exist.

Revisit if: T23.3 shows more than 30 MB RSS per open brain at 100 brains, if
the free cap fills, or if full-limit load on t4g.small exceeds 70 percent CPU.
