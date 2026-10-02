# Recovery billing observer development-contract proposal receipt

Status: draft proposal for coordinator and independent review. It is not a frozen contract, implementation receipt, provider qualification, or acceptance claim.

## Scope and evidence

This docs-only proposal is based on main `c3d491b03980310d7d59688d092b8119640e5250`, readiness report `df94fbe3cf30a90fe98837685353ab1a20c5500e`, and integration packet `5335401dba661e05d16348c9a1caf44cb3252de6`. It specifies a read-only, bounded observer implementing the already additive `contracts.RecoveryBillingObserver`, preserves existing billing reconciliation and schema behavior, and leaves production wiring separate.

The proposal specifies explicit constructor-validated immutable limits; a wrapper observer sharing the exact billing account lock; only fixed-host GET requests with no redirects/proxy lookup/connection reuse/compression; strict bounded event/resource decoding; two local status/binding/checkout checks; and failure-closed behavior for incomplete history. Its past-due plan binds Basil invoice lines to the exact subscription item and service period, and uses a strictly ordered history of subscription-created/updated state plus payment-failed events. A lone invoice-paid event does not reset grace; a proven active/trialing provider state can reset, while a complete no-reset history uses the earliest applicable failure.

Provider shape facts were checked against official Stripe API references and the tagged `stripe-go v82.3.0` generated schema at the links in the proposal. No live Stripe request, source implementation, production assembly change, Go test, provider mutation, migration, or activation was performed. `git diff --check` is the only validation performed.

## Important limits

The interface remains a point observation and process-local lock; it does not establish cross-process serialization, freshness, fencing, deletion/adoption, or recovery authorization. Stripe's event API is limited to 30 days, so the exact period history can be unprovable. A later implementation must refuse incomplete or ambiguous evidence. Numeric limits are proposed explicit ceilings and still need independent review/freeze. No live-provider qualification is implied by SDK schema review or fake-provider tests.

## Claim and pin

This receipt and its companion proposal are owned under canonical claim `R-billing-observer-contract`, initially won at `772a5f1cd4cc4c4d161660b9aaa20ff83f8e39b4`. Exact proposal/source pin and claim release are recorded in the commit handoff; no other file ownership is implied.
