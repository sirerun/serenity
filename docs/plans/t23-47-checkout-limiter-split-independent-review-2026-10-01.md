# T23.47 checkout limiter and billing split independent review — 2026-10-01

## Verdict

The checkout limiter implements the reviewed fixed-window policy, rejects before provider use when local preflight fails, returns typed retry metadata, and is preserved by the subsequent mechanical billing split. The focused race suite passed and a limiter-removal negative control failed the expected tests. This is process-local abuse throttling, not a durable or fleet-wide quota.

## Reviewed source

- Limiter behavior: `074bfad` (`Limit provider-capable billing checkout attempts`).
- HTTP fixture correction: `2c361e7` (`Fix checkout limiter HTTP test session`).
- Mechanical split and follow-up import/format fixes: `3193c34`, `ea744ff`, `5fbda55`.
- Final candidate checkout: `e7f77b846d4c7d1683164a00a083b79d3705c3f3`.
- Review worktree: `/Volumes/BuildOffload/worktrees/serenity-t23-47-checkout-limit-review-20261001`, branch `review/t23-47-checkout-limit-20261001`.

## Findings

`checkoutRateLimiter.charge` admits five `Checkout` invocations per account in a fixed one-minute window. The per-Service limiter map is mutex-protected and capped at 10,000 account keys. Expired entries are swept on a bounded cadence, with a forced sweep before rejecting a new key at capacity. When the cap is still full, a new account is conservatively rejected for one minute. That fixed global cap bounds memory but can temporarily deny otherwise eligible accounts if 10,000 distinct accounts are active in the same window.

`Checkout` validates the configured plan, checks for a local nonterminal subscription, verifies the account is active, and checks context cancellation before charging. Thus invalid plan, frozen/nonexistent account, existing local subscription, or cancellation observed before charging does not consume quota. A valid account with no saved provider customer is still charged before `customer()` may create one; that is a provider-capable attempt. Provider failures consume one unit. The unit is one `Checkout` invocation, which can make multiple Stripe API requests internally; it is not a per-HTTP-request provider quota.

Rate-limit errors use `*CheckoutRateLimitError`, whose `RetryAfter()` returns the remaining window duration. The HTTP handler maps that type to 429 and rounds the duration up to whole seconds for `Retry-After`; the fixture expects `Retry-After: 60` and proves a rejected sixth attempt does not make another provider request. The limiter is in-memory on each `billing.Service`: a restart or multiple service processes reset or partition the quota. It does not provide fleet-wide enforcement.

For the mechanical split, I parsed the pre-split source at `074bfad` and all final billing package Go files into ASTs, then compared formatted function declarations. All 30 original function declarations were found in the final package with identical AST output (`missing=0`, `differing=0`; final package had 86 declarations including tests and newly introduced functions). This supports behavior preservation across moving the methods into `checkout.go`, `closure.go`, `reconcile.go`, and `webhook.go`; import and formatting edits are excluded from function-body semantics.

## Validation

Focused race tests passed from the isolated final candidate checkout:

`GOCACHE=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-gocache GOTMPDIR=/Volumes/BuildOffload/tmp/t23-47-billing-lock-review-tmp go test -race -count=1 ./internal/hosted/billing -run 'TestCheckoutRateLimiter|TestCheckoutPreflightDoesNotConsumeRateLimitOrCallProvider|TestCheckoutProviderFailureConsumesAttemptAndHTTPReturnsRetryAfter'`

Result: PASS (`1.797s`). The provider fixtures use local `httptest.Server`; no live provider calls occurred.

For a negative control, I changed only the limiter's threshold check to never reject, ran the window/isolation and HTTP provider-failure tests, and observed runtime failures: the sixth account attempt was admitted and direct Checkout returned the provider 503 instead of the typed rate-limit error. I restored the original limiter and confirmed the review checkout is clean.

## Limits

The tests cover local fixed-window accounting, account isolation, bounded idle cleanup, preflight/cancellation behavior, provider-failure charging, and HTTP retry metadata. Cancellation is checked before quota charging; cancellation that races after a successful charge can still consume an attempt. The policy is process-local and fixed-window, and the key-cap fallback can cause bounded temporary denial. This review does not establish multi-process enforcement, live Stripe behavior, or complete T23.47 acceptance.
