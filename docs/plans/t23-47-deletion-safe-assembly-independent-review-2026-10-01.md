# T23.47 deletion-safe billing assembly independent review — 2026-10-01

## Verdict

The reviewed service assembly now freezes accounts before attempting billing closure and does not purge brain data until the closer certifies a closed result. Startup replay constructs the configured billing closer before processing deleting accounts. Local focused race tests and a negative-control mutation support those claims. I found no deletion-order blocker in this patch. This is limited to local assembly/lifecycle behavior and does not complete journal, backup, privacy, restore activation, periodic reconciliation, or live-provider qualification.

## Reviewed source

- Implementation: `83aee44e1b903d88f125db3188c5b61d0085d061` (`freeze deletion before certifying billing closure`).
- Pending/unconfigured test additions: `200c2dbaeadf4ffbf6a05c01cd606eaee7e6842d` (`cover pending and unconfigured deletion closure`).
- Review checkout: `/Volumes/BuildOffload/worktrees/serenity-billing-lifecycle-review-20261001`, branch `review/billing-lifecycle-20261001`, based on `200c2db`.

## Findings

`service.DeleteAccount` changes an active account to `deleting` inside a transaction, reads the durable status/customer/subscription evidence, then calls `BillingCloser.CloseBillingAccount`. It returns on provider error or any result other than `CloseStatusClosed`, preserving brain bytes. If no closer is configured, it fails closed when billing is enabled or a provider customer/subscription remains persisted. Only after closure is certified does it call the existing `Gateway.DeleteAccount` purge path. Calls repeated after the account has reached `deleted` return successfully; calls on other frozen states are rejected.

`Assemble` builds the provisioner and runs provisioner recovery, whose query only selects allocations for accounts still `active`. It then constructs `billing.Service` when the billing config is present, assigns it as the service's closer, wires the dashboard deletion callback to `Service.DeleteAccount`, and only then invokes deletion recovery. Thus startup replay of a `deleting` row uses the same closure-before-purge path. If provider closure is unavailable, assembly returns an error and the brain remains present; this is fail-closed but makes service startup unavailable until recovery can be retried. The billing provider client has a 15-second timeout, but recovery uses a background context and is not operator-cancelable through `Assemble`.

The dashboard no longer performs legacy `CancelAccount` before invoking the gateway purge directly; it delegates the operation to the service coordinator. Existing gateway lifecycle code remains intact, including its final partner-link revocation, brain drop, credential/session revocation, and account status transition. The partner handler is still mounted in the service mux after successful recovery. The existing service partner suite exercises partner route assembly and gateway account deletion, though this candidate adds no new end-to-end partner deletion assertion.

Billing operations already use a per-account lock. Checkout/reconciliation/webhook activity that began before the freeze can finish, while new credential verification requires `accounts.status='active'`. Gateway brain deletion uses the existing per-account gateway lock before dropping the runtime and removing files, so an already-running account call is allowed to settle before purge. If provider closure fails, the account stays restricted and its bytes remain for retry.

One retryable concurrency edge remains: two DeleteAccount requests can both read `deleting`; if one finishes the purge and marks the account `deleted` while the other is waiting for the billing account lock, the latter's `CloseBillingAccount` can observe `deleted` and return `ErrBillingAccountFrozen`. The second request may report a retryable failure even though deletion completed; a later retry observes `deleted` and returns success. This does not permit premature purge or leave the account active, but concurrent duplicate-request behavior is not directly tested.

## Validation

Ran focused race tests in the isolated checkout:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/hosted/service -run 'TestDeleteAccountFreezesBeforeClosureAndRetainsUntilClosed|TestDeletionWithoutConfiguredCloserRetainsExistingCustomer|TestStartupDeletionClosesBillingBeforePurging'`

Result: PASS (`2.000s`). Startup tests use a local `httptest.Server`; no live provider calls were made.

For a negative control, I temporarily changed only the isolated `DeleteAccount` freeze update to leave the account active and allowed the active status through to the closure call. `TestDeleteAccountFreezesBeforeClosureAndRetainsUntilClosed` failed at runtime because the fake provider observed `active`, not the required durable `deleting` state. I restored the reviewed source and confirmed the checkout is clean.

## Limits

The local tests establish the order of freeze, closure result handling, startup replay, retention on error/pending, and eventual purge after certification. They do not establish hosted Stripe behavior, interruption recovery during a real external request, complete concurrency semantics for simultaneous delete requests, partner deletion behavior in the newly assembled service path, or any of the broader T23.47 release gates.
