package memory

import (
	"context"
	"encoding/json"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
)

type entityRequest struct {
	Slug string `json:"slug"`
}

type entityResponse struct {
	Envelope
	Found   bool     `json:"found"`
	Slug    string   `json:"slug,omitempty"`
	Type    string   `json:"type,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

func (h *Handlers) entityTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"slug": {"type": "string", "description": "The entity's slug, e.g. \"acme-corp\"."}
		},
		"required": ["slug"]
	}`
	return mcp.Tool{
		Name:        "entity",
		Description: "Look up one entity by slug: its type, aliases, and every live claim recorded about it.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.entity),
	}
}

// entity is a point lookup, not a repo-wide walk: internal/store.FenceWriter
// has no "find entity by slug alone" primitive (PathFor needs the type
// folder up front), so this globs brain/entities/*/<slug>.md directly --
// exactly the same shape internal/compose.AllClaims's own page glob uses,
// narrowed to one slug instead of every page. A miss is a normal,
// successful response with found:false, never an error -- this task's own
// acc line ("entity miss -> found:false not an error").
//
// Live claims only, both tiers: fence-tier claims come from the matched
// page itself; shard-tier claims come from every family
// internal/store.ShardStore.Families reports for this slug, each resolved
// to its live head via store.ResolveHeadLines -- the same liveness rule
// internal/compose.resolveLive applies, without that function's ancestor-
// chain bookkeeping, which entity() does not surface (disclosed: a
// caller wanting supersession history uses synthesize, which does).
//
// Fence-tier claims carry a thinner Provenance than the domain type
// promises: T4.6 already found and disclosed that
// store.FenceWriter.RenderEntity/ParseEntity's table round-trip drops
// everything but SourceRef (RFC §7.2's own table has no sha256 column) --
// this verb inherits that same, already-documented gap rather than
// rediscovering it.
func (h *Handlers) entity(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req entityRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return entityResponse{Envelope: errorEnvelope("invalid_argument", "entity: malformed request", "send a JSON object with a non-empty \"slug\" string")}, true, nil
	}
	if req.Slug == "" {
		return entityResponse{Envelope: errorEnvelope("invalid_argument", "entity: slug is required", "pass a non-empty \"slug\" string")}, true, nil
	}

	matches, err := globEntityPage(h.deps.Root, req.Slug)
	if err != nil {
		return nil, false, err
	}

	env := newEnvelope()
	if len(matches) == 0 {
		return entityResponse{Envelope: env, Found: false, Slug: req.Slug}, false, nil
	}

	page, err := h.deps.Fence.ParseEntity(matches[0])
	if err != nil {
		return nil, false, err
	}

	var evidence []Fact
	for _, cl := range page.Claims {
		evidence = append(evidence, factOfClaim(cl))
	}

	families, err := h.deps.Shard.Families(req.Slug)
	if err != nil {
		return nil, false, err
	}
	for _, family := range families {
		lines, err := h.deps.Shard.Lines(req.Slug, family)
		if err != nil {
			return nil, false, err
		}
		heads := store.ResolveHeadLines(lines)
		for _, key := range store.HeadKeys(heads) {
			evidence = append(evidence, factOfClaim(heads[key]))
		}
	}

	env.Evidence = evidence
	return entityResponse{
		Envelope: env,
		Found:    true,
		Slug:     page.Entity.Slug,
		Type:     page.Entity.Type,
		Aliases:  page.Entity.Aliases,
	}, false, nil
}

func factOfClaim(c domain.Claim) Fact {
	f := Fact{
		Subject:      c.SubjectSlug,
		Predicate:    c.Predicate,
		Object:       c.Object,
		ClaimID:      c.ID,
		SupersededBy: c.SupersededBy,
		SourceRef:    c.SourceRef,
		Provenance: FactProvenance{
			SourceSHA256: c.Provenance.SourceSHA256,
			Span:         c.Provenance.Span,
			Model:        c.Provenance.Model,
			ObservedAt:   c.Provenance.ObservedAt,
			Actor:        c.Provenance.Actor,
		},
	}
	if c.Confidence != 0 {
		conf := c.Confidence
		f.Confidence = &conf
	}
	return f
}
