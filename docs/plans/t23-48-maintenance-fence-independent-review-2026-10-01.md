# Independent review: T23.48 maintenance fence

Reviewed exact source `62550e9a1c3f5f89338bf7785ebd43352d848477` on PR #333 base `6eaa43bbca06649f4a0a2e5a6c30e7de868a1a91`. This review covers only the maintenance/account-lock lifecycle change. It does not qualify or claim the deletion journal, journal durability, production journal credentials, activation, or T23.48 acceptance.

## Result

I found no blocking defect in the reviewed scope. Account deletion now holds `Gateway.Maintenance.RLock` and the target account's existing hash-stripe mutex from the trusted service preflight through billing closure and all account/brain cleanup. `deleteAccountUnderMaintenance` and `deleteBrainUnderAccountLock` are private helpers that assume those locks are already held, so the account path does not recursively take the read lock or account mutex while a backup writer may be queued.

The service preflight commits `status='deleting'` before calling the billing closer. It returns `proceed=false, nil` only for an already-deleted account; provider errors, non-closed results, invalid states, and missing required closure return errors and do not continue to purge. Recovery uses the same top-level read fence and per-account lock, releases the SQL row cursor before provider work, and skips only an account the trusted preflight has confirmed already deleted. The compatibility `Gateway.DeleteAccount` and `RecoverDeletions` callbacks fail closed when persisted provider-customer/subscription state requires the service deletion path.

The lock order is consistently maintenance read fence then account stripe. Tool calls and export use the same order; backup remains the exclusive maintenance writer and flushes the pool while holding it. Account deletion's nested brain work does not reacquire either lock. The regression test holds provider closure open, queues backup, verifies that the brain remains until closure completes, and then proves the queued backup completes after deletion without deadlocking. `Gateway.Inventory` remains outside the maintenance fence as scoped; it keeps its existing account-stripe read serialization and was not changed here.

Cross-account source review found no global account mutex: the gateway uses 64 hash stripes, so different stripes can proceed concurrently while hash collisions can serialize unrelated accounts. Brain lookup and deletion queries remain scoped by both account ID and brain ID; account cleanup queries are scoped to the target account. I did not run a separate concurrent two-account throughput test.

## Independent runtime evidence

At a fresh `uptime` sample with one-minute load 8.40, the following one-package race run passed in 2.167 seconds:

```text
go test -race -count=1 -run 'Test(DeleteAccountFreezesBeforeClosureAndRetainsUntilClosed|AccountDeletionMaintenanceFenceBlocksBackupUntilPurgeCompletes|DeletionWithoutConfiguredCloserRetainsExistingCustomer|DashboardPendingDeletionFreezesAndRetriesRetainingMemory)$' ./internal/hosted/service
```

The test exercised durable freeze-before-closure, pending/error retention, absent-closer fail-closed behavior, dashboard retry, and the queued-backup/nested-brain-helper path. I ran no multi-package suite or full repository gate. The candidate's implementation remains maintenance-only: it does not add a journal append/read, journal durability, or journal-backed recovery claim. Production provider behavior and cross-process locking were not exercised.
