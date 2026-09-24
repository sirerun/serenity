# Checkout interruption recovery — proposed integration

Status: proposed, not implemented or accepted. The false-closure regression is
fixed at ee6b7f2; automatic recovery remains a launch requirement.

## Required durable identity

A checkout attempt is already committed before the provider call. Extend that
record with an immutable request version and the exact non-secret creation
parameters (server customer, price, quantity, mode, return URLs, account reference
and attempt metadata). Persist them in the same transaction that allocates the
attempt. Never persist the API key or authorization headers.

New-version requests include an opaque attempt ID in Checkout Session metadata.
Every retry uses the stored version and parameters. Adding metadata indiscriminately
to existing attempts would change the parameters of a previously submitted
idempotent request; legacy attempts must retain their existing request format.
Schema changes require the shared store owner; this billing lane does not apply them.

## Resolution rules

1. A known session ID is fetched and checked against server-owned customer and
   attempt identity. Expiration must confirm the same ID and expired status.
2. A missing session ID with a versioned attempt is discovered through the
   customer-filtered, paginated Checkout Session list. Match the exact opaque
   attempt metadata, validate mode/customer/account reference, reject duplicate
   matches or malformed pagination, and persist the recovered ID conditionally
   against the still-current attempt. Do not choose a session by nearest timestamp.
3. A successful create followed by a failed local save is then recoverable without
   issuing another create. A timeout after recovery or expiration can resume from
   that same identity. Re-list subscriptions after session completion/expiration;
   a newly discovered subscription must be canceled before closure.
4. No match is not proof that an in-flight create never succeeded. Preserve the
   pending attempt until request outcome and absence are established; do not turn
   a partial list, provider timeout, or local age threshold into successful cleanup.
5. Legacy missing-ID attempts lack exact provider metadata. Keep them explicitly
   pending until a provider-qualified migration/reconciliation resolves them.
   Do not guess among historical sessions or recreate with a fresh key.

The same resolution primitive must serve ordinary reconciliation and deletion;
checkout retry must not overwrite an unresolved attempt. Frozen/deleting accounts
must never receive a usable checkout URL as part of repair.

## Required qualification matrix

- Provider create succeeds; local session-ID save fails; fresh Service discovers
  the exact session, expires it, and certifies closure only after subscription recheck.
- Creation response is lost and reconciliation runs while creation is still in
  flight: no absence-based closure or second checkout.
- Matching session appears beyond the first list page; duplicate matching IDs,
  foreign customer, wrong account reference, invalid cursor and provider outage
  remain pending with the original attempt intact.
- Provider expiration succeeds; subsequent local save fails; retry converges.
- Completed session creates a subscription between the initial and final list;
  closure stays pending until cancellation and recheck converge.
- Legacy attempt predates request versioning; retries preserve original parameters
  and lack of unique discovery evidence never produces false closure.

## Source basis

Stripe supports customer filtering and starting_after pagination for Checkout
Sessions: https://docs.stripe.com/api/checkout/sessions/list . Stripe may prune
idempotency keys after at least24h and rejects changed parameters for a retained
key: https://docs.stripe.com/api/idempotent_requests . These documented capabilities
do not qualify our pinned API version, credentials or deployed recovery behavior.
Only local HTTP fixtures have been used in this lane.
