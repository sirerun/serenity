package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type rememberRequest struct {
	Fact       string `json:"fact"`
	Provenance string `json:"provenance"`
	TTL        string `json:"ttl,omitempty"`
	Entity     string `json:"entity,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

type rememberResponse struct {
	ProtocolVersion int     `json:"protocol_version"`
	ID              string  `json:"id"`
	Status          string  `json:"status"` // inserted | duplicate | superseded
	StatusText      string  `json:"status_text"`
	EntitySlug      *string `json:"entity_slug"`
	ValidUntil      *string `json:"valid_until"`
	// DegradedDedup is never set true (this task's own acc line: "no
	// embedder degraded flag") -- exact-duplicate matching (this
	// implementation's only dedup) needs no embedder at all, so there is
	// nothing degraded to disclose; kept, always omitted, for schema
	// completeness against upstream's own optional field.
	DegradedDedup bool `json:"degraded_dedup,omitempty"`
}

const provenanceMaxChars = 500

func (h *Handlers) rememberTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"fact": {"type": "string", "description": "The fact to remember, one claim per call."},
			"provenance": {"type": "string", "description": "Where this fact came from (required, free text, max 500 chars)."},
			"ttl": {"type": "string", "description": "Duration shorthand (\"30d\", \"12h\", \"45m\") or absolute ISO 8601 timestamp. Omit = never expires."},
			"entity": {"type": "string", "description": "Person/company/project this fact is about."},
			"kind": {"type": "string", "enum": ["event", "preference", "commitment", "belief", "fact"]},
			"visibility": {"type": "string", "enum": ["world", "private"]}
		},
		"required": ["fact", "provenance"]
	}`
	return mcp.Tool{
		Name:        "remember",
		Description: "Save one fact to durable agent memory, with mandatory attribution.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.remember),
	}
}

// remember writes fact as a new canonical memory_fact source
// (internal/writer.MemoryFact, T4.20) -- raw attributed material, never an
// automatic accepted domain.Claim (memory-compat-mapping.md: "never
// automatic accepted belief").
func (h *Handlers) remember(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req rememberRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "remember: malformed request", "send a JSON object with \"fact\" and \"provenance\" strings"), true, nil
	}
	fact := trimmed(req.Fact)
	if fact == "" {
		return verbError(ErrCodeInvalidParams, "remember: fact must be a non-empty string", "pass the claim to remember, e.g. fact: \"picked Stripe over Adyen -- onboarding speed\""), true, nil
	}
	provenance := trimmed(req.Provenance)
	if provenance == "" {
		return verbError(ErrCodeProvenanceRequired, "remember: provenance is required and must be non-empty", "pass where the fact came from, e.g. provenance: \"user told me, 2026-06-12\" or \"import: notes.md\""), true, nil
	}
	if len(provenance) > provenanceMaxChars {
		return verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: provenance exceeds %d chars (got %d)", provenanceMaxChars, len(provenance)), "shorten the attribution -- provenance is a pointer, not a transcript"), true, nil
	}

	kind := req.Kind
	if kind == "" {
		kind = string(store.MemoryFactKindFact)
	}
	if !store.ValidMemoryFactKind(kind) {
		return verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: kind %q is not a fact kind", kind), "use one of: event | preference | commitment | belief | fact"), true, nil
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = string(store.MemoryVisibilityWorld)
	}
	if visibility != string(store.MemoryVisibilityWorld) && visibility != string(store.MemoryVisibilityPrivate) {
		return verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: visibility %q is not valid", visibility), "use \"world\" (default -- agents can recall it) or \"private\" (local CLI reads only)"), true, nil
	}

	now := h.deps.now()
	validUntil, err := parseTTL(req.TTL, now)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "remember: "+err.Error(), "use duration shorthand (\"30d\", \"12h\", \"45m\") or an absolute ISO 8601 timestamp (\"2026-07-12T00:00:00Z\"), never an ISO-8601 duration like \"P30D\""), true, nil
	}

	var entitySlug, entityType string
	if req.Entity != "" {
		t, s, ok := canonicalEntityRef(req.Entity)
		if !ok {
			return verbError(ErrCodeInvalidParams, "remember: entity is not a valid reference", "pass a plain name or a \"type/slug\" reference with no path separators beyond the one splitting them"), true, nil
		}
		entityType, entitySlug = t, s
	}

	mw := h.deps.memoryWriter()
	result, err := mw.Remember(writer.RememberInput{
		Fact:       fact,
		Provenance: provenance,
		EntitySlug: entitySlug,
		EntityType: entityType,
		Kind:       store.MemoryFactKind(kind),
		Visibility: store.MemoryVisibility(visibility),
		ValidUntil: validUntil,
	}, now)
	if err != nil {
		return nil, false, fmt.Errorf("remember: %w", err)
	}

	status := "inserted"
	statusText := fmt.Sprintf("remembered as fact #%d", result.Record.Payload.LegacyID)
	if !result.Inserted {
		status = "duplicate"
		statusText = fmt.Sprintf("already knew this -- kept fact #%d", result.Record.Payload.LegacyID)
	}

	return rememberResponse{
		ProtocolVersion: ProtocolVersion,
		ID:              result.Record.SHA256,
		Status:          status,
		StatusText:      statusText,
		EntitySlug:      stringPtr(result.Record.Payload.EntitySlug),
		ValidUntil:      isoPtr(result.Record.Payload.ValidUntil),
	}, false, nil
}
