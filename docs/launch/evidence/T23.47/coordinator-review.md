# Coordinator review: not accepted

The period-start/webhook-event-created grace calculation is not accepted. A provider-owned timestamp is insufficient unless it identifies the current payment failure. Both reconcile and webhook paths must resolve the same failure identity, preserve its first failure time, and avoid reopening grace for retries or delayed unrelated events. Current passing timing tests encode a narrower assumption.

Proposed implementation: bind grace to the outstanding subscription invoice and its first failed payment event, retain that evidence durably, and reconcile missed events through provider history with explicit handling for missing history. Stripe lists events for only 30 days and renders each event according to its original API version, so an unbounded-history assumption or current-version-only decoder is unsafe. This proposal is not yet implemented or a frozen interface change.

Sources reviewed:
- https://docs.stripe.com/api/events/list
- https://docs.stripe.com/api/events
- https://docs.stripe.com/changelog/acacia/2024-10-28/customer-portal-schedule-downgrades

Coordinator added race-tested lifecycle projection and webhook-outage/retry fixtures. Scheduled cancellation is not a scheduled price downgrade. Portal configuration, proration, Checkout-based re-subscription remain unqualified. An actual local transaction failure after provider cancellation is now injected and verified: pending response, unchanged local subscription, then fresh-Service retry completes without double cancellation. No provider calls, charges or deployment occurred.
