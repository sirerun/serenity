package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/router"
)

// noopLedger is a router.SpendLedger test double that records nothing --
// these tests are about the request the composer sends and the answer
// synthesize shapes, not the spend ledger. Test-file only, per the
// zero-stub policy.
type noopLedger struct{}

func (noopLedger) Record(context.Context, router.SpendEntry) error { return nil }

// TestSynthesizeRateLimitedPerAccountPerMinute: the 61st well-formed
// synthesize call within one minute for one account returns
// error "rate_limited" (AI-04: the local synthesize path had no rate limit
// at all); once the window rolls over the account is served again. No
// composer is configured here on purpose -- the limiter must sit ahead of
// the expensive composer work, so each of the 60 admitted calls is the
// cheap "unavailable" answer, and the 61st never reaches that far.
func TestSynthesizeRateLimitedPerAccountPerMinute(t *testing.T) {
	h, _ := newTestHandlers(t)
	h.deps.Clock = fixedClock{testNow}

	for i := 1; i <= synthesizeCallsPerMinute; i++ {
		got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "anything"})
		if got["error"] != ErrCodeUnavailable {
			t.Fatalf("call %d: error = %v, want %q (admitted, no composer configured)", i, got["error"], ErrCodeUnavailable)
		}
	}

	got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "anything"})
	if got["error"] != ErrCodeRateLimited {
		t.Fatalf("call %d: error = %v, want %q", synthesizeCallsPerMinute+1, got["error"], ErrCodeRateLimited)
	}
	if msg, _ := got["message"].(string); !strings.Contains(msg, "60") {
		t.Fatalf("rate_limited message does not state the limit: %v", got)
	}

	// A malformed request is rejected before it is counted, and is not
	// what the limiter answers with.
	got = acceptanceCall(t, h, "synthesize", map[string]any{"question": ""})
	if got["error"] != ErrCodeInvalidParams {
		t.Fatalf("empty question while rate limited: error = %v, want %q (validation runs first)", got["error"], ErrCodeInvalidParams)
	}

	// The window is one minute: at +61s the account is served again.
	h.deps.Clock = fixedClock{testNow.Add(61 * time.Second)}
	got = acceptanceCall(t, h, "synthesize", map[string]any{"question": "anything"})
	if got["error"] != ErrCodeUnavailable {
		t.Fatalf("after the window rolled over: error = %v, want %q (admitted again)", got["error"], ErrCodeUnavailable)
	}
}

// TestSynthesizeOpenAICompatiblePathSetsMaxTokens wires the real composer
// path -- providers.BuildComposerRouter over an OpenAI-compatible endpoint
// (a net/http test server standing in for it) -- into synthesize and
// proves the chat-completions request carries max_tokens (AI-04: the
// OpenAI-compatible path previously sent no output bound at all). It also
// checks the priced cost the adapter now reports flows out as
// usd_estimate.
func TestSynthesizeOpenAICompatiblePathSetsMaxTokens(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = nil
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Local answer."}}],"usage":{"prompt_tokens":1000,"completion_tokens":500}}`))
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	h, root := newTestHandlers(t)
	acceptanceCall(t, h, "remember", map[string]any{"fact": "maxtokens fixture fact", "provenance": "limits review"})
	if err := index.Rebuild(t.Context(), root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}

	cfg := *h.deps.Config
	cfg.Models.Provider = "openai"
	cfg.Models.Composer = "qwen3.8-27b@v1" // self-hosted, priced at $0 in the price table
	rt, ok, note := providers.BuildComposerRouter(&cfg, &noopLedger{})
	if !ok {
		t.Fatalf("BuildComposerRouter: %s", note)
	}
	h.deps.Composer = rt
	h.deps.ComposerModelVersion = cfg.Models.Composer

	got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "maxtokens"})
	if got["error"] != nil {
		t.Fatalf("synthesize failed: %v", got)
	}
	if gotBody == nil {
		t.Fatal("the composer never reached the OpenAI-compatible endpoint")
	}
	mt, present := gotBody["max_tokens"].(float64)
	if !present || mt <= 0 {
		t.Fatalf("chat-completions request from synthesize carries max_tokens = %v (present=%v), want a positive bound", gotBody["max_tokens"], present)
	}
	cost, _ := got["cost"].(map[string]any)
	if cost["usd_estimate"] != float64(0) {
		t.Fatalf("usd_estimate = %v, want 0 (self-hosted model priced at $0, a real number, not null)", cost["usd_estimate"])
	}
}

// TestSynthesizeUnpricedModelReportsNullUSDEstimate: an unlisted model's
// cost is +Inf inside the router (fail closed against MaxUSD); the JSON
// answer cannot carry +Inf, so usd_estimate is null -- "unknown", never a
// fabricated zero -- while the token counts are still reported.
func TestSynthesizeUnpricedModelReportsNullUSDEstimate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Local answer."}}],"usage":{"prompt_tokens":1000,"completion_tokens":500}}`))
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	h, root := newTestHandlers(t)
	acceptanceCall(t, h, "remember", map[string]any{"fact": "unpriced fixture fact", "provenance": "limits review"})
	if err := index.Rebuild(t.Context(), root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}

	cfg := *h.deps.Config
	cfg.Models.Provider = "openai"
	cfg.Models.Composer = "model-nobody-priced@v1"
	rt, ok, note := providers.BuildComposerRouter(&cfg, &noopLedger{})
	if !ok {
		t.Fatalf("BuildComposerRouter: %s", note)
	}
	h.deps.Composer = rt

	got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "unpriced"})
	if got["error"] != nil {
		t.Fatalf("synthesize failed: %v", got)
	}
	cost, _ := got["cost"].(map[string]any)
	if cost["usd_estimate"] != nil {
		t.Fatalf("usd_estimate = %v for an unpriced model, want null", cost["usd_estimate"])
	}
	if cost["input_tokens"] != float64(1000) || cost["output_tokens"] != float64(500) {
		t.Fatalf("token counts not reported for an unpriced model: %v", cost)
	}
}
