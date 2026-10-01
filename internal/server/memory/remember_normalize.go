package memory

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/store"
)

// NormalizedRememberRequest is the validated semantic input shared by the
// remember handler and hosted operation replay checks.
type NormalizedRememberRequest struct {
	OperationKey string
	Fact         string
	Provenance   string
	EntityType   string
	EntitySlug   string
	Kind         store.MemoryFactKind
	Visibility   store.MemoryVisibility
	ValidUntil   *time.Time
}

// NormalizeRememberRequest performs the remember handler's input validation
// and defaults without writing. Callers that must inspect an idempotent replay
// can use the same normalized values and error response as the handler.
func NormalizeRememberRequest(args json.RawMessage, now time.Time) (NormalizedRememberRequest, VerbError, bool) {
	var req rememberRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: malformed request", "send a JSON object with \"fact\" and \"provenance\" strings"), true
	}
	if !store.ValidMemoryOperationKey(req.OperationKey) {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: invalid operation_key", "use at most 128 ASCII letters, digits, dot, colon, underscore or hyphen"), true
	}
	if req.OperationKey != "" && ttlDurationPattern.MatchString(req.TTL) {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: keyed TTL must be absolute", "use a fixed ISO 8601 timestamp or omit ttl so retries do not move expiry"), true
	}
	fact := req.Fact
	if trimmed(fact) == "" {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: fact must be a non-empty string", "pass the claim to remember, e.g. fact: \"picked Stripe over Adyen -- onboarding speed\""), true
	}
	provenance := req.Provenance
	if trimmed(provenance) == "" {
		return NormalizedRememberRequest{}, verbError(ErrCodeProvenanceRequired, "remember: provenance is required and must be non-empty", "pass where the fact came from, e.g. provenance: \"user told me, 2026-06-12\" or \"import: notes.md\""), true
	}
	if utf8.RuneCountInString(provenance) > provenanceMaxChars {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: provenance exceeds %d chars (got %d)", provenanceMaxChars, utf8.RuneCountInString(provenance)), "shorten the attribution -- provenance is a pointer, not a transcript"), true
	}
	kind := req.Kind
	if kind == "" {
		kind = string(store.MemoryFactKindFact)
	}
	if !store.ValidMemoryFactKind(kind) {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: kind %q is not a fact kind", kind), "use one of: event | preference | commitment | belief | fact"), true
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = string(store.MemoryVisibilityWorld)
	}
	if visibility != string(store.MemoryVisibilityWorld) && visibility != string(store.MemoryVisibilityPrivate) {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, fmt.Sprintf("remember: visibility %q is not valid", visibility), "use \"world\" (default -- agents can recall it) or \"private\" (local CLI reads only)"), true
	}
	validUntil, err := parseTTL(req.TTL, now)
	if err != nil {
		return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: "+err.Error(), "use duration shorthand (\"30d\", \"12h\", \"45m\") or an absolute ISO 8601 timestamp (\"2026-07-12T00:00:00Z\"), never an ISO-8601 duration like \"P30D\""), true
	}
	var entitySlug, entityType string
	if req.Entity != "" {
		var ok bool
		entityType, entitySlug, ok = canonicalEntityRef(req.Entity)
		if !ok {
			return NormalizedRememberRequest{}, verbError(ErrCodeInvalidParams, "remember: entity is not a valid reference", "pass a plain name or a \"type/slug\" reference with no path separators beyond the one splitting them"), true
		}
	}
	return NormalizedRememberRequest{
		OperationKey: req.OperationKey,
		Fact:         fact,
		Provenance:   provenance,
		EntityType:   entityType,
		EntitySlug:   entitySlug,
		Kind:         store.MemoryFactKind(kind),
		Visibility:   store.MemoryVisibility(visibility),
		ValidUntil:   validUntil,
	}, VerbError{}, false
}

// RememberOperationConflict is the stable domain response for reusing a key
// with a different normalized remember payload.
func RememberOperationConflict() VerbError {
	return verbError(ErrCodeOperationConflict, "remember: operation_key already has different input", "retry the original payload; use a new key only for a genuinely new operation")
}
