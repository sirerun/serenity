package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// defaultAnthropicBaseURL is the production Anthropic API host.
const defaultAnthropicBaseURL = "https://api.anthropic.com"

// defaultAnthropicAPIVersion is the Messages API version header value
// (distinct from Model/Version, which is the pinned-model-set provenance
// identifier, RFC section 7.5).
const defaultAnthropicAPIVersion = "2023-06-01"

// defaultAnthropicMaxTokens bounds a single completion when MaxTokens is
// unset.
const defaultAnthropicMaxTokens = 4096

// AnthropicProvider reaches the Anthropic Messages API directly over
// net/http (ADR 003 -- no SDK dependency at this edge). Zero value is
// usable except Model/APIKey, which callers must set.
type AnthropicProvider struct {
	// BaseURL defaults to the production API host when empty.
	BaseURL string
	APIKey  string
	// Model is the pinned model identifier (e.g. "claude-haiku-4-5-20251001").
	Model string
	// Version is the pinned-model-set version tag (RFC section 7.5);
	// ModelVersion() composes "<Model>@<Version>".
	Version string
	// APIVersion is the anthropic-version request header; defaults to
	// defaultAnthropicAPIVersion when empty.
	APIVersion string
	MaxTokens  int
	HTTPClient *http.Client
}

var _ Provider = (*AnthropicProvider)(nil)

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) ModelVersion() string {
	return fmt.Sprintf("%s@%s", p.Model, p.Version)
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

func (p *AnthropicProvider) Send(ctx context.Context, prompt string) (Response, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	apiVersion := p.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAnthropicAPIVersion
	}
	maxTokens := p.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultAnthropicMaxTokens
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     p.Model,
		MaxTokens: maxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, fmt.Errorf("anthropic: decode response: %w", err)
	}

	var text string
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return Response{
		Text: text,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
	}, nil
}
