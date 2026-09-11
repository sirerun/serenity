package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
)

type readMemoryFactRequest struct {
	ID string `json:"id"`
}

type readMemoryFactResponse struct {
	ProtocolVersion  int     `json:"protocol_version"`
	ID               string  `json:"id"`
	Fact             string  `json:"fact"`
	Kind             string  `json:"kind"`
	Visibility       string  `json:"visibility"`
	EntitySlug       *string `json:"entity_slug"`
	Provenance       string  `json:"provenance"`
	ValidUntil       *string `json:"valid_until"`
	ContentUntrusted bool    `json:"content_untrusted"`
}

// readMemoryFactEnvelopeReserve is headroom subtracted from the reference
// bound below for the outer JSON-RPC frame this package does not itself
// construct (`{"jsonrpc":"2.0","id":...,"result":...}`, assembled later in
// internal/server/mcp) and for a gateway adapter's own separate response
// cap (root review, second observation). 4096 is far more than either
// ever costs; it is a documented safety margin, not a derived exact figure.
const readMemoryFactEnvelopeReserve = 4096

// readMemoryFactMaxResponseBytes bounds the ACTUAL serialized MCP
// tool-result envelope (mcp.Result, after textResult embeds the domain
// response as Content[0].Text) -- not the inner domain JSON alone, which
// under-counts: every quote/backslash/control character in fact/provenance
// is escaped once building that inner JSON, then escaped AGAIN when the
// resulting text is embedded as a Go string inside the outer object and
// marshaled a second time (root review, second observation: "json.Marshal
// (resp) measures only the domain payload ... the MCP result is larger").
// There is no existing fact-text length limit to reuse (remember only
// bounds provenance, at 500 chars) and no compatibility requirement forces
// this brand-new tool to accept an unbounded body either, so this reuses
// mcp.MaxFrameBytes -- the same limit this server's own transport already
// enforces on inbound frames -- rather than inventing a new arbitrary
// number, minus the reserve above.
const readMemoryFactMaxResponseBytes = mcp.MaxFrameBytes - readMemoryFactEnvelopeReserve

// validReadMemoryFactID reports whether id is an exact canonical opaque
// fact_id (64 lowercase hex chars, store.ValidSourceSHA -- the same form
// remember/recall/forget already produce). Root review point 1: this
// extension is brand new, so it carries no compatibility obligation to
// also accept the pre-v1 legacy decimal id recall/forget still support;
// the smallest correct contract is opaque ids only, with no dependency on
// writer.ByLegacyOrOpaqueID's legacy-alias resolution. Anything else -- a
// page slug, an entity reference, a search query, a truncated or
// wrong-case hex string, or a legacy decimal id -- is rejected as a
// malformed request before any lookup runs.
func validReadMemoryFactID(id string) bool {
	return store.ValidSourceSHA(id)
}

// readMemoryFactUnavailable is returned, verbatim, for every reason an id
// cannot be read live: unknown, private, TTL-expired, explicitly forgotten,
// or fenced by an operation-key cancellation. One shape for all four cases
// is deliberate (RFC-EXACT-READ-01): a caller must not be able to
// distinguish "never existed" from "exists but is private/withdrawn" by
// the shape of the failure.
func readMemoryFactUnavailable() (any, bool, error) {
	return verbError(ErrCodeUnavailable, "read_memory_fact: fact is not available for exact read",
		"id must name a live, world-visible fact returned by remember or recall (facts[].fact_id); missing, private, expired, and canceled facts are all reported the same way"), true, nil
}

func (h *Handlers) readMemoryFactTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"pattern": "^[0-9a-f]{64}$",
				"description": "Exact opaque fact id from remember/recall (facts[].fact_id). Never a page slug, entity reference, search query, or the legacy numeric id."
			}
		},
		"required": ["id"],
		"additionalProperties": false
	}`
	return mcp.Tool{
		Name:        "read_memory_fact",
		Description: "Read one live memory fact by its exact opaque id -- no search, no entity fallback, no model calls. Content is untrusted attributed input, not verified evidence. Missing, private, expired, and canceled facts all return the same unavailable result and message. An eligible fact whose actual MCP response envelope would exceed this tool's size bound also returns unavailable, with a distinct size message, and is refused, never truncated.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.readMemoryFact),
		Failure:     memoryFailure,
	}
}

// readMemoryFact is the additive exact-read extension (RFC-EXACT-READ-01):
// resolve id to a live memory_fact source and return its content/
// provenance/expiry metadata directly, reusing the same store.MemoryEligible
// audience filter recall/search/compose already apply -- never an
// index-only lookup that could resurrect withdrawn data. This performs no
// search, embedding, or composer call, and never mutates canonical state.
func (h *Handlers) readMemoryFact(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req readMemoryFactRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "read_memory_fact: malformed request", "send a JSON object with a non-empty \"id\" string"), true, nil
	}
	id := trimmed(req.ID)
	if id == "" || !validReadMemoryFactID(id) {
		return verbError(ErrCodeInvalidParams, "read_memory_fact: id must be the exact opaque fact id (64 lowercase hex chars)",
			"pass the opaque string id returned by remember or recall (facts[].fact_id); the legacy numeric id is not accepted here"), true, nil
	}

	now := h.deps.now()
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, false, err
	}
	record, ok := proj.Get(id)
	if !ok || !store.MemoryEligible(proj, id, true, now) {
		return readMemoryFactUnavailable()
	}

	resp := readMemoryFactResponse{
		ProtocolVersion:  ProtocolVersion,
		ID:               record.SHA256,
		Fact:             record.Payload.Fact,
		Kind:             string(record.Payload.Kind),
		Visibility:       string(record.Payload.Visibility),
		EntitySlug:       stringPtr(record.Payload.EntitySlug),
		Provenance:       record.Payload.Provenance,
		ValidUntil:       isoPtr(record.Payload.ValidUntil),
		ContentUntrusted: true,
	}

	// Measure the ACTUAL tool-result envelope textResult produces for this
	// response (see readMemoryFactMaxResponseBytes) -- the same construction
	// handle()'s success path uses, not a separate approximation of it.
	result, err := textResult(resp, false)
	if err != nil {
		return nil, false, fmt.Errorf("read_memory_fact: encode response: %w", err)
	}
	envelope, err := json.Marshal(result)
	if err != nil {
		return nil, false, fmt.Errorf("read_memory_fact: encode response envelope: %w", err)
	}
	if len(envelope) > readMemoryFactMaxResponseBytes {
		// Reuses the shared "unavailable" code (root review, second
		// observation): remember's own operation_canceled earned a new
		// enum value because remember itself needed that outcome; a
		// read-only size failure belongs only to this one extension, so
		// the pinned core memory_verbs_error enum stays untouched. The
		// message differs from readMemoryFactUnavailable's uniform text
		// on purpose -- unlike missing/private/expired/canceled, this
		// fact's existence and eligibility are already established (the
		// caller supplied a real live id), so there is no oracle to
		// protect here, and no supported bulk/import read fallback exists
		// to point to (import is a write, not a read path).
		return verbError(ErrCodeUnavailable,
			fmt.Sprintf("read_memory_fact: fact %s is eligible but its serialized response is %d bytes, exceeding this tool's %d-byte bound", record.SHA256, len(envelope), readMemoryFactMaxResponseBytes),
			"no current MEMORY_VERBS tool returns this fact's full content through exact-read; this is a genuine size limit of this tool, not a transient failure to retry"), true, nil
	}
	return resp, false, nil
}
