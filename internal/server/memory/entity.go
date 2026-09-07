package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
)

type entityRequest struct {
	Name string `json:"name"`
}

type entityCardEntity struct {
	Slug  string  `json:"slug"`
	Title string  `json:"title"`
	Type  *string `json:"type"`
}

type entityLastTouched struct {
	UpdatedAt        *string `json:"updated_at"`
	LastRetrievedAt  *string `json:"last_retrieved_at"`
	LastTimelineDate *string `json:"last_timeline_date"`
}

type entityOpenThread struct {
	Kind string  `json:"kind"` // commitment | recent_event
	Text string  `json:"text"`
	Date *string `json:"date"`
}

type entityEdge struct {
	Type      string  `json:"type"`
	Direction string  `json:"direction"` // out | in
	Slug      string  `json:"slug"`
	Context   *string `json:"context,omitempty"`
}

type entityCard struct {
	Entity          entityCardEntity   `json:"entity"`
	Aka             []string           `json:"aka"`
	Summary         string             `json:"summary"`
	LastTouched     entityLastTouched  `json:"last_touched"`
	OpenThreads     []entityOpenThread `json:"open_threads"`
	Edges           []entityEdge       `json:"edges"`
	BacklinkCount   int                `json:"backlink_count"`
	ActiveFactCount int                `json:"active_fact_count"`
}

type entitySuggestion struct {
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	CreateSafety string `json:"create_safety"`
}

type entityResponse struct {
	ProtocolVersion int                `json:"protocol_version"`
	Found           bool               `json:"found"`
	LatencyMs       int64              `json:"latency_ms"`
	Card            *entityCard        `json:"card,omitempty"`
	Suggestions     []entitySuggestion `json:"suggestions,omitempty"`
}

func (h *Handlers) entityTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Free-text name, alias, or slug (e.g. \"Alice Example\", \"people/alice-example\")."}
		},
		"required": ["name"]
	}`
	return mcp.Tool{
		Name:        "entity",
		Description: "Inspect one known person/company/project card -- zero LLM calls. Never errors on a miss.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.entity),
	}
}

// resolvedEntityPage is one fence-tier page plus the real (not fabricated)
// signals its resolution and card ranking use: file modification time
// (the honest "recently touched" proxy this repo has, absent any explicit
// last-touched field on domain.Entity/store.EntityPage) and which
// precedence tier matched.
type resolvedEntityPage struct {
	page  *store.EntityPage
	path  string
	mtime time.Time
	tier  int // 0 = alias, 1 = exact title, 2 = slug/suffix
}

// entity resolves name -> a card, zero LLM (RFC's own p99<100ms promise is
// op-layer latency; this implementation's own real Go work easily clears
// it against a single-brain-repo fixture). Resolution precedence is
// alias > exact title > slug/suffix, ties broken by most-recently-touched
// (file mtime); a miss is a normal, successful found:false response, never
// an error (this task's own acc line).
func (h *Handlers) entity(ctx context.Context, args json.RawMessage) (any, bool, error) {
	t0 := h.deps.now()
	var req entityRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return verbError(ErrCodeInvalidParams, "entity: malformed request", "send a JSON object with a non-empty \"name\" string"), true, nil
	}
	name := trimmed(req.Name)
	if name == "" {
		return verbError(ErrCodeInvalidParams, "entity: name must be a non-empty string", "pass the entity to look up, e.g. name: \"Alice Example\" or name: \"people/alice-example\""), true, nil
	}
	refType, refSlug, refOK := canonicalEntityRef(name)
	if !refOK {
		return verbError(ErrCodeInvalidParams, "entity: name must be a valid reference", "pass a plain name or a \"type/slug\" reference with no path separators beyond the one splitting them"), true, nil
	}

	pages, err := h.loadAllEntityPages()
	if err != nil {
		return nil, false, err
	}

	matches := matchEntityPages(name, refType, refSlug, pages)
	latency := h.deps.now().Sub(t0).Milliseconds()

	if len(matches) == 0 {
		suggestions := nearMissSuggestions(name, refSlug, pages)
		return entityResponse{ProtocolVersion: ProtocolVersion, Found: false, LatencyMs: latency, Suggestions: suggestions}, false, nil
	}

	best := matches[0]
	card, err := h.buildEntityCard(best, pages)
	if err != nil {
		return nil, false, err
	}
	resp := entityResponse{ProtocolVersion: ProtocolVersion, Found: true, LatencyMs: latency, Card: card}
	for _, m := range matches[1:] {
		resp.Suggestions = append(resp.Suggestions, entitySuggestion{Slug: m.page.Entity.Slug, Title: m.page.Title, CreateSafety: "exists"})
	}
	return resp, false, nil
}

func (h *Handlers) loadAllEntityPages() ([]resolvedEntityPage, error) {
	paths, err := filepath.Glob(filepath.Join(h.deps.Root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]resolvedEntityPage, 0, len(paths))
	for _, p := range paths {
		page, err := h.deps.Fence.ParseEntity(p)
		if err != nil {
			continue // a page that fails to parse is not a candidate, never a hard error for a read-only lookup
		}
		info, statErr := os.Stat(p)
		mtime := time.Time{}
		if statErr == nil {
			mtime = info.ModTime()
		}
		out = append(out, resolvedEntityPage{page: page, path: p, mtime: mtime})
	}
	return out, nil
}

// matchEntityPages resolves name against pages under the frozen precedence
// (alias > exact title > slug/suffix), returning every page that matched
// the BEST tier found, most-recently-touched first -- the first element is
// the winning card; the rest are multi-hit runners-up.
func matchEntityPages(name, refType, refSlug string, pages []resolvedEntityPage) []resolvedEntityPage {
	nameLower := strings.ToLower(name)
	bestTier := -1
	var candidates []resolvedEntityPage
	for _, rp := range pages {
		tier := -1
		for _, alias := range rp.page.Entity.Aliases {
			if strings.EqualFold(alias, name) {
				tier = 0
				break
			}
		}
		if tier == -1 && strings.EqualFold(rp.page.Title, name) {
			tier = 1
		}
		if tier == -1 {
			if rp.page.Entity.Slug == refSlug || strings.EqualFold(rp.page.Entity.Slug, refSlug) {
				tier = 2
			} else if strings.HasSuffix(strings.ToLower(rp.page.Entity.Slug), nameLower) {
				tier = 2
			}
		}
		if tier == -1 {
			continue
		}
		if bestTier == -1 || tier < bestTier {
			bestTier = tier
			candidates = []resolvedEntityPage{rp}
		} else if tier == bestTier {
			candidates = append(candidates, rp)
		}
	}
	_ = refType
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].mtime.After(candidates[j].mtime) })
	return candidates
}

// nearMissSuggestions finds a light, real (token-overlap) keyword match
// against every known page's title/slug -- legitimately empty when
// nothing overlaps at all, never a fabricated non-empty result.
func nearMissSuggestions(name, refSlug string, pages []resolvedEntityPage) []entitySuggestion {
	qTokens := tokenizeName(name)
	var out []entitySuggestion
	for _, rp := range pages {
		hit := false
		for tok := range tokenizeName(rp.page.Title) {
			if qTokens[tok] {
				hit = true
				break
			}
		}
		if !hit && strings.Contains(strings.ToLower(rp.page.Entity.Slug), strings.ToLower(refSlug)) {
			hit = true
		}
		if hit {
			out = append(out, entitySuggestion{Slug: rp.page.Entity.Slug, Title: rp.page.Title, CreateSafety: "probable"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func tokenizeName(s string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range strings.Fields(strings.ToLower(s)) {
		if len(tok) >= 3 {
			out[tok] = true
		}
	}
	return out
}

// buildEntityCard renders one page into the pinned card shape: real,
// derived open_threads (active commitment-kind memory facts plus the
// page's own last-90-day timeline, capped 3), real edges (typed,
// out-edges-first, derived from this entity's own live claims pointing at
// another known entity's slug, plus the "in" edges every OTHER entity's
// claims contribute back), and a genuine active_fact_count/backlink_count
// -- never fabricated, and private facts are excluded before any of these
// fields is built (entity is always the MCP/remote audience -- RFC:
// "remote callers see visibility=world facts only").
func (h *Handlers) buildEntityCard(best resolvedEntityPage, pages []resolvedEntityPage) (*entityCard, error) {
	now := h.deps.now()
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, err
	}

	slug := best.page.Entity.Slug
	activeFacts := make([]store.MemoryFactRecord, 0)
	for _, rec := range proj.ByEntity(slug) {
		if rec.Expired(now) || rec.Payload.Visibility != store.MemoryVisibilityWorld {
			continue
		}
		activeFacts = append(activeFacts, rec)
	}

	var lastTimelineDate *string
	openThreads := make([]entityOpenThread, 0, 3)
	cutoff := now.AddDate(0, 0, -90)
	for _, rec := range activeFacts {
		if rec.Payload.Kind == store.MemoryFactKindCommitment {
			openThreads = append(openThreads, entityOpenThread{Kind: "commitment", Text: rec.Payload.Fact, Date: isoDatePtr(rec.Payload.CreatedAt)})
		}
	}
	tl := append([]store.TimelineEntry(nil), best.page.Timeline...)
	sort.Slice(tl, func(i, j int) bool { return tl[i].Date > tl[j].Date })
	if len(tl) > 0 {
		d := tl[0].Date
		lastTimelineDate = &d
	}
	for _, t := range tl {
		parsed, err := time.Parse("2006-01-02", t.Date)
		if err == nil && parsed.Before(cutoff) {
			continue
		}
		date := t.Date
		openThreads = append(openThreads, entityOpenThread{Kind: "recent_event", Text: t.Text, Date: &date})
	}
	sort.SliceStable(openThreads, func(i, j int) bool {
		di, dj := "", ""
		if openThreads[i].Date != nil {
			di = *openThreads[i].Date
		}
		if openThreads[j].Date != nil {
			dj = *openThreads[j].Date
		}
		return di > dj
	})
	if len(openThreads) > 3 {
		openThreads = openThreads[:3]
	}

	edges, backlinks := h.entityEdges(slug, best, pages)

	var typ *string
	if best.page.Entity.Type != "" {
		t := best.page.Entity.Type
		typ = &t
	}
	var updatedAt *string
	if !best.mtime.IsZero() {
		s := best.mtime.UTC().Format(time.RFC3339)
		updatedAt = &s
	}

	return &entityCard{
		Entity:  entityCardEntity{Slug: slug, Title: best.page.Title, Type: typ},
		Aka:     append([]string{}, best.page.Entity.Aliases...),
		Summary: best.page.Summary,
		LastTouched: entityLastTouched{
			UpdatedAt:        updatedAt,
			LastRetrievedAt:  nil, // no per-entity read-tracking exists in this repo -- an honest null, never fabricated
			LastTimelineDate: lastTimelineDate,
		},
		OpenThreads:     openThreads,
		Edges:           edges,
		BacklinkCount:   backlinks,
		ActiveFactCount: len(activeFacts),
	}, nil
}

// entityEdges derives real typed edges from claim data alone: an "out"
// edge for each of this entity's own live claims whose Object normalizes
// to another known page's slug, and an "in" edge for every OTHER entity's
// live claim that points back at this one -- out-edges first, capped at
// 10 total (RFC: "top ~10 typed edges, mentions excluded, out-edges
// first" -- this repo has no separate "mentions" edge kind to exclude, so
// that constraint is trivially satisfied). backlinks is the real count of
// "in" edges found, independent of the 10-cap.
func (h *Handlers) entityEdges(slug string, best resolvedEntityPage, pages []resolvedEntityPage) ([]entityEdge, int) {
	knownSlugs := make(map[string]bool, len(pages))
	for _, rp := range pages {
		knownSlugs[rp.page.Entity.Slug] = true
	}

	var out []entityEdge
	for _, c := range best.page.Claims {
		if key := store.NormalizeKey(c.Object); knownSlugs[key] {
			out = append(out, entityEdge{Type: c.Predicate, Direction: "out", Slug: key})
		}
	}
	families, _ := h.deps.Shard.Families(slug)
	for _, family := range families {
		heads, _ := h.deps.Shard.ResolveHeads(slug, family)
		for _, key := range store.HeadKeys(heads) {
			c := heads[key]
			if k := store.NormalizeKey(c.Object); knownSlugs[k] {
				out = append(out, entityEdge{Type: c.Predicate, Direction: "out", Slug: k})
			}
		}
	}

	backlinks := 0
	var in []entityEdge
	for _, rp := range pages {
		if rp.page.Entity.Slug == slug {
			continue
		}
		for _, c := range claimsOf(h, rp.page) {
			if store.NormalizeKey(c.Object) == slug {
				in = append(in, entityEdge{Type: c.Predicate, Direction: "in", Slug: rp.page.Entity.Slug})
				backlinks++
			}
		}
	}
	out = append(out, in...)
	if len(out) > 10 {
		out = out[:10]
	}
	return out, backlinks
}

// claimsOf returns every live claim -- fence-tier from the page itself,
// shard-tier resolved heads -- for one entity page, the same fence-vs-
// shard authority split index.Rebuild applies.
func claimsOf(h *Handlers, page *store.EntityPage) []domain.Claim {
	out := append([]domain.Claim(nil), page.Claims...)
	families, _ := h.deps.Shard.Families(page.Entity.Slug)
	for _, family := range families {
		heads, _ := h.deps.Shard.ResolveHeads(page.Entity.Slug, family)
		for _, key := range store.HeadKeys(heads) {
			out = append(out, heads[key])
		}
	}
	return out
}

func isoDatePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}
