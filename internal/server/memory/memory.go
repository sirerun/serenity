// Package memory implements MEMORY_VERBS v1 (RFC 0001 §8.1, T4.5) over MCP
// (internal/server/mcp, T4.2): recall, remember, entity, synthesize, forget
// -- "Serenity is a conformant MEMORY_VERBS v1 server ... over MCP", not
// HTTP, unlike DISPOSITION (T4.4) and DIRECTION (T4.6), which both ride the
// wire protocols RFC §8.2/§8.3 leave HTTP-shaped and both stayed
// package-complete/not-live-wired. This package's five verbs ARE wired
// live: Tools() returns the exact []mcp.Tool registrations
// internal/cli/serve.go passes to mcp.New, replacing that command's
// previous mcp.New(Version, nil).
//
// Adds no domain logic beyond what wiring five verbs onto already-shipped
// packages requires, the same "no privileged internal path" discipline
// internal/server/disposition and internal/server/direction both state:
// recall wraps internal/search.Search (T1.11) exactly as `serenity
// search` does; synthesize wraps internal/compose.Composer.Ask (T1.12)
// exactly as `serenity ask` does; remember and forget are new write paths
// -- MEMORY_VERBS has no existing CLI verb for either -- built directly on
// the same writer-queue entry points (internal/writer.Shard/Fence) and
// tier-routing (config.TierOf) internal/ingest.Writer already established
// for "commit a domain.Claim to its fence or shard", plus
// internal/reconcile (T2.2) and internal/supersede's tombstone/retract
// path (T2.21) for remember/forget's own semantics respectively (see
// remember.go and forget.go).
//
// The gbrain envelope (protocol_version, evidence, provenance, budget
// meta, cost, an enumerated error with a populated suggestion) is not
// vendored in this repo yet -- that is T4.13's job, "vendored gbrain
// memory-verbs cases.json" under testdata/conformance/, which has not
// shipped. Envelope is this package's own literal reading of RFC 0001
// §8.1's named field list and docs/plans/E4-m4-serve-protocols.md's design
// anchors ("gbrain MEMORY_VERBS_v1.md at dndungu/gbrain@d35c9c9e441e --
// five verbs, uniform error envelope with populated suggestion, integer
// protocol_version"), documented per-field below rather than waiting on a
// schema that does not exist in this repo -- the same disclosed-scope
// precedent T4.4/T4.6 both set for their own RFC-silent wire shapes. Once
// T4.13 vendors the real schema, T4.14's conformance job is what proves
// (or disproves) this reading; nothing here claims certified conformance
// today.
package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ProtocolVersion is MEMORY_VERBS v1's integer protocol_version (ADR 010:
// "Protocol versioning is the MEMORY_VERBS policy already adopted by the
// RFC: an integer protocol_version on every response").
const ProtocolVersion = 1

// Clock is the same single-method seam internal/cron.Clock,
// internal/events.Clock, and internal/server/disposition.Clock use.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Fact is one piece of evidence a verb response returns. It carries both
// shapes this package's five verbs produce evidence in, each verb
// populating only the fields that apply to it (the rest stay their zero
// value, omitted from JSON):
//
//   - Chunk-shaped (recall, over internal/search's chunk-level retrieval):
//     ChunkRef/EntitySlug/Kind/Text/Score.
//   - Claim-shaped (entity, remember, synthesize's citations): Subject/
//     Predicate/Object/ClaimID/SupersededBy/Confidence, RFC 0001 §8.1's own
//     "Serenity-specific additions ride as optional fields: per-fact
//     confidence, claim_id, superseded_by".
//
// Provenance rides inline per fact rather than as a second top-level
// envelope array -- the RFC's own field list ("evidence, provenance") is
// read here as one concept (an evidence item's provenance), not two
// independent lists that would otherwise need a join key neither RFC 0001
// §8.1 nor the design-anchors line names.
type Fact struct {
	// Chunk-shaped fields (recall).
	ChunkRef   string  `json:"chunk_ref,omitempty"`
	EntitySlug string  `json:"entity_slug,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Text       string  `json:"text,omitempty"`
	Score      float64 `json:"score,omitempty"`

	// Claim-shaped fields (entity, remember, synthesize citations).
	Subject      string   `json:"subject,omitempty"`
	Predicate    string   `json:"predicate,omitempty"`
	Object       string   `json:"object,omitempty"`
	ClaimID      string   `json:"claim_id,omitempty"`
	SupersededBy string   `json:"superseded_by,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`

	SourceRef  string         `json:"source_ref,omitempty"`
	Provenance FactProvenance `json:"provenance,omitzero"`
}

// FactProvenance is one fact's provenance (RFC §7.5), the wire shape of
// domain.Provenance.
type FactProvenance struct {
	SourceSHA256 string    `json:"source_sha256,omitempty"`
	Span         string    `json:"span,omitempty"`
	Model        string    `json:"model,omitempty"`
	ObservedAt   time.Time `json:"observed_at,omitzero"`
	Actor        string    `json:"actor,omitempty"`
}

// BudgetMeta is recall's attention budget accounting: RFC-named "budget
// meta", populated for recall (the only verb this task's own acc line
// tests a budget for -- "recall budget_used <= budget_tokens with
// dropped_count consistent").
type BudgetMeta struct {
	BudgetTokens int `json:"budget_tokens"`
	BudgetUsed   int `json:"budget_used"`
	DroppedCount int `json:"dropped_count"`
}

// Cost is one verb call's model spend, populated only for synthesize (the
// sole judgment-tier verb among the five -- recall/entity/remember/forget
// call no model).
type Cost struct {
	USD float64 `json:"usd"`
}

// VerbError is the gbrain-contract "uniform error envelope with populated
// suggestion": Code is a stable, enumerated snake_case symbol a caller can
// branch on (e.g. "provenance_required", "unavailable", "invalid_argument",
// "unsupported_tier"), Message is a human-readable description, and
// Suggestion is always non-empty -- a concrete fix, never a bare
// complaint, matching this task's own acc line ("remember with empty
// provenance -> provenance_required with a populated suggestion") and
// pitfall note ("synthesize without a model returns unavailable with a
// fix").
type VerbError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// Envelope is the gbrain envelope every MEMORY_VERBS v1 response carries
// (RFC 0001 §8.1): ProtocolVersion is always set; Evidence/Budget/Cost are
// populated only by the verbs that produce them; Error is set, and every
// other field left at its zero value, exactly when the call failed at the
// protocol layer (a domain outcome like recall finding nothing, or
// synthesize's own explicit Gap, is not a protocol error and does not set
// Error -- RFC §11's "an explicit gap statement... never a fabricated
// answer" is a successful, structured response, not a failure).
type Envelope struct {
	ProtocolVersion int         `json:"protocol_version"`
	Evidence        []Fact      `json:"evidence,omitempty"`
	Budget          *BudgetMeta `json:"budget,omitempty"`
	Cost            *Cost       `json:"cost,omitempty"`
	Error           *VerbError  `json:"error,omitempty"`
}

func newEnvelope() Envelope { return Envelope{ProtocolVersion: ProtocolVersion} }

// globEntityPage finds a fence-tier entity page by slug alone.
// store.FenceWriter has no such lookup (PathFor needs the type folder up
// front) -- this globs brain/entities/*/<slug>.md directly, the same
// shape internal/compose.AllClaims's own page glob uses, narrowed to one
// slug. Zero matches is a normal outcome (no error), used by both
// entity() and forget() to mean "no fence-tier page for this subject".
func globEntityPage(root, slug string) ([]string, error) {
	return filepath.Glob(filepath.Join(root, "brain", "entities", "*", slug+".md"))
}

// Deps is everything Handlers needs, built once by the caller (production:
// internal/cli/serve.go, opening the brain root MEMORY_VERBS serves) and
// held for the life of the MCP session -- unlike `serenity search`/`serenity
// ask`, which reopen everything per invocation, a live daemon process
// building this once per `serve` call avoids reopening the index and
// rebuilding the model routers on every tool call.
type Deps struct {
	Root   string
	Config *config.Config

	// Index is the derived index (search.Store, *index.SQLite in
	// production): chunk-level retrieval for recall/synthesize.
	Index *index.SQLite
	// Embedder is nil when no embedding model is pinned/credentialed --
	// recall/synthesize then degrade to FTS-only relevance, the same
	// honest degraded mode internal/search.Search and internal/cli/ask.go
	// already document; never an error.
	Embedder embed.Embedder

	// Composer is nil under the same explicit-skip contract
	// providers.BuildComposerRouter documents; synthesize then returns
	// VerbError{Code: "unavailable"} with ComposerUnavailableNote as the
	// message and a populated suggestion, never a silent no-op.
	Composer compose.Completer
	// ComposerModelVersion is the pinned composer model's version string
	// (cfg.Models.Composer's version half), threaded into
	// compose.New exactly as internal/cli/ask.go does.
	ComposerModelVersion string
	// ComposerUnavailableNote is the exact note
	// providers.BuildComposerRouter returned when Composer is nil -- the
	// same text `serenity ask` itself prints when the composer model is
	// unpinned or uncredentialed -- surfaced verbatim as synthesize's
	// VerbError.Message.
	ComposerUnavailableNote string

	// Disposition stages remember's reconcile-conflict review items
	// (internal/reconcile.Engine, T2.2) -- see remember.go.
	Disposition *coredisp.Store

	// Queue/Fence/Shard are the deterministic writer's own entry points
	// (internal/writer.Fence/Shard, the only sanctioned way to commit a
	// canonical claim after M0) -- remember writes a brand-new claim
	// through them; forget writes a retraction line through Shard via
	// internal/supersede.Writer.Retract.
	Queue *writer.Queue
	Fence *store.FenceWriter
	Shard *store.ShardStore

	// Clock is real time in production; tests inject a fixed Clock so
	// Provenance.ObservedAt and a staged disposition item's CreatedAt are
	// deterministic.
	Clock Clock
}

func (d Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now()
	}
	return d.Clock.Now()
}

// Handlers implements MEMORY_VERBS v1's five verbs over Deps.
type Handlers struct {
	deps   Deps
	engine *reconcile.Engine
}

// New builds Handlers over deps. deps.Root, deps.Config, deps.Index,
// deps.Queue, deps.Fence, and deps.Shard must be non-nil -- every verb
// needs at least read access to the brain's claims, and remember/forget
// need the writer queue. deps.Disposition must be non-nil for remember's
// reconcile-conflict staging (nil panics on first genuine conflict rather
// than silently skipping the human-review gate RFC §10.3 requires).
// Embedder/Composer may be nil (see their own field docs).
func New(deps Deps) *Handlers {
	if deps.Clock == nil {
		deps.Clock = realClock{}
	}
	h := &Handlers{deps: deps}
	if deps.Disposition != nil {
		h.engine = reconcile.NewEngine(deps.Disposition)
	}
	return h
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

// textResult renders resp (a verb's own response type, always embedding
// Envelope by value so its fields flatten into the same top-level JSON
// object -- Go's anonymous-struct-embedding marshal rule) as one MCP text
// content block, the transport convention internal/server/mcp.Result
// documents ("domain envelopes can be serialized in Text"). isError marks
// a protocol-layer failure -- MCP's own Result.IsError, distinct from the
// embedded Envelope.Error a caller reads to branch on the specific
// enumerated gbrain error code; both are set together so a generic MCP
// client that only understands IsError still sees a failed call, and a
// MEMORY_VERBS-aware client reads Envelope.Error for the code and
// suggestion.
func textResult(resp any, isError bool) (mcp.Result, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(data)}}, IsError: isError}, nil
}

// errorEnvelope builds a bare Envelope carrying only the enumerated
// error -- the response shape for a verb whose request never got far
// enough to build its own richer response type (a malformed request, or a
// validation failure with nothing else to report).
func errorEnvelope(code, message, suggestion string) Envelope {
	env := newEnvelope()
	env.Error = &VerbError{Code: code, Message: message, Suggestion: suggestion}
	return env
}

// verbFunc adapts a typed (ctx, request) -> (response, isError, error)
// function into the mcp.Tool.Handler shape: response is always a value
// embedding Envelope (never a bare Envelope unless the verb has no
// verb-specific fields to add). A non-nil err is this package's own bug
// (JSON marshal failure), never a domain outcome -- see mcp.invoke's own
// recover()/error-swallowing contract, which is exactly why every domain
// failure must be reported via the response's embedded Envelope.Error/
// isError rather than a Go error.
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
