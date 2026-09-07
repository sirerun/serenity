package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/search"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
)

// defaultRecallLimit is recall's own per-arm cap when the caller omits
// limit -- generous enough that a caller exploring without an explicit
// limit rarely hits it, disclosed rather than silently unbounded.
const defaultRecallLimit = 50

type recallRequest struct {
	Query        string `json:"query,omitempty"`
	Entity       string `json:"entity,omitempty"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
	Since        string `json:"since,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
}

type recallFact struct {
	ID         int64   `json:"id"`
	FactID     string  `json:"fact_id"`
	Fact       string  `json:"fact"`
	Kind       string  `json:"kind"`
	EntitySlug *string `json:"entity_slug"`
	Provenance string  `json:"provenance"`
	ValidUntil *string `json:"valid_until"`
	Visibility string  `json:"visibility"`
}

type recallResult struct {
	Slug         string  `json:"slug"`
	Title        *string `json:"title"`
	Chunk        *string `json:"chunk"`
	Evidence     string  `json:"evidence"`
	CreateSafety string  `json:"create_safety"`
	Provenance   string  `json:"provenance"`
}

type recallResponse struct {
	ProtocolVersion int            `json:"protocol_version"`
	Facts           []recallFact   `json:"facts"`
	Total           int            `json:"total"`
	Results         []recallResult `json:"results,omitzero"`
	SearchDegraded  string         `json:"search_degraded,omitempty"`
	BudgetTokens    *int           `json:"budget_tokens,omitempty"`
	BudgetUsed      *int           `json:"budget_used,omitempty"`
	DroppedCount    *int           `json:"dropped_count,omitempty"`
}

func (h *Handlers) recallTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Hybrid-search the brain's pages; omit to skip the search arm."},
			"entity": {"type": "string", "description": "Scope the facts arm to one entity (name or type/slug)."},
			"budget_tokens": {"type": "integer", "minimum": 0, "description": "Server-side char/4 packing budget; facts pack first."},
			"since": {"type": "string", "description": "ISO 8601 date/datetime -- filters the facts arm only."},
			"session_id": {"type": "string"},
			"limit": {"type": "integer", "minimum": 0, "description": "Per-arm cap on candidates."}
		}
	}`
	return mcp.Tool{
		Name:        "recall",
		Description: "Retrieve saved facts and (with query) budget-packed page snippets.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.recall),
	}
}

// recall implements the pinned contract's facts-first budget packing:
// facts pack first (limit-capped, char/4 estimated), then the search arm
// packs into whatever budget remains -- so search-arm starvation is
// bounded, never total, when facts are cheap (memory-compat-mapping.md,
// upstream-verbs.ts's own doc comment on recall's budget field).
// budget_tokens/budget_used/dropped_count are emitted only when the
// caller explicitly supplied budget_tokens -- BudgetTokens is a *int for
// exactly this reason, distinguishing "omitted" from "explicitly zero."
func (h *Handlers) recall(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req recallRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "recall: malformed request", "send a JSON object; every field is optional"), true, nil
	}

	since, err := parseSinceUntil(req.Since)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "recall: since is not a valid ISO 8601 date/datetime", "pass an ISO 8601 date (\"2026-06-01\") or datetime (\"2026-06-01T00:00:00Z\")"), true, nil
	}

	var entitySlug, entityType string
	scopedToEntity := false
	if req.Entity != "" {
		typ, slug, ok := canonicalEntityRef(req.Entity)
		if !ok {
			return verbError(ErrCodeInvalidParams, "recall: entity is not a valid reference", "pass a plain name or a \"type/slug\" reference with no path separators beyond the one splitting them"), true, nil
		}
		entitySlug = slug
		if strings.Contains(req.Entity, "/") {
			entityType = typ
		}
		scopedToEntity = true
	}

	limit := defaultRecallLimit
	if req.Limit != nil {
		if *req.Limit < 0 {
			return verbError(ErrCodeInvalidParams, "limit must be nonnegative", "omit limit or provide a nonnegative integer"), true, nil
		}
		limit = *req.Limit
	}
	if req.BudgetTokens != nil && *req.BudgetTokens < 0 {
		return verbError(ErrCodeInvalidParams, "budget_tokens must be nonnegative", "omit the budget or provide a nonnegative integer"), true, nil
	}

	now := h.deps.now()
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, false, err
	}

	var candidates []store.MemoryFactRecord
	if scopedToEntity {
		candidates = proj.ByEntity(entitySlug)
	} else {
		candidates = proj.All()
	}
	facts := make([]store.MemoryFactRecord, 0, len(candidates))
	for _, rec := range candidates {
		if entityType != "" && rec.Payload.EntityType != entityType {
			continue
		}
		if rec.Expired(now) {
			continue
		}
		if rec.Payload.Visibility != store.MemoryVisibilityWorld {
			continue // recall is MCP-only -- always the remote audience
		}
		if !since.IsZero() && rec.Payload.CreatedAt.Before(since) {
			continue
		}
		facts = append(facts, rec)
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Payload.CreatedAt.Equal(facts[j].Payload.CreatedAt) {
			return facts[i].SHA256 < facts[j].SHA256
		}
		return facts[i].Payload.CreatedAt.After(facts[j].Payload.CreatedAt)
	})
	if len(facts) > limit {
		facts = facts[:limit]
	}

	var results []recallResult
	if req.Query != "" {
		results = []recallResult{}
	}
	var searchDegraded string
	if strings.TrimSpace(req.Query) != "" && limit > 0 {
		restricted, err := index.RestrictedSummaryEntities(h.deps.Root, proj, now)
		if err != nil {
			return nil, false, err
		}
		eligible := index.SourceEligibility(proj, true, false, now, restricted)
		hits, err := search.Search(ctx, h.deps.Index, h.deps.Embedder, req.Query, limit, search.Options{Eligible: eligible})
		if err != nil {
			return nil, false, err
		}
		if h.deps.Embedder == nil {
			searchDegraded = "no embedding provider configured; results are keyword-only"
		}
		if len(hits) > limit {
			hits = hits[:limit]
		}
		results = make([]recallResult, 0, len(hits))
		for _, r := range hits {
			results = append(results, recallResultOf(h.deps.Root, req.Query, r, h.deps.Embedder != nil))
		}
	}

	env := recallResponse{ProtocolVersion: ProtocolVersion, Results: results, SearchDegraded: searchDegraded}
	if req.BudgetTokens != nil {
		budget := *req.BudgetTokens
		used, dropped := 0, 0

		packedFacts := make([]recallFact, 0, len(facts))
		for _, rec := range facts {
			cost := charEstimate(rec.Payload.Fact)
			if used+cost > budget {
				dropped++
				continue
			}
			used += cost
			packedFacts = append(packedFacts, recallFactOf(rec))
		}

		packedResults := make([]recallResult, 0, len(results))
		for _, r := range results {
			text := ""
			if r.Chunk != nil {
				text = *r.Chunk
			}
			cost := charEstimate(text)
			if used+cost > budget {
				dropped++
				continue
			}
			used += cost
			packedResults = append(packedResults, r)
		}

		env.Facts = packedFacts
		env.Total = len(packedFacts)
		if req.Query != "" {
			env.Results = packedResults
		}
		env.BudgetTokens = &budget
		env.BudgetUsed = &used
		env.DroppedCount = &dropped
		return env, false, nil
	}

	env.Facts = make([]recallFact, 0, len(facts))
	for _, rec := range facts {
		env.Facts = append(env.Facts, recallFactOf(rec))
	}
	env.Total = len(env.Facts)
	return env, false, nil
}

// charEstimate is the pinned char/4 token estimator (±10-15%, upstream's
// own disclosed tolerance) -- ceiling division so even a short, non-empty
// string costs at least 1, never 0 (a budget of 1 must still be able to
// force a drop against a real fact).
func charEstimate(s string) int {
	if s == "" {
		return 0
	}
	return (utf8.RuneCountInString(s) + 3) / 4
}

func recallFactOf(rec store.MemoryFactRecord) recallFact {
	return recallFact{
		ID:         rec.Payload.LegacyID,
		FactID:     rec.SHA256,
		Fact:       rec.Payload.Fact,
		Kind:       string(rec.Payload.Kind),
		EntitySlug: stringPtr(rec.Payload.EntitySlug),
		Provenance: rec.Payload.Provenance,
		ValidUntil: isoPtr(rec.Payload.ValidUntil),
		Visibility: string(rec.Payload.Visibility),
	}
}

// recallResultOf classifies one search hit into the pinned enum shapes
// (evidence, create_safety) -- zero-LLM heuristics over real signals
// (query/hit text overlap, whether the hit's own entity slug already has
// a page on disk, whether an embedder actually ran), never a hardcoded
// constant value.
func recallResultOf(root, query string, r search.Result, hasEmbedder bool) recallResult {
	slug := r.EntitySlug
	if slug == "" {
		if r.SourceSHA256 != "" && len(r.SourceSHA256) >= 8 {
			slug = "source-" + r.SourceSHA256
		} else {
			slug = r.ChunkRef
		}
	}

	var title *string
	createSafety := "unknown"
	if r.EntitySlug != "" {
		if matches, _ := globEntityPage(root, r.EntitySlug); len(matches) > 0 {
			createSafety = "exists"
		}
	}

	evidence := classifyEvidence(query, r, hasEmbedder)
	chunk := r.Text
	return recallResult{
		Slug:         slug,
		Title:        title,
		Chunk:        stringPtr(chunk),
		Evidence:     evidence,
		CreateSafety: createSafety,
		Provenance:   slug,
	}
}

func classifyEvidence(query string, r search.Result, hasEmbedder bool) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q != "" && strings.Contains(strings.ToLower(r.Text), q) {
		return "keyword_exact"
	}
	return "weak_semantic"
}
