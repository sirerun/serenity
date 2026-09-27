package router

import (
	"math"
	"testing"
	"time"
)

// defaultModelIDs is every model id a config default, an eval entry point,
// or a shipped eval report relies on. Each must be priced in prices.go so
// the MaxUSD ceiling is a real number for the paths the repo itself runs
// -- an id missing here would silently make every call on that path
// "unpriced" (+Inf, see TestCostUSDUnlistedModelIsInfinite), which is the
// fail-closed contract for unknown models, but wrong for a model the repo
// picks by default. Keep the source citation next to each id.
var defaultModelIDs = []string{
	"claude-haiku-4-5-20251001", // cmd/eval-runner -model default (nightly-eval.yml live run)
	"claude-haiku-4-5",          // AnthropicProvider.Model doc example, undated alias of the above
	"qwen3.8-27b",               // docs/evals/m1-report.md: self-hosted SGLang extraction/composer model
	"claude-sonnet-4-5",         // docs/providers.md: the documented example pin for models.provider: anthropic
}

func TestPriceTableCoversDefaultModels(t *testing.T) {
	for _, id := range defaultModelIDs {
		if _, ok := PriceFor(id); !ok {
			t.Errorf("default model %q is missing from the price table (prices.go); every default must be priced so MaxUSD is real on the repo's own paths", id)
		}
	}
}

func TestPriceTableDateIsADate(t *testing.T) {
	if _, err := time.Parse("2006-01-02", PriceTableDate); err != nil {
		t.Fatalf("PriceTableDate %q is not a YYYY-MM-DD date: %v", PriceTableDate, err)
	}
}

func TestPriceTableEntriesAreNonNegativeFinite(t *testing.T) {
	for id, p := range prices {
		if p.InputUSDPerMillion < 0 || p.OutputUSDPerMillion < 0 {
			t.Errorf("%s: negative price %+v", id, p)
		}
		if math.IsInf(p.InputUSDPerMillion, 0) || math.IsInf(p.OutputUSDPerMillion, 0) || math.IsNaN(p.InputUSDPerMillion) || math.IsNaN(p.OutputUSDPerMillion) {
			t.Errorf("%s: non-finite price %+v -- an unlisted model is the only thing that may cost +Inf", id, p)
		}
	}
}

func TestCostUSDFromPriceTable(t *testing.T) {
	// claude-haiku-4-5: $1.00 per million input tokens, $5.00 per million
	// output tokens (Anthropic list price as of PriceTableDate).
	if got, want := CostUSD("claude-haiku-4-5-20251001", 1_000_000, 1_000_000), 6.00; math.Abs(got-want) > 1e-9 {
		t.Fatalf("CostUSD(haiku, 1M in, 1M out) = %v, want %v", got, want)
	}
	if got, want := CostUSD("claude-haiku-4-5-20251001", 1000, 500), 0.0035; math.Abs(got-want) > 1e-9 {
		t.Fatalf("CostUSD(haiku, 1000 in, 500 out) = %v, want %v", got, want)
	}
	// A self-hosted model is priced at zero, not treated as unknown.
	if got := CostUSD("qwen3.8-27b", 1_000_000, 1_000_000); got != 0 {
		t.Fatalf("CostUSD(qwen3.8-27b self-hosted) = %v, want 0", got)
	}
	// Zero tokens on a priced model is a real $0, never +Inf.
	if got := CostUSD("claude-haiku-4-5-20251001", 0, 0); got != 0 {
		t.Fatalf("CostUSD(haiku, 0, 0) = %v, want 0", got)
	}
}

func TestCostUSDUnlistedModelIsInfinite(t *testing.T) {
	got := CostUSD("model-nobody-priced", 1, 1)
	if !math.IsInf(got, 1) {
		t.Fatalf("CostUSD(unlisted) = %v, want +Inf (fail closed: an unknown price must trip any MaxUSD)", got)
	}
	if !Unpriced(got) {
		t.Fatal("Unpriced(+Inf) = false, want true")
	}
	if Unpriced(0.5) {
		t.Fatal("Unpriced(0.5) = true, want false")
	}
}

func TestModelFromVersionStripsPinTag(t *testing.T) {
	cases := map[string]string{
		"claude-haiku-4-5-20251001@v1": "claude-haiku-4-5-20251001",
		"qwen3.8-27b@local":            "qwen3.8-27b",
		"bare-model":                   "bare-model",
		"":                             "",
	}
	for in, want := range cases {
		if got := modelFromVersion(in); got != want {
			t.Errorf("modelFromVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
