package router

import "context"

// Usage is what one provider call cost/consumed.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// Response is what a Provider returns for one call. Confidence is the
// RAW, unclamped, model-reported figure (0 when the provider does not
// report one) -- Complete is the only place it gets clamped, via
// NewConfidence.
type Response struct {
	Text       string
	Confidence float64
	Usage      Usage
}

// Provider is one tier's model backend. Real implementations reach the
// provider over net/http directly (ADR 003): AnthropicProvider and
// OpenAICompatibleProvider in this package. Test doubles live in _test.go
// files only, per the zero-stub policy.
type Provider interface {
	// Name is the provider identifier recorded in spend ledger rows
	// (e.g. "anthropic", "openai-compatible").
	Name() string
	// ModelVersion is the RFC section 7.5 provenance string, shaped
	// "<model>@<version>".
	ModelVersion() string
	// Send issues one completion call.
	Send(ctx context.Context, prompt string) (Response, error)
}
