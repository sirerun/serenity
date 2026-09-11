// Package memory implements the five MEMORY_VERBS v1 tools over canonical sources.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	ErrCodeOperationConflict   = "operation_conflict"
	ErrCodeOperationCanceled   = "operation_canceled"
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

// globEntityPage finds literal page basenames without interpreting user text as
// a glob or following links outside the brain's entity directories.
func globEntityPage(root, slug string) ([]string, error) {
	if !validSlug(slug) {
		return nil, nil
	}
	base := filepath.Join(root, "brain", "entities")
	for _, dir := range []string{filepath.Join(root, "brain"), base} {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe entity directory")
		}
	}
	types, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	matches := []string{}
	for _, typ := range types {
		if !typ.IsDir() || typ.Type()&os.ModeSymlink != 0 {
			continue
		}
		page := filepath.Join(base, typ.Name(), slug+".md")
		info, err := os.Lstat(page)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			matches = append(matches, page)
		}
	}
	return matches, nil
}

// validSlug accepts one literal path component.
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
		n, err := strconv.ParseInt(m[1], 10, 64)
		unit := time.Minute
		switch m[2] {
		case "d":
			unit = 24 * time.Hour
		case "h":
			unit = time.Hour
		}
		if err != nil || n <= 0 || n > math.MaxInt64/int64(unit) {
			return nil, fmt.Errorf("TTL duration must be positive and within range")
		}
		d := time.Duration(n) * unit
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
	tools := []mcp.Tool{
		h.recallTool(),
		h.rememberTool(),
		h.entityTool(),
		h.synthesizeTool(),
		h.forgetTool(),
	}
	for i := range tools {
		tools[i].Failure = memoryFailure
	}
	return tools
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

// verbFunc adapts a typed operation to the MCP handler. Operational failures are
// converted to a uniform domain error by handle; their internals stay local.
type verbFunc func(ctx context.Context, args json.RawMessage) (resp any, isError bool, err error)

func handle(fn verbFunc) func(context.Context, json.RawMessage) (mcp.Result, error) {
	return func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
		resp, isError, err := fn(ctx, args)
		if err != nil {
			return memoryFailure(mcp.ExecutionFailed), nil
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
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// memoryFailure keeps domain failures machine-readable without exposing internal paths.
func memoryFailure(kind mcp.FailureKind) mcp.Result {
	e := verbError(ErrCodeInternal, "Memory operation failed", "retry the operation; inspect local diagnostics if it persists")
	if kind == mcp.InvalidArguments {
		e = verbError(ErrCodeInvalidParams, "Arguments do not match the tool input schema", "check tools/list for required fields and types")
	}
	result, _ := textResult(e, true)
	return result
}
