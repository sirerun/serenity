# T23.47 effective-plan independent review — 2026-10-01

## Verdict

The effective-access projection change is locally correct for the reviewed hosted meter rules. Reconciliation and webhook now set the account plan from effective access, rather than provider subscription status alone; grace parsing occurs before the reconciliation transaction can commit an invalid deadline. I found no blocker in those behaviors. This review excludes the separately pending teardown follow-up and does not establish complete T23.47 acceptance or live-provider qualification.

## Reviewed source

- Projection change: `9fd2d1c7de04e7dc9ad81702c1e567b4deae3afc` (`Match billing plans to effective subscription access`).
- RED regression tests: `eea0cea9250135973c262cb6540412507b5289de` (`Assert effective grace and rollback behavior`), included in the reviewed source history.
- Review checkout: `/Volumes/BuildOffload/worktrees/serenity-t23-47-effective-plan-review-20261001`, branch `review/t23-47-effective-plan-20261001`, based directly on the projection commit.

## Findings

`effectiveSubscriptionAccess` in `internal/hosted/billing/billing.go` follows `internal/hosted/meter/meter.go:128-154`: active and trialing subscriptions qualify only while their current period end is strictly after now; past-due subscriptions qualify only while their grace deadline is strictly after now; other statuses do not qualify. The strict time comparison matches the meter's `After` checks and SQL `grace_until > now` filter.

`ReconcileCustomer` evaluates the chosen provider subscription using its current period end and the exact grace value returned by `graceDeadline`. It writes a paid account plan only when the account is active and access is effective. It returns the same eligibility result while preserving the provider plan ID and window as reconciliation provenance. For `restore_pending` and `deleting`, reconciliation records the safe free plan, then returns the frozen-account error; the added regression fixture verifies those accounts remain free. Webhook projection applies the same effective-access calculation and writes a paid plan only for an active account with currently effective access.

Malformed grace is parsed during the reconciliation transaction before subscription or account projection updates can commit. The regression fixture starts with a deliberately malformed persisted deadline and a different failure invoice, then checks that the reconcile error leaves the old subscription fields and preexisting account plan unchanged. Webhook grace parse errors likewise return from the transaction callback before event processing can commit.

## Regression check

For a negative control, I changed only `effectiveSubscriptionAccess` in the isolated checkout to the prior status-only rule (`active || trialing || past_due`). `TestBillingAccountPlanTracksEffectiveSubscriptionAccess` failed at runtime for an expired active period and expired past-due grace: account plans remained `builder` and reconciliation reported `Eligible=true`. `TestReconcileMalformedPersistedGraceRollsBack` also failed because the malformed-deadline parse occurred only after the transaction had already updated the subscription and account. I restored the reviewed implementation before validation.

After restoration, `GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/hosted/billing` passed (`6.944s`). The temporary mutation was confined to the review checkout and restored; the checkout is clean. No provider calls were made.

## Limits

These checks demonstrate local calculation, projection, rollback, and race-suite behavior. They do not test exact equality-at-expiry with a controllable clock, although the production comparisons match the meter's strict boundary. The separately pending account teardown behavior was not reviewed here. No live Stripe behavior, deployment, cross-process coordination, complete T23.47 acceptance, or hosted qualification is claimed.
