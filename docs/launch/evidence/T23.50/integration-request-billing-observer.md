# Recovery billing observation integration request

Status: design proposal only. Main baseline `6e829e0d2eb2eece398cf572e63f4b574a8e8e28`. This request follows the file-change handshake in `docs/launch/hosted-completion/interfaces.md`; it does not authorize a worker to modify shared contracts.

## Problem and proposed seam

Recovery must inspect current provider entitlement while accounts remain `restore_pending`. Existing `BillingReconciler.ReconcileCustomer` deliberately returns `ErrBillingAccountFrozen` with an empty result for that state. It also mutates checkout, subscription, billing-failure and audit rows. Calling it or temporarily activating the account cannot implement read-only recovery observation.

Propose an additive `RecoveryBillingObserver` interface in the consuming hosted contracts package, leaving existing reconciler semantics and method signatures unchanged:

```go
type RecoveryBillingObservation struct {
    AccountID string
    Eligible bool
    PlanID string
    CurrentWindowStart, CurrentWindowEnd, GraceUntil time.Time
    ObservedAt time.Time
    Source string
}

type RecoveryBillingObserver interface {
    ObserveRestorePending(ctx context.Context, accountID string) (RecoveryBillingObservation, error)
}
```

The integrator must separately review and apply the exact shared-file diff, update the frozen-interface table, and reopen dependent receipts before implementation. No migration is proposed.

## Required failure and ownership semantics

Context is first and cancellation is checked before I/O. Acquire the existing per-account keyed lock, freshly read exact `restore_pending` status and the server-stored customer binding, and conservatively refuse every unresolved checkout-attempt row. Never accept a provider customer ID from input or expose one in output. Missing/invalid customer binding or provider configuration is an error; no billing-disabled or missing-customer Free inference is permitted.

Perform only bounded provider GET requests through existing controlled provider transport. Subscription listing must prove ownership, supported shape, known configured price, supported status, and a complete bounded result; unknown price/state, duplicate nonterminal subscriptions, malformed response, timeout or pagination uncertainty fail closed. `listSubscriptions` is reusable as a read-only base validator, but its checks alone are insufficient.

Do not reuse `reconcileOldCheckoutAttempt` or `failureEvidence`: the former may expire a provider checkout and delete/write rows, and the latter may trust restored SQL evidence or insert/upsert `billing_failures`. For `past_due`, a separately factored pure provider evidence derivation must fetch and verify the current exact invoice, customer/subscription identity and bounded failure-event history, derive the first failure and its 72-hour grace without persistence, and refuse history outside the provider retention boundary. Restored grace alone is not fresh provider evidence. Validate timestamps, event identity and pagination progress. Never extend grace using local delivery time or an unverified restored row.

Active/trialing access uses the existing pure eligibility rule with valid current-window timestamps and known plan. Canceled/incomplete-expired or no current subscription can produce an explicit non-entitled Free observation only after a verified bound-customer provider result and clean checkout state. Ambiguous or incomplete data must not become successful Free observation.

After provider I/O, reread account status, exact binding and checkout state while holding the same lock; any change/refusal returns no observation. There are no SQL entitlement, subscription, checkout, failure, audit or account-status writes, and no provider POST/DELETE, cleanup or unfreeze.

`ObservedAt` is a UTC point observation with a fixed non-sensitive source tag. The process-local lock cannot serialize another service/process or provider changes. A future plan/apply operation must enforce its own freshness and re-observe before activation; this seam is not cross-process atomic truth or proof of fencing/adoption.

## Verification required before implementation can qualify

Use real local SQL stores with a fake provider only in tests. Compare every relevant table before/after both successful and failing calls. Prove provider-derived paid observation for restore_pending; refusal for active/deleting/deleted; no provider request for missing configuration/binding or pending checkout; binding/status/checkout change during blocked GET refusal; current past-due invoice/event derivation without trusting restored evidence; known terminal/no-subscription outcome; wrong price, duplicate subscriptions, unsupported status, malformed windows/events, incomplete pages, repeated cursors, expired event history and cancellation refusal. Assert all provider requests are GET and no customer identity leaks in output.

Regression guards must fail at runtime when ownership, final reread, checkout refusal, provider-evidence derivation or SQL read-only boundaries are removed. Focused race/vet/lint and the assembled full Go gate remain required after an implementation. No live Stripe qualification is claimed.

## Review provenance and open decisions

Audit-local read-only checkpoint identified the mutation hazards and process-local lock limitation; coordinator independently checked `reconcile.go`, `billing.go` and `webhook.go`. This proposal is not yet an approved/frozen contract or implementation assignment. A concrete diff and integrator review remain to be prepared. The snapshot inspector is independently assigned under contract `69a0ca4` and does not consume this proposed seam.

Provider enrollment, production factory wiring, live qualification, physical storage ceilings, complete deletion history, old-writer stop/revocation, durable generation adoption and activation gates remain open.

## Independent proposal review and exact proposed diff

Audit-local independently reviewed the proposal and requires an observer-specific strict bounded GET decoder: max+1 reads, exact single JSON document, duplicate-key rejection, required data array and has_more boolean, explicit page cardinality and event cursor progress/uniqueness. Existing Service.request and listSubscriptions do not prove those properties; keep existing reconciliation semantics unchanged.

The accompanying patch is a concrete shared-interface proposal only, not applied source or a frozen contract. T23.41 remains claimed at fc2b716 on the canonical remote; its recorded purpose is generation-bound restore fence receipts. The coordinator has neither pruned nor assumed transfer of that task claim. The source-resource handshake and exact approved implementation assignment remain required before applying the patch.

Audit-local exact-patch review of b5332f1 cleared the additive shape and requested explicit effective-access semantics. The proposal comments now pin exact restore_pending, active/trialing future provider period end, and past_due first-provider-failure plus 72 hours strictly after observation time. Known canceled/unpaid/paused/expired states are ineligible; ambiguous/incomplete states remain errors. No shared source has been changed.
