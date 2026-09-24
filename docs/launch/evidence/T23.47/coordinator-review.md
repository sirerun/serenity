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

Required combined verification after checkout fix: `go test -race -count=1 -json ./internal/hosted/billing ./internal/hosted/meter` passed18 billing and2 meter top-level tests, zero failures/skips, on source874dd90 (docs-only HEAD543f39d). Shared build lease was acquired and released. The separately retained grace-order reproducer remains FAIL.

## Checkout expiration response validation

TestOldCheckoutRequiresConfirmedExpiration reproduced cleanup success for three unconfirmed responses: still-open status, completed status, and a different session ID. The corrected code requires the requested session identity on retrieval and matching expired identity/status after expiration, clearing the response fields before decoding to prevent reuse of omitted values. All19 default billing tests pass under race detection; final scoped lint zero issues after test-output handling cleanup. Successful expiration removes one attempt and adds one repair audit; rejected responses retain the attempt without a successful repair audit. Local HTTP fixtures only. Grace qualification remains FAIL; no launch acceptance is claimed.

## Deletion closure expiration proof

TestClosureRequiresConfirmedExpiration reproduced a closed result for open/completed/wrong-session expiration replies. Closure now requires matching session ID and expired status after the expiration request, or returns pending while retaining the attempt. Shared fixture helper verifies both reconciliation and deletion paths. The default billing suite now has20 passing top-level tests under race detection; scoped lint zero issues. These20 billing tests differ from the earlier18 billing plus2 metering run. Grace-order qualification remains FAIL. No live provider calls or deployment.

## Independent correctness fallback at37996b8

Read-only review found no new regression in49590a8..37996b8, but identified an inherited P1 launch blocker: Checkout persists its attempt before provider creation and writes session_id afterward. Provider success followed by response loss or local save failure leaves an empty session_id. CloseBillingAccount skips that attempt, can see empty subscription lists, deletes the attempt and certifies closure without resolving the open checkout. The old-attempt reconciler likewise drops empty IDs after23h without provider evidence. This is code-path review, not an executed interruption fixture. Known-ID expiration tests do not cover it. Recovery must resolve or retain the ambiguous attempt; local age alone does not establish closure. No provider calls or test execution were performed by the independent reviewer.

## Executed provider-success/session-save failure

TestClosureAfterCheckoutProviderSuccessLocalSaveFailure injects SQLite failure after actual fixture checkout creation, confirms a retained empty session ID, then uses a fresh Service for deletion. Before correction it returned closed while the provider session remained open. Now it returns pending/ErrBillingProviderAmbiguous and retains the attempt. The old-attempt reconciler also refuses to delete unresolved empty IDs based on age alone. All21 billing tests pass with race detection; scoped lint zero issues. This prevents false closure; automatic resolution is still required for launch and is not claimed complete. Stripe supports customer-filtered Checkout session listing and paginated discovery; idempotency keys may be pruned after24h, so replay alone is insufficient. See https://docs.stripe.com/api/checkout/sessions/list and https://docs.stripe.com/api/idempotent_requests. No live provider traffic.
