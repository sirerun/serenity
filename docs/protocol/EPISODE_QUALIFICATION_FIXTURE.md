# Offline episode qualification fixture — blocked operator entry point

This packet implements an isolated fixture engine using the existing hosted
Store, Provisioner and credential Issuer. **It is not executable against a live
host.** The separate `evals/episode-qualification` command always exits 1 with a
fixed refusal, before opening any target, configuration or credential file.
It accepts no enabling flag or environment variable. It is not linked into the
hosted service, adds no endpoint, and does not change the local-only guards in
`evals/hosted-load/fixtureprep`.

T3.7 remains blocked on maintenance integration and independent verification.
Fixture preparation does not complete T3.3, enable capture, qualify deployment,
or demonstrate authenticated production behavior.

## Concrete integration blocker

The checked-in `deploy/hosted/deploy.sh`, service units and
`docs/launch/hosted-runbook.md` do not define the existing root maintenance hold
format or a closed-authority-session verifier. The supplied external design
requires both. Its preflight checks absence of `maintenance.hold.json`; absence
is not permission to write. A process-local `Gateway.Maintenance` lock and a
brain writer flock cannot establish this host-wide authority.

Inventing a replacement hold JSON format, trusting a caller boolean, accepting
an arbitrary proof digest, or treating stopped systemd units alone as authority
would create an unreviewed bypass. This packet deliberately provides none.
No mock-systemd success or isolated fixture test establishes live maintenance.

Before connecting `main` to the engine, the parent must supply and independently
review the actual existing maintenance/activation contract and adapter:

1. Verify the root-owned hold's canonical path, no-symlink ancestors, ownership,
   permissions, descriptor/inode identity, target binding, expiry and exclusive
   lock semantics. Verify the authoritative session is closed through its
   established verifier. Preserve the hold across the complete operation and
   interruption recovery; do not make it readable by the service user.
2. Verify known loaded hosted, broker, backup service and backup timer units are
   stopped, with no running/transitioning jobs or process. Prevent restart for
   the entire operation. Unknown state refuses. Recheck on child exit; do not
   start or stop services in this tool.
3. Bind the exact reviewed child binary SHA256 and source commit to the same
   configuration path/hash and data directory used by the hosted service. Read
   only the necessary non-secret configuration fields, including `account_cap`.
   Do not read credential files. The engine's supplied `target` values are test
   inputs, **not** independently verified live configuration evidence.
4. Inspect the existing control database read-only before `Store.Open`: exact
   supported schema 10, regular private service-owned DB/WAL/SHM and safe
   ancestors. Refuse absent, older, newer or inconsistent schemas. `Store.Open`
   runs schema initialization/migrations and must never be used as a live
   schema preflight. Verify the store's bound `control.db` path after opening.
5. If a wrapper is necessary, root opens the private hold/proof FD, checks the
   preceding evidence, and starts the exact child with bounded Python
   `subprocess` `user`, `group`, empty supplementary groups and `pass_fds`.
   The child must run as the existing service UID/GID, not root. No Go setuid,
   ownership repair, Keychain changes or broad config permissions. The inherited
   FD must carry verified existing custody, not self-authored authority.
6. Independently review/test the adapter, its races and fail-closed behavior.
   Production execution remains a separate parent-owned operation. This source
   packet contains no operational launch recipe or deploy permission.

## Implemented engine contract (isolated tests only)

The private manifest binds a cryptographically random fixture namespace to
exactly two reserved `episode-<namespace>-1/2@qualification.invalid` addresses,
account IDs, default brain IDs, credential IDs, source commit, configuration
path/hash, data directory and cap. No email verification assertion or mail is
created. Accounts use `CreateAccount`'s normal free plan/version; no entitlement,
quota, billing or provider rows are synthesized by the engine. Counting all
active accounts, including partner accounts, is conservatively stricter than
the public-signup cap. A nonpositive cap is refused rather than defaulted.

The manifest is the only plaintext key output: atomic same-directory 0600 file,
file fsync and directory fsync inside a current-UID-only directory validated by
`privatefs`. A stable private flock serializes fixture operations. Manifests
with foreign target bindings, unknown fields, wrong cardinality, unsafe paths,
symlinks, hardlinks or permissions refuse. New preparation refuses any existing
reserved identity when there is no valid manifest, and refuses unexplained
files in the private output directory. It never repairs foreign permissions or
adopts a database, account or directory by name alone.

Before changing either account, the engine validates both accounts' identities,
free plans, brain ownership/default state, credential bindings and absence of
browser sessions, WorkOS identity/login records, login tokens, OAuth authority,
partner links/entitlements or billing adoption. Queries inspect only fixture
identities, schema metadata and minimal aggregate counts. It does not dump
customer rows, DB contents, configuration or credential stores. Ready brain
trees are checked for foreign ownership, links and shared writable state before
reuse. The normal Provisioner and writer ownership lock initialize brains.
No customer-state SQL writes are introduced.

### Durable intent and compensation

The manifest is synced before each mutation:

- Account creation persists the reserved address and `account` intent before
  `Store.CreateAccount`. That API generates the ID internally. Recovery can
  bind the same account ID only with that prior owned intent and pristine
  fixture identity checks. This relies on exclusive offline custody across the
  interruption; a private manifest alone cannot prove the origin of a row
  created by another actor. That is one reason live entry remains disabled.
- A brain ID and `brain` intent are persisted before `Store.InsertBrain`
  creates its allocating row. `Provisioner.Provision` finishes that exact
  default brain. Recovery retries the same ID; it never calls fleet-wide
  `Provisioner.Recover`, which would touch unrelated accounts.
- `key` intent and the previously owned credential IDs are persisted before
  `Issuer.Issue`. That API generates credential IDs internally. Under exclusive
  offline custody, exactly one additional credential with the expected account,
  brain, generation, scopes and no partner binding is the interrupted orphan.
  Recovery records and revokes it with `Issuer.RevokeKey` before issuing a new
  key. Zero is a pre-issuance interruption; multiple or differently bound keys
  refuse. Foreign credential IDs in a manifest refuse before revocation.
- Reissue and revoke persist cleared plaintext keys and intent **before**
  revoking. An interruption after DB revocation can therefore replay without
  needing an already-revoked plaintext credential to authenticate. There is no
  broad brain revoke or OAuth epoch mutation.

`prepare` replay preserves existing valid keys. `reissue` revokes only the
manifest's owned keys and issues fresh normal `memory:read,memory:write` keys.
`revoke` is idempotent and clears the private manifest's keys while retaining
synthetic account rows, brain data and revoked credential records. Partial
preparation remains resumable. These retained accounts have no fixture-issued
live credentials after successful cleanup; accounts are not deleted or claimed
physically purged. Crash-left private manifest temporary files may retain old
revoked key bytes; this packet never claims secure erasure or removes unknown
files. Missing/corrupt custody evidence requires explicit reconciliation.

The engine does not change capture settings. Parent-owned qualification must
keep real capture disabled, temporarily scope any separately authorized archive
qualification to the two exact synthetic brains, then disable it and revoke
fixture credentials. Retained data, signed release/fallback qualification,
journal exclusion, actual HTTP behavior and operational cleanup all need their
own receipts.

## Verification requested

Use an existing authorized isolated Linux/DGX worker with Git and Go 1.26.9,
private temporary storage and bounded concurrency. Only source and test logs
may be transferred; no host fixture manifests, keys, DBs or brain data.

```sh
GOMAXPROCS=2 go test -p 2 -count=1 -timeout 180s ./evals/episode-qualification
GOMAXPROCS=2 go test -race -p 2 -count=1 -timeout 180s ./evals/episode-qualification
GOMAXPROCS=2 go vet -p 2 ./evals/episode-qualification
```

Tests create fresh temporary synthetic control databases through `store.Open`
and real Provisioner/Issuer interfaces. They cover two-account preparation,
replay, interrupted account/brain/key provisioning, revocation interruption,
reissue, foreign/colliding/adopted identity refusal, caps, path privacy and
symlink refusal, unsupported schema, unrelated fixture preservation and the
constant content-free CLI refusal. SQL mutation in tests only injects foreign
or unsupported synthetic states. These are source tests, not live authority or
production qualification. Parent owns execution, independent review, merge and
landed verification; no Mac builds/tests/vet were run for this packet.
