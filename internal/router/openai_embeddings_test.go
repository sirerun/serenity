package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAIEmbeddingsProviderSendsEmbeddingsRequestOverHTTP stands a real
// net/http test server in for an OpenAI-compatible /embeddings API (no
// real API key, no real network egress) and proves
// OpenAIEmbeddingsProvider composes the documented request shape, parses
// the documented response shape into internal/embed's JSON-array-string
// interop convention, and omits the Authorization header entirely when no
// API key is configured -- the Ollama-class local-server case (RFC
// section 9).
func TestOpenAIEmbeddingsProviderSendsEmbeddingsRequestOverHTTP(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		var gotPath, gotAuth string
		var gotBody openAIEmbeddingsRequest

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [{"embedding": [0.5, -0.25, 1]}],
				"usage": {"prompt_tokens": 3, "total_tokens": 3}
			}`))
		}))
		defer server.Close()

		p := &OpenAIEmbeddingsProvider{BaseURL: server.URL, APIKey: "sk-test", Model: "text-embedding-3-small", Version: "v1"}
		resp, err := p.Send(context.Background(), "embed me")
		if err != nil {
			t.Fatal(err)
		}

		if gotPath != "/embeddings" {
			t.Fatalf("request path = %q, want /embeddings", gotPath)
		}
		if gotAuth != "Bearer sk-test" {
			t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer sk-test")
		}
		if gotBody.Model != "text-embedding-3-small" || gotBody.Input != "embed me" {
			t.Fatalf("request body = %+v, want model=text-embedding-3-small input=%q", gotBody, "embed me")
		}
		if resp.Text != "[0.5,-0.25,1]" {
			t.Fatalf("Response.Text = %q, want the embedding re-encoded as a JSON array", resp.Text)
		}
		if resp.Usage.InputTokens != 3 {
			t.Fatalf("Response.Usage.InputTokens = %d, want 3", resp.Usage.InputTokens)
		}
	})

	t.Run("without API key (Ollama-class local server)", func(t *testing.T) {
		var sawAuthHeader bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, sawAuthHeader = r.Header["Authorization"]
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"data": [{"embedding": [1, 2]}], "usage": {"prompt_tokens": 1, "total_tokens": 1}}`))
		}))
		defer server.Close()

		p := &OpenAIEmbeddingsProvider{BaseURL: server.URL, Model: "local-embed", Version: "local"}
		if _, err := p.Send(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		if sawAuthHeader {
			t.Fatal("Authorization header was present when APIKey was empty, want absent")
		}
	})

	t.Run("empty data errors instead of returning a zero-length vector", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"data": [], "usage": {"prompt_tokens": 1, "total_tokens": 1}}`))
		}))
		defer server.Close()

		p := &OpenAIEmbeddingsProvider{BaseURL: server.URL, Model: "m", Version: "v1"}
		if _, err := p.Send(context.Background(), "hi"); err == nil {
			t.Fatal("expected an error for an empty data array, got nil")
		}
	})
}

func TestOpenAIEmbeddingsProviderIdentity(t *testing.T) {
	p := &OpenAIEmbeddingsProvider{Model: "text-embedding-3-small", Version: "v1"}
	if p.Name() != "openai-embeddings" {
		t.Fatalf("Name() = %q, want openai-embeddings", p.Name())
	}
	if p.ModelVersion() != "text-embedding-3-small@v1" {
		t.Fatalf("ModelVersion() = %q, want text-embedding-3-small@v1", p.ModelVersion())
	}
}
