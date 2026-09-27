package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// defaultOpenAICompatibleBaseURL is the production OpenAI API host. The
// same adapter also covers Ollama-class local servers (RFC section 9) by
// pointing BaseURL at the local server and leaving APIKey empty.
const defaultOpenAICompatibleBaseURL = "https://api.openai.com/v1"

// OpenAICompatibleProvider reaches any OpenAI-compatible chat-completions
// API directly over net/http (ADR 003 -- no SDK dependency at this
// edge). Zero value is usable except Model, which callers must set;
// APIKey may be left empty for a local server that requires none.
type OpenAICompatibleProvider struct {
	// BaseURL defaults to the production OpenAI API host when empty.
	// Point at an Ollama-class local server's OpenAI-compatible endpoint
	// to use this adapter for local models.
	BaseURL string
	// APIKey is sent as an Authorization: Bearer header when non-empty.
	// Left empty, no Authorization header is sent -- the local-server case.
	APIKey string
	// Model is the pinned model identifier.
	Model string
	// Version is the pinned-model-set version tag (RFC section 7.5);
	// ModelVersion() composes "<Model>@<Version>".
	Version    string
	HTTPClient *http.Client
	// ExtraBody merges additional top-level fields into every outgoing
	// chat-completions request, verbatim (T1.31). It exists for
	// deployment-specific tuning knobs outside the OpenAI API's own
	// schema -- the motivating case is SGLang/vLLM's Qwen3
	// chat_template_kwargs.enable_thinking flag: measured directly
	// against a live qwen3.8-27b SGLang endpoint, the model's default
	// "thinking" reasoning trace accounted for 65-95% of every
	// extraction call's completion tokens and a 6-15x wall-clock
	// multiplier per call (docs/devlog.md, 2026-09-06 entry), which is
	// what made a 92k-chunk real corpus project to ~2-3 months
	// (docs/evals/m1-report.md). Left nil by default so a request is
	// byte-identical to before this field existed -- callers must opt in
	// (internal/providers wires it from serenity.yml's
	// models.disable_thinking) because an unrecognized top-level field is
	// a hard 400 on OpenAI's/OpenRouter's own APIs, which this same
	// adapter also serves.
	ExtraBody map[string]any
	// MaxTokens, when > 0, is sent as the chat-completions max_tokens
	// field, bounding one completion's output (T24.13, AI-04: the
	// composer/synthesize path previously sent no bound at all, so a
	// runaway completion had no ceiling but the server's own). 0 omits
	// the field, keeping every request that does not opt in
	// byte-identical to before this field existed -- the extraction
	// path is deliberately left at 0 (internal/providers wires the
	// composer only). Unlike AnthropicProvider there is no built-in
	// default here: the Anthropic API requires max_tokens, an
	// OpenAI-compatible one does not.
	MaxTokens int
}

var _ Provider = (*OpenAICompatibleProvider)(nil)

func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible" }

func (p *OpenAICompatibleProvider) ModelVersion() string {
	return fmt.Sprintf("%s@%s", p.Model, p.Version)
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

func (p *OpenAICompatibleProvider) Send(ctx context.Context, prompt string) (Response, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenAICompatibleBaseURL
	}
	client := httpClientOrDefault(p.HTTPClient)

	fields := map[string]any{
		"model":    p.Model,
		"messages": []openAIMessage{{Role: "user", Content: prompt}},
	}
	if p.MaxTokens > 0 {
		fields["max_tokens"] = p.MaxTokens
	}
	for k, v := range p.ExtraBody {
		fields[k] = v
	}
	reqBody, err := json.Marshal(fields)
	if err != nil {
		return Response{}, fmt.Errorf("openai_compatible: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return Response{}, fmt.Errorf("openai_compatible: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("openai_compatible: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("openai_compatible: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("openai_compatible: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, fmt.Errorf("openai_compatible: decode response: %w", err)
	}

	var text string
	if len(parsed.Choices) > 0 {
		text = parsed.Choices[0].Message.Content
	}

	// CostUSD is priced from the API's own usage block against
	// prices.go (T24.13, closing lore L-0008); an unlisted Model prices
	// at +Inf so any Budget.MaxUSD trips.
	return Response{
		Text: text,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			CostUSD:      CostUSD(p.Model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens),
		},
	}, nil
}
