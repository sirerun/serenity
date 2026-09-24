package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostedProviderSendsPrivacyControls(t *testing.T) {
	var body struct {
		Model    string `json:"model"`
		Provider struct {
			Only           []string `json:"only"`
			AllowFallbacks *bool    `json:"allow_fallbacks"`
			ZDR            bool     `json:"zdr"`
			DataCollection string   `json:"data_collection"`
		} `json:"provider"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,2,3]}],"model":"pplx-embed-v1-0.6b"}`))
	}))
	defer server.Close()
	p := newEmbeddingProvider(Config{EmbeddingModel: "perplexity/pplx-embed-v1-0.6b", EmbeddingBaseURL: server.URL, EmbeddingVersion: "test"}, "synthetic-key")
	if _, err := p.Send(context.Background(), "Synthetic readiness probe"); err != nil {
		t.Fatal(err)
	}
	if len(body.Provider.Only) != 1 || body.Provider.Only[0] != "Perplexity" || body.Provider.AllowFallbacks == nil || *body.Provider.AllowFallbacks || !body.Provider.ZDR || body.Provider.DataCollection != "deny" {
		t.Fatalf("missing hosted privacy controls: %+v", body.Provider)
	}
}

func TestHostedProviderDefaultsAndOtherEndpoints(t *testing.T) {
	p := newEmbeddingProvider(Config{EmbeddingModel: "perplexity/pplx-embed-v1-0.6b"}, "")
	if p.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatal("approved model default must use OpenRouter")
	}
	p = newEmbeddingProvider(Config{EmbeddingModel: "other", EmbeddingBaseURL: "https://openrouter.ai/api/v1"}, "")
	if !p.ZDR || p.DataCollection != "deny" {
		t.Fatal("OpenRouter privacy defaults missing")
	}
	p = newEmbeddingProvider(Config{EmbeddingModel: "local", EmbeddingBaseURL: "http://127.0.0.1:9999"}, "")
	if p.ZDR || p.DataCollection != "" || len(p.ProviderOnly) != 0 {
		t.Fatal("unrelated local provider was changed")
	}
}

func TestApprovedProductionEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"", "https://openrouter.ai/api/v1", "https://openrouter.ai/api/v1/", "https://other.example/v1", "https://openrouter.ai.example/api/v1", "https://openrouter.ai:444/api/v1"} {
		cfg := Config{Bind: "127.0.0.1:8090", PublicOrigin: "https://memory.example", DataDir: t.TempDir(), SecretsDir: t.TempDir(), EmbeddingModel: "perplexity/pplx-embed-v1-0.6b", EmbeddingVersion: "v1", EmbeddingBaseURL: endpoint}
		err := cfg.Validate(false)
		allowed := endpoint == "" || endpoint == "https://openrouter.ai/api/v1" || endpoint == "https://openrouter.ai/api/v1/"
		if (err == nil) != allowed {
			t.Fatalf("endpoint %q allowed=%t error=%v", endpoint, allowed, err)
		}
	}
}

func TestPrivateEmbeddingDoesNotFollowRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(200) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	p := newEmbeddingProvider(Config{EmbeddingModel: "perplexity/pplx-embed-v1-0.6b", EmbeddingBaseURL: origin.URL}, "synthetic-key")
	if _, err := p.Send(context.Background(), "Synthetic input"); err == nil {
		t.Fatal("redirect must fail closed")
	}
	if reached {
		t.Fatal("private embedding input followed a redirect")
	}
}
