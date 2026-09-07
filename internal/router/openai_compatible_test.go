package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAICompatibleProviderSendsChatRequestOverHTTP stands a real
// net/http test server in for an OpenAI-compatible chat-completions API
// (no real API key, no real network egress) and proves
// OpenAICompatibleProvider composes the documented request shape,
// parses the documented response shape, and omits the Authorization
// header entirely when no API key is configured -- the Ollama-class
// local-server case (RFC section 9).
func TestOpenAICompatibleProviderSendsChatRequestOverHTTP(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		var gotPath, gotAuth string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{
				"choices": [{"message": {"role": "assistant", "content": "hello from openai"}}],
				"usage": {"prompt_tokens": 5, "completion_tokens": 7}
			}`))
		}))
		defer server.Close()

		p := &OpenAICompatibleProvider{BaseURL: server.URL, APIKey: "sk-test", Model: "gpt-x", Version: "v1"}
		resp, err := p.Send(context.Background(), "hi")
		if err != nil {
			t.Fatal(err)
		}

		if gotPath != "/chat/completions" {
			t.Fatalf("request path = %q, want /chat/completions", gotPath)
		}
		if gotAuth != "Bearer sk-test" {
			t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer sk-test")
		}
		if resp.Text != "hello from openai" {
			t.Fatalf("Response.Text = %q, want %q", resp.Text, "hello from openai")
		}
		if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 7 {
			t.Fatalf("Response.Usage = %+v, want {InputTokens:5 OutputTokens:7}", resp.Usage)
		}
	})

	t.Run("without API key (Ollama-class local server)", func(t *testing.T) {
		var sawAuthHeader bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, sawAuthHeader = r.Header["Authorization"]
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{
				"choices": [{"message": {"role": "assistant", "content": "local"}}],
				"usage": {"prompt_tokens": 1, "completion_tokens": 1}
			}`))
		}))
		defer server.Close()

		p := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "llama3", Version: "local"}
		if _, err := p.Send(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		if sawAuthHeader {
			t.Fatal("Authorization header was present when APIKey was empty, want absent")
		}
	})
}

// TestOpenAICompatibleProviderExtraBody covers T1.31: ExtraBody merges
// verbatim into the request when set (the SGLang/vLLM
// chat_template_kwargs.enable_thinking case), and — the negative-space
// companion — a nil ExtraBody produces a request with no such field at
// all, so real OpenAI/OpenRouter calls (also served by this adapter) stay
// byte-identical to before this field existed.
func TestOpenAICompatibleProviderExtraBody(t *testing.T) {
	var gotBody map[string]any

	newServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody = nil
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "ok"}}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}}`))
		}))
	}

	t.Run("nil ExtraBody omits the field entirely", func(t *testing.T) {
		server := newServer()
		defer server.Close()

		p := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "m", Version: "v"}
		if _, err := p.Send(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		if _, present := gotBody["chat_template_kwargs"]; present {
			t.Fatalf("chat_template_kwargs present with nil ExtraBody: %+v", gotBody)
		}
		if gotBody["model"] != "m" {
			t.Fatalf("model = %v, want m", gotBody["model"])
		}
	})

	t.Run("ExtraBody merges verbatim", func(t *testing.T) {
		server := newServer()
		defer server.Close()

		p := &OpenAICompatibleProvider{
			BaseURL: server.URL, Model: "m", Version: "v",
			ExtraBody: map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false}},
		}
		if _, err := p.Send(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		ctk, ok := gotBody["chat_template_kwargs"].(map[string]any)
		if !ok {
			t.Fatalf("chat_template_kwargs missing or wrong shape: %+v", gotBody)
		}
		if enable, ok := ctk["enable_thinking"].(bool); !ok || enable {
			t.Fatalf("chat_template_kwargs.enable_thinking = %v, want false", ctk["enable_thinking"])
		}
		if gotBody["model"] != "m" {
			t.Fatalf("model = %v, want m (ExtraBody must not clobber the base fields)", gotBody["model"])
		}
	})
}
