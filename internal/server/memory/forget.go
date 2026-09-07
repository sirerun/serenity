package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/supersede"
)

type forgetRequest struct {
	Subject string `json:"subject"`
	ClaimID string `json:"claim_id"`
	Actor   string `json:"actor,omitempty"`
}

type forgetResponse struct {
	Envelope
	// Forgotten is true whenever the named claim is retracted (or already
	// was) once this call returns -- true on a fresh retraction AND on a
	// repeat call against an already-retracted or never-existing claim,
	// which is exactly what makes forget idempotent (this task's own acc
	// line): a caller can retry blindly and always land on the same
	// observed state.
	Forgotten bool `json:"forgotten"`
}

func (h *Handlers) forgetTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"subject": {"type": "string", "description": "The claim's subject slug."},
			"claim_id": {"type": "string", "description": "The claim's id, as returned by remember/recall/entity/synthesize."},
			"actor": {"type": "string", "description": "Who is forgetting this (defaults to \"mcp\")."}
		},
		"required": ["subject", "claim_id"]
	}`
	return mcp.Tool{
		Name:        "forget",
		Description: "Retract one claim by id. Idempotent: forgetting an already-forgotten or unknown claim succeeds with no effect.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.forget),
	}
}

// forget locates the named claim within its subject's own shard/fence
// data (bounded to one entity, not a repo-wide walk -- the request
// requires subject alongside claim_id for exactly this reason) and, for a
// still-live shard-tier claim, retracts it via
// internal/supersede.Writer.Retract (T2.21) -- the same right-to-forget
// primitive TombstoneCascade's accept side already uses, reusing its
// "reuses target's own id" tombstone-append semantics unchanged.
//
// Idempotent by construction, not by luck: a claim that is not found, or
// found but already State=Retracted, returns Forgotten:true and writes
// nothing -- Retract is called at most once per genuinely-live claim.
//
// Disclosed, not built here: fence-tier retraction. T2.21's own package
// doc names why -- "a fence-tier retraction would be a smaller, different
// operation (an in-place row edit, since the file is already truth
// there)" -- that this task does not build either, inheriting the exact
// same, already-accepted gap rather than re-deciding it. A fence-tier
// claim_id returns VerbError{Code: "unsupported_tier"} with a suggestion
// naming the limitation, never a crash or a silent no-op.
func (h *Handlers) forget(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req forgetRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return forgetResponse{Envelope: errorEnvelope("invalid_argument", "forget: malformed request", "send a JSON object with non-empty \"subject\" and \"claim_id\" strings")}, true, nil
	}
	if req.Subject == "" || req.ClaimID == "" {
		return forgetResponse{Envelope: errorEnvelope("invalid_argument", "forget: subject and claim_id are both required", "pass the claim's subject slug and its claim_id")}, true, nil
	}
	if !validSlug(req.Subject) {
		return forgetResponse{Envelope: errorEnvelope("invalid_argument", "forget: subject must be a single path segment", "pass a subject slug with no \"/\", \"\\\\\", \".\", or \"..\" -- e.g. \"acme-corp\", not a path")}, true, nil
	}
	actor := req.Actor
	if actor == "" {
		actor = "mcp"
	}

	target, tier, found, err := h.findClaim(req.Subject, req.ClaimID)
	if err != nil {
		return nil, false, fmt.Errorf("forget: %w", err)
	}
	if !found || target.State == domain.StateRetracted {
		return forgetResponse{Envelope: newEnvelope(), Forgotten: true}, false, nil
	}
	if tier != domain.TierShard {
		return forgetResponse{Envelope: errorEnvelope("unsupported_tier", "forget: this claim is fence-tier", "fence-tier retraction is not supported yet (T2.21's own disclosed gap) -- hand-edit the entity page directly and commit the change")}, true, nil
	}

	sw := supersede.New(h.deps.Queue, h.deps.Fence, h.deps.Shard, h.deps.Config)
	if _, err := sw.Retract(target, actor, h.deps.now()); err != nil {
		return nil, false, fmt.Errorf("forget: retract: %w", err)
	}

	env := newEnvelope()
	env.Evidence = []Fact{factOfClaim(target)}
	return forgetResponse{Envelope: env, Forgotten: true}, false, nil
}

// findClaim looks up claimID among subject's own claims, across both
// tiers -- fence-tier claims from the entity page (if it exists), shard-
// tier claims from every family internal/store.ShardStore.Families
// reports for subject. Returns found=false, not an error, when nothing
// matches (a never-existing id is one of the two idempotent cases forget
// documents).
func (h *Handlers) findClaim(subject, claimID string) (claim domain.Claim, tier domain.Tier, found bool, err error) {
	matches, err := globEntityPage(h.deps.Root, subject)
	if err != nil {
		return domain.Claim{}, "", false, err
	}
	if len(matches) > 0 {
		page, err := h.deps.Fence.ParseEntity(matches[0])
		if err == nil {
			for _, c := range page.Claims {
				if c.ID == claimID {
					return c, domain.TierFence, true, nil
				}
			}
		}
	}

	families, err := h.deps.Shard.Families(subject)
	if err != nil {
		return domain.Claim{}, "", false, err
	}
	for _, family := range families {
		lines, err := h.deps.Shard.Lines(subject, family)
		if err != nil {
			return domain.Claim{}, "", false, err
		}
		for _, c := range lines {
			if c.ID == claimID {
				// A retracted line reuses its target's id (T2.21), so the
				// most recent line with this id -- last in append order --
				// reflects the claim's true current state.
				claim, found = c, true
			}
		}
		if found {
			return claim, domain.TierShard, true, nil
		}
	}
	return domain.Claim{}, "", false, nil
}
