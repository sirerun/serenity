package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/server/mcp"
)

// synthesizeCallsPerMinute bounds how many well-formed synthesize calls
// one account is served per minute (T24.13, AI-04: the local synthesize
// path had no rate or budget limit at all, and every call is an LLM
// completion). The 61st call in a window answers rate_limited; the window
// is fixed, one minute from the first admitted call.
const synthesizeCallsPerMinute = 60

// synthesizeLimiter is a fixed-window, per-key call counter. The key is
// the account (synthesizeAccountKey). Kept package-local and
// standard-library only; it is deliberately not the hosted OAuth layer's
// per-IP limiter (internal/hosted/oauth), which guards a different edge.
type synthesizeLimiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	windows map[string]*synthesizeWindow
}

type synthesizeWindow struct {
	start time.Time
	count int
}

func newSynthesizeLimiter(limit int, window time.Duration) *synthesizeLimiter {
	return &synthesizeLimiter{limit: limit, window: window, windows: make(map[string]*synthesizeWindow)}
}

// allow admits one call for key at now, or refuses it once limit calls
// have been admitted in the current window. Expired windows are reset on
// their next call, so the map never grows past the number of distinct
// keys seen.
func (l *synthesizeLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.windows[key]
	if w == nil || now.Sub(w.start) >= l.window {
		w = &synthesizeWindow{start: now}
		l.windows[key] = w
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// synthesizeAccountKey identifies the account a synthesize call is
// counted against. One Handlers serves exactly one brain root, and one
// brain is one account (the local daemon has no other identity: RFC 0001
// section 14's bearer token authenticates the process, not a user), so
// the root is the key. A hosted deployment that built one Handlers per
// account inherits per-account limiting from this without change.
func (h *Handlers) synthesizeAccountKey() string {
	return h.deps.Root
}

type synthesizeRequest struct {
	Question string `json:"question"`
	Since    string `json:"since,omitempty"`
	Until    string `json:"until,omitempty"`
}

type synthesizeCost struct {
	Model        string   `json:"model"`
	InputTokens  *int     `json:"input_tokens"`
	OutputTokens *int     `json:"output_tokens"`
	UsdEstimate  *float64 `json:"usd_estimate"`
}

type synthesizeResponse struct {
	ProtocolVersion int            `json:"protocol_version"`
	Answer          string         `json:"answer"`
	Sources         []string       `json:"sources"`
	Gaps            []string       `json:"gaps,omitempty"`
	Cost            synthesizeCost `json:"cost"`
}

func (h *Handlers) synthesizeTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"question": {"type": "string", "description": "The question to answer."},
			"since": {"type": "string", "description": "Optional temporal window start (ISO 8601 date or datetime)."},
			"until": {"type": "string", "description": "Optional temporal window end (ISO 8601 date or datetime)."}
		},
		"required": ["question"]
	}`
	return mcp.Tool{
		Name:        "synthesize",
		Description: "[EXPENSIVE/SLOW] Answer a broad question using cross-page LLM reasoning with citations and gap analysis.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.synthesize),
	}
}

// synthesize wraps the shared internal/compose.Composer.AskWithOptions
// (T4.20: "both CLI ask and MCP synthesize call the same implementation")
// -- the same claim retrieval `serenity ask` uses, plus source-evidence
// retrieval over MEMORY_VERBS facts, both date-bounded by since/until. No
// composer configured returns VerbError{Error: "unavailable"} with a fix,
// never a fake successful answer/model/cost (RFC §11, this task's own
// pitfall note).
func (h *Handlers) synthesize(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req synthesizeRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: malformed request", "send a JSON object with a non-empty \"question\" string"), true, nil
	}
	question := trimmed(req.Question)
	if question == "" {
		return verbError(ErrCodeInvalidParams, "synthesize: question must be a non-empty string", "pass the question to synthesize an answer for, e.g. question: \"what is our payments strategy?\""), true, nil
	}
	since, err := parseSinceUntil(req.Since)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: since is not a valid ISO 8601 date/datetime", "pass an ISO 8601 date (\"2026-06-01\") or datetime (\"2026-06-01T00:00:00Z\")"), true, nil
	}
	until, err := parseSinceUntil(req.Until)
	if err != nil {
		return verbError(ErrCodeInvalidParams, "synthesize: until is not a valid ISO 8601 date/datetime", "pass an ISO 8601 date (\"2026-06-01\") or datetime (\"2026-06-01T00:00:00Z\")"), true, nil
	}

	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return verbError(ErrCodeInvalidParams, "since must not be later than until", "provide an ordered date window"), true, nil
	}

	// Rate limit ahead of any composer work, so an over-limit caller
	// costs nothing (no retrieval, no completion) -- and after
	// validation, so a malformed request is answered as malformed and is
	// not counted.
	if !h.synthLimiter.allow(h.synthesizeAccountKey(), h.deps.now()) {
		return verbError(ErrCodeRateLimited,
			fmt.Sprintf("synthesize: more than %d calls in one minute for this account", synthesizeCallsPerMinute),
			"wait for the minute to roll over and retry; use recall for lookups that do not need cross-page reasoning"), true, nil
	}

	if h.deps.Composer == nil {
		note := h.deps.ComposerUnavailableNote
		if note == "" {
			note = "synthesize needs an LLM and none is configured"
		}
		return verbError(ErrCodeUnavailable, note, "set an API key (e.g. `serenity config` a provider credential) and retry -- recall and entity work without one"), true, nil
	}

	c := compose.New(h.deps.Root, h.deps.Config, h.deps.Index, h.deps.Embedder, h.deps.Composer, h.deps.ComposerModelVersion)
	answer, err := c.AskWithOptions(ctx, question, compose.AskOptions{Since: since, Until: until, Now: h.deps.now()})
	if err != nil {
		return nil, false, err
	}

	resp := synthesizeResponse{
		ProtocolVersion: ProtocolVersion,
		Cost:            synthesizeCost{Model: h.deps.ComposerModelVersion},
	}
	if answer.ModelVersion != "" {
		resp.Cost.Model = answer.ModelVersion
	}
	if answer.Usage != nil {
		in, out, usd := answer.Usage.InputTokens, answer.Usage.OutputTokens, answer.Usage.CostUSD
		resp.Cost.InputTokens = &in
		resp.Cost.OutputTokens = &out
		// An unlisted model's cost is +Inf inside the router (fail
		// closed against MaxUSD); JSON cannot carry +Inf, so the
		// estimate is reported as null -- unknown -- never a fabricated
		// zero. The token counts above are still real and still shown.
		if !router.Unpriced(usd) {
			resp.Cost.UsdEstimate = &usd
		}
	}

	if answer.Gap != "" {
		resp.Answer = answer.Gap
		resp.Sources = []string{}
		resp.Gaps = []string{answer.Gap}
		return resp, false, nil
	}

	resp.Answer = answer.Text
	seen := map[string]bool{}
	for _, cit := range answer.Citations {
		if cit.Subject != "" && !seen[cit.Subject] {
			seen[cit.Subject] = true
			resp.Sources = append(resp.Sources, cit.Subject)
		}
	}
	for _, sc := range answer.SourceCitations {
		source := sc.EntitySlug
		if source == "" {
			source = "source-" + sc.SHA256
		}
		if !seen[source] {
			seen[source] = true
			resp.Sources = append(resp.Sources, source)
		}
	}
	sort.Strings(resp.Sources)
	if resp.Sources == nil {
		resp.Sources = []string{}
	}
	return resp, false, nil
}
