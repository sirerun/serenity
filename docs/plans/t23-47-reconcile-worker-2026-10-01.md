# T23.47 billing reconciliation worker receipt — 2026-10-01

This candidate adds a service-owned billing reconciliation loop when and only
when the concrete billing service is configured. The loop pages server-owned
account IDs in keyset order, selecting only `active` and `restore_pending`
accounts with a provider customer reference. It closes each SQL page before
calling the provider, reconciles one customer at a time with a 20-second
context, waits at least one minute between full pages, and starts no more than
one full sweep every 15 minutes. Per-account failures receive bounded
exponential backoff held in a 1,024-entry in-memory map, capped at 15 minutes.

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
`35bc921`, `4b37176`, and `002d13b`.

Validation used the external SSD Go cache and temporary directory:

- `go test ./internal/hosted/service -count=1` passed.
- `go test ./internal/hosted/service -run 'TestBilling(ReconcileWorker|Retries)' -count=1` passed after the final worker tests.
- `go test -race ./internal/hosted/service -run 'TestBilling(ReconcileWorker|Retries)' -count=1` passed.
- `git diff --check` passed. No multi-package build, live provider call, deployment, or multi-replica qualification was performed.
