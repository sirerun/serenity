package router_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/router"
)

// These tests live in the external router_test package so the Embed path
// can be driven through internal/embed.RouterEmbedder (which imports
// router) without an import cycle.

// recordingProvider is a test double implementing router.Provider that
// keeps every prompt it was handed. Its canned response is a JSON vector
// so the same double serves both Complete and RouterEmbedder.Embed.
// Test-file only, per the zero-stub policy.
type recordingProvider struct {
	name         string
	modelVersion string
	sent         []string
}

func (p *recordingProvider) Name() string         { return p.name }
func (p *recordingProvider) ModelVersion() string { return p.modelVersion }
func (p *recordingProvider) Send(_ context.Context, prompt string) (router.Response, error) {
	p.sent = append(p.sent, prompt)
	return router.Response{Text: "[0.5,-0.25,1]"}, nil
}

// discardLedger is a test double implementing router.SpendLedger.
type discardLedger struct{}

func (discardLedger) Record(context.Context, router.SpendEntry) error { return nil }

// Synthetic, obviously fake key values (deep review 001, AI-02). Built
// from a TEST marker plus a zero run so no secret scanner fires.
var (
	fakeAnthropicKey  = "sk-ant-api03-TEST" + strings.Repeat("0", 74) + "AA"
	fakeOpenRouterKey = "sk-or-v1-" + strings.Repeat("0", 64)
	fakeStripeKey     = "sk_live_TEST" + strings.Repeat("0", 24)
	fakeAWSKey        = "AKIATEST" + strings.Repeat("0", 12)
	fakeCardNumber    = "4111 1111 1111 1111"

	sensitivePrompt = "Rotate " + fakeAnthropicKey + ", " + fakeOpenRouterKey + ", " +
		fakeStripeKey + " and " + fakeAWSKey + "; card on file " + fakeCardNumber + "."

	mustNotLeak = []string{fakeAnthropicKey, fakeOpenRouterKey, fakeStripeKey, fakeAWSKey, fakeCardNumber}
)

// assertRedacted fails when any sensitive value survives in text or the
// expected placeholders are missing.
func assertRedacted(t *testing.T, label, text string) {
	t.Helper()
	for _, v := range mustNotLeak {
		if strings.Contains(text, v) {
			t.Fatalf("%s: text reaching the provider still carries %q:\n%s", label, v, text)
		}
	}
	if n := strings.Count(text, "[REDACTED:API_KEY]"); n != 4 {
		t.Fatalf("%s: want 4 API_KEY placeholders, got %d:\n%s", label, n, text)
	}
	if !strings.Contains(text, "[REDACTED:CARD_NUMBER]") {
		t.Fatalf("%s: card number placeholder missing:\n%s", label, text)
	}
}

// TestCompleteRedactsEveryTaskClassBeforeTheProvider proves the router is
// the chokepoint (ADR 021): whatever the task class and tier, and
// whether the call arrives as Complete or as RouterEmbedder.Embed, the
// provider receives redacted text. No caller has to redact.
func TestCompleteRedactsEveryTaskClassBeforeTheProvider(t *testing.T) {
	classes := []router.TaskClass{
		router.TaskClassEmbedding, router.TaskClassTranscription, router.TaskClassClassification,
		router.TaskClassExtractionCandidates, router.TaskClassSummarization, router.TaskClassConsolidation,
		router.TaskClassDecompositionProposals, router.TaskClassHardConflictReconciliation,
		router.TaskClassPlanVsPreceptAnalysis, router.TaskClassComposerSynthesis,
	}
	for _, tc := range classes {
		t.Run("Complete/"+string(tc), func(t *testing.T) {
			p := &recordingProvider{name: "recording", modelVersion: "rec@v1"}
			r := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p, router.TierJudgment: p}, discardLedger{})
			if _, err := r.Complete(context.Background(), tc, router.Prompt{Text: sensitivePrompt}, router.Budget{}); err != nil {
				t.Fatalf("Complete(%s): %v", tc, err)
			}
			if len(p.sent) != 1 {
				t.Fatalf("provider received %d prompts, want 1", len(p.sent))
			}
			assertRedacted(t, string(tc), p.sent[0])
		})
	}

	t.Run("Embed/RouterEmbedder", func(t *testing.T) {
		p := &recordingProvider{name: "recording", modelVersion: "rec@v1"}
		r := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p}, discardLedger{})
		e := &embed.RouterEmbedder{Router: r, Pin: "rec@v1"}
		vec, err := e.Embed(context.Background(), sensitivePrompt)
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if len(vec) != 3 {
			t.Fatalf("Embed returned %d dims, want 3", len(vec))
		}
		if len(p.sent) != 1 {
			t.Fatalf("provider received %d inputs, want 1", len(p.sent))
		}
		assertRedacted(t, "embed", p.sent[0])
	})
}

// wireRecorder is a real net/http test server standing in for the
// Anthropic Messages API, an OpenAI-compatible chat-completions endpoint
// (OpenAI itself and OpenRouter differ only in base URL and routing
// fields) and an OpenAI-shaped /embeddings endpoint. It records every
// raw request body so the assertion is on the exact bytes that would
// have left the machine, not on an adapter's own view of them.
type wireRecorder struct {
	*httptest.Server
	bodies []string
}

func newWireRecorder(t *testing.T) *wireRecorder {
	t.Helper()
	w := &wireRecorder{}
	w.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("server: read body: %v", err)
		}
		w.bodies = append(w.bodies, string(body))
		rw.Header().Set("content-type", "application/json")
		switch {
		case r.URL.Path == "/v1/messages":
			_, _ = rw.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = rw.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
		case strings.HasSuffix(r.URL.Path, "/embeddings"):
			_, _ = rw.Write([]byte(`{"data":[{"embedding":[0.5,-0.25,1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
		default:
			t.Errorf("server: unexpected path %q", r.URL.Path)
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(w.Close)
	return w
}

// lastBody returns the single recorded request body, failing when the
// count is not exactly one.
func (w *wireRecorder) lastBody(t *testing.T) string {
	t.Helper()
	if len(w.bodies) != 1 {
		t.Fatalf("server recorded %d request bodies, want 1", len(w.bodies))
	}
	return w.bodies[0]
}

// promptText extracts the user-visible text from a recorded body:
// messages[0].content for a chat-shaped request, input for embeddings.
func promptText(t *testing.T, body string) string {
	t.Helper()
	var parsed struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("decode recorded body: %v\n%s", err, body)
	}
	if len(parsed.Messages) > 0 {
		return parsed.Messages[0].Content
	}
	return parsed.Input
}

// TestRedactionReachesEveryRealAdapterOnTheWire proves, for each real
// provider adapter in this package, that the request body built for the
// wire carries the redacted prompt (Complete) or embedding input (Embed).
// Anthropic has no embeddings endpoint, so its Embed row does not exist;
// OpenRouter embeddings use the same OpenAI-shaped adapter the hosted
// service configures (ZDR, data_collection deny, provider pin).
func TestRedactionReachesEveryRealAdapterOnTheWire(t *testing.T) {
	complete := func(t *testing.T, p router.Provider) {
		t.Helper()
		r := router.New(map[router.Tier]router.Provider{router.TierJudgment: p}, discardLedger{})
		if _, err := r.Complete(context.Background(), router.TaskClassComposerSynthesis, router.Prompt{Text: sensitivePrompt}, router.Budget{}); err != nil {
			t.Fatalf("Complete via %s: %v", p.Name(), err)
		}
	}
	embedVia := func(t *testing.T, p router.Provider) {
		t.Helper()
		r := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p}, discardLedger{})
		e := &embed.RouterEmbedder{Router: r, Pin: p.ModelVersion()}
		if _, err := e.Embed(context.Background(), sensitivePrompt); err != nil {
			t.Fatalf("Embed via %s: %v", p.Name(), err)
		}
	}

	t.Run("anthropic Complete", func(t *testing.T) {
		w := newWireRecorder(t)
		complete(t, &router.AnthropicProvider{BaseURL: w.URL, APIKey: "test-key", Model: "claude-test", Version: "v1"})
		body := w.lastBody(t)
		assertRedacted(t, "anthropic wire body", body)
		assertRedacted(t, "anthropic messages[0].content", promptText(t, body))
	})

	t.Run("openai-compatible Complete", func(t *testing.T) {
		w := newWireRecorder(t)
		complete(t, &router.OpenAICompatibleProvider{BaseURL: w.URL + "/v1", APIKey: "test-key", Model: "gpt-test", Version: "v1"})
		body := w.lastBody(t)
		assertRedacted(t, "openai-compatible wire body", body)
		assertRedacted(t, "openai-compatible messages[0].content", promptText(t, body))
	})

	t.Run("openrouter Complete", func(t *testing.T) {
		w := newWireRecorder(t)
		complete(t, &router.OpenAICompatibleProvider{BaseURL: w.URL + "/api/v1", APIKey: "test-key", Model: "vendor/model-test", Version: "v1"})
		body := w.lastBody(t)
		assertRedacted(t, "openrouter wire body", body)
		assertRedacted(t, "openrouter messages[0].content", promptText(t, body))
	})

	t.Run("openai-compatible Embed", func(t *testing.T) {
		w := newWireRecorder(t)
		embedVia(t, &router.OpenAIEmbeddingsProvider{BaseURL: w.URL + "/v1", APIKey: "test-key", Model: "text-embedding-test", Version: "v1"})
		body := w.lastBody(t)
		assertRedacted(t, "openai embeddings wire body", body)
		assertRedacted(t, "openai embeddings input", promptText(t, body))
	})

	t.Run("openrouter Embed", func(t *testing.T) {
		w := newWireRecorder(t)
		embedVia(t, &router.OpenAIEmbeddingsProvider{
			BaseURL: w.URL + "/api/v1", APIKey: "test-key", Model: "perplexity/pplx-embed-test", Version: "v1",
			ZDR: true, DataCollection: "deny", ProviderOnly: []string{"Perplexity"},
		})
		body := w.lastBody(t)
		assertRedacted(t, "openrouter embeddings wire body", body)
		assertRedacted(t, "openrouter embeddings input", promptText(t, body))
	})
}
