# T23.47 recovery billing observer implementation receipt

Status: implementation candidate for independent review; not merged, activated, or production-qualified.

This work starts from Serenity `main` baseline `c3d491b03980310d7d59688d092b8119640e5250` and implements the frozen observer contract `66e431b24481456e4a3d86dcbc0ee2c3c4609d7d`, as clarified by root decision commit `47f3242` and its independent rereview in the completion-progress checkout. It adds only the new observer implementation, its focused package tests, and this receipt.

The new `internal/hosted/billing/recovery_observer.go` and `recovery_provider.go` implement a bounded, GET-only Stripe observer and local SELECT-only preflight/final recheck. The constructor snapshots configuration and builds a private fixed-host transport. Calls use a deadline that includes account-lock wait, require `restore_pending` and a bound customer without a pending checkout, return zero observations on errors, and re-read local state before returning success. Provider parsing pins `2025-06-30.basil`, bounds response and aggregate bytes, JSON depth/nodes, GETs, event pages, invoices, and invoice-line pages, and refuses malformed or ambiguous decision evidence. Event history is paginated account-wide and failures are selected from the earliest applicable episode failure after proven reset evidence. Invoice event-time line evidence is compared against a finite decision projection from current invoice reads; current reads do not fill missing event-time proof. Recognized schema-field aliases and invalid UTF-8 or unpaired Unicode escapes fail closed, while bounded unrelated provider fields are ignored.

`internal/hosted/billing/recovery_observer_test.go` exercises the real SQLite store and observer with a local HTTP test transport. It covers the constructor and limits, private GET behavior, read-only database state, lock-inclusive deadline, checkout preflight, final reread, strict JSON and Unicode, schema aliases, status mapping, 407 account-wide event history with five pages and earliest-failure selection, reset episodes, request and response budgets, redirects, and in-flight cancellation.

Validation on 2026-10-02, with SSD Go caches and load below the repository's limit:

- `go test -race ./internal/hosted/billing -count=1` — passed (`11.072s`).
- `go vet ./internal/hosted/billing` — passed.
- `golangci-lint run ./internal/hosted/billing` — passed with 0 issues.

These checks validate the focused package only. Independent source review and the repository's broader integration gates remain outstanding. No live Stripe request, credential use, deployment, merge, or source-acceptance claim was made.
