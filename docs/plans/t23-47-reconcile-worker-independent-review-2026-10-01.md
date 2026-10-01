# T23.47 reconciliation worker independent review — 2026-10-01

## Verdict

The corrected worker locally closes the page-error cursor loss and retry-classification issues from the initial candidate. Its lifecycle and old-checkout handling are fail-closed in the reviewed paths. I found no local safety blocker in the reviewed scope. The fixed 1,024-account retry map intentionally bounds in-memory state by evicting the least recently touched account; under more than 1,024 simultaneous failures, an evicted account can be retried before its configured exponential delay. This is an operational pressure tradeoff rather than an entitlement or deletion safety defect: retries remain sequential and bounded by page/sweep pacing, and failures do not grant access. It is local source and focused-test evidence, not live-provider or hosted qualification.

## Reviewed source

- Corrected implementation: `7c3dcaed6cc7e3d9aa4edf237f0299043d99cdfa` (`fix(hosted): classify billing reconciliation retries`).
- Original candidate: `b21f2e039dba0381ba25d944bc7fa5027a8a6834`.
- Review checkout: `/Volumes/BuildOffload/worktrees/serenity-t23-47-reconcile-worker-final-review-20261001`, branch `review/t23-47-reconcile-worker-final-20261001`, at the corrected implementation SHA.

## Findings

The worker starts only when the concrete billing reconciler is configured. Its page query uses keyset ordering, selects only `active` and `restore_pending` accounts with a nonempty customer ID, and excludes `deleting` rows. It closes the query rows before any provider work. Each account call has a 20-second timeout. The worker does not activate accounts; it rereads lifecycle state after reconciliation and keeps restore-pending accounts on the free plan. If deletion or another freeze races with reconciliation, the post-read repairs the derived plan to free.

`Service.Close` cancels the worker and waits for its done channel before closing the gateway, pool, and store. `sync.Once` makes repeated close calls return the stored close result. Worker tests exercise cancellation of an in-flight provider request before close returns.

The corrected loop keeps the keyset cursor between page calls. `runBillingPage` advances it only after every row in the page has been classified; a page read or scan failure returns without moving the cursor. The loop retries a failed page with bounded exponential delay and retains that cursor, then resets it only after an empty or short final page completes the sweep. Completed sweeps wait the full 15 minutes. Per-account provider ambiguity uses a 15-minute exponential base capped at 6 hours; unavailable, timeout, and other errors use a 1-minute base capped at 1 hour. A change in error class resets that account's attempt exponent. Failure logs include a sanitized category and delay, not account IDs, customer IDs, or raw provider/database errors.

The bounded retry map is an exception to per-account delay guarantees during sustained failure pressure. At capacity, adding another failed account evicts the least recently touched entry. With more than 1,024 persistently failing accounts, a later sweep can encounter an evicted account without retry state and retry it earlier than its prior 6-hour ambiguity or 1-hour transient schedule; adding it back can evict another account. `TestBillingRetriesAreBoundedAndExponentiallyDelayed` verifies the memory bound, not persistence of every account's retry history. This follows the accepted bounded-map design, but operators should understand that those exponential maxima apply only while an account's retry entry is retained. Sequential account processing and the 1-minute inter-page / 15-minute inter-sweep pacing keep this from becoming unbounded concurrency or bypassing the worker's global cadence.

The existing checkout reconciler operates only on the account-scoped attempt and customer. It validates recovered legacy attempts against account/customer metadata, retains malformed timestamps, requires the GET session ID to match, uses a deterministic idempotency key for expiry, and requires the POST response to confirm the same ID and `expired` status. Ambiguous or mismatched provider state returns before deleting the attempt or writing the reconciliation audit record. Focused tests cover confirmed expiry and retention when the provider returns a different session ID. This protects the reviewed path from clearing a checkout attempt based on an uncertain response.

## Validation

Ran from the isolated review checkout with Go cache and temporary files on the external SSD:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-reconcile-worker-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-reconcile-worker-review-tmp go test -race -count=1 ./internal/hosted/service -run 'TestBilling(ReconcileWorker|Retries|Retry|PageError|WorkerFailure)'`

Result: PASS (`2.656s`). This covers the worker lifecycle/provider tests, retry bounds/class changes, cursor retention on page error, and sanitized failure logging. No live provider calls were made. The candidate author separately reported a full service-package race pass after the correction; I did not rerun that broader suite.

## Limits

Retry and cursor state remain in memory and process-local. This review does not establish cross-replica coordination, startup recovery of worker retry state, production Stripe behavior, backup/privacy acceptance, or complete T23.47 acceptance. The 1,024-entry eviction behavior is a documented tradeoff; a stricter aggregate provider request budget would require a separate global limiter or durable retry schedule.
