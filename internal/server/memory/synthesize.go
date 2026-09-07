package memory

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/server/mcp"
)

type synthesizeRequest struct {
	Question string `json:"question"`
	Since    string `json:"since,omitempty"`
	Until    string `json:"until,omitempty"`
}

type synthesizeCost struct {
	Model        string   `json:"model"`
	InputTokens  *int     `json:"input_tokens"`
	OutputTokens *int     `json:"output_tokens"`
	UsdEstimate  *float64 `json:"usd_estimate"`
}

type synthesizeResponse struct {
	ProtocolVersion int            `json:"protocol_version"`
	Answer          string         `json:"answer"`
	Sources         []string       `json:"sources"`
	Gaps            []string       `json:"gaps,omitempty"`
	Cost            synthesizeCost `json:"cost"`
}

func (h *Handlers) synthesizeTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"question": {"type": "string", "description": "The question to answer."},
			"since": {"type": "string", "description": "Optional temporal window start (ISO 8601 date or datetime)."},
			"until": {"type": "string", "description": "Optional temporal window end (ISO 8601 date or datetime)."}
		},
		"required": ["question"]
	}`
	return mcp.Tool{
		Name:        "synthesize",
		Description: "[EXPENSIVE/SLOW] Answer a broad question using cross-page LLM reasoning with citations and gap analysis.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.synthesize),
	}
}

// synthesize wraps the shared internal/compose.Composer.AskWithOptions
// (T4.20: "both CLI ask and MCP synthesize call the same implementation")
// -- the same claim retrieval `serenity ask` uses, plus source-evidence
// retrieval over MEMORY_VERBS facts, both date-bounded by since/until. No
// composer configured returns VerbError{Error: "unavailable"} with a fix,
// never a fake successful answer/model/cost (RFC §11, this task's own
// pitfall note).
func (h *Handlers) synthesize(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req synthesizeRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: malformed request", "send a JSON object with a non-empty \"question\" string"), true, nil
	}
	question := trimmed(req.Question)
	if question == "" {
		return verbError(ErrCodeInvalidParams, "synthesize: question must be a non-empty string", "pass the question to synthesize an answer for, e.g. question: \"what is our payments strategy?\""), true, nil
	}
	since, err := parseSinceUntil(req.Since)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: since is not a valid ISO 8601 date/datetime", "pass an ISO 8601 date (\"2026-06-01\") or datetime (\"2026-06-01T00:00:00Z\")"), true, nil
	}
	until, err := parseSinceUntil(req.Until)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: until is not a valid ISO 8601 date/datetime", "pass an ISO 8601 date (\"2026-06-01\") or datetime (\"2026-06-01T00:00:00Z\")"), true, nil
	}

	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return verbError(ErrCodeInvalidParams, "since must not be later than until", "provide an ordered date window"), true, nil
	}

	if h.deps.Composer == nil {
		note := h.deps.ComposerUnavailableNote
		if note == "" {
			note = "synthesize needs an LLM and none is configured"
		}
		return verbError(ErrCodeUnavailable, note, "set an API key (e.g. `serenity config` a provider credential) and retry -- recall and entity work without one"), true, nil
	}

	c := compose.New(h.deps.Root, h.deps.Config, h.deps.Index, h.deps.Embedder, h.deps.Composer, h.deps.ComposerModelVersion)
	answer, err := c.AskWithOptions(ctx, question, compose.AskOptions{Since: since, Until: until, Now: h.deps.now()})
	if err != nil {
		return nil, false, err
	}

	resp := synthesizeResponse{
		ProtocolVersion: ProtocolVersion,
		Cost:            synthesizeCost{Model: h.deps.ComposerModelVersion},
	}
	if answer.ModelVersion != "" {
		resp.Cost.Model = answer.ModelVersion
	}
	if answer.Usage != nil {
		in, out, usd := answer.Usage.InputTokens, answer.Usage.OutputTokens, answer.Usage.CostUSD
		resp.Cost.InputTokens = &in
		resp.Cost.OutputTokens = &out
		resp.Cost.UsdEstimate = &usd
	}

	if answer.Gap != "" {
		resp.Answer = answer.Gap
		resp.Sources = []string{}
		resp.Gaps = []string{answer.Gap}
		return resp, false, nil
	}

	resp.Answer = answer.Text
	seen := map[string]bool{}
	for _, cit := range answer.Citations {
		if cit.Subject != "" && !seen[cit.Subject] {
			seen[cit.Subject] = true
			resp.Sources = append(resp.Sources, cit.Subject)
		}
	}
	for _, sc := range answer.SourceCitations {
		source := sc.EntitySlug
		if source == "" {
			source = "source-" + sc.SHA256
		}
		if !seen[source] {
			seen[source] = true
			resp.Sources = append(resp.Sources, source)
		}
	}
	sort.Strings(resp.Sources)
	if resp.Sources == nil {
		resp.Sources = []string{}
	}
	return resp, false, nil
}
