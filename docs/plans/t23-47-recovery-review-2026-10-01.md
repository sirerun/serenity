# T23.47 billing lifecycle recovery review — 2026-10-01

## Verdict

The grace-order defect recorded against `ba905380` is corrected in the current source (`42f6dfd`). The latest regression, `TestGraceDeadlineIndependentOfWebhookAndReconciliationOrder`, runs both local fixture orderings through the shared invoice-failure evidence path. That is local closure of the previously rejected clock rule, not T23.47 acceptance: current code still discards three SQL read errors explicitly called out by task acceptance, production assembly does not call `ReconcileCustomer` or `CloseBillingAccount`, and the committed task receipt remains PARTIAL with older source/evidence. Live Stripe, portal behavior, and full hosted lifecycle qualification remain unproven.

No tests or provider calls were run in this read-only audit. Existing test names below were inspected in source, not rerun here.

## Grace clock: historical failure and current behavior

The recorded failure is concrete: `docs/launch/evidence/T23.47/coordinator-review.md` documents the `ba905380` reproducer with identical provider state yielding September 14 versus September 4 depending on webhook/reconcile order. Its historical test artifact is `grace_order_regression_test.go.txt`; the committed `result.json` still records that older finding and has `source_sha` `9c6b88a`, status `PARTIAL`, with grace acceptance `FAIL`.

At `42f6dfd`, `internal/hosted/billing/billing.go:843-962` has since added `failureEvidence`. For a past-due subscription it validates the exact latest invoice and its customer/subscription identity, reuses durable `billing_failures` evidence keyed by account, subscription, and invoice, or scans provider `invoice.payment_failed` events and persists the earliest event timestamp (stable event-ID tie-break). Both `ReconcileCustomer` and webhook handling call this function before storing grace. `TestGraceDeadlineIndependentOfWebhookAndReconciliationOrder` (`billing_test.go:539`) now checks both arrival orders; `TestWebhookLateFailureAfterPaymentDoesNotReopenGrace` (`:656`) covers a stale failure after recovery. These are local HTTP fixtures, not live Stripe evidence.

Bounds remain material: an invoice older than the provider's 30-day event retention with no locally persisted failure evidence returns `ErrBillingProviderAmbiguous` rather than inventing a deadline (`billing.go:875-882`). Provider pagination and payload compatibility, real provider clocks/events, actual portal proration/settings, and the complete renewal/deletion lifecycle have not been qualified. The receipt must be refreshed against the new source before treating the acceptance criterion as closed.

## SQL read failures: unclosed acceptance and testable fixes

`docs/launch/hosted-completion/tasks.json` T23.47 explicitly requires the three ignored `Scan` errors to be propagated and a regression named FUN-05. Current source still has all three discard sites:

- `billing.go:330`, reconciliation's prior subscription/window read. A missing row is expected when first observing a provider subscription, but any other read error is collapsed into the same all-empty prior state. If a current-period renewal is being applied, that can skip `recordWindowClosed` and overwrite the live row without the closure audit. It also feeds empty old grace/identity to `graceDeadline`, bypassing existing-state safeguards.
- `billing.go:388`, post-transaction grace projection read. Reconciliation can return `Eligible=true` and `nil` error while `GraceUntil` remains zero after the DB read fails. That contradicts the `ReconcileResult` comment that zero means no active grace (`internal/hosted/contracts/billing.go:41-46`) and makes a successful result incomplete for callers.
- `billing.go:1047`, webhook's prior subscription/window read. It can likewise treat unreadable existing state as a new row, skip the closure audit on a period change, and calculate/write from empty prior grace metadata.

For the first and third sites, accept only `errors.Is(err, sql.ErrNoRows)` as an absent prior row; return every other error from the transaction before mutation. For the projection read, return a typed/wrapped read error instead of returning a successful result with an invented zero value. A focused SQLite failure-injection test should force each read to fail while subsequent transaction work would otherwise proceed, assert error propagation, and check there is no overwritten grace/window or `subscription_window_closed` side effect. This audit did not reproduce those injected DB failures; they are specific acceptance gaps with a clear regression shape, not a claim that a particular production outage has occurred.

## Assembly and remaining acceptance

The task/interface packets define production reconciliation and resumable closure as core lifecycle seams (`docs/launch/hosted-completion/interfaces.md:55-56`). `rg` over non-test hosted service, CLI, and dashboard code found no call to `ReconcileCustomer` or `CloseBillingAccount`; `internal/hosted/service/service.go:286` only injects `billing.Service` into the dashboard. Account deletion calls the legacy `CancelAccount` adapter at `internal/hosted/dashboard/dashboard.go:469`, then calls `Gateway.DeleteAccount`. Thus package-level tests of those methods do not establish restore/reconcile or durable deletion assembly. `T23.47` acceptance also still requires checkout repair, the expanded lifecycle fixture matrix, and billing portal/proration policy evidence; `result.json` says checkout automatic resolution and portal behavior remain unqualified.

## Holds and source authority

The local project channel records the chief-facing proposal rejecting period-start and arbitrary webhook-created timestamps, and the committed coordinator review contains the explicit order reproducer. The merged PR 274 has no submitted reviews or comments in the current GitHub record. Ajent feed access was unavailable in this session, so this review cannot certify whether an external Ajent hold was newly posted or lifted. Do not treat this report as clearance for deployment or hosted qualification.

## Evidence index

- Baseline: `42f6dfd8615b193fffb797175daf4b5f5d02dc94` (`origin/main` at audit start).
- Task acceptance: `docs/launch/hosted-completion/tasks.json`, T23.47.
- Current billing source/tests: `internal/hosted/billing/billing.go`; `internal/hosted/billing/billing_test.go`.
- Historical receipt and event-order reproducer: `docs/launch/evidence/T23.47/result.json`; `coordinator-review.md`; `grace_order_regression_test.go.txt`.
- Frozen callable seams: `docs/launch/hosted-completion/interfaces.md`; `internal/hosted/contracts/billing.go`.
