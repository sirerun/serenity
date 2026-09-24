# Coordinator review: not accepted

The period-start/webhook-event-created grace calculation is not accepted. A provider-owned timestamp is insufficient unless it identifies the current payment failure. Both reconcile and webhook paths must resolve the same failure identity, preserve its first failure time, and avoid reopening grace for retries or delayed unrelated events. Current passing timing tests encode a narrower assumption.

Proposed implementation: bind grace to the outstanding subscription invoice and its first failed payment event, retain that evidence durably, and reconcile missed events through provider history with explicit handling for missing history. Stripe lists events for only 30 days and renders each event according to its original API version, so an unbounded-history assumption or current-version-only decoder is unsafe. This proposal is not yet implemented or a frozen interface change.

Sources reviewed:
- https://docs.stripe.com/api/events/list
- https://docs.stripe.com/api/events
- https://docs.stripe.com/changelog/acacia/2024-10-28/customer-portal-schedule-downgrades

Coordinator added race-tested lifecycle projection and webhook-outage/retry fixtures. Scheduled cancellation is not a scheduled price downgrade. Portal configuration, proration, Checkout-based re-subscription remain unqualified. An actual local transaction failure after provider cancellation is now injected and verified: pending response, unchanged local subscription, then fresh-Service retry completes without double cancellation. No provider calls, charges or deployment occurred.

## Executed delivery-order regression

On source ba9053808b64c0ab96fe08f575b0a426e8f2e794, the adjacent `grace_order_regression_test.go.txt` was temporarily installed as `internal/hosted/billing/grace_order_regression_test.go` and run with `go test -count=1 ./internal/hosted/billing -run TestGraceDeadlineIndependentOfDeliveryOrder`.

It FAILS (exit 1): the same past-due subscription snapshot, invoice ID and subscription event produce `2026-09-14T00:00:00.000000000Z` when webhook delivery is first, versus `2026-09-04T00:00:00.000000000Z` when reconciliation is first. Both paths are executed in each case against fresh local databases. The fixture runs an HTTP test server only; no external provider is contacted. It asserts convergence, without selecting either incorrect timestamp as the desired deadline.

The source is retained as an explicit failing qualification reproducer outside the default test set until the frozen failure-evidence decision is resolved. Existing 18 passing package tests do not include or override this FAIL. The future correction must move this test into the normal billing suite and provide any additional invoice/failure-history fixtures its implementation needs. No production behavior was changed by this verification.

## Pending checkout preserved across reconciliation

Source 874dd90 fixes a separately reproduced duplicate-checkout path. `TestCheckoutSurvivesRestartAndPreventsSecondPlan` now runs ReconcileCustomer after initial checkout and before recreating Service. Before correction it failed with "second plan checkout allowed": an empty subscription list deleted the still-open local checkout attempt. Reconciliation now retains that attempt; existing explicit old-attempt resolution is preserved. After correction, a different plan is rejected and same-plan retry reuses the original session with one create request. The billing package race suite passes (18 default tests); scoped lint reports zero issues. The separate delivery-order qualification remains FAIL and is not included in those18 tests.
