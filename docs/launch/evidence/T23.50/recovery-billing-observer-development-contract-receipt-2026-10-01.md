# Recovery billing observer development-contract proposal receipt

Status: draft proposal for coordinator and independent review. It is not a frozen contract, implementation receipt, provider qualification, or acceptance claim.

## Scope and evidence

This docs-only proposal is based on main `c3d491b03980310d7d59688d092b8119640e5250`, readiness report `df94fbe3cf30a90fe98837685353ab1a20c5500e`, and integration packet `5335401dba661e05d16348c9a1caf44cb3252de6`. It specifies a read-only, bounded observer implementing the already additive `contracts.RecoveryBillingObserver`, preserves existing billing reconciliation and schema behavior, and leaves production wiring separate.

The proposal specifies explicit constructor-validated immutable limits; a wrapper observer sharing the exact billing account lock; only fixed-host GET requests with no redirects/proxy lookup/connection reuse/compression; strict bounded event/resource decoding; two local status/binding/checkout checks; and failure-closed behavior for incomplete history. Its past-due plan binds Basil invoice lines to the exact subscription item and service period, and uses a strictly ordered history of subscription-created/updated state plus payment-failed events. A lone invoice-paid event does not reset grace; a proven active/trialing provider state can reset, while a complete no-reset history uses the earliest applicable failure.

Provider shape facts were checked against official Stripe API references and the tagged `stripe-go v82.3.0` generated schema at the links in the proposal. No live Stripe request, source implementation, production assembly change, Go test, provider mutation, migration, or activation was performed. `git diff --check` is the only validation performed.

## Important limits

The interface remains a point observation and process-local lock; it does not establish cross-process serialization, freshness, fencing, deletion/adoption, or recovery authorization. Stripe's event API is limited to 30 days, so the exact period history can be unprovable. A later implementation must refuse incomplete or ambiguous evidence. Numeric limits are proposed explicit ceilings and still need independent review/freeze. No live-provider qualification is implied by SDK schema review or fake-provider tests.

## Claim and pin

This receipt and its companion proposal were first edited under canonical claim `R-billing-observer-contract`, won at `772a5f1cd4cc4c4d161660b9aaa20ff83f8e39b4`. The independent-review revision was mistakenly recorded as using a different claim identifier; the revision is now held under the canonical `R-billing-observer-contract` claim, WON at `6d423d9c8b5f48d0489e8ce5abb1aeb40d9930d3` and verified against the remote ref before editing. Exact proposal/source pin and claim release are recorded in the commit handoff; no other file ownership is implied.

## Independent review revision

Independent review artifact `recovery-billing-observer-contract-review-2026-10-01.md` found that several behavior choices were still prose-level and the original event-page ceiling did not define a useful account-volume envelope. The coordinator supplied an exact revision decision set. The proposal was revised while preserving the original `85d0c209367ec10792aa34712972395a4170d818` commit in history. The earlier receipt incorrectly named `R-billing-observer-contract-r1`; the current revision uses the canonical claim documented above.

The revision pins: Basil-only event snapshots; the full subscription status/result table; `stripe_recovery_observation`; separate config/account-state sentinels and existing provider errors; exact typed-ID/key bounds; item/price replacement refusal and limited quantity/proration rules; immutable event-time invoice-line proof compared against current-line GETs; `types[]` pagination/order/tie semantics; a 60-second margin before the 30-day event cutoff; and explicit limits of 128 total GETs, 32 event pages of 100, 16 invoices, 4 line pages per invoice, and 64 MiB aggregate response data (with unchanged 30-second timeout, 1 MiB response, depth 32, and 100k nodes). It also pins the wrapper constructor as no-I/O, with no production fake transport seam, and starts the full-call deadline before account-lock wait.

The episode rule uses only verified subscription active/trialing event snapshots to reset grace; an invoice payment success alone is not a reset. Complete status/failure history with no verified recovery keeps the earliest applicable failure for continuous `past_due`. A proven active/trialing transition permits a new episode anchored at the earliest verified subsequent applicable failure. Relevant same-second conflicts or incomplete association refuse. The decoder rejects unknown decision fields/discriminators but may bound and ignore nondecision extras outside the decision subtrees. A generated fixture with more than 400 unrelated matching-type events plus relevant evidence must succeed inside the limits; separate cap-exhaustion fixtures must refuse.

Ajent coordination could not be polled in this session: no Ajent MCP tool or project-root `ajent.social` file was available, and the public feed page was inaccessible. No Go source or tests were added. These revisions remain a proposal pending independent rereview; the contract is not frozen and does not authorize implementation.
