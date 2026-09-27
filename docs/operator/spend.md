# Spend: the price table, `MaxUSD`, and synthesize limits

Every judgment- and local-cheap-tier model call goes through one chokepoint,
`internal/router` (RFC 0001 §9). Since E24 T24.13 that chokepoint prices each
call in real dollars, and every USD limit layered on it -- a per-call
`Budget.MaxUSD`, the nightly eval's aggregate cap, the monthly spend ceiling --
reads that real number. Before T24.13 the providers never reported a cost, so
those limits were wired but inert (deep review 001 AI-04, lore L-0008).

## The price table

`internal/router/prices.go` is the single price table. Each row is a bare model
id (the id the provider is called with, never the `<model>@<version>` pin
string) mapped to USD per million input tokens and USD per million output
tokens, as the provider published them on `PriceTableDate`.

- **The table carries its date.** `PriceTableDate` names the day the numbers
  were taken from the providers' published list prices. A price is only as
  current as that date; bump it whenever a row changes.
- **Every model the repo itself relies on must be listed.** `prices_test.go`
  keeps a list of the model ids used by config defaults, evals, and documented
  example pins, and fails when any of them has no row -- a default model that
  is "unpriced" would trip every budget on the repo's own paths.
- **A self-hosted model is listed at `0/0`.** That is a known zero, not an
  unknown: no per-token bill exists for weights served from your own endpoint.
- **An unlisted model is priced at `+Inf`.** There is no honest finite number
  for a price nobody recorded. `+Inf` is greater than any positive `MaxUSD`, so
  the budget trips (fail closed) instead of silently passing as `$0`. The
  router logs one warning per model id per process naming the model, the table
  file, and the table date, so the fix -- add the row -- is in the log line.

Adding a row: bare model id, input and output USD per million tokens, both as
the provider publishes them on `PriceTableDate`. Do not guess a price; an
unlisted model already fails closed, which is the safe state.

OpenRouter's vendor-prefixed ids (`anthropic/claude-...`, `google/...`) are
not in the table yet. With `models.provider: openrouter` (the default adapter,
ADR 013) a pinned model therefore runs as unpriced: the call still completes,
the warning is logged once, the result's cost is `+Inf`, and any `MaxUSD` you
set trips. Pin a listed id through the `anthropic` or `openai` provider, or add
the OpenRouter row, to get a finite cost.

## What a call's cost means

`Router.Complete` prices the call itself from the provider's reported token
counts when the provider did not report a cost, so a third-party `Provider`
that only returns `InputTokens`/`OutputTokens` is budgeted the same way the
built-in Anthropic and OpenAI-compatible adapters are. The result's
`Usage.CostUSD` is the number every budget check reads: finite for a listed
model, `+Inf` for an unlisted one (`router.Unpriced` reports which).

The spend ledger row for the call (`router.SpendEntry`) is always finite. Ledger
rows are persisted as JSON (`index.SpendRow`), and JSON cannot carry `+Inf`, so
an unlisted model's row records `CostUSD: 0` with `Unpriced: true`. A ledger
that sums `CostUSD` therefore under-counts unpriced calls unless it also reads
`Unpriced`; the eval ledger below does, and `serenity status`'s month-to-date
spend line only ever adds the finite rows.

## `MaxUSD`: the per-call budget

`Budget{MaxUSD: x}` on a `Complete` call marks the result `BudgetExceeded` when
the call's priced cost is greater than `x`. The call has already happened by
then -- the router cannot know the cost before the provider answers -- so the
flag is the caller's signal to stop, and the spend is still recorded. A zero
`MaxUSD` means no per-call cap. On an unlisted model every positive `MaxUSD`
trips.

## The nightly eval cap

`.github/workflows/nightly-eval.yml` runs `cmd/eval-runner -mode live` under
`SERENITY_EVAL_BUDGET_USD` (default `2.00`). `internal/eval/runner`'s
`TrackingLedger` sums the real priced cost across the run and stops scheduling
spans once the total reaches the cap. It also counts unpriced calls separately
and reports over budget as soon as one occurs: a run whose model is not in the
price table stops after its first call rather than running the whole split at
an unknown price. The runner's default live model is a listed id, and a test
pins that.

## Synthesize limits

The MCP `synthesize` verb is one LLM completion per call, so it carries the two
limits the review asked for:

- **Rate limit: 60 calls per account per minute.** The 61st well-formed call in
  a one-minute window answers the `rate_limited` verb error (with a suggestion
  to wait for the minute to roll over, or use `recall` for lookups that need no
  cross-page reasoning). The window is fixed, one minute from the first
  admitted call. A malformed request is answered as malformed and is not
  counted; an over-limit call costs nothing -- it is refused before retrieval
  and before the composer runs. The account is the brain root the server was
  started on: one `serenity serve` process serves one brain, and one brain is
  one account.
- **Output bound on the OpenAI-compatible path.** The composer's
  OpenAI-compatible adapter (OpenRouter, OpenAI, or a local `OPENAI_BASE_URL`
  server) sends `max_tokens: 4096`, so a runaway completion is bounded on the
  provider side. The extraction adapter is deliberately left unbounded: a
  reasoning model's thinking trace was measured there
  (`docs/providers.md`, "Disabling a reasoning model's thinking pass"), and a
  cap would truncate the answer, not the trace. The Anthropic adapter already
  sends its own `max_tokens`.

`synthesize`'s `cost.usd_estimate` is the priced cost for a listed model and
`null` -- unknown, never a fabricated zero -- for an unlisted one; the token
counts are reported either way.

## Retry honours cancellation

Transient provider failures are retried with backoff. The retry loop returns
the context's error as soon as the context is done -- before the first attempt,
between attempts, and during a backoff wait -- and a cancellation surfaced by
the provider is never treated as transient. A caller that cancels does not pay
for one more attempt.
