package memory

import (
	"context"
	"encoding/json"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/server/mcp"
)

type synthesizeRequest struct {
	Query string `json:"query"`
}

type synthesizeResponse struct {
	Envelope
	Text          string                `json:"text,omitempty"`
	Gap           string                `json:"gap,omitempty"`
	Supersessions []synthesizeSupersede `json:"supersessions,omitempty"`
}

type synthesizeSupersede struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Chain     []Fact `json:"chain"`
}

func (h *Handlers) synthesizeTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "The question to answer from the brain's accumulated claims."}
		},
		"required": ["query"]
	}`
	return mcp.Tool{
		Name:        "synthesize",
		Description: "Compose a cited answer to a question from the brain's accumulated claims, or an explicit gap statement if none answer it.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.synthesize),
	}
}

// synthesize wraps internal/compose.Composer.Ask exactly as `serenity ask`
// does (internal/cli/ask.go): the same claim retrieval, the same
// structural whitelist-filtered citations, the same explicit gap
// statement when nothing answers the question -- never a fabricated
// answer, never a silent empty result (RFC §11). synthesize without a
// model returns unavailable with a fix (this task's own pitfall note):
// when h.deps.Composer is nil (providers.BuildComposerRouter's own
// explicit-skip contract -- no composer model pinned, or its credential
// is not configured), this verb returns VerbError{Code: "unavailable"}
// with h.deps.ComposerUnavailableNote as Message (the identical text
// `serenity ask` itself prints in that case) and a populated Suggestion,
// rather than a bare error or a silent skip.
//
// Disclosed, not built here: Envelope.Cost is never populated.
// internal/compose.Composer.Ask records its one judgment-tier call's
// spend as a side effect, through the router.Router already wired into
// h.deps.Composer at Deps-construction time -- production spend
// accounting (`serenity status`, T4.10's ceiling/projection) is correct
// and unaffected -- but Ask does not return the call's cost to its
// caller, so this verb has nothing to put in Cost without either
// changing Ask's signature (out of this task's scope: T1.12 has already
// shipped and Ask has other, unrelated callers) or reconstructing a
// second Router here with its own cost-observing ledger, double-recording
// spend. No acc line for this task tests Cost.
func (h *Handlers) synthesize(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req synthesizeRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return synthesizeResponse{Envelope: errorEnvelope("invalid_argument", "synthesize: malformed request", "send a JSON object with a non-empty \"query\" string")}, true, nil
	}
	if req.Query == "" {
		return synthesizeResponse{Envelope: errorEnvelope("invalid_argument", "synthesize: query is required", "pass a non-empty \"query\" string")}, true, nil
	}
	if h.deps.Composer == nil {
		note := h.deps.ComposerUnavailableNote
		if note == "" {
			note = "no composer model pinned"
		}
		return synthesizeResponse{Envelope: errorEnvelope("unavailable", note, "pin models.composer in serenity.yml and set the matching provider credential (ANTHROPIC_API_KEY, OPENAI_API_KEY, or OPENROUTER_API_KEY)")}, true, nil
	}

	c := compose.New(h.deps.Root, h.deps.Config, h.deps.Index, h.deps.Embedder, h.deps.Composer, h.deps.ComposerModelVersion)
	answer, err := c.Ask(ctx, req.Query)
	if err != nil {
		return nil, false, err
	}

	env := newEnvelope()
	resp := synthesizeResponse{Envelope: env}
	if answer.Gap != "" {
		resp.Gap = answer.Gap
		return resp, false, nil
	}
	resp.Text = answer.Text
	for _, cit := range answer.Citations {
		env.Evidence = append(env.Evidence, factOfCitation(cit))
	}
	for _, s := range answer.Supersessions {
		chain := make([]Fact, 0, len(s.Chain))
		for _, cit := range s.Chain {
			chain = append(chain, factOfCitation(cit))
		}
		resp.Supersessions = append(resp.Supersessions, synthesizeSupersede{Subject: s.Subject, Predicate: s.Predicate, Chain: chain})
	}
	resp.Envelope = env
	return resp, false, nil
}

func factOfCitation(c compose.Citation) Fact {
	conf := c.Confidence
	return Fact{
		Subject:    c.Subject,
		Predicate:  c.Predicate,
		Object:     c.Object,
		ClaimID:    c.ClaimID,
		SourceRef:  c.SourceRef,
		Confidence: &conf,
		Provenance: FactProvenance{ObservedAt: c.ObservedAt},
	}
}
