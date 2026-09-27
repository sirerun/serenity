package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type forgetRequest struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

type forgetResponse struct {
	ProtocolVersion int     `json:"protocol_version"`
	ID              string  `json:"id"`
	Expired         bool    `json:"expired"`
	Reason          *string `json:"reason"`
}

func (h *Handlers) forgetTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Opaque fact id from remember/recall (facts[].fact_id). Never a page slug."},
			"reason": {"type": "string", "description": "Optional reason, written to the fact's audit trail."}
		},
		"required": ["id"]
	}`
	return mcp.Tool{
		Name:        "forget",
		Description: "Expire a remembered fact by id. Idempotent: re-forgetting an already-expired fact succeeds with expired:false.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.forget),
	}
}

// forget erases an accessible public fact: it records an immutable expiry
// event and removes the fact's bytes and index rows (ADR 019).
func (h *Handlers) forget(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req forgetRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "forget: malformed request", "send a JSON object with a non-empty \"id\" string"), true, nil
	}
	id := trimmed(req.ID)
	if id == "" {
		return verbError(ErrCodeNotFound, fmt.Sprintf("no fact with id %q", req.ID), "pass the opaque string id returned by remember or recall (facts[].fact_id) -- page slugs are not fact ids"), true, nil
	}

	now := h.deps.now()
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, false, err
	}
	sha, ok := writer.ByLegacyOrOpaqueID(proj, id)
	if !ok {
		// Forget removed this fact's bytes (ADR 019); only its expiry
		// event remains, so re-forgetting it is the idempotent success.
		// Forget removed this fact's bytes (ADR 019); only its expiry event
		// remains. The writer finishes any partial erasure and reports the
		// idempotent Expired=false.
		if _, erased := proj.ErasedExpiry(id); erased {
			result, err := h.deps.memoryWriter().Forget(id, req.Reason, now)
			if err != nil {
				return nil, false, fmt.Errorf("forget: %w", err)
			}
			resp := forgetResponse{ProtocolVersion: ProtocolVersion, ID: id, Expired: result.Expired}
			if req.Reason != "" {
				resp.Reason = &req.Reason
			}
			return resp, false, nil
		}
		return verbError(ErrCodeNotFound, fmt.Sprintf("no fact with id %q", id), "ids come from remember/recall (facts[].fact_id). recall the entity first to find the right fact"), true, nil
	}

	record, found := proj.Get(sha)
	if !found {
		return verbError(ErrCodeNotFound, "Fact not found", "use a fact id returned by recall"), true, nil
	}
	if record.Payload.Visibility != store.MemoryVisibilityWorld {
		return verbError(ErrCodeScopeDenied, "Fact is outside the remote scope", "manage private facts through a local interface"), true, nil
	}
	reason := req.Reason
	mw := h.deps.memoryWriter()
	result, err := mw.Forget(sha, reason, now)
	if err != nil {
		if errors.Is(err, writer.ErrMemoryFactNotFound) {
			return verbError(ErrCodeNotFound, fmt.Sprintf("no fact with id %q", id), "ids come from remember/recall (facts[].fact_id). recall the entity first to find the right fact"), true, nil
		}
		return nil, false, fmt.Errorf("forget: %w", err)
	}

	resp := forgetResponse{ProtocolVersion: ProtocolVersion, ID: id, Expired: result.Expired}
	if reason != "" {
		resp.Reason = &reason
	}
	return resp, false, nil
}
