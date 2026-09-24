package memory

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type cancelOperationRequest struct {
	OperationKey string `json:"operation_key"`
	Reason       string `json:"reason,omitempty"`
}
type cancelOperationResponse struct {
	ProtocolVersion int    `json:"protocol_version"`
	OperationKey    string `json:"operation_key"`
	Canceled        bool   `json:"canceled"`
	ID              string `json:"id"`
	Expired         bool   `json:"expired"`
}

// ExtensionTools are additive Serenity capabilities, separate from the pinned
// five-verb MEMORY_VERBS v1 surface returned by Tools.
func (h *Handlers) ExtensionTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "cancel_memory_operation", Description: "Durably cancel a memory operation key, including a missing or unacknowledged write. Never replay a revoked body to recover its ID.", InputSchema: json.RawMessage(`{"type":"object","properties":{"operation_key":{"type":"string","minLength":1,"maxLength":128,"pattern":"^[A-Za-z0-9_.:-]+$"},"reason":{"type":"string"}},"required":["operation_key"],"additionalProperties":false}`), Handler: handle(h.cancelOperation), Failure: memoryFailure},
		h.readMemoryFactTool(),
	}
}

func (h *Handlers) cancelOperation(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req cancelOperationRequest
	if err := json.Unmarshal(args, &req); err != nil || req.OperationKey == "" || !store.ValidMemoryOperationKey(req.OperationKey) {
		return verbError(ErrCodeInvalidParams, "cancel_memory_operation: a valid operation_key is required", "use the original immutable request key, at most 128 ASCII letters, digits, dot, colon, underscore or hyphen"), true, nil
	}
	result, err := h.deps.memoryWriter().CancelRemoteOperation(req.OperationKey, req.Reason, h.deps.now())
	if errors.Is(err, writer.ErrMemoryScopeDenied) {
		return verbError(ErrCodeScopeDenied, "Fact is outside the remote scope", "manage private facts through a local interface"), true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cancelOperationResponse{ProtocolVersion: ProtocolVersion, OperationKey: req.OperationKey, Canceled: true, ID: result.Record.SHA256, Expired: result.Expired}, false, nil
}
