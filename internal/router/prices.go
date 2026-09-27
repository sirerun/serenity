package router

import (
	"math"
	"strings"
)

// PriceTableDate is the date the USD prices below were taken from the
// providers' published list prices (Anthropic first-party API rates). A
// price is only as current as this date: bump it whenever a row changes.
// Operators who need a newer or provider-specific rate (a reseller, a
// negotiated contract) edit this table; there is no runtime override,
// so the number a budget trips on is always one a reviewer can read
// here.
const PriceTableDate = "2026-06-24"

// ModelPrice is one model's list price in USD per million tokens, input
// and output priced separately (the shape every hosted provider bills
// in). A self-hosted model is listed at 0/0 -- a real, known zero, which
// is different from an UNLISTED model (see CostUSD).
type ModelPrice struct {
	InputUSDPerMillion  float64
	OutputUSDPerMillion float64
}

// prices is the per-model price table, keyed by the bare model id the
// provider is called with (AnthropicProvider.Model /
// OpenAICompatibleProvider.Model -- never the "<model>@<version>" pin
// string; modelFromVersion strips the pin tag). Every model id a config
// default or an eval entry point relies on MUST be here: prices_test.go's
// defaultModelIDs list fails the build otherwise, because a default model
// that is "unpriced" would trip every MaxUSD on the repo's own paths.
//
// Adding a row: bare model id -> {input, output} USD per million tokens,
// both as the provider publishes them on PriceTableDate. Do not guess a
// price; an unlisted model already fails closed.
var prices = map[string]ModelPrice{
	// Anthropic first-party API list prices, PriceTableDate. The dated
	// id is the pinned snapshot cmd/eval-runner runs by default; the
	// undated id is the same model addressed by its alias.
	"claude-haiku-4-5-20251001": {InputUSDPerMillion: 1.00, OutputUSDPerMillion: 5.00},
	"claude-haiku-4-5":          {InputUSDPerMillion: 1.00, OutputUSDPerMillion: 5.00},
	"claude-sonnet-4-6":         {InputUSDPerMillion: 3.00, OutputUSDPerMillion: 15.00},
	"claude-sonnet-5":           {InputUSDPerMillion: 2.00, OutputUSDPerMillion: 10.00},
	"claude-opus-4-6":           {InputUSDPerMillion: 5.00, OutputUSDPerMillion: 25.00},
	"claude-opus-4-7":           {InputUSDPerMillion: 5.00, OutputUSDPerMillion: 25.00},
	"claude-opus-4-8":           {InputUSDPerMillion: 5.00, OutputUSDPerMillion: 25.00},
	"claude-opus-5":             {InputUSDPerMillion: 5.00, OutputUSDPerMillion: 25.00},

	// Self-hosted: docs/evals/m1-report.md's extraction/composer model,
	// served from the operator's own SGLang endpoint via OPENAI_BASE_URL.
	// No per-token bill exists for it, so its known cost is $0. A hosted
	// reseller of the same weights uses a vendor-prefixed id (e.g.
	// "qwen/..."), which is NOT this row and stays unlisted -> +Inf.
	"qwen3.8-27b": {InputUSDPerMillion: 0, OutputUSDPerMillion: 0},
}

// PriceFor returns model's list price. ok is false for a model the table
// does not list; callers that need a cost use CostUSD, which turns that
// into the fail-closed +Inf.
func PriceFor(model string) (ModelPrice, bool) {
	p, ok := prices[model]
	return p, ok
}

// CostUSD prices one call: inputTokens and outputTokens at model's list
// price. An UNLISTED model costs +Inf -- there is no honest finite number
// for a price nobody recorded, and +Inf is the one value that trips any
// positive Budget.MaxUSD (fail closed, AI-04) rather than silently
// passing as $0 (lore L-0008). A listed model with zero tokens is a real
// $0.
func CostUSD(model string, inputTokens, outputTokens int) float64 {
	p, ok := prices[model]
	if !ok {
		return math.Inf(1)
	}
	return float64(inputTokens)*p.InputUSDPerMillion/1e6 + float64(outputTokens)*p.OutputUSDPerMillion/1e6
}

// Unpriced reports whether cost is the +Inf CostUSD returns for an
// unlisted model. Spend-ledger implementations that persist rows as JSON
// (index.SpendRow) cannot encode +Inf, which is why Router.Complete
// records such a call with SpendEntry.Unpriced = true and a finite
// CostUSD of 0 instead -- the +Inf lives in Result.Usage.CostUSD, where
// the budget check reads it.
func Unpriced(cost float64) bool {
	return math.IsInf(cost, 1)
}

// modelFromVersion strips the "@<version>" pin tag from a
// Provider.ModelVersion() string, yielding the bare model id the price
// table is keyed on. A string with no "@" is returned unchanged.
func modelFromVersion(modelVersion string) string {
	if i := strings.LastIndex(modelVersion, "@"); i >= 0 {
		return modelVersion[:i]
	}
	return modelVersion
}
