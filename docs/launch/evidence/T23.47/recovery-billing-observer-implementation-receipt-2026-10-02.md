# T23.47 recovery billing observer implementation receipt

Status: corrected implementation candidate awaiting independent rereview; original candidate `1cbf6013c39a09bb58b959771e1de1196d76adf8` was held. This revision is not merged, activated, or production-qualified.

This work starts from Serenity `main` baseline `c3d491b03980310d7d59688d092b8119640e5250` and implements the frozen observer contract `66e431b24481456e4a3d86dcbc0ee2c3c4609d7d`, as clarified by root decision commit `47f3242` and its independent rereview in the completion-progress checkout. It adds only the new observer implementation, its focused package tests, and this receipt.

The new `internal/hosted/billing/recovery_observer.go` and `recovery_provider.go` implement a bounded, GET-only Stripe observer and local SELECT-only preflight/final recheck. The constructor snapshots configuration and builds a private fixed-host transport. Calls use a deadline that includes account-lock wait, require `restore_pending` and a bound customer without a pending checkout, return zero observations on errors, and re-read local state before returning success. Provider parsing pins `2025-06-30.basil`, bounds response and aggregate bytes, JSON depth/nodes, GETs, event pages, invoices, and invoice-line pages, and refuses malformed or ambiguous decision evidence. Event history is paginated account-wide and failures are selected from the earliest applicable episode failure after proven reset evidence. Invoice event-time line evidence is compared against a finite decision projection from current invoice reads; current reads do not fill missing event-time proof. Recognized schema-field aliases and invalid UTF-8 or unpaired Unicode escapes fail closed, while bounded unrelated provider fields are ignored.

`internal/hosted/billing/recovery_observer_test.go` exercises the real SQLite store and observer with a local HTTP test transport. It covers the constructor and limits, private GET behavior, read-only database state, lock-inclusive deadline, checkout preflight, final reread, strict JSON and Unicode, schema aliases, status mapping, 407 account-wide event history with five pages and earliest-failure selection, reset episodes, request and response budgets, redirects, and in-flight cancellation.

The independent review found that the original resolver discarded selected-subscription non-reset status transitions, allowing conflicting `past_due→active` and `active→past_due` updates in the same second to be ordered by event ID while a later failure supplied a grace anchor. The correction preserves validated current and previous statuses for every selected-subscription update, refuses multiple distinct transitions in one second, and refuses an applicable failure sharing a second with a selected status transition. The permanent public HTTP regression brackets the conflicting updates with event-time invoice failures and asserts a zero observation with `ErrBillingProviderAmbiguous`; it was observed failing before the correction with a populated eligible observation.

The redirect control now counts private-client RoundTripper invocations and requires exactly one; it therefore detects removal of the redirect callback. The response-byte control uses a complete JSON object followed by valid whitespace beyond the configured cap, so it detects byte-check removal independently of JSON syntax rejection.

Initial pre-review candidate validation (commit `1cbf6013c39a09bb58b959771e1de1196d76adf8`) reported `go test -race ./internal/hosted/billing -count=1` at `11.072s`. The independent reviewer held that candidate because of same-second transition ambiguity and test-control gaps.

Corrected-candidate validation on 2026-10-02, with SSD Go caches and load below the repository's limit:

- `go test -race ./internal/hosted/billing -count=1` — passed (`9.740s`; load 2.94).
- `go vet ./internal/hosted/billing` — passed.
- `golangci-lint run ./internal/hosted/billing` — passed with 0 issues.
- Focused past-due episode tests — passed, including the preserved reset-to-next-failure positive control, conflicting same-second transitions, failure tied to a non-reset transition, and duplicate identical transition acceptance.
- Deliberate source mutants each failed at their intended regression assertion and were restored byte-for-byte: removing the status-transition conflict guard made the same-second public HTTP case return a populated eligible observation; removing per-response byte enforcement made valid JSON plus oversized whitespace get accepted; setting `CheckRedirect` to nil followed the redirect and returned the synthetic target's success response.

The captured final test/vet/lint transcript is `/Volumes/BuildOffload/serenity-recovery-observer-validation-20261002.log` (SHA-256 `5493e2308c4c8eae22691a128e3e3d7a47710ae851ef58d7051df469e325979f`).

These checks validate the focused package only. The corrected candidate still requires independent source rereview and broader integration gates. No live Stripe request, credential use, deployment, merge, or source-acceptance claim was made.
