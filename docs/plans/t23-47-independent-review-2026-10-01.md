# T23.47 independent review — 2026-10-01

## Outcome

The billing read-error change is correctly integrated in `ffb7971`: its `billing.go` and `persisted_read_errors_test.go` match the corresponding changes from `c4c3d23`. The evidence follow-up from `0a8b9e1` is present in descendant `13ae1a4` and in the current hosted recovery branch; it records four focused passing cases and explicitly limits the injected fault to an application-level seam. Independent tests passed, and a detached-worktree mutation that discarded non-`sql.ErrNoRows` errors failed at runtime in all three targeted regression tests.

This does not complete T23.47. The task registry remains `planned`, the receipt remains `PARTIAL`, global billing serialization and production lifecycle wiring remain open, and the receipt's grace-order acceptance remains `FAIL`.

## Read-error handling and transaction behavior

`internal/hosted/billing/billing.go:57-71` centralizes the prior subscription read. Only `errors.Is(err, sql.ErrNoRows)` produces empty prior state; all other errors become `persistedBillingReadError` with the original error unwrapped. It also returns an empty state rather than partially scanned values when `Scan` fails.

`ReconcileCustomer` checks this result inside the same `Store.Transaction` that updates subscriptions, account plan, and reconciliation/window-closure audit rows (`billing.go:341-408`). The two transaction paths in `ReconcileCustomer` and `Webhook` both return immediately on read failure (`billing.go:363-365`, `1085-1088`). The webhook marks `processed_at` only after the subscription/account updates (`billing.go:1090-1112`). This preserves a receipt for retry while rolling back subscription state, entitlement changes, and processing status.

The focused tests exercise these lifecycle effects in `internal/hosted/billing/persisted_read_errors_test.go:64-150`: reconcile leaves prior account/subscription state and audit counts unchanged; webhook leaves the prior subscription window intact, keeps its receipt unprocessed, and writes no closure audit. `TestReadPriorSubscriptionAllowsOnlyMissingRow` separately checks that an actual missing SQLite row is allowed while an injected non-missing error is wrapped and propagated. `ReconcileCustomer` also now returns the grace value computed inside its transaction (`billing.go:339-340, 382-385, 425-430`) rather than issuing a fallible post-commit read.

The failure injection is a private per-Service reader override called with the real `*sql.Tx`; tests return an ordinary Go error, then assert both `errors.As` to the typed wrapper and `errors.Is` to the injected error while checking durable database state. This demonstrates runtime error propagation and transaction rollback. It does not simulate an SQLite driver, disk, or engine I/O failure; the evidence follow-up correctly says so. The reconcile/webhook fixture has no stale checkout attempt, so it does not exercise independent old-checkout cleanup that can happen before the subscription transaction.

The grace-return test verifies the computed, expired deadline is returned exactly. By inspection, retaining the value from inside the transaction removes the post-commit read race. The fixture does not force a concurrent grace update between commit and response, so it is not itself a deterministic race reproducer. The existing evidence continues to mark the broader delayed/replayed invoice-grace ordering acceptance `FAIL`; this read fix does not resolve that timing requirement.

## Independent verification

On exact code revision `ffb7971`, `go test ./internal/hosted/billing -count=1` passed. After restoring the source, `go test -race ./internal/hosted/billing -count=1` also passed. On the integrated period-key change, `go test ./internal/hosted/operation -run TestRetryKeyIsScopedToQuotaPeriod -count=1` passed; the two schema-upgrade/equivalence tests in `internal/hosted/store` passed.

For the required negative control, I created a detached worktree at `ffb7971`, changed only `readPriorSubscription` to return `(priorSubscription{}, nil)` for every non-`sql.ErrNoRows` error, and ran the three focused tests. The package compiled; the tests then failed at runtime because reconcile/webhook returned nil and the helper no longer distinguished error from absence. I restored the one changed file and removed only that clean, temporary worktree. No billing implementation file was changed in this review branch.

## Remaining task gaps

- `billing.Service.mu` is still one global mutex (`billing.go:38`). `ReconcileCustomer`, `Checkout`, `Webhook`, and closure hold it over provider I/O (`billing.go:280-281, 469-470, 1002-1004, 1213-1215`). A slow webhook for one account can still serialize another account's checkout; the T23.47 registry step explicitly requires per-account locking and checkout rate limiting.
- Production assembly still has no caller of `ReconcileCustomer` or `CloseBillingAccount`; dashboard closure still invokes the legacy `CancelAccount` adapter (`internal/hosted/dashboard/dashboard.go:469`). The integrated evidence follow-up records this as an open lifecycle gap. T23.47 therefore remains `planned` in `tasks.json` and `PARTIAL` in `docs/launch/evidence/T23.47/result.json`.
- The ledger period-scope fix (`58dd112`, migration 9) correctly scopes `(account, brain, client_key)` lookup/uniqueness by quota period, and its focused ledger test passes. The canonical memory writer still stores the raw client `OperationKey` across periods: Gateway forwards `input.OperationKey` (`internal/hosted/gateway/gateway.go:376-397`), the remember handler passes it through (`internal/server/memory/remember.go:126-136`), and `writer.MemoryFact` scans canonical history for that key without a quota-period component (`internal/writer/memoryfact.go:114-128`). Thus a client reusing the same key in a later quota period can reserve a distinct ledger row but still conflict with or resolve to an older canonical writer operation. This is a future Gateway/writer integration issue; it was not changed here.

## Scope

This is an independent, read-only review of the billing implementation at `ffb7971`, the `0a8b9e1` evidence follow-up as integrated by `13ae1a4`, and the current hosted recovery branch. The only retained change from this lane is this report. No production source, task registry, or evidence receipt was edited.
