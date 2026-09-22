package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// defaultOpenAIEmbeddingsBaseURL is the production OpenAI API host. The
// same adapter also covers Ollama-class local embedding servers exposing
// an OpenAI-compatible /embeddings route (RFC section 9) by pointing
// BaseURL at the local server and leaving APIKey empty -- the same
// local-server convention OpenAICompatibleProvider uses.
const defaultOpenAIEmbeddingsBaseURL = "https://api.openai.com/v1"

// OpenAIEmbeddingsProvider reaches a real OpenAI-compatible /embeddings
// endpoint directly over net/http (ADR 003). It exists as a distinct type
// from OpenAICompatibleProvider because embeddings and chat completions are
// different real HTTP endpoints with different request/response shapes --
// no existing provider adapter in this package can produce a real
// embedding vector. Zero value is usable except Model, which callers must
// set; APIKey may be left empty for a local server that requires none.
type OpenAIEmbeddingsProvider struct {
	// BaseURL defaults to the production OpenAI API host when empty.
	BaseURL string
	// APIKey is sent as an Authorization: Bearer header when non-empty.
	APIKey string
	// Model is the pinned embedding model identifier.
	Model string
	// Version is the pinned-model-set version tag (RFC section 7.5);
	// ModelVersion() composes "<Model>@<Version>".
	Version        string
	HTTPClient     *http.Client
	Dimensions     int
	MaxInputBytes  int
	ProviderOnly   []string
	ZDR            bool
	DataCollection string
}

// EmbeddingProviderError is sanitized so upstream bodies cannot echo input or credentials.
type EmbeddingProviderError struct {
	Status    int
	Retryable bool
}

func (e *EmbeddingProviderError) Error() string {
	if e.Status == 0 {
		return "embedding provider request failed"
	}
	return fmt.Sprintf("embedding provider returned status %d", e.Status)
}

var _ Provider = (*OpenAIEmbeddingsProvider)(nil)

func (p *OpenAIEmbeddingsProvider) Name() string { return "openai-embeddings" }

func (p *OpenAIEmbeddingsProvider) ModelVersion() string {
	return fmt.Sprintf("%s@%s", p.Model, p.Version)
}

type openAIEmbeddingsRequest struct {
	Model    string                  `json:"model"`
	Input    string                  `json:"input"`
	Provider *openRouterProviderRule `json:"provider,omitempty"`
}

type openRouterProviderRule struct {
	Only           []string `json:"only,omitempty"`
	AllowFallbacks *bool    `json:"allow_fallbacks,omitempty"`
	ZDR            bool     `json:"zdr,omitempty"`
	DataCollection string   `json:"data_collection,omitempty"`
}

type openAIEmbeddingsDatum struct {
	Embedding []float32 `json:"embedding"`
}

type openAIEmbeddingsUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type openAIEmbeddingsResponse struct {
	Data  []openAIEmbeddingsDatum `json:"data"`
	Usage openAIEmbeddingsUsage   `json:"usage"`
}

// Send issues one embeddings call and re-encodes the returned vector as a
// JSON array string in Response.Text -- the interop convention
// internal/embed.RouterEmbedder decodes (see that package's doc comment on
// RouterEmbedder). CostUSD is left at zero, the same as
// AnthropicProvider/OpenAICompatibleProvider: this package has no pricing
// table yet (RFC section 9's T4.10 spend ceiling is later work).
func (p *OpenAIEmbeddingsProvider) Send(ctx context.Context, prompt string) (Response, error) {
	if strings.TrimSpace(p.Model) == "" {
		return Response{}, fmt.Errorf("openai_embeddings: model is required")
	}
	if p.MaxInputBytes > 0 && len([]byte(prompt)) > p.MaxInputBytes {
		return Response{}, fmt.Errorf("openai_embeddings: input exceeds configured limit")
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenAIEmbeddingsBaseURL
	}
	client := httpClientOrDefault(p.HTTPClient)

	var routing *openRouterProviderRule
	if len(p.ProviderOnly) > 0 || p.ZDR || p.DataCollection != "" {
		fallbacks := false
		routing = &openRouterProviderRule{Only: append([]string(nil), p.ProviderOnly...), AllowFallbacks: &fallbacks, ZDR: p.ZDR, DataCollection: p.DataCollection}
	}
	reqBody, err := json.Marshal(openAIEmbeddingsRequest{Model: p.Model, Input: prompt, Provider: routing})
	if err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, &EmbeddingProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500}
	}

	var parsed openAIEmbeddingsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: decode response: %w", err)
	}
	if len(parsed.Data) == 0 {
		return Response{}, fmt.Errorf("openai_embeddings: response carried no embedding data")
	}
	vec := parsed.Data[0].Embedding
	if len(vec) == 0 {
		return Response{}, fmt.Errorf("openai_embeddings: response carried an empty embedding")
	}
	if p.Dimensions > 0 && len(vec) != p.Dimensions {
		return Response{}, fmt.Errorf("openai_embeddings: dimension mismatch: got %d want %d", len(vec), p.Dimensions)
	}
	for _, value := range vec {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return Response{}, fmt.Errorf("openai_embeddings: response carried a non-finite value")
		}
	}

	vecJSON, err := json.Marshal(vec)
	if err != nil {
		return Response{}, fmt.Errorf("openai_embeddings: encode vector: %w", err)
	}

	return Response{
		Text: string(vecJSON),
		Usage: Usage{
			InputTokens: parsed.Usage.PromptTokens,
		},
	}, nil
}
