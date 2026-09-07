package memory

import (
	"context"
	"encoding/json"

	"github.com/sirerun/serenity/internal/briefing"
	"github.com/sirerun/serenity/internal/search"
	"github.com/sirerun/serenity/internal/server/mcp"
)

// defaultRecallBudgetTokens is recall's own default attention budget when
// a caller omits budget_tokens -- there is no existing RFC-named default
// for MEMORY_VERBS's recall the way briefing.DefaultMovedForwardLookback
// exists for briefing's own "recent" window, so this task picks one:
// RFC 0001 §7's own daily-briefing hard cap (800 words) times roughly 2.5,
// generous enough that a caller exploring without an explicit budget
// rarely sees drops, disclosed rather than silently unbounded.
const defaultRecallBudgetTokens = 2000

// recallPoolSize bounds how many ranked, deduplicated hits recall asks
// internal/search.Search for before budget-packing -- a fixed candidate
// pool, the same bounded-prompt-size reasoning internal/compose's own
// retrievalLimit/searchPoolMultiplier apply, sized generously enough that
// a realistic budget_tokens value is very unlikely to still want a hit
// recall never even considered.
const recallPoolSize = 50

type recallRequest struct {
	Query        string `json:"query"`
	BudgetTokens int    `json:"budget_tokens"`
}

type recallResponse struct {
	Envelope
}

func (h *Handlers) recallTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "What to recall."},
			"budget_tokens": {"type": "integer", "minimum": 0, "description": "Attention budget in words (0 or omitted uses the default)."}
		},
		"required": ["query"]
	}`
	return mcp.Tool{
		Name:        "recall",
		Description: "Retrieve relevant memory for a query: ranked, deduplicated evidence packed within a token budget.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.recall),
	}
}

// recall wraps internal/search.Search exactly as `serenity search` does
// (internal/cli/search.go): the same hybrid-search/dedup pipeline, the
// same honest FTS-only degradation when h.deps.Embedder is nil. What this
// verb adds beyond the CLI's own wiring is budget-packing: RFC 0001 §8.1's
// gbrain envelope names a "budget meta" field this task's own acc line
// tests directly ("recall budget_used <= budget_tokens with dropped_count
// consistent"). internal/briefing.Pack (T2.17) is the wrong primitive here
// -- it drops whole SECTIONS, never individual items within one -- so this
// walks the ranked pool in rank order, including each hit while it still
// fits the remaining budget and counting every hit that does not as
// dropped, using briefing.WordEstimator for the same word-count
// convention T2.17/T4.6 already established for "budget_estimator: words".
// A later, smaller hit can still be included after an earlier, larger one
// was dropped -- a deliberate greedy-fit choice, not a bug: recall's own
// acc line only requires budget_used <= budget_tokens and a consistent
// dropped_count, not strict rank-order truncation.
func (h *Handlers) recall(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req recallRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return recallResponse{errorEnvelope("invalid_argument", "recall: malformed request", "send a JSON object with a non-empty \"query\" string")}, true, nil
	}
	if req.Query == "" {
		return recallResponse{errorEnvelope("invalid_argument", "recall: query is required", "pass a non-empty \"query\" string")}, true, nil
	}
	budget := req.BudgetTokens
	if budget <= 0 {
		budget = defaultRecallBudgetTokens
	}

	results, err := search.Search(ctx, h.deps.Index, h.deps.Embedder, req.Query, recallPoolSize, search.Options{})
	if err != nil {
		return nil, false, err
	}

	evidence, used, dropped := PackFacts(results, budget)

	env := newEnvelope()
	env.Evidence = evidence
	env.Budget = &BudgetMeta{BudgetTokens: budget, BudgetUsed: used, DroppedCount: dropped}
	return recallResponse{env}, false, nil
}

// PackFacts converts ranked search.Results into recall's own Fact/budget
// projection (RFC 0001 section 8.1's own gbrain "budget meta" field),
// exactly as recall itself does. Exported (T4.9) so a CLI vs protocol
// drift test can build recall's own evidence/budget projection from the
// exact search results `serenity search`'s own code path
// (internal/cli.searchResults) produces, without reimplementing this
// mapping -- a hand-rolled duplicate in a test would not catch a real
// one-sided change to it.
func PackFacts(results []search.Result, budgetTokens int) (evidence []Fact, budgetUsed, dropped int) {
	evidence = make([]Fact, 0, len(results))
	for _, r := range results {
		cost := briefing.WordEstimator(r.Text)
		if budgetUsed+cost > budgetTokens {
			dropped++
			continue
		}
		budgetUsed += cost
		evidence = append(evidence, Fact{
			ChunkRef:   r.ChunkRef,
			EntitySlug: r.EntitySlug,
			Kind:       r.Kind,
			Text:       r.Text,
			Score:      r.RRFScore,
			SourceRef:  r.SourceSHA256,
		})
	}
	return evidence, budgetUsed, dropped
}
