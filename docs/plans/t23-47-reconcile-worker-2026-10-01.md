# T23.47 billing reconciliation worker receipt — 2026-10-01

This candidate adds a service-owned billing reconciliation loop when and only
when the concrete billing service is configured. The loop pages server-owned
account IDs in keyset order, selecting only `active` and `restore_pending`
accounts with a provider customer reference. It closes each SQL page before
calling the provider, reconciles one customer at a time with a 20-second
context, and waits at least one minute between full pages. A completed sweep
waits 15 minutes before the next sweep starts. A page-query error retries from
the last completed page cursor using a separate transient backoff, so it cannot
restart at the first IDs and starve later accounts.

Ambiguous provider outcomes back off exponentially from 15 minutes to a
6-hour cap. Provider unavailability, timeouts, and other per-customer errors
back off from 1 minute to a 1-hour cap. A change in failure class resets that
account's exponent. Page/database read failures use the transient 1-minute to
1-hour backoff. The per-account retry map is bounded at 1,024 entries; under
pressure, its least recently touched entry can be evicted and lose its saved
backoff. Per-customer warnings include only a failure category; page-error records
also include the retry delay. Neither includes account/customer identifiers
or raw provider error text.

The worker never changes account lifecycle status or uses reconciliation
eligibility to activate an account. A restore-pending result remains frozen;
the existing frozen error is treated as expected only while the row is still
`restore_pending`. The worker checks status again after reconciliation and
repairs the derived plan projection to `free` when a concurrent freeze or
deletion made the provider result stale. `Service.Close` cancels and joins the
worker before closing the gateway, pool, and store, and repeated close calls
return the first stored close result.

The existing reconciliation path may inspect an old checkout attempt and
issue an idempotent expiry POST for a provider-confirmed open session. Worker
tests verify that a confirmed expiry clears the durable attempt and that a
mismatched session ID leaves it for retry. No new provider operation or
credential path was added. The worker is process-local; it does not coordinate
multiple service replicas or persist its schedule/backoff state. Production
billing remains disabled by default, so the loop is absent unless billing is
explicitly configured.

The implementation and tests are on
`hosted/t23-47-reconcile-worker-20261001`, based on `e387fe4` in the external
continuation clone. The source commits are `d87aee8`, `29ebb4e`, `763015b`,
`35bc921`, `4b37176`, `002d13b`, and retry-policy correction `7c3dcae`.

Validation used the external SSD Go cache and temporary directory:

- `go test ./internal/hosted/service -count=1` passed.
- `go test ./internal/hosted/service -run 'TestBilling' -count=1` passed after the retry-policy correction.
- `go test -race ./internal/hosted/service -count=1` passed on the full service package after the correction.
- `git diff --check` passed. No multi-package build, live provider call, deployment, or multi-replica qualification was performed.

## Coordinator integration qualification

Integrated Go source `5d73741378e294c2928a0b7c3fd1b6693bfa6111` passed full local race in79 tested packages, full vet and full lint0 at load3.04 under the shared lease, released immediately. Five packages have no tests; seven gated tests remain unexecuted. Independent receipt `2414d43` clears local safety blockers and records eviction pressure. An isolated runtime mutation omitting worker registration failed waiting for reconciliation; restored focused test passed. Later changes are documentation only. Receipt: `docs/launch/evidence/hosted-billing-reconcile-worker-2026-10-01.json`.
