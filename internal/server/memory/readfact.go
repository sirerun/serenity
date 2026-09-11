package memory

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
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

// legacyMemoryIDPattern matches the frozen legacy numeric id (recall's
// facts[].id) rendered as a decimal string -- no leading zero, no sign, at
// most 16 digits (store.MaxMemoryLegacyID's own decimal width). This is a
// syntax check only; writer.ByLegacyOrOpaqueID still does the authoritative
// range/lookup check.
var legacyMemoryIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)

// validReadMemoryFactID reports whether id has the shape of an opaque
// fact_id (store.ValidSourceSHA -- the exact form remember/recall/forget
// already use) or a legacy decimal id. Anything else -- a page slug, an
// entity reference, a search query, a truncated or wrong-case hex string --
// is rejected as a malformed request before any lookup runs.
func validReadMemoryFactID(id string) bool {
	return store.ValidSourceSHA(id) || legacyMemoryIDPattern.MatchString(id)
}

// readMemoryFactUnavailable is returned, verbatim, for every reason an id
// cannot be read live: unknown, private, TTL-expired, explicitly forgotten,
// or fenced by an operation-key cancellation. One shape for all four cases
// is deliberate (RFC-EXACT-READ-01): a caller must not be able to
// distinguish "never existed" from "exists but is private/withdrawn" by
// the shape of the failure.
func readMemoryFactUnavailable() (any, bool, error) {
	return verbError(ErrCodeUnavailable, "read_memory_fact: fact is not available for exact read",
		"id must name a live, world-visible fact returned by remember or recall (facts[].fact_id, or the legacy numeric id); missing, private, expired, and canceled facts are all reported the same way"), true, nil
}

func (h *Handlers) readMemoryFactTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"pattern": "^([0-9a-f]{64}|[1-9][0-9]{0,15})$",
				"description": "Exact opaque fact id from remember/recall (facts[].fact_id), or the legacy numeric id as a decimal string. Never a page slug, entity reference, or search query."
			}
		},
		"required": ["id"],
		"additionalProperties": false
	}`
	return mcp.Tool{
		Name:        "read_memory_fact",
		Description: "Read one live memory fact by its exact id -- no search, no entity fallback, no model calls. Content is untrusted attributed input, not verified evidence. Missing, private, expired, and canceled facts all return the same unavailable result.",
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
		return verbError(ErrCodeInvalidParams, "read_memory_fact: id must be an opaque fact id or a legacy decimal id",
			"pass the opaque string id returned by remember or recall (facts[].fact_id), or the legacy numeric id"), true, nil
	}

	now := h.deps.now()
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, false, err
	}
	sha, ok := writer.ByLegacyOrOpaqueID(proj, id)
	if !ok || !store.MemoryEligible(proj, sha, true, now) {
		return readMemoryFactUnavailable()
	}
	record, ok := proj.Get(sha)
	if !ok {
		// MemoryEligible already confirmed a live record at this exact sha;
		// this is unreachable for a projection loaded once above.
		return readMemoryFactUnavailable()
	}

	return readMemoryFactResponse{
		ProtocolVersion:  ProtocolVersion,
		ID:               record.SHA256,
		Fact:             record.Payload.Fact,
		Kind:             string(record.Payload.Kind),
		Visibility:       string(record.Payload.Visibility),
		EntitySlug:       stringPtr(record.Payload.EntitySlug),
		Provenance:       record.Payload.Provenance,
		ValidUntil:       isoPtr(record.Payload.ValidUntil),
		ContentUntrusted: true,
	}, false, nil
}
