// Package memory implements MEMORY_VERBS v1 (T4.20 repair) over MCP
// (internal/server/mcp, T4.2): recall, remember, entity, synthesize,
// forget -- Serenity as a conformant MEMORY_VERBS v1 server against the
// FROZEN contract at dndungu/gbrain@d35c9c9e441e
// (docs/protocol/MEMORY_VERBS_v1.md, testdata/pinned-response-schemas.json
// and testdata/memory-cases-upstream.json, vendored under this package's
// own testdata/), superseding T4.5's own good-faith but wrong reading of
// an RFC field list that never named these exact shapes. Every wire type
// in this package (request AND response) is read directly off the pinned
// TypeScript operation catalog and response registry, not off RFC 0001
// §8.1's own prose or this package's earlier Envelope guess -- see
// memory-compat-mapping.md's architecture note for why that guess could
// not be patched in place (Claim/Observation/Source's own existing
// authority layers each rejected fitting an arbitrary attributed fact into
// them unchanged) and had to be replaced with a genuine, if narrow, new
// persistence seam instead (memoryfact.go in internal/store and
// internal/writer).
//
// Every response, success or error, carries integer protocol_version at
// the TOP level, never nested. A domain error is a FLAT VerbError value in
// place of the verb's own success response type -- {error, message,
// suggestion, detail?, protocol_version} -- never wrapped inside the
// success shape's own fields (see VerbError's own doc comment).
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ProtocolVersion is MEMORY_VERBS v1's integer protocol_version.
const ProtocolVersion = 1

// Clock is the same single-method seam internal/cron.Clock,
// internal/events.Clock, and internal/server/disposition.Clock use.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Error codes -- upstream-verbs.ts's ERROR_SCHEMA enum, verbatim.
// ErrCodeBudgetUnsatisfiable is schema-listed but reserved: this package
// never emits it (this task's own scope has no budget-infeasibility case
// to report -- recall's own tiny-budget behavior is "empty facts plus
// accurate drops," never this code).
const (
	ErrCodeInvalidParams       = "invalid_params"
	ErrCodeProvenanceRequired  = "provenance_required"
	ErrCodeNotFound            = "not_found"
	ErrCodeScopeDenied         = "scope_denied"
	ErrCodeUnavailable         = "unavailable"
	ErrCodeBudgetUnsatisfiable = "budget_unsatisfiable" // reserved -- never emitted
	ErrCodeInternal            = "internal"
)

// VerbError is MEMORY_VERBS v1's uniform error envelope (upstream-verbs.ts
// ERROR_SCHEMA): every field flat at the top level of the JSON object this
// package returns in place of a verb's own success response type whenever
// a call fails at the protocol layer. Suggestion is always populated
// (task acc line: "malformed inputs and internal failures emit versioned
// string-code errors with suggestions").
type VerbError struct {
	ProtocolVersion int    `json:"protocol_version"`
	Error           string `json:"error"`
	Message         string `json:"message"`
	Suggestion      string `json:"suggestion"`
	Detail          string `json:"detail,omitempty"`
}

func verbError(code, message, suggestion string) VerbError {
	return VerbError{ProtocolVersion: ProtocolVersion, Error: code, Message: message, Suggestion: suggestion}
}

func verbErrorDetail(code, message, suggestion, detail string) VerbError {
	e := verbError(code, message, suggestion)
	e.Detail = detail
	return e
}

// globEntityPage finds a fence-tier entity page by slug alone.
// store.FenceWriter has no such lookup (PathFor needs the type folder up
// front) -- this globs brain/entities/*/<slug>.md directly, the same
// shape internal/compose.AllClaims's own page glob uses, narrowed to one
// slug. Zero matches is a normal outcome (no error).
func globEntityPage(root, slug string) ([]string, error) {
	return filepath.Glob(filepath.Join(root, "brain", "entities", "*", slug+".md"))
}

// validSlug reports whether slug is safe to use as a single path segment
// (T4.11): store.FenceWriter.PathFor/store.ShardStore.PathFor both plain
// filepath.Join it in with no sanitization of their own, and
// globEntityPage globs the same pattern directly, so any '/' or '\', or
// the exact traversal segments "." or "..", are rejected before ever
// reaching a store call.
func validSlug(slug string) bool {
	if slug == "" || slug == "." || slug == ".." {
		return false
	}
	return !strings.ContainsAny(slug, "/\\")
}

var slugifyNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify canonicalizes free text into a single safe path segment: lower-
// case, non-alphanumeric runs collapsed to one hyphen, leading/trailing
// hyphens trimmed. Used for the free-text half of canonicalEntityRef
// (a name with no "type/slug" shape) -- never for a value that already
// looks like an existing slug, which is validated via validSlug instead of
// re-derived.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugifyNonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// defaultEntityType is the fence-tier type bucket a free-text entity
// reference (no "type/slug" shape) canonicalizes into -- this repo's own
// brain/entities/<type>/<slug>.md layout requires SOME type folder, and
// upstream's own contract does not name one for a bare "Alice Example"-
// shaped reference, so this task picks a single, disclosed generic bucket
// rather than guessing a domain-specific one.
const defaultEntityType = "entity"

// canonicalEntityRef parses a MEMORY_VERBS entity reference (remember's
// entity param, entity()'s name param) into this repo's existing fence-tier
// (type, slug) pair -- brain/entities/<type>/<slug>.md. A "type/slug"-
// shaped reference (this repo's own existing convention, and the one every
// pinned fixture uses, e.g. "people/conformance-<marker>") maps directly,
// after validating each segment as a safe single path component (T4.11's
// validSlug posture extended to free-text names, memory-compat-mapping.md
// coordinator refinement #7: "resolve names safely without globs/path
// interpolation"). Free text with no "/" is canonicalized into a slug
// under defaultEntityType. Malformed refs -- more than one "/", an empty
// segment, a traversal-shaped segment -- return ok=false rather than ever
// building a filesystem path from unsanitized input.
func canonicalEntityRef(name string) (entityType, slug string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			return "", "", false
		}
		t, s := parts[0], parts[1]
		if !validSlug(t) || !validSlug(s) {
			return "", "", false
		}
		return t, s, true
	}
	s := slugify(name)
	if !validSlug(s) {
		return "", "", false
	}
	return defaultEntityType, s, true
}

// ttlDurationPattern matches the pinned shorthand -- "30d", "12h", "45m" --
// a bare positive integer plus exactly one of the three unit letters
// upstream-verbs.ts names.
var ttlDurationPattern = regexp.MustCompile(`^(\d+)(d|h|m)$`)

// parseTTL parses remember's ttl param (upstream-verbs.ts's parseTtlParam):
// duration shorthand or an absolute ISO 8601 timestamp; empty means never
// expires (nil, nil). ISO-8601 DURATIONS ("P30D") are rejected with a
// self-correcting suggestion naming the actual accepted shapes -- this
// task's own named "P30D trap" acc case.
func parseTTL(ttl string, now time.Time) (*time.Time, error) {
	if ttl == "" {
		return nil, nil
	}
	if strings.HasPrefix(ttl, "P") || strings.HasPrefix(ttl, "p") {
		return nil, fmt.Errorf("ISO-8601 durations like %q are not accepted", ttl)
	}
	if m := ttlDurationPattern.FindStringSubmatch(ttl); m != nil {
		var n int
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		var d time.Duration
		switch m[2] {
		case "d":
			d = time.Duration(n) * 24 * time.Hour
		case "h":
			d = time.Duration(n) * time.Hour
		case "m":
			d = time.Duration(n) * time.Minute
		}
		t := now.Add(d)
		return &t, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, ttl); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("not a recognized duration (\"30d\", \"12h\", \"45m\") or ISO 8601 timestamp")
}

// parseSinceUntil parses recall/synthesize's since/until params -- ISO
// 8601 date or datetime, per RFC. Empty is the zero time (unbounded).
func parseSinceUntil(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("not a recognized ISO 8601 date or datetime")
}

// Deps is everything Handlers needs, built once by the caller (production:
// internal/cli/serve.go, opening the brain root MEMORY_VERBS serves) and
// held for the life of the MCP session.
type Deps struct {
	Root   string
	Config *config.Config

	// Index is the derived index (search.Store, *index.SQLite in
	// production): chunk-level retrieval for recall/synthesize's search
	// arm, and the memory-fact chunks Rebuild indexes (internal/index/
	// rebuild.go, T4.20).
	Index *index.SQLite
	// Embedder is nil when no embedding model is pinned/credentialed --
	// recall/synthesize then degrade to FTS-only relevance (recall's own
	// search_degraded field), never an error.
	Embedder embed.Embedder

	// Composer is nil under the same explicit-skip contract
	// providers.BuildComposerRouter documents; synthesize then returns
	// VerbError{Error: "unavailable"} with ComposerUnavailableNote as the
	// message and a populated suggestion, never a silent no-op.
	Composer compose.Completer
	// ComposerModelVersion is the pinned composer model's version string.
	ComposerModelVersion string
	// ComposerUnavailableNote is the exact note
	// providers.BuildComposerRouter returned when Composer is nil.
	ComposerUnavailableNote string

	// Queue/Sources are the deterministic writer's own entry points for
	// MEMORY_VERBS's fact/expiry sources (internal/writer.MemoryFact,
	// T4.20) -- remember/forget write through them exclusively, never
	// touching store.SourceStore.Write directly.
	Queue   *writer.Queue
	Sources *store.SourceStore
	// Fence/Shard remain for entity()'s own read-side combination of
	// public accepted claim evidence alongside memory-verbs commitments
	// (mapping doc: "Entity cards may combine public accepted evidence
	// with public memory commitments while keeping origin distinction
	// internally").
	Fence *store.FenceWriter
	Shard *store.ShardStore

	// Clock is real time in production; tests inject a fixed Clock so
	// CreatedAt/ExpiredAt are deterministic.
	Clock Clock
}

func (d Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now()
	}
	return d.Clock.Now()
}

func (d Deps) memoryWriter() *writer.MemoryFact {
	return &writer.MemoryFact{Queue: d.Queue, Sources: d.Sources}
}

// Handlers implements MEMORY_VERBS v1's five verbs over Deps.
type Handlers struct {
	deps Deps
}

// New builds Handlers over deps. Root, Config, Index, Queue, Sources,
// Fence, and Shard must be non-nil. Embedder/Composer may be nil (see
// their own field docs).
func New(deps Deps) *Handlers {
	if deps.Clock == nil {
		deps.Clock = realClock{}
	}
	return &Handlers{deps: deps}
}

// Tools returns the five MEMORY_VERBS v1 tool registrations, ready to pass
// to mcp.New alongside any other domain tools a future task adds.
func (h *Handlers) Tools() []mcp.Tool {
	return []mcp.Tool{
		h.recallTool(),
		h.rememberTool(),
		h.entityTool(),
		h.synthesizeTool(),
		h.forgetTool(),
	}
}

// textResult renders resp (a verb's own response type, or a VerbError) as
// one MCP text content block (internal/server/mcp.Result documents
// "domain envelopes can be serialized in Text"). isError marks a protocol-
// layer failure -- MCP's own Result.IsError -- set together with a
// VerbError body so a generic MCP client that only understands IsError
// still sees a failed call, and a MEMORY_VERBS-aware client reads the
// flat error/message/suggestion fields for the code and fix.
func textResult(resp any, isError bool) (mcp.Result, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(data)}}, IsError: isError}, nil
}

// verbFunc adapts a typed (ctx, request) -> (response, isError, error)
// function into the mcp.Tool.Handler shape: response is always either the
// verb's own success type or a VerbError. A non-nil err is this package's
// own bug (JSON marshal failure), never a domain outcome -- every domain
// failure is reported via the response's own VerbError/isError rather than
// a Go error.
type verbFunc func(ctx context.Context, args json.RawMessage) (resp any, isError bool, err error)

func handle(fn verbFunc) func(context.Context, json.RawMessage) (mcp.Result, error) {
	return func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
		resp, isError, err := fn(ctx, args)
		if err != nil {
			return mcp.Result{}, err
		}
		return textResult(resp, isError)
	}
}

// stringPtr/isoPtr are small nil-vs-value helpers used across every verb's
// response builder to echo "omitted optional inputs as explicit null,
// never absent" (remember's own pinned requirement, applied consistently
// across every nullable field in this package).
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isoPtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func trimmed(s string) string { return strings.TrimSpace(s) }
