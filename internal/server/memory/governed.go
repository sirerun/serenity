package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type governedRetraction struct {
	CommandID string `json:"adapter_command_id"`
	ClaimID   string `json:"claim_id"`
	Version   int64  `json:"version"`
	Reason    string `json:"reason"`
}

func (h *Handlers) retractGovernedMemoryTool() mcp.Tool {
	return mcp.Tool{Name: "retract_governed_memory", Description: "Remove a governed memory from active recall, retaining command history. Use the governed UUID from provenance and exact current version (initially 1). A stable caller command UUID makes retry safe; this does not erase history.", InputSchema: json.RawMessage(`{"type":"object","properties":{"adapter_command_id":{"type":"string"},"claim_id":{"type":"string"},"version":{"type":"integer","minimum":1},"reason":{"type":"string","minLength":1,"maxLength":8192}},"required":["adapter_command_id","claim_id","version","reason"],"additionalProperties":false}`), Handler: handle(h.retractGoverned), Failure: memoryFailure}
}
func (h *Handlers) retractGoverned(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req governedRetraction
	if err := json.Unmarshal(args, &req); err != nil || !store.ValidGovernedUUID(req.CommandID) || !store.ValidGovernedUUID(req.ClaimID) || req.Version < 1 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 8192 {
		return verbError(ErrCodeInvalidParams, "Invalid governed retraction", "Supply claim UUID, version, reason and stable command UUID"), true, nil
	}
	canonical, err := json.Marshal(req)
	if err != nil {
		return nil, false, err
	}
	sum := sha256.Sum256(canonical)
	digest := hex.EncodeToString(sum[:])
	p, err := store.LoadGovernedMemory(h.deps.Sources)
	if err != nil {
		return nil, false, err
	}
	if old, ok := p.Commands[req.CommandID]; ok {
		if old.RequestDigest != digest {
			return RememberOperationConflict(), true, nil
		}
		if err := writer.GovernedMemory(ctx, h.deps.Queue, h.deps.Sources, h.deps.Root, old); err != nil {
			return nil, false, err
		}
		return old.Response, false, nil
	}
	c, ok := p.Claims[req.ClaimID]
	if !ok {
		return verbError(ErrCodeNotFound, "Governed claim unavailable", "Use the UUID from memory provenance"), true, nil
	}
	if c.Version != req.Version {
		return verbError(ErrCodeOperationConflict, "Claim version changed", "Inspect current claim before retrying"), true, nil
	}
	var projectID string
	for _, rec := range p.Commands {
		if rec.Response.Kind == "remember" && len(rec.Response.Claims) == 1 && rec.Response.Claims[0].ID == req.ClaimID {
			projectID = rec.Response.ProjectID
			break
		}
	}
	if !c.Active {
		return verbError(ErrCodeOperationConflict, "Claim is already inactive", "Replay the original command to recover its result; no new tombstone is needed"), true, nil
	}
	if c.Active {
		c.Active = false
		c.Version++
	}
	removed, erased := true, false
	now := h.deps.now().UTC()
	response := store.GovernedResponse{Schema: store.GovernedMemorySchema, ProjectID: projectID, BrainID: p.BrainID, CommandID: req.CommandID, Kind: "retract", Status: "completed", Claims: []store.GovernedClaim{c}, Revisions: []store.GovernedRevision{}, Cost: store.GovernedMoney{Currency: "USD"}, Destinations: []string{}, WriterOwner: p.WriterOwner, LookupAuthoritative: true, ActiveRecallRemoved: &removed, HistoricalErasure: &erased}
	rec := store.GovernedRecord{Schema: store.GovernedMemorySchema, RequestDigest: digest, Response: response, RecordedAt: now, Request: canonical}
	if err := writer.GovernedMemory(ctx, h.deps.Queue, h.deps.Sources, h.deps.Root, rec); err != nil {
		return nil, false, err
	}
	return response, false, nil
}
