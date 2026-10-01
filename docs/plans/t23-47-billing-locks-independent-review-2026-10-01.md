# T23.47 billing lock independent review — 2026-10-01

## Verdict

The account-scoped lock candidate and its receipt are locally sound for the reviewed in-process concurrency contract. I found no blocker in lock lifecycle, lock ordering, cross-account progress, same-account Checkout/closure serialization, or webhook snapshot freshness. This is a source and focused-test review, not live Stripe qualification, deployment approval, or proof of cross-process serialization.

## Reviewed source

- Candidate implementation: `035d0deef8e5f1ec5480163b9b96600393fc5b1a` (`Serialize hosted billing operations per account`).
- Candidate test/evidence correction: `f4cf7fdadd0b1b27c9e571d82c884b79857cb69a` (`Record account-scoped billing lock validation`).
- Review checkout: `/Volumes/BuildOffload/worktrees/serenity-t23-47-billing-locks-review-20261001`, branch `review/t23-47-billing-locks-20261001`, based on the receipt commit.

## Findings

`internal/hosted/billing/locks.go` implements a context-aware, reference-counted keyed lock registry. It increments references while holding the registry mutex, releases that mutex before waiting on a per-key token, decrements references on cancellation, and removes an entry only when no holder or waiter references it and the map still points to that entry. The post-acquire context check returns the token before dropping the reference. I found no path that holds the registry mutex across provider I/O or a waiter that can leave a stale entry behind.

Lock namespaces separate `event:<id>` from `account:<id>`. `Webhook` acquires the event lock, makes an identity-only provider fetch, resolves the account, acquires that account lock, rechecks the stored customer binding, then refetches the subscription. Only the second snapshot is used for projection. `ReconcileCustomer`, `Checkout`, and billing closure use the account key. No code path found acquires an event lock while holding an account lock, so the observed order has no inverse-lock cycle.

The tests added with the candidate exercise different-account progress while one provider request is blocked (`TestAccountLocksAllowDifferentAccountsToProgress`), same-account Checkout/closure serialization (`TestCheckoutAndClosureSerializeForSameAccount`), and the stale identity-fetch race against reconciliation (`TestWebhookRefetchesAfterReconcileWinsAccountLock`). The lock cancellation/reference lifecycle is covered by `TestKeyedLocksCancellationRetainsHeldEntryUntilLastReference`. The webhook deduplication test's expected provider fetch count correctly changes to two for identity lookup plus authoritative refetch.

The account lifecycle code uses a separate gateway maintenance lock, but account deletion first persists `status='deleting'`; the billing webhook's locked status recheck gates entitlement projection to active accounts and writes the free plan for other statuses. The account row is marked deleted rather than physically removed, so the reviewed recheck does not encounter a disappearing-row case in this lifecycle. Billing and gateway locks do not provide a global cross-process mutex; database/provider coordination across multiple service processes remains outside what these tests prove.

The receipt correctly narrows the webhook race assertion to subscription status. It does not claim that canceled subscription status necessarily projects the account plan to free. The effective-plan projection discrepancy is a separate known follow-up and must remain open until independently fixed and tested; it is not evidence against the lock ordering itself.

## Validation and limits

Ran `GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/hosted/billing` from the isolated review checkout: PASS (`5.949s`). No provider calls were made.

This review covers only the candidate source and local package tests. It does not establish external Stripe behavior, multi-process lock coordination, billing plan projection correctness, complete T23.47 acceptance, or production qualification.
