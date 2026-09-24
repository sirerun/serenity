# T23.47 shared-file integration request

Status: proposed; no schema or assembly authority transferred. Owner:
Integrator41/57 (`R-hosted-schema`, `R-hosted-assembly`). Billing implementation
remains in draft PR274. This is the file-change handshake required by
`docs/launch/hosted-completion/interfaces.md`.

## Checkout recovery request

Apply the SQL payload in `checkout-request-schema-proposal.sql` as a new migration,
without rewriting existing migrations or assigning its number in this lane.
Review `checkout-recovery-design.md` for parameter identity, legacy handling,
conditional session-ID persistence, pagination and failure semantics. Return the
shared commit SHA before billing starts depending on these columns.

Shared files requiring an integrator patch:

- `internal/hosted/store/migrations.go`: append the reviewed migration and its
  version record; preserve applied versions.
- `internal/hosted/store/store.go`: advance both future-version checks and invoke
  the migration transactionally for older databases. Do not merely add columns
  while leaving the version guard unchanged.
- Store migration tests: upgrade a populated current-version database containing
  legacy attempts, reopen without duplicate DDL, reject a future version, preserve
  unrelated account/subscription/OAuth data, and exercise immutable request guards.
- Frozen schema/interface receipt: record exact shared commit and reopen affected
  billing, backup, restore, and release compatibility evidence.

An existing version1 attempt must replay its persisted form body exactly. Legacy
missing-ID attempts remain pending; do not synthesize missing historical request
parameters from present configuration. Invalid or unknown request versions fail
closed. The SQL proposal does not itself implement provider recovery or decide
whether an attempt can safely be replaced.

## Compatibility boundary

Current deployed v0.1.10 and live store code reject schema versions greater than5.
Once a numbered migration advances that version, rolling back to that binary
cannot be assumed to work. Deployment must retain a pre-migration backup and
qualify the actual version pair, or use isolated restore with deletion/provider
reconciliation. Updating the schema declaration invalidates previous claims of
unchanged schema between candidates. Backup/restore schema validation must agree
with the newly accepted version before this is merged into a deployable build.

## Remaining architecture decision

The grace-order regression remains FAIL. Request a frozen invoice-bound payment
failure identity and timestamp rule, including durable first-failure evidence,
repeated failures for the same invoice, transitions to a different invoice,
out-of-order delivery, and unavailable provider history. Neither subscription
period start nor an arbitrary subscription event timestamp proves when the
current invoice first failed. No migration for this unresolved policy is proposed
as accepted, and a passing Checkout migration must not close the grace gate.

## Assembly handoff

After the shared commit, the billing lane implements and qualifies recovery through
both reconciliation and closure, including provider-success/local-save-failure
and fresh-Service retry. Integrator wires the real reconciler and closer into
service/deletion and runs assembled recovery tests. Package tests and the local
SQL checks are partial evidence only; PR274 stays draft until the stated gates
are satisfied.
