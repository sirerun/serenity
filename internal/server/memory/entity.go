package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/compose"
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
	Suggestions     []entitySuggestion `json:"suggestions"`
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
}

// entity resolves alias, title, then slug, with recency breaking ties.
func (h *Handlers) entity(ctx context.Context, args json.RawMessage) (any, bool, error) {
	started := time.Now()
	now := h.deps.now()
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

	if len(matches) == 0 {
		suggestions := nearMissSuggestions(name, refSlug, pages)
		return entityResponse{ProtocolVersion: ProtocolVersion, Found: false, LatencyMs: time.Since(started).Milliseconds(), Suggestions: suggestions}, false, nil
	}

	best := matches[0]
	card, err := h.buildEntityCard(best, pages, now)
	if err != nil {
		return nil, false, err
	}
	resp := entityResponse{ProtocolVersion: ProtocolVersion, Found: true, LatencyMs: time.Since(started).Milliseconds(), Card: card, Suggestions: []entitySuggestion{}}
	for _, m := range matches[1:] {
		resp.Suggestions = append(resp.Suggestions, entitySuggestion{Slug: m.page.Entity.Slug, Title: m.page.Title, CreateSafety: "exists"})
	}
	return resp, false, nil
}

func (h *Handlers) loadAllEntityPages() ([]resolvedEntityPage, error) {
	base := filepath.Join(h.deps.Root, "brain", "entities")
	for _, dir := range []string{h.deps.Root, filepath.Join(h.deps.Root, "brain"), base} {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return []resolvedEntityPage{}, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("entity: unsafe directory %s", dir)
		}
	}
	types, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	out := []resolvedEntityPage{}
	for _, typ := range types {
		if typ.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("entity: symlink type directory")
		}
		if !typ.IsDir() {
			continue
		}
		dir := filepath.Join(base, typ.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if file.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("entity: symlink page")
			}
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			info, err := file.Info()
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("entity: nonregular page")
			}
			page, err := h.deps.Fence.ParseEntity(path)
			if err != nil {
				return nil, fmt.Errorf("entity: parse page: %w", err)
			}
			out = append(out, resolvedEntityPage{page: page, path: path, mtime: info.ModTime()})
		}
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
		if strings.Contains(name, "/") && !strings.EqualFold(rp.page.Entity.Type, refType) {
			continue
		}
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
			} else if strings.HasSuffix(strings.ToLower(rp.page.Entity.Slug), "/"+nameLower) || strings.HasSuffix(strings.ToLower(rp.page.Entity.Slug), "-"+nameLower) {
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
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].mtime.After(candidates[j].mtime) })
	return candidates
}

// nearMissSuggestions finds a light, real (token-overlap) keyword match
// against every known page's title/slug -- legitimately empty when
// nothing overlaps at all, never a fabricated non-empty result.
func nearMissSuggestions(name, refSlug string, pages []resolvedEntityPage) []entitySuggestion {
	qTokens := tokenizeName(name)
	out := []entitySuggestion{}
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

// buildEntityCard derives remote prose and threads from active public source
// facts. Cached summaries and timelines lack per-entry privacy attribution and
// cannot safely be reused. Edges use canonical public claim heads.
func (h *Handlers) buildEntityCard(best resolvedEntityPage, pages []resolvedEntityPage, now time.Time) (*entityCard, error) {
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		return nil, err
	}

	slug := best.page.Entity.Slug
	activeFacts := make([]store.MemoryFactRecord, 0)
	for _, rec := range proj.ByEntity(slug) {
		if typ := rec.Payload.EntityType; typ != "" && typ != defaultEntityType && !strings.EqualFold(typ, best.page.Entity.Type) {
			continue
		}
		if rec.Expired(now) || rec.Payload.CreatedAt.After(now) || rec.Payload.Visibility != store.MemoryVisibilityWorld {
			continue
		}
		activeFacts = append(activeFacts, rec)
	}

	sort.Slice(activeFacts, func(i, j int) bool {
		if activeFacts[i].Payload.CreatedAt.Equal(activeFacts[j].Payload.CreatedAt) {
			return activeFacts[i].SHA256 < activeFacts[j].SHA256
		}
		return activeFacts[i].Payload.CreatedAt.After(activeFacts[j].Payload.CreatedAt)
	})
	var lastTimelineDate *string
	openThreads := make([]entityOpenThread, 0, 3)
	cutoff := now.AddDate(0, 0, -90)
	var summary []string
	for _, rec := range activeFacts {
		// Cached page prose and timelines have no per-entry provenance or
		// visibility. Reconstruct remote prose only from eligible sources.
		summary = append(summary, rec.Payload.Fact)
		switch rec.Payload.Kind {
		case store.MemoryFactKindCommitment:
			openThreads = append(openThreads, entityOpenThread{Kind: "commitment", Text: rec.Payload.Fact, Date: isoDatePtr(rec.Payload.CreatedAt)})
		case store.MemoryFactKindEvent:
			if lastTimelineDate == nil {
				lastTimelineDate = isoDatePtr(rec.Payload.CreatedAt)
			}
			if !rec.Payload.CreatedAt.Before(cutoff) {
				openThreads = append(openThreads, entityOpenThread{Kind: "recent_event", Text: rec.Payload.Fact, Date: isoDatePtr(rec.Payload.CreatedAt)})
			}
		}
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

	claims, err := compose.PublicClaims(h.deps.Root, h.deps.Config, proj, now)
	if err != nil {
		return nil, err
	}
	edges, backlinks := entityEdges(slug, pages, claims)

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
		Summary: strings.Join(summary, "\n"),
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

// entityEdges uses resolved canonical public heads. Counts and edge limits
// apply after privacy, validity and supersession filtering.
func entityEdges(slug string, pages []resolvedEntityPage, claims map[string][]domain.Claim) ([]entityEdge, int) {
	known := map[string]bool{}
	for _, page := range pages {
		known[page.page.Entity.Slug] = true
	}
	out, incoming := []entityEdge{}, []entityEdge{}
	seen := map[string]bool{}
	add := func(c domain.Claim, direction, target string) {
		if c.Predicate == "mentions" || c.Predicate == "mention" {
			return
		}
		key := direction + "\x00" + c.Predicate + "\x00" + target
		if seen[key] {
			return
		}
		seen[key] = true
		edge := entityEdge{Type: c.Predicate, Direction: direction, Slug: target}
		if direction == "out" {
			out = append(out, edge)
		} else {
			incoming = append(incoming, edge)
		}
	}
	for _, c := range claims[slug] {
		if target := store.NormalizeKey(c.Object); known[target] {
			add(c, "out", target)
		}
	}
	for subject, list := range claims {
		if subject == slug || !known[subject] {
			continue
		}
		for _, c := range list {
			if store.NormalizeKey(c.Object) == slug {
				add(c, "in", subject)
			}
		}
	}
	less := func(a, b entityEdge) bool {
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Slug < b.Slug
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	sort.Slice(incoming, func(i, j int) bool { return less(incoming[i], incoming[j]) })
	backlinks := len(incoming)
	out = append(out, incoming...)
	if len(out) > 10 {
		out = out[:10]
	}
	return out, backlinks
}

func isoDatePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}
