# Independent rereview: recovery billing observer contract

Reviewed exact revision: `66e431b24481456e4a3d86dcbc0ee2c3c4609d7d` (40-character commit SHA), parent `85d0c209367ec10792aa34712972395a4170d818`.

Required source baseline: `c3d491b03980310d7d59688d092b8119640e5250` (40-character commit SHA). The author checkout reported that exact revision at `HEAD`, with a clean worktree; its branch is two commits ahead of `origin/main`. Review was read-only. No source, tests, Git refs, provider account, or credentials were changed or accessed. Ajent was not available in this checkout (`ajent.social` is absent); the companion receipt records the prior coordination limitation as well.

## Verdict

**No remaining contract-level blocker to freezing this bounded observer contract and beginning the separate source implementation.** The revision answers the prior review's material decisions: full subscription status table, Basil-only event snapshots, stable source/errors, reset and item/proration semantics, exact pagination/order behavior, retention margin, response parser policy, account-volume envelope, and numeric budgets. Its central safety properties remain implementable: server-derived account/customer binding, exact `restore_pending` precondition, unresolved-checkout refusal, same-process lock, final state reread, GET-only private transport, SELECT-only SQL, zero result on errors, and no reuse of write-capable reconciliation.

This is a contract-design verdict only. It does not establish runtime correctness, real-account shape coverage, live completeness, recovery authority, or activation. The requested fixtures and tests are explicitly future implementation qualification and are not evidence already obtained.

## Material blockers

None found that requires another proposal revision before source work. The following are intentionally fail-closed support limits and should remain visible in implementation/review claims:

- Only exact `2025-06-30.basil` event snapshots are accepted. Accounts whose relevant retained event snapshots use another or null API version will get an ambiguous observation, even though direct GETs request Basil. This is a clear unsupported-history outcome, not a decoder fallback gap.
- The API exposes only 30 days of events. Refusing periods within 60 seconds of the retrieval cutoff, querying from the exact period start, and never clipping to a retained suffix avoids claiming incomplete history. This necessarily makes some otherwise ordinary long-running past-due periods unobservable.
- Event listing is account-wide. The 32-page/3,200-event cap and 128-total-GET cap support the specified envelope; the required generated case with more than 400 unrelated events demonstrates the intended >400 case, while higher account volume can still exhaust the cap and refuse. This is acceptable if exhaustion never becomes Free or Eligible.
- Immutable invoice-line evidence is required from the payment-failed event snapshot, with current invoice/line GETs used only for exact consistency checks. Snapshot truncation, later differences, or nonstandard line associations refuse. Do not soften this to a current mutable lookup.
- Only a validated active/trialing event-time subscription snapshot resets an episode. A lone invoice-payment-succeeded event does not. The earliest applicable failure remains the anchor absent a proven reset; same-second conflicting transitions refuse.

## Remaining implementation qualifications

Before claiming the method is implemented, qualify real Basil-shaped fixtures for each accepted subscription/event/invoice/line shape, including nullable and harmless optional fields. The method has several uncommon but normal account shapes that must safely refuse under the pinned envelope: accounts configured to emit different event API versions; invoices with truncated embedded lines; item/price replacement; proration without complete base-window proof; old periods near the retention cutoff; and histories above configured budgets. These refusals are expected unsupported cases, not reasons to weaken the observer's evidence rules.

The implementation must also demonstrate strict duplicate-key/case-alias/type/required-field parsing, bounded unknown extras outside decision inputs, all-status mappings, complete pagination and cursor progression, no redirect/proxy/retry leakage, deadline coverage including lock wait, final local reread, and unchanged relevant SQL state on both success and error. The contract already calls these out as future focused verification; they are not yet proven by this docs-only review. Any fixture set generated later must be described as implementation qualification, not live Stripe qualification.

One reviewer-facing caution: “exact agreement on every event-time decision field” should be implemented as a finite named projection of the decision fields and line identity/association/period, not byte equality of entire invoice objects. Stripe documents many mutable/nondecision invoice and line attributes. Reject changes to decision inputs; allow only bounded harmless fields outside those inputs per the contract's parser rule. This is a source-implementation interpretation of the existing contract, not a reason to reopen it.

## Evidence reviewed

Read the revised development contract, its proposal receipt, and prior independent review. Inspected the baseline readiness/integration packet references and relevant existing billing source declarations/helpers at the specified checkout. Verified provider-shape references against official Stripe documentation for event versioning/list filters and Basil line-item parent/pricing/period fields. Those documents support the pinned shape facts but cannot prove every production account has usable history.
