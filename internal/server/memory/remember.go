package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/writer"
)

type rememberRequest struct {
	OperationKey string `json:"operation_key,omitempty"`
	Fact         string `json:"fact"`
	Provenance   string `json:"provenance"`
	TTL          string `json:"ttl,omitempty"`
	Entity       string `json:"entity,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Visibility   string `json:"visibility,omitempty"`
}

type rememberResponse struct {
	SearchState     string  `json:"search_state,omitempty"`
	Expired         bool    `json:"expired,omitempty"`
	ProtocolVersion int     `json:"protocol_version"`
	ID              string  `json:"id"`
	Status          string  `json:"status"` // inserted | duplicate | superseded
	StatusText      string  `json:"status_text"`
	EntitySlug      *string `json:"entity_slug"`
	ValidUntil      *string `json:"valid_until"`
	// Exact matching is available; semantic duplicate detection is not.
	DegradedDedup bool `json:"degraded_dedup,omitempty"`
}

const provenanceMaxChars = 500

func (h *Handlers) rememberTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
 "operation_key": {"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9_.:-]+$", "description": "Optional brain-scoped immutable request key. Same key and normalized payload recover the same fact even after expiry; changed payload conflicts. With a key, ttl must be absolute or omitted."},
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
	now := h.deps.now()
	input, validation, invalid := NormalizeRememberRequest(args, now)
	if invalid {
		return validation, true, nil
	}

	writerID := LocalWriter
	if p, ok := principalFrom(ctx); ok {
		writerID = p.ID
	}
	mw := h.deps.memoryWriter()
	result, err := mw.RememberContext(ctx, writer.RememberInput{
		OperationKey: input.OperationKey,
		Fact:         input.Fact,
		Provenance:   input.Provenance,
		EntitySlug:   input.EntitySlug,
		EntityType:   input.EntityType,
		Kind:         input.Kind,
		Visibility:   input.Visibility,
		ValidUntil:   input.ValidUntil,
		Writer:       writerID,
	}, now)
	if errors.Is(err, writer.ErrMemoryOperationCanceled) {
		return verbError(ErrCodeOperationCanceled, "remember: operation was canceled, or its fact was forgotten", "do not retry a withdrawn operation with another key"), true, nil
	}
	if errors.Is(err, writer.ErrMemoryOperationConflict) {
		return RememberOperationConflict(), true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("remember: %w", err)
	}

	status := "inserted"
	statusText := fmt.Sprintf("remembered as fact #%d", result.Record.Payload.LegacyID)
	if !result.Inserted {
		status = "duplicate"
		statusText = fmt.Sprintf("already knew this -- kept fact #%d", result.Record.Payload.LegacyID)
	}

	// Canonical bytes are already durable. Search failure is a recoverable
	// projection state, not an ambiguous failed canonical write.
	searchState := "unavailable"
	expired := result.Record.Expired(now)
	if h.deps.Index != nil {
		indexed := h.deps.Queue.Submit(writer.Job{Render: func() ([]byte, error) {
			indexCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			var indexErr error
			searchState, expired, indexErr = index.RefreshMemoryFactSearch(indexCtx, h.deps.Root, result.Record.SHA256, h.deps.Index, h.deps.Embedder, h.deps.now)
			return nil, indexErr
		}})
		if indexed.Err != nil {
			if searchState == "unavailable" {
				statusText += "; search cache unavailable, retry the same operation or stop the daemon and run serenity sync"
			} else {
				statusText += "; semantic indexing incomplete, retry the same operation to recover"
			}
		}
	} else {
		statusText += "; search cache unavailable, run serenity sync to rebuild"
	}

	return rememberResponse{
		SearchState:     searchState,
		Expired:         expired,
		ProtocolVersion: ProtocolVersion,
		DegradedDedup:   true,
		ID:              result.Record.SHA256,
		Status:          status,
		StatusText:      statusText,
		EntitySlug:      stringPtr(result.Record.Payload.EntitySlug),
		ValidUntil:      isoPtr(result.Record.Payload.ValidUntil),
	}, false, nil
}
