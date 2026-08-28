package router

import (
	"context"
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
