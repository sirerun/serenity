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

// forget expires the memory_fact source named by req.ID through a
// canonical, audit-preserving memory_expiry source
// (internal/writer.MemoryFact.Forget, T4.20) -- the original fact is
// never edited or deleted. Works for every id remember ever returned,
// regardless of the chosen canonical representation (the opaque SHA256,
// or the frozen legacy numeric id rendered as a string) -- private ids are
// inaccessible via this call's own not_found path below, since only the
// projection this handler consults ever resolves an id to a target in the
// first place, and forget's own not_found/idempotent semantics apply
// identically either way.
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
		return verbError(ErrCodeNotFound, fmt.Sprintf("no fact with id %q", id), "ids come from remember/recall (facts[].fact_id). recall the entity first to find the right fact"), true, nil
	}

	reason := trimmed(req.Reason)
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
	} else if result.Record.ExpiredReason != "" {
		resp.Reason = &result.Record.ExpiredReason
	}
	return resp, false, nil
}
