# ADR 016: Versioned plans, atomic metering, and Stripe-hosted billing

## Status
Accepted (plan table and publication: David, 2026-09-11; cost ceilings remain hypotheses until T23.3 and T23.36 measure them)

## Date
2026-09-11

## Context

The hosted offer sells operation and capacity: a free plan with no card and
two paid monthly plans. The launch prompt proposed a plan table as unratified
defaults. On 2026-09-11 David ratified the table as plan configuration version
1 and chose to publish it on the pricing page rather than hold it behind the
cost gate. Sire's own product billing lives only in the frozen legacy API
repository (`sirerun/legacy/api/internal/billing`), and the live Sire monorepo
uses a plain Payment Link (Sire ADR 0091), so nothing is reusable as code; the
Sire Run, Inc. Stripe account is confirmed ready and is reused.

## Decision

### Plan configuration version 1 (ratified, published)

| Limit, across the whole account | Free | Builder | Scale |
| --- | ---: | ---: | ---: |
| Monthly price (USD) | 0 | 19 | 49 |
| Brains | 1 | 3 | 10 |
| Live stored memories | 1,000 | 10,000 | 50,000 |
| Successful new writes per month | 500 | 5,000 | 20,000 |
| Recall calls per month | 10,000 | 100,000 | 300,000 |
| New input tokens per month | 100,000 | 1,000,000 | 4,000,000 |
| Total durable storage including history | 100 MB | 1 GB | 5 GB |

Definitions: a "write" is a `remember` call that returns success; a retried
call with the same `operation_key` counts once. A "recall call" is any
`recall` or `read_memory_fact` call that passes authentication and
authorization; authentication failures, readiness checks and `tools/list` do
not count. "Input tokens" are counted by the pinned embedding tokenizer over
the `fact` text; the per-memory input ceiling is 4 KB of UTF-8. "Storage" is
the on-disk size of the brain directory including git history and the index.
Multiple brains or credentials never multiply an allowance. Semantic recall,
export and deletion are included in every plan. Paid tiers buy capacity, not
team permissions or an SLA.

The table lives in `internal/hosted/plans` as a versioned Go value with a
JSON export used by the pricing page build check, so the site and the server
cannot disagree.

### Metering

- Every metered operation reserves before doing work: one SQLite transaction
  inserts a reservation row against the account's current window and fails
  with a typed `limit_exceeded` error (carrying `reset_at` and an upgrade URL)
  when the committed total plus open reservations would exceed the allowance.
  Success commits the reservation; failure releases it; a reservation whose
  lease expires (worker crash) is released by a sweep.
- Windows: UTC calendar month for Free; the Stripe billing period for paid
  plans. The window key is stored with the reservation, so month boundaries
  and period changes never double count.
- A single resolver returns the current entitlement from the plan assignment
  and the reconciled subscription state; nothing else reads plan limits.
- At capacity the server returns the typed limit and never drops a write
  silently or charges overage. Downgrade never deletes existing memory;
  writes above the new allowance are refused until the account is within it.
- Export, deletion, cancellation and cleanup work when allowances are
  exhausted; they have abuse limits only.

### Billing

- Stripe-hosted Checkout and the Stripe Customer Portal, test mode until the
  release task creates live objects. Products and prices for plan version 1
  are Serenity-specific; no other Sire product, price, subscription or webhook
  is touched.
- The server creates every Checkout Session with the authenticated account's
  Stripe customer, the server-chosen price for the requested plan, and
  `client_reference_id` set to the account id. Client-supplied price or
  customer ids are never accepted.
- Return URLs grant nothing. The webhook receiver verifies the signature over
  the raw body, stores every event id before processing (duplicate ids are
  ignored), and applies changes by fetching the current subscription from
  Stripe rather than trusting event order. Restored or replayed events cannot
  revive cancelled access or create a second subscription for one account.
- Lifecycle handled: incomplete or failed initial payment, activation,
  renewal, payment failure with a documented grace period, upgrade (prorated
  by Stripe), downgrade and cancellation at period end, portal sessions bound
  to the account's own customer id.
- Quota windows follow the billing period; a Checkout refresh or webhook
  replay never resets a window or grants repeated credit.
- Third-party dependency: the Stripe Go SDK is allowed at the provider edge
  under ADR 003's posture, pinned by version.

### Economics

Proposed variable-cost ceilings per active account per month are USD 0.50
(Free), 3.80 (Builder) and 9.80 (Scale). They are hypotheses. T23.3 measures
the inputs and T23.36 tests full-limit workloads; if a ceiling fails, the
remedy is an explicit ADR amendment to the offer or the architecture, never
an undisclosed runtime cap that denies an advertised allowance.

## Consequences

Positive: one source of truth for limits shared by server and site; metering
is crash-safe by construction; billing state is reconciled, not inferred from
delivery order; prices are founder-ratified so the pricing page can ship with
the app.

Negative: publishing before the cost gate means a failed ceiling forces a
visible correction; SQLite serializes reservation writes (acceptable at launch
scale, revisit with the pool); Stripe is now a launch-critical dependency for
paid access, though free access never depends on it.
