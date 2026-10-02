# Recovery billing observer implementation contract proposal

Status: **proposal for independent review; not frozen and not an implementation authorization**. This document narrows the additive `RecoveryBillingObserver` candidate to a bounded, GET-only billing implementation. It does not establish live Stripe qualification, recovery admission, or production activation.

## Baseline and ownership boundary

The source baseline is main `c3d491b03980310d7d59688d092b8119640e5250`. The additive interface is in `internal/hosted/contracts/recovery_billing.go`; its readiness findings and precise hazards are in `docs/launch/evidence/T23.50/recovery-billing-readiness-review-2026-10-01.md` and `docs/launch/evidence/T23.50/integration-request-billing-observer.md`.

The implementation should be a separate observer path in `internal/hosted/billing`, leaving `ReconcileCustomer`, `failureEvidence`, checkout reconciliation, webhook handling, and schema unchanged. The likely owned files are new `recovery_observer.go`, `recovery_provider.go`, and focused tests. The frozen interface does not require the method to be on `*billing.Service`; use a new `RecoveryObserver` wrapper implementing it. `NewRecoveryObserver(*Service, RecoveryObserverLimits)` performs validation only (no database or network I/O) and copies explicit limits, the Store pointer, provider key, and known plan IDs into private wrapper state. The wrapper also retains the original service reference solely to acquire its exact process-local account lock. Publish the wrapper only after successful construction. This supersedes the readiness report's tentative receiver assumption. A later factory may wire the wrapper under separate ownership; no production assembly edit is required in this implementation slice.

## Method contract

The method accepts only `ctx` and an internal account ID. It reads the provider customer binding from the server database; no caller-provided customer, subscription, invoice, or checkout ID is accepted. It acquires the existing `account:<id>` keyed lock for the complete initial-read, provider-read, final-reread sequence. This lock is process-local and does not fence other service processes or provider changes.

At method entry, start the configured whole-call timeout before waiting for the account lock; the caller's earlier deadline takes precedence. Require a live context, initialized Store, explicit observer configuration, an account ID of 16–64 ASCII alphanumeric bytes (matching the existing store ID validator), and an account row whose status is exactly `restore_pending`. Require a valid bound customer ID and no `checkout_attempts` row. A present checkout row is unresolved regardless of age or session state: return an error without making a provider request. A missing account, any other account status, invalid/missing binding, or missing configuration is an error, never an inferred Free result.

Capture one UTC `ObservedAt` after the initial local read and use one whole-call deadline. After all GETs, reread status, exact customer binding, and checkout absence while retaining the account lock. Any changed value, newly present checkout, query error, cancellation, or provider ambiguity returns the zero observation. All SQL is SELECT-only. No local billing failure/grace is used as positive evidence, and no database or provider write occurs.

On success, set `AccountID` to the input, `ObservedAt` to that single captured time, and `Source` to a fixed non-sensitive provenance value. Never include provider IDs in the result or errors. Return the zero observation for every error. Error text must not include response bodies, credentials, customer/subscription/invoice IDs, or raw provider diagnostics; wrap only stable package sentinels and context errors.

## Explicit immutable limits

Add `RecoveryObserverLimits` and require every value explicitly at construction. Validate once, copy to private wrapper state, and publish the wrapper only after validation. Zero, negative, overflowed, or above-ceiling values fail construction; there is no “zero means default.” These accepted values are pinned for this draft:

| Limit | Proposed accepted range / ceiling |
|---|---:|
| Whole observer timeout | `1ms..30s` |
| Bytes per response | `1..1MiB` |
| Aggregate response bytes per observation | `1..64MiB` |
| JSON nesting depth | `1..32` |
| JSON value/member nodes per document | `1..100,000` |
| Total provider GETs | `1..128` |
| Event pages | `1..32`, page size exactly 100 |
| Distinct invoices examined | `1..16` |
| Pages for any invoice-line collection | `1..4`, page size at most 100 |

Each provider request, including subscription list, event list pages, invoice retrieval, and invoice-line pages, consumes the same total GET budget. The 128-GET ceiling covers one subscription page, 32 event pages, 16 invoice fetches, and four line pages per invoice (113 GETs). Calls are serial. Any budget exhaustion, non-advancing/repeated cursor, duplicate object ID, or incomplete `has_more` result is an error, not a partial observation. The aggregate and per-response limits are both enforced with `max+1` reads so an oversized body is detected rather than parsed as a valid truncated prefix. JSON parsing must reject duplicate object keys, case aliases for recognized fields, missing required fields, wrong types, trailing documents, and depth/node overages. Use a Basil-versioned decoder that pins required decision subtrees and all discriminators (subscription status, event API version/type, line parent type, pricing type, and object/list type). Unknown values in those fields or unknown fields within decision subtrees fail closed. Bounded unknown fields outside decision subtrees may be ignored if they do not alter a parsed discriminator or decision input; never surface them. A generated fixture with more than 400 unrelated matching-type events plus relevant evidence must succeed inside these limits; separate cap-exhaustion cases must refuse. These limits describe a bounded usage envelope, not arbitrary Stripe account volume.

The observer gets its own HTTP client/transport policy. It must perform only fixed-host HTTPS GETs to `api.stripe.com`, reject all redirects, and never mutate or weaken the shared client used by reconciliation. Do not inherit an arbitrary `Config.Client`/RoundTripper or production `BaseURL`; those can change transport, host, or redirect behavior. Use a private `http.Transport` with proxy lookup disabled, keep-alives disabled to avoid transparent reused-connection GET retries, and compression disabled so byte accounting observes wire-body bytes consistently; retain standard TLS validation and set bounded dial/TLS/header timeouts. Validate the API secret's explicit live/test prefix and snapshot it. One request deadline covers account-lock wait, every GET, response read, and parsing; earlier caller cancellation wins. Tests should be same-package `_test.go` helpers that construct real wrapper state with `httptest` transport; do not add a production fake transport/base-URL constructor or injection field.

## Local state and frozen status/result table

Use the account lock above and strict, bounded SELECTs for the initial and final snapshots. The only relevant local fields are account ID/status/customer binding and checkout-attempt existence. Do not consult `billing_failures`, `subscriptions`, restored grace fields, or audit rows to create provider truth. Existing helpers may be reused only when pure and semantically identical (`planForPrice`, `subscriptionTerminal`, and the active/trialing predicate); do not call write-capable reconciliation helpers.

The provider subscription list must explicitly contain a `data` array and boolean `has_more`; require one complete page. Every returned object must have all decision-relevant fields with correct types. Validate customer ownership, unique IDs, one current item per selected subscription, its valid item ID and price ID, and explicit supported status. Active/trialing/past_due require a price mapped to a known configured plan. Read current window bounds from that selected item's `current_period_start` and `current_period_end` (Basil does not supply them as subscription-level fields). Reject multiple nonterminal subscriptions, duplicate items, unknown statuses, missing period timestamps, nonpositive or reversed periods, invalid Unix timestamps, future timestamps where prohibited, and subscription/customer mismatches.

The status-to-result table is pinned as follows. In all cases, an error returns the zero observation.

| Provider state after complete customer-bound list | Result |
|---|---|
| Empty list, or only `canceled` / `incomplete_expired` terminal rows | Success, `PlanID="free"`, `Eligible=false`, zero window and grace. Terminal rows need a valid typed `price_` ID, but it need not map to a currently configured plan. |
| Exactly one `active` or `trialing` subscription | Requires known configured plan and valid selected item window. Return that plan/window even when the period is expired; `Eligible` follows the existing pure access predicate. `GraceUntil` is zero. |
| Exactly one `past_due` subscription | Requires known configured plan, valid selected item window, and complete provider evidence. Return plan/window and derived `GraceUntil`; `Eligible` is true only when grace is strictly after `ObservedAt`. |
| Exactly one `unpaid` or `paused` subscription | Requires valid supported shape and typed price ID; success as `PlanID="free"`, `Eligible=false`, zero window and grace. These are authoritative ineligible states, never grace states. |
| More than one nonterminal row, including multiple unpaid/paused rows or any ambiguous combination | `ErrBillingProviderAmbiguous`. |
| `incomplete`, any unrecognized status, malformed/unknown identity, or an incomplete list | `ErrBillingProviderAmbiguous`; never infer Free. |

`cancel_at_period_end` does not itself erase current-period access. Terminal status handling is intentionally wider than `subscriptionTerminal` used by reconciliation; this observer must not change that helper.

## Period-scoped past-due evidence

Do not reuse `failureEvidence`: it can trust restored rows and persists failure evidence. Do not use only `latest_invoice`; `graceDeadline` preserves old grace when an invoice changes during the same period, and a latest-only query can produce a later deadline than the existing rule. Do not use local webhook/delivery time, invoice creation time as a substitute anchor, or restored SQL grace to grant or extend access.

Start the event-history query at the exact selected item's provider `current_period_start` and end it at `ObservedAt`; never clip the start forward to a provider-retention boundary and call the remaining suffix complete. Refuse when the item period starts within 60 seconds of or earlier than the 30-day retrieval cutoff; query bounds still use the exact period start. This fixed margin covers cutoff and clock uncertainty only, not delayed event publication or a production-fencing guarantee. Stripe's Events API only exposes the last 30 days, so older/near-cutoff periods are unprovable through this source alone. Do not fall back to local stored failure rows.

The Events list supports `types[]` (up to 20 types), `created[gte]`, `created[lte]`, cursor, and limit, but does not provide customer/subscription filtering. Request exactly `customer.subscription.created`, `customer.subscription.updated`, and `invoice.payment_failed` together, with page size 100. Every returned event must be strictly parsed and associated before it can affect the result. Require list `data`, `has_more`, unique opaque event IDs, cursor progression via the returned final ID (never lexical ID ordering), timestamps within the requested interval, and provider order nonincreasing by `created` within and across pages. Collect all pages before deriving state, then sort ascending by `(created, opaque event ID)` only for deterministic processing; ID tie order is not chronological. Same-second events that can change/reset/fail this subscription in conflicting order make the result ambiguous; unrelated same-second events do not. Complete pagination is required within the global request/page budgets. Any unsupported event version/shape or ambiguous event that could affect the selected subscription makes the whole result unknown; do not skip it merely because an old object shape lacks a subscription field.

For every candidate invoice event, use the immutable invoice and line snapshot embedded in the `invoice.payment_failed` event to prove event-time association. Require the event snapshot to contain a complete line collection (`has_more=false`) and all decision fields; if absent or truncated, refuse rather than treating a later GET of current lines as historical evidence. Retrieve the current invoice and all bounded line pages as a consistency check, and require exact agreement on every event-time decision field and line identity; any difference fails closed. The current GET cannot fill a missing event-time field. Verify invoice/customer identity, exact selected subscription association, unique line IDs, and complete line coverage. Under API version `2025-06-30.basil`, select the line's `parent.type == "subscription_item_details"` branch and require its subscription and subscription-item string IDs to match the selected subscription and item; require `pricing.type == "price_details"` and exact known price ID. Match line `period.start`/`period.end` to prove the represented service interval. Do not treat invoice aggregate `period_start`/`period_end` as the service period: those fields are a lookback interval. Do not require the aggregate invoice interval to equal the current subscription window, and do not dismiss a different-window event without complete line-level proof. Any missing or contradictory association fails closed. Capture the selected subscription-item ID from the strict subscription response.

**Episode-reset policy:** a lone `invoice.payment_succeeded` is not proof that the subscription recovered and must not reset grace. Query complete bounded history for `customer.subscription.created`, `customer.subscription.updated`, and `invoice.payment_failed`, ordered by event `created`. A reset requires a fully validated provider event-time subscription snapshot showing `active` or `trialing` for this exact customer, subscription, item, price, and applicable service window. For `customer.subscription.updated`, validate both the `data.object` snapshot and the supported `previous_attributes` shape. If complete validated status/failure history proves active/trialing after an earlier failure, anchor at the earliest verified applicable failure after that transition. If complete failure/status history proves no recovery transition in the current period, keep the earliest applicable failure in the period; absence of reset evidence alone does not invalidate continuous `past_due`. Relevant same-second conflicts or insufficient proof return ambiguity. Never invent a later anchor.

Item/proration semantics are pinned: an item ID or price change within/overlapping the current item window is ambiguous; do not carry an old item's failure into a replacement item. A quantity-only update on the same item, price, and unchanged window is supported. A normal invoice line applies only when its exact item/price and full service period match. A proration line is supported only if the interval is strictly contained within the selected item's current window, item and price are unchanged, and the immutable full invoice snapshot also proves the non-proration base window. Otherwise refuse. Renewal-boundary overlap and insufficient event-time proof refuse. Fixtures must cover replacement, quantity-only update, proration, and renewal boundary.

After the frozen algorithm has a complete applicable history, compute `GraceUntil = verifiedFailureTime + 72h` with checked time arithmetic. Past-due eligibility is strictly `GraceUntil.After(ObservedAt)`. Failure time must be provider event creation time, not local receipt time. Provider-side retries, duplicate delivery, invoice churn, missing data, expired retention, unsupported versions, ambiguous periods, or budget exhaustion cannot move the anchor later or produce a successful Free/eligible result.

## Version and schema pinning

Direct Stripe GETs use the existing pinned `APIVersion` (`2025-06-30.basil`). The accepted event `api_version` allowlist is exactly `2025-06-30.basil`; empty, null, older, newer, or unknown values fail closed. There is no compatibility fallback. Event snapshots carry the API version at event creation; sending the current request `Stripe-Version` does not reinterpret their stored objects. For Basil, use only the documented event/invoice-line subscription parent shape; do not try a direct `data.object.subscription` fallback. Require event `type`, event ID, `created`, `livemode`, object type/identity, and every association field needed by the algorithm. Event live mode must match the configured key mode. Future timestamps, unknown envelope types, mixed-mode data, missing required fields, noncanonical JSON key casing, and unknown decision-bearing fields fail closed. Bounded documented nondecision fields may be ignored.

Payment-failed event snapshots must contain exact invoice-line facts for event-time subscription/item/price/window association; a later current invoice GET alone cannot establish historical truth. Check in Basil full-shape fixtures, including harmless optional fields. Decoder support remains Basil-only.

## Constructor integration

Add `NewRecoveryObserver(service *Service, limits RecoveryObserverLimits) (*RecoveryObserver, error)` in the new billing files. It performs no I/O. Validate nonnil service/store, all explicit bounds, Stripe key syntax, and both configured plan-price values; accept only `sk_live_`/`rk_live_` or `sk_test_`/`rk_test_` prefixes, with ASCII non-control keys no longer than 256 bytes. Provider opaque IDs require the expected typed prefix, ASCII, and a 256-byte ceiling; price IDs must be valid `price_` IDs. Snapshot needed config and limits into private immutable wrapper state and retain the exact service lock coordinator. No mutable limit field, arbitrary client, proxy, or production endpoint override. Same-package `_test.go` helpers may construct real wrapper state with fake transport. Do not add production test injection. A future factory may retain this wrapper where the interface is consumed; that assembly change is separate. Do not change generic `request`/`providerRequest`.

## Stable Source and error contract

Use exactly `stripe_recovery_observation` for successful `Source`; never append status/provider details.

| Failure | Returned error |
|---|---|
| Invalid observer config at construction | billing `ErrRecoveryObserverConfiguration`, before I/O |
| Missing account, wrong status, invalid/missing customer binding, or unresolved checkout | billing `ErrRecoveryObserverAccountState`; preflight cases perform no provider request |
| Provider transport or non-2xx status | `contracts.ErrBillingProviderUnavailable` |
| Unsupported/malformed/conflicting provider response/history or any cap exhaustion | `contracts.ErrBillingProviderAmbiguous` |
| Request context canceled or deadline exceeded | Preserve `context.Canceled` or `context.DeadlineExceeded` as an unwrap-able cause |
| Final local state changed or could not be verified | billing `ErrRecoveryObserverAccountState` |

Every method error returns the zero observation. Do not include provider IDs, account IDs, key material, request URLs, response bodies, or raw transport diagnostics in returned messages.

## Focused verification before any integration claim

Use `httptest` fakes and real local SQLite stores; do not call Stripe. Prove:

- exact `restore_pending` and bound-customer checks, pending checkout refusal before any HTTP request, and status/binding/checkout final-reread refusal while a GET is blocked;
- active/trialing future and expired periods, explicit complete Free states, unsupported/incomplete status refusal, unknown price, duplicate subscriptions, malformed windows, and strict zero-value behavior;
- current-period earliest applicable failure across multiple invoices, same-period invoice churn, no reset from a lone invoice-success event, a complete no-reset continuous-past-due history, a verified active/trialing snapshot followed by a new failure, strict event ordering, exact item/window association, wrong customer/subscription/item/price, item/price replacement refusal, quantity-only update acceptance, proration containment and base-window proof, renewal-boundary refusal, cross-window event rejection only after complete line proof, and no latest-invoice-only regression;
- 30-day cutoff with 60-second margin and boundary ±1 second, Basil-only event `api_version`, live-mode mismatch, complete `types[]` pagination and cross-page created ordering, repeated/nonadvancing cursor, duplicate opaque IDs, relevant same-second conflicts, out-of-window/future times, exhausted 128-request/32-event-page/16-invoice/4-line-pages-per-invoice budgets, 64MiB aggregate bytes, depth/nodes, and truncated immutable event-time invoice lines;
- duplicate keys, case aliases, missing required fields, unknown decision fields, trailing JSON values, oversized bodies (including a valid prefix plus extra bytes), and malformed/null list fields;
- every HTTP method is GET, redirects are refused without contacting the redirect target, no retry/method rewrite occurs, context/deadline interrupts lock wait and in-flight GETs, errors expose no provider bodies or identifiers, and all relevant SQL tables are unchanged on success and failure.

Every request remains GET; redirects and ambient proxy configuration are rejected; no retry/method rewrite or keep-alive reuse occurs. Context/deadline interrupts lock wait and in-flight GET; errors expose no provider body/identifier; and all relevant SQL tables are unchanged on success/failure. Runtime mutation controls should demonstrate that using latest-invoice-only, clipping event history, omitting the final reread, permitting redirects, accepting omitted fields, or trusting `billing_failures` causes test failure. Focused package race, vet, and lint are required after implementation; integration/full gates are owned by the coordinator. No live Stripe qualification follows from fake-provider tests.

## Authoritative provider references checked 2026-10-01

These references support the API-shape facts above; they do not establish live-account completeness or live-service qualification:

- [List events](https://docs.stripe.com/api/events/list): event-type/created filters (including multiple event types), cursor pagination, and limited event-history window.
- [Event type catalog](https://docs.stripe.com/api/events/types): `customer.subscription.updated` snapshots represent subscription changes, including status changes.
- [stripe-go v82.3.0 event schema](https://raw.githubusercontent.com/stripe/stripe-go/v82.3.0/event.go): pinned schema records event `api_version`, event-time resource snapshot, and `previous_attributes` on update events; this is schema corroboration only, not a runtime dependency.
- [Event object](https://docs.stripe.com/api/events/object) and [retrieve event](https://docs.stripe.com/api/events/retrieve): event data is rendered at the event's creation `api_version`; current request headers do not rewrite the snapshot; retrieval is limited to recent events.
- [Pagination](https://docs.stripe.com/api/pagination): cursor and page-size behavior.
- [Invoice object, Basil](https://docs.stripe.com/api/invoices/object?api-version=2025-06-30.basil): invoice-level period fields are not the subscription line service interval.
- [Invoice line item object, Basil](https://docs.stripe.com/api/invoice-line-item/object?api-version=2025-06-30.basil): line-level `period`, price, and subscription-item parent association used to bind a service interval.
- [stripe-go v82.3.0 invoice-line-item schema](https://raw.githubusercontent.com/stripe/stripe-go/v82.3.0/invoicelineitem.go): pinned SDK type shape for the Basil `parent.subscription_item_details` string IDs and `pricing.price_details.price`; this is schema corroboration only, not a runtime dependency.
- [stripe-go v82.3.0 subscription-item schema](https://raw.githubusercontent.com/stripe/stripe-go/v82.3.0/subscriptionitem.go): subscription-item current-period timestamps and item ID shape for the Basil subscription response; this is schema corroboration only, not a runtime dependency.
- [Basil subscription object](https://docs.stripe.com/api/subscriptions/object?api-version=2025-06-30.basil) and [Basil item-period changelog](https://docs.stripe.com/changelog/basil/2025-03-31/deprecate-subscription-current-period-start-and-end): current period boundaries are item-level in Basil.

## Non-goals and gates that remain open

This proposal supplies no provider credentials, source implementation, service assembly change, recovery workflow, pending-review decision, database write, provider mutation, job/cron, live Stripe check, cross-process lock, freshness guarantee, stop proof, fencing/adoption proof, or activation. A successful point observation is not authority to restore an account. The behavior choices and limits remain a proposal pending independent rereview; source ownership and implementation authorization are separate. Existing acceptance/readiness state remains unchanged.
