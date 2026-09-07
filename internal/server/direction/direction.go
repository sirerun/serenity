// Package direction implements the DIRECTION v1 wire protocol (RFC 0001
// §8.3, T4.6) over HTTP: brief, check_plan, propose. Like
// internal/server/disposition (T4.4), it is a thin transport layer over
// already-shipped domain packages -- internal/direction/check.Matcher/
// Classifier (T3.5/T3.7) for check_plan, internal/disposition.Store
// (T2.1+) for propose's only write, internal/briefing.Pack (T2.17) for
// brief's server-side packing -- adding no governance logic of its own.
//
// propose NEVER calls a ledger.Store write method (Create/Put/CreateDraft/
// Confirm/Supersede): every accepted proposal lands in the DISPOSITION
// queue exactly like a human-authored one, and only a later, separate
// accept (internal/direction.ApplyDisposedPreceptDraft or
// internal/spend.ApplyDisposedEffect) ever writes .dira/ or the brain
// repo. That is what makes "propose(precept_draft) creates a disposition
// item and .dira/ hash is unchanged" (T4.6's own acc line) true by
// construction, not by care -- there is no call in this file's propose
// path capable of doing otherwise.
//
// check_plan's wire verdict is produced by internal/direction/check.ToWire
// -- the same converter `serenity check --json` (internal/cli/check.go)
// uses -- so the two surfaces can never drift on field names or omission
// rules (T4.6's acc line: "check_plan wire verdicts equal `serenity check
// --json` on the same fixture").
//
// brief's per-section caps (12/8/8/5, RFC 0001 §12) are applied to each
// section's own candidate list before briefing.Pack ever runs -- "the
// per-section caps are maxima *within* the budget" (§12) -- and the
// caller's token_budget is approximated by briefing.WordEstimator (a word
// count, the same approximation internal/briefing's own daily-briefing
// caller uses for its word budget), named explicitly in the wire response
// as budget_estimator: "words" so a caller is never left to guess what
// unit governed packing. A real tokenizer-backed Estimator is future work;
// disclosed here rather than silently assumed.
//
// brief's entities section has no existing "entity relevant to task_hint"
// primitive to reuse (internal/search's chunk-level hybrid search resolves
// to a chunk, not a full entity page, and needs a live embedder this
// package is not guaranteed to have): entityItems walks
// root/brain/entities/**/*.md directly via internal/store.FenceWriter,
// ranked by a simple lexical token-overlap heuristic against task_hint (or
// by most-recent-claim recency when task_hint is empty) -- the same
// simpler-than-embedding, disclosed-heuristic precedent T2.12's alias
// candidates and T2.15/T4.10's own metric formulas already set in this
// codebase.
//
// Not wired into a live `serenity serve` command yet: T4.1 (`serenityd`
// core) ships the daemon's ticker/pidfile/shutdown skeleton but does not
// itself register any protocol routes (that is each protocol package's own
// Register method, called by whatever assembles the daemon's route table).
// Registrar mirrors internal/server/disposition.Registrar exactly, so a
// future daemon-assembly task wires both packages onto the same
// *internal/server.Server identically.
package direction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/sirerun/serenity/internal/briefing"
	"github.com/sirerun/serenity/internal/dira/ledger"
	coredirection "github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/direction/check"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/spend"
	"github.com/sirerun/serenity/internal/store"
)

// Clock is the same one-method seam internal/server/disposition.Clock and
// internal/events.Clock use.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Per-section caps, RFC 0001 §12: "standing precepts (cap 12) -> current
// intents chained to ambitions (cap 8) -> relevant entities with claim
// heads + provenance (cap 8) -> open blocking questions (cap 5)". Applied
// to each section's own candidate list before briefing.Pack runs -- these
// are maxima within the caller's token budget, not a substitute for it.
const (
	preceptCap  = 12
	intentCap   = 8
	entityCap   = 8
	questionCap = 5
)

// The four brief section names, RFC 0001 §12's own priority order.
// briefing.SectionName is just a string type (RFC 0001 §7's five daily-
// briefing names are not a closed enum this package is bound by), so
// DIRECTION v1's brief defines its own distinct vocabulary here.
const (
	sectionPrecepts  briefing.SectionName = "precepts"
	sectionIntents   briefing.SectionName = "intents"
	sectionEntities  briefing.SectionName = "entities"
	sectionQuestions briefing.SectionName = "questions"
)

// briefBudgetEstimatorName is named verbatim in every brief response's
// budget_estimator field (T4.6's own acc line: "budget_estimator named").
// "words" discloses exactly what unit governs packing: a word count
// approximating tokens, not a real tokenizer.
const briefBudgetEstimatorName = "words"

// Registrar is the subset of *internal/server.Server this package needs,
// declared locally exactly as internal/server/disposition.Registrar is --
// no hard dependency on the server package's concrete type.
type Registrar interface {
	Handle(pattern string, h http.Handler)
}

// Handlers implements DIRECTION v1 over a ledger store, a disposition
// store, and the brain repo root brief's entities section reads directly.
type Handlers struct {
	ledger        ledger.Store
	disposition   *coredisp.Store
	root          string
	router        *router.Router
	modelVersion  string
	classifyCache check.ClassifyCache
	clock         Clock
}

// Option configures Handlers at construction.
type Option func(*Handlers)

// WithRouter wires a live router for check_plan's free-text classification
// stage. The default (no option) leaves it nil, mirroring
// internal/cli.runCheck's own disclosed nil-router precedent: no
// provider-from-config wiring exists yet, so a nil router is not a stub --
// it is exactly the "no model configured" condition
// check.Classifier.MatchFreeText already documents as one of the two
// conditions that yield StatusUnverified.
func WithRouter(rtr *router.Router) Option { return func(h *Handlers) { h.router = rtr } }

// WithModelVersion pins the classification model check_plan's free-text
// stage asserts against (mirrors internal/cli.newCheckCmd's own "" default
// when no pin is configured).
func WithModelVersion(v string) Option { return func(h *Handlers) { h.modelVersion = v } }

// WithClassifyCache overrides check_plan's classification cache. Nil (the
// default) lets check.NewClassifier fall back to its own in-memory cache.
func WithClassifyCache(c check.ClassifyCache) Option {
	return func(h *Handlers) { h.classifyCache = c }
}

// WithClock overrides the real clock. Test-only hook.
func WithClock(c Clock) Option { return func(h *Handlers) { h.clock = c } }

// New builds Handlers over ledgerStore (typically a *internal/direction.Store,
// DIRECTION's own boundary onto .dira/ -- see that package's doc comment
// on why it is the only package permitted to touch .dira/ at all),
// dispStore (propose's only write target) and root (the brain repo root,
// read directly by brief's entities section, nowhere else).
func New(ledgerStore ledger.Store, dispStore *coredisp.Store, root string, opts ...Option) *Handlers {
	h := &Handlers{
		ledger:      ledgerStore,
		disposition: dispStore,
		root:        root,
		clock:       realClock{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Register wires all three DIRECTION v1 routes onto s.
func (h *Handlers) Register(s Registrar) {
	s.Handle("/direction/brief", http.HandlerFunc(h.handleBrief))
	s.Handle("/direction/check_plan", http.HandlerFunc(h.handleCheckPlan))
	s.Handle("/direction/propose", http.HandlerFunc(h.handlePropose))
}

// ProtoError mirrors internal/server/disposition.ProtoError: a stable
// machine-checkable code plus a human message, for every 4xx/5xx this
// package emits. Each protocol package keeps its own copy rather than
// sharing one across a boundary neither owns -- disposition.go's own
// identical types make the same choice.
type ProtoError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ProtoError{Code: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}

// --- check_plan -------------------------------------------------------

// CheckPlanRequest is check_plan's request body (RFC 0001 §8.3:
// "check_plan(plan_text, actions?)"). Exactly one of PlanText or Actions
// is required -- the same mutual-exclusivity rule
// internal/cli.runCheck enforces for `serenity check`'s own two input
// forms.
type CheckPlanRequest struct {
	PlanText string            `json:"plan_text,omitempty"`
	Actions  []CheckPlanAction `json:"actions,omitempty"`
}

// CheckPlanAction is one structured action, decoded before conversion to
// check.Action.
type CheckPlanAction struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// handleCheckPlan mirrors internal/cli.runCheck's own branch exactly:
// structured Actions go straight to stage 1 (check.Matcher.Match, fully
// offline); free-text PlanText goes through stage 2
// (check.Classifier.MatchFreeText) first. Either way the response is
// check.ToWire's output -- the same converter `serenity check --json`
// calls -- so the two surfaces can never disagree on shape.
func (h *Handlers) handleCheckPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "check_plan requires POST")
		return
	}
	var req CheckPlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}
	if (req.PlanText != "") == (len(req.Actions) > 0) {
		writeError(w, http.StatusBadRequest, "invalid_request", "exactly one of plan_text or actions is required")
		return
	}

	matcher := check.New(h.ledger, h.router)

	var (
		result         check.Result
		matched        []check.MatchedAction
		confidence     float64
		haveConfidence bool
	)

	if len(req.Actions) > 0 {
		actions := make([]check.Action, len(req.Actions))
		for i, a := range req.Actions {
			actions[i] = check.Action{Action: a.Action, Params: a.Params}
		}
		var err error
		result, err = matcher.Match(r.Context(), actions)
		if err != nil {
			writeCheckPlanError(w, err)
			return
		}
	} else {
		classifier := check.NewClassifier(matcher, h.modelVersion, h.classifyCache)
		ftResult, err := classifier.MatchFreeText(r.Context(), req.PlanText, router.Budget{})
		if err != nil {
			writeCheckPlanError(w, err)
			return
		}
		result = ftResult.Result
		matched = ftResult.MatchedActions
		confidence = ftResult.Confidence
		haveConfidence = true
	}

	writeJSON(w, http.StatusOK, check.ToWire(result, matched, confidence, haveConfidence))
}

// writeCheckPlanError maps a Match/MatchFreeText error to a protocol
// error: ErrUnknownAction (a caller-input problem, an action outside
// domain.ActionSet) is 400; anything else (a ledger read failure, a
// malformed applies_when block on an active constraint) is 500 -- the same
// distinction internal/cli.runCheck's own returned-error handling draws,
// carried over into HTTP status rather than an exit code.
func writeCheckPlanError(w http.ResponseWriter, err error) {
	if errors.Is(err, check.ErrUnknownAction) {
		writeError(w, http.StatusBadRequest, "invalid_action", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
}

// --- propose ------------------------------------------------------------

// ProposeRequest is propose's request body (RFC 0001 §8.3:
// "propose(kind, payload)"). Payload is opaque to the wire layer -- its
// shape depends on Kind, validated by validateProposePayload below before
// anything is staged.
type ProposeRequest struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type ProposeResponse struct {
	ItemID string `json:"item_id"`
}

// proposeKinds maps propose's wire kind strings to disposition.Kind.
// Deliberately narrower than disposition.Kind's full vocabulary: RFC 0001
// §8.3 names exactly two things an agent may propose -- "precept drafts
// and effects" -- so a caller cannot use this operation to stage a
// reconcile/distill/tombstone/entity_merge/compact/decompose item, each of
// which has its own, non-agent-facing origin elsewhere in this codebase.
var proposeKinds = map[string]coredisp.Kind{
	"precept_draft": coredisp.KindPreceptDraft,
	"effect":        coredisp.KindEffect,
}

// handlePropose stages a disposition item and NEVER calls a ledger write
// method -- see this package's doc comment for why that is what makes the
// "propose(precept_draft) creates a disposition item and .dira/ hash is
// unchanged" acc line true by construction.
func (h *Handlers) handlePropose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "propose requires POST")
		return
	}
	var req ProposeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}
	kind, ok := proposeKinds[req.Kind]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_kind", fmt.Sprintf("kind must be one of precept_draft, effect; got %q", req.Kind))
		return
	}
	if len(req.Payload) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_payload", "payload is required")
		return
	}
	if err := validateProposePayload(kind, req.Payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	item, err := h.disposition.Create(r.Context(), kind, req.Payload, "", h.clock.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ProposeResponse{ItemID: item.ID})
}

// validateProposePayload rejects a malformed payload before it is ever
// staged, so a broken propose call fails loudly at propose-time rather
// than staging garbage that only chokes later, at accept-time.
//
// precept_draft's checks mirror internal/direction.ApplyDisposedPreceptDraft's
// own validation exactly (title required, at least one alternative
// required -- "the interview always seeds the 'do not adopt this' floor")
// so the two validation points can never silently disagree about what a
// well-formed precept draft looks like. effect's check is deliberately
// lighter: T4.6's own acc line names precept_draft explicitly, not effect,
// and internal/spend.Checker.ApplyDisposedEffect already validates deeply
// at accept-time -- this function only confirms the payload decodes as a
// well-formed EffectPayload at all, catching a caller's JSON-shape
// mistake early without duplicating the accept-time business validation.
func validateProposePayload(kind coredisp.Kind, payload json.RawMessage) error {
	switch kind {
	case coredisp.KindPreceptDraft:
		var p coredirection.PreceptDraftPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode precept_draft payload: %w", err)
		}
		if p.Title == "" {
			return errors.New("precept_draft payload carries no title")
		}
		if len(p.Alternatives) == 0 {
			return errors.New(`precept_draft payload carries no alternatives; propose must seed at least the "do not adopt this" floor`)
		}
		return nil
	case coredisp.KindEffect:
		var e spend.EffectPayload
		if err := json.Unmarshal(payload, &e); err != nil {
			return fmt.Errorf("decode effect payload: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported propose kind %q", kind)
	}
}

// --- brief --------------------------------------------------------------

// BriefRequest is brief's request body (RFC 0001 §8.3:
// "brief(task_hint?, token_budget)").
type BriefRequest struct {
	TaskHint    string `json:"task_hint,omitempty"`
	TokenBudget int    `json:"token_budget"`
}

// BriefResponse is brief's wire object (RFC 0001 §12): the four fixed
// sections in priority order, plus the estimator name every caller needs
// to interpret token_budget correctly (T4.6's own acc line:
// "budget_estimator named").
type BriefResponse struct {
	BudgetEstimator string             `json:"budget_estimator"`
	Sections        []BriefSectionWire `json:"sections"`
}

// BriefSectionWire is one packed section's wire form. Items is always a
// non-nil (possibly empty) array -- never included and omitted at once --
// mirroring briefing.PackedSection's own "included whole XOR dropped
// whole" invariant: Omitted > 0 implies Items is empty, and vice versa.
type BriefSectionWire struct {
	Name    string   `json:"name"`
	Items   []string `json:"items"`
	Omitted int      `json:"omitted"`
}

// handleBrief builds each of the four RFC 0001 §12 sections (each already
// capped at its own per-section maximum), then packs them through
// briefing.Pack against the caller's token_budget -- the identical pure
// packer T2.17's daily briefing uses, reused unchanged per that task's own
// acc line. A token_budget of 0 still yields a structurally valid
// response: every section with zero candidate items has zero estimated
// cost and is therefore always "included, 0 items" rather than "omitted"
// (briefing.Pack's own behavior, unchanged here) -- T4.6's acc line's "a
// zero budget returns the minimal valid object".
func (h *Handlers) handleBrief(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "brief requires POST")
		return
	}
	var req BriefRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}
	if req.TokenBudget < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "token_budget must be >= 0")
		return
	}

	data, err := h.BuildBrief(r.Context(), req.TaskHint, req.TokenBudget)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// BuildBrief builds DIRECTION v1's brief object (RFC 0001 §12) exactly as
// handleBrief does -- the same four capped sections packed through
// briefing.Pack against tokenBudget -- and returns its JSON encoding
// (toBriefWire's own marshaled form, byte-identical to what handleBrief
// itself now calls this function to write). This is the shared core a
// `serenity brief` CLI verb (T4.9, RFC 0001 §13.1: "CLI and protocol
// surfaces are thin wrappers over one engine") calls directly, the same
// single-source-of-truth guarantee internal/direction/check.ToWire
// already gives check/check_plan -- wire equality holds by construction,
// not by two implementations happening to agree.
func (h *Handlers) BuildBrief(ctx context.Context, taskHint string, tokenBudget int) (json.RawMessage, error) {
	precepts, err := h.preceptItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("precepts: %w", err)
	}
	intents, err := h.intentItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("intents: %w", err)
	}
	entities, err := h.entityItems(taskHint)
	if err != nil {
		return nil, fmt.Errorf("entities: %w", err)
	}
	questions, err := h.questionItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("questions: %w", err)
	}

	sections := []briefing.Section{
		{Name: sectionPrecepts, Items: precepts},
		{Name: sectionIntents, Items: intents},
		{Name: sectionEntities, Items: entities},
		{Name: sectionQuestions, Items: questions},
	}
	packed := briefing.Pack(sections, tokenBudget, briefing.WordEstimator)
	return json.Marshal(toBriefWire(packed))
}

func toBriefWire(b briefing.Briefing) BriefResponse {
	resp := BriefResponse{BudgetEstimator: briefBudgetEstimatorName}
	for _, sec := range b.Sections {
		items := make([]string, len(sec.Items))
		for i, it := range sec.Items {
			items[i] = it.Text
		}
		resp.Sections = append(resp.Sections, BriefSectionWire{
			Name:    string(sec.Name),
			Items:   items,
			Omitted: sec.Omitted,
		})
	}
	return resp
}

// filteredEntries returns up to cap entries of the given kind/state, in
// h.ledger.List's own sorted-by-id order (stable and deterministic: ids
// share a kind-specific prefix and a zero-padded counter, ledger.Add's own
// allocation scheme, so ascending id order is ascending creation order
// within one kind).
func (h *Handlers) filteredEntries(ctx context.Context, kind ledger.Kind, state ledger.State, cap int) ([]*ledger.Entry, error) {
	infos, err := h.ledger.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []*ledger.Entry
	for _, info := range infos {
		if len(out) >= cap {
			break
		}
		e, err := h.ledger.Get(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		if e.Kind != kind || e.State != state {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// preceptItems is brief's "standing precepts" section: every active
// constraint, capped at preceptCap, each rendered with its first
// alternative's why_not when one is on record -- an agent reading a
// precept needs the reason it exists, not only its title.
func (h *Handlers) preceptItems(ctx context.Context) ([]briefing.Item, error) {
	entries, err := h.filteredEntries(ctx, ledger.KindConstraint, ledger.StateActive, preceptCap)
	if err != nil {
		return nil, err
	}
	items := make([]briefing.Item, len(entries))
	for i, e := range entries {
		items[i] = briefing.Item{Text: renderPrecept(e)}
	}
	return items, nil
}

func renderPrecept(e *ledger.Entry) string {
	s := e.ID + ": " + e.Title
	if len(e.Alternatives) > 0 && e.Alternatives[0].WhyNot != "" {
		s += " (why_not: " + e.Alternatives[0].WhyNot + ")"
	}
	return s
}

// intentItems is brief's "current intents chained to ambitions" section.
// dira's Kind vocabulary is closed at five (internal/dira/ledger's own
// doc comment) with no separate "ambition" entity type, so the "chained
// to ambitions" half of RFC 0001 §12's phrase is read here via each
// intent's own derives_from edges -- the same reading questions.go's
// matchQuestions doc comment gives elsewhere in this codebase for reusing
// dira's existing edge vocabulary rather than inventing a sixth kind.
func (h *Handlers) intentItems(ctx context.Context) ([]briefing.Item, error) {
	entries, err := h.filteredEntries(ctx, ledger.KindIntent, ledger.StateActive, intentCap)
	if err != nil {
		return nil, err
	}
	items := make([]briefing.Item, len(entries))
	for i, e := range entries {
		items[i] = briefing.Item{Text: renderIntent(e)}
	}
	return items, nil
}

func renderIntent(e *ledger.Entry) string {
	s := e.ID + ": " + e.Title
	var derives []string
	for _, edge := range e.Edges {
		if edge.Type == ledger.EdgeDerivesFrom {
			derives = append(derives, edge.To)
		}
	}
	if len(derives) > 0 {
		s += " (derives_from: " + strings.Join(derives, ", ") + ")"
	}
	return s
}

// questionItems is brief's "open blocking questions" section: every open
// question, capped at questionCap. Unlike check.Matcher.matchQuestions
// (which filters warnings against a specific checked actions[] list),
// brief takes no actions -- so every open question is a candidate here,
// consistent with matchQuestions' own doc comment naming a future brief()
// section as a second, unfiltered consumer of the same open-question
// concept.
func (h *Handlers) questionItems(ctx context.Context) ([]briefing.Item, error) {
	entries, err := h.filteredEntries(ctx, ledger.KindQuestion, ledger.StateOpen, questionCap)
	if err != nil {
		return nil, err
	}
	items := make([]briefing.Item, len(entries))
	for i, e := range entries {
		items[i] = briefing.Item{Text: e.ID + ": " + e.Title}
	}
	return items, nil
}

// entityItems is brief's "relevant entities with claim heads + provenance"
// section: see this package's doc comment for why it walks
// root/brain/entities directly rather than reusing internal/search.
func (h *Handlers) entityItems(taskHint string) ([]briefing.Item, error) {
	dir := filepath.Join(h.root, "brain", "entities")
	fw := &store.FenceWriter{Root: h.root}

	var pages []*store.EntityPage
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		page, perr := fw.ParseEntity(path)
		if perr != nil {
			return fmt.Errorf("parse entity %s: %w", path, perr)
		}
		pages = append(pages, page)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	ranked := rankEntities(pages, taskHint)
	if len(ranked) > entityCap {
		ranked = ranked[:entityCap]
	}
	items := make([]briefing.Item, len(ranked))
	for i, p := range ranked {
		items[i] = briefing.Item{Text: renderEntity(p)}
	}
	return items, nil
}

// rankEntities orders pages by lexical token-overlap against taskHint
// (highest first); when taskHint is empty every page scores 0 and the
// order falls back entirely to descending most-recent-claim recency. Ties
// within one score also break by recency. sort.SliceStable keeps the
// result deterministic for equal (score, recency) pairs, matching
// filteredEntries' own determinism.
func rankEntities(pages []*store.EntityPage, taskHint string) []*store.EntityPage {
	hintTokens := tokenize(taskHint)
	type candidate struct {
		page    *store.EntityPage
		score   int
		recency time.Time
	}
	cands := make([]candidate, len(pages))
	for i, p := range pages {
		cands[i] = candidate{page: p, score: lexicalOverlap(hintTokens, p), recency: mostRecentClaimTime(p)}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].recency.After(cands[j].recency)
	})
	out := make([]*store.EntityPage, len(cands))
	for i, c := range cands {
		out[i] = c.page
	}
	return out
}

// tokenize lowercases and splits on non-letter/non-digit runes.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// lexicalOverlap counts how many hintTokens appear as substrings of p's
// type, slug, title, summary, and claim predicate/object text -- 0 when
// hintTokens is empty (task_hint omitted), which is what makes rankEntities
// fall back to pure recency in that case.
func lexicalOverlap(hintTokens []string, p *store.EntityPage) int {
	if len(hintTokens) == 0 {
		return 0
	}
	var haystack strings.Builder
	haystack.WriteString(strings.ToLower(p.Entity.Type + " " + p.Entity.Slug + " " + p.Title + " " + p.Summary))
	for _, c := range p.Claims {
		haystack.WriteByte(' ')
		haystack.WriteString(strings.ToLower(c.Predicate + " " + c.Object))
	}
	text := haystack.String()
	score := 0
	for _, tok := range hintTokens {
		if strings.Contains(text, tok) {
			score++
		}
	}
	return score
}

// mostRecentClaimTime is the latest parseable ValidFrom among p's claims,
// the zero time for an entity with no dated claims at all (which then
// sorts last among its score-tied peers). domain.Claim.Provenance
// (SourceSHA256/ObservedAt/Model/Actor) is deliberately not consulted
// here: the fence-tier canonical page format (RenderEntity/ParseEntity
// above) never round-trips it through brain/entities/**/*.md at all --
// only SourceRef, the "human-readable src cell (e.g. 'e42#3')" its own
// doc comment names, survives a parse -- so an entity read back off disk,
// exactly what this section reads, always carries a zero Provenance
// regardless of what was true when the claim was first written.
func mostRecentClaimTime(p *store.EntityPage) time.Time {
	var latest time.Time
	for _, c := range p.Claims {
		if t, err := time.Parse(time.RFC3339, c.ValidFrom); err == nil && t.After(latest) {
			latest = t
		}
	}
	return latest
}

// renderEntity is one entity's brief line: its claim head (the active
// claim with the highest confidence, ties broken toward the first
// encountered) plus its SourceRef cell -- "claim heads + provenance" (RFC
// 0001 §12) read, for a fence-tier page, as the same human-readable
// provenance cell RenderEntity's own claims table renders (see
// mostRecentClaimTime's doc comment on why the fuller domain.Provenance
// struct is not available here). An entity with no active claims on
// record still renders its own type/slug, so a brand-new entity page is
// never silently dropped from the section.
func renderEntity(p *store.EntityPage) string {
	head := pickClaimHead(p.Claims)
	if head == nil {
		return fmt.Sprintf("%s/%s", p.Entity.Type, p.Entity.Slug)
	}
	src := head.SourceRef
	if src == "" {
		src = "unknown"
	}
	return fmt.Sprintf("%s/%s: %s %s (confidence %.2f, source %s)",
		p.Entity.Type, p.Entity.Slug, head.Predicate, head.Object, head.Confidence, src)
}

func pickClaimHead(claims []domain.Claim) *domain.Claim {
	var best *domain.Claim
	for i := range claims {
		c := &claims[i]
		if c.State != domain.StateActive {
			continue
		}
		if best == nil || c.Confidence > best.Confidence {
			best = c
		}
	}
	return best
}
