package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAnthropicProviderSendsMessagesRequestOverHTTP stands a real
// net/http test server in for the Anthropic API (no real API key, no
// real network egress) and proves AnthropicProvider composes the
// documented request shape and parses the documented response shape.
func TestAnthropicProviderSendsMessagesRequestOverHTTP(t *testing.T) {
	var gotPath, gotAPIKey, gotAPIVersion, gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotAPIVersion = r.Header.Get("anthropic-version")

		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("server: decode request body: %v", err)
		}
		gotModel = body.Model

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [{"type": "text", "text": "hello from anthropic"}],
			"usage": {"input_tokens": 12, "output_tokens": 34}
		}`))
	}))
	defer server.Close()

	p := &AnthropicProvider{
		BaseURL: server.URL,
		APIKey:  "test-key-123",
		Model:   "claude-haiku-4-5-20251001",
		Version: "20251001",
	}

	resp, err := p.Send(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/v1/messages" {
		t.Fatalf("request path = %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "test-key-123" {
		t.Fatalf("x-api-key header = %q, want test-key-123", gotAPIKey)
	}
	if gotAPIVersion == "" {
		t.Fatal("anthropic-version header was not set")
	}
	if gotModel != "claude-haiku-4-5-20251001" {
		t.Fatalf("request body model = %q, want claude-haiku-4-5-20251001", gotModel)
	}
	if resp.Text != "hello from anthropic" {
		t.Fatalf("Response.Text = %q, want %q", resp.Text, "hello from anthropic")
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 34 {
		t.Fatalf("Response.Usage = %+v, want {InputTokens:12 OutputTokens:34}", resp.Usage)
	}

	if p.Name() != "anthropic" {
		t.Fatalf("Name() = %q, want anthropic", p.Name())
	}
	if p.ModelVersion() != "claude-haiku-4-5-20251001@20251001" {
		t.Fatalf("ModelVersion() = %q, want claude-haiku-4-5-20251001@20251001", p.ModelVersion())
	}
}
