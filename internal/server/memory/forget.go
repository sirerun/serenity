package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type hostedForgetPublicationKey struct{}

// WithHostedForgetPublication marks the internal hosted path whose source
// mutation must commit inline before the handler returns. It is never derived
// from tool request data.
func WithHostedForgetPublication(ctx context.Context) context.Context {
	return context.WithValue(ctx, hostedForgetPublicationKey{}, true)
}

func hasHostedForgetPublication(ctx context.Context) bool {
	trusted, _ := ctx.Value(hostedForgetPublicationKey{}).(bool)
	return trusted
}

// ErrCodeForbidden is a Serenity extension to the error enum: the caller is
// authenticated and scoped for forget but is neither the fact's writer nor a
// human actor (AI-L03).
const ErrCodeForbidden = "forbidden"

// ScopeForget is the credential scope forget requires of a non-human
// principal, separate from memory:write so a credential can save facts
// without being able to expire them.
const ScopeForget = "memory:forget"

// LocalWriter is the writer recorded for a remember that carries no
// Principal: the local stdio or CLI path run by the brain's own operator.
const LocalWriter = "local"

// Principal identifies the caller of a verb. A transport that authenticates
// callers (the hosted gateway) attaches one with WithPrincipal; a context
// without one is the local operator path, which keeps its prior behavior.
type Principal struct {
	// ID is the stable identity recorded as a fact's writer, such as a
	// credential id. It must not be reused across distinct clients.
	ID string
	// Human marks an interactive human actor (the account owner), who may
	// forget any accessible fact.
	Human bool
	// Scopes are the credential's granted scopes.
	Scopes []string
}

type principalKey struct{}

// WithPrincipal returns ctx carrying p for the memory verbs.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

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

// forget records an immutable expiry event for an accessible public fact.
// A non-human Principal needs ScopeForget and must be the fact's recorded
// writer; a fact with no recorded writer is forgettable only by a human
// actor or the local path (AI-L03).
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
			var result writer.ForgetResult
			var err error
			if hasHostedForgetPublication(ctx) {
				result, err = h.deps.memoryWriter().ForgetContext(ctx, id, req.Reason, now)
			} else {
				result, err = h.deps.memoryWriter().Forget(id, req.Reason, now)
			}
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
	mw := h.deps.memoryWriter()
	if p, ok := principalFrom(ctx); ok && !p.Human {
		if !slices.Contains(p.Scopes, ScopeForget) {
			return verbError(ErrCodeScopeDenied, "forget requires the "+ScopeForget+" scope", "ask the brain owner for a credential that includes "+ScopeForget+", or forget the fact from the dashboard or local CLI"), true, nil
		}
		writerID, err := mw.FactWriter(sha)
		if err != nil {
			return nil, false, fmt.Errorf("forget: read writer: %w", err)
		}
		if writerID == "" || writerID == LocalWriter || writerID != p.ID {
			return verbError(ErrCodeForbidden, "Fact was written by another principal", "only the client that remembered a fact, or the brain owner, can forget it"), true, nil
		}
	}
	reason := req.Reason
	var result writer.ForgetResult
	var err error
	if hasHostedForgetPublication(ctx) {
		result, err = mw.ForgetContext(ctx, sha, reason, now)
	} else {
		result, err = mw.Forget(sha, reason, now)
	}
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
