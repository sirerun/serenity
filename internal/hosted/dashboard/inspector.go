package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/domain"
	hostgateway "github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	inspectorDefaultLimit  = 50
	inspectorMaxLimit      = 100
	inspectorMaxNodes      = 40000
	inspectorMaxContext    = 100
	inspectorMaxFiles      = 50000
	inspectorMaxFileBytes  = 1 << 20
	inspectorMaxReadBytes  = 64 << 20
	inspectorMaxMetaBytes  = 32 << 20
	inspectorMaxMetaFile   = 64 << 10
	inspectorMaxCursor     = 512
	inspectorMaxQueryRunes = 128
)

type inspectorNode struct {
	Type         string     `json:"type"`
	ID           string     `json:"id"`
	Kind         string     `json:"kind,omitempty"`
	Label        string     `json:"label,omitempty"`
	Text         string     `json:"text,omitempty"`
	EntityID     string     `json:"entityId,omitempty"`
	Scope        string     `json:"scope,omitempty"`
	Status       string     `json:"status,omitempty"`
	SourceIDs    []string   `json:"sourceIds,omitempty"`
	CreatedAt    *time.Time `json:"createdAt"`
	CapturedAt   *time.Time `json:"capturedAt"`
	ObservedAt   *time.Time `json:"observedAt"`
	ValidUntil   *string    `json:"validUntil"`
	DateKind     string     `json:"dateKind"`
	Confidence   *float64   `json:"confidence,omitempty"`
	Supersedes   string     `json:"supersedes,omitempty"`
	SupersededBy string     `json:"supersededBy,omitempty"`
}

type inspectorEdge struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type inspectorBrain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type inspectorBrainList struct {
	Version int              `json:"version"`
	Brains  []inspectorBrain `json:"brains"`
}

type inspectorGraphPage struct {
	Version          int             `json:"version"`
	BrainID          string          `json:"brainId"`
	PrivateExcluded  bool            `json:"privateExcluded"`
	Nodes            []inspectorNode `json:"nodes"`
	ContextNodes     []inspectorNode `json:"contextNodes"`
	ContextTruncated bool            `json:"contextTruncated"`
	Edges            []inspectorEdge `json:"edges"`
	TotalMatching    int             `json:"totalMatching"`
	NextCursor       string          `json:"nextCursor,omitempty"`
}

type inspectorNodeDetail struct {
	Node         inspectorNode   `json:"node"`
	RelatedNodes []inspectorNode `json:"relatedNodes"`
	Edges        []inspectorEdge `json:"edges"`
	Truncated    bool            `json:"truncated"`
}

type inspectorError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type inspectorErrorEnvelope struct {
	Error inspectorError `json:"error"`
}

type inspectorFilters struct {
	Scope string
	Year  string
	Query string
	Limit int
}

type inspectorCursor struct {
	Version int    `json:"v"`
	BrainID string `json:"b"`
	Scope   string `json:"s"`
	Year    string `json:"y"`
	Query   string `json:"q"`
	Limit   int    `json:"n"`
	LastID  string `json:"l"`
}

type inspectorSourceMeta struct {
	Kind       string            `yaml:"kind"`
	URI        string            `yaml:"uri"`
	OccurredAt string            `yaml:"occurred_at"`
	IndexOnly  bool              `yaml:"index_only"`
	Meta       map[string]string `yaml:"meta"`
}

type inspectorSource struct {
	Source domain.Source
	Fact   *brainstore.MemoryFactPayload
	Expiry *brainstore.MemoryExpiryPayload
	Gone   bool
}

type inspectorFact struct {
	Record    brainstore.MemoryFactRecord
	IndexOnly bool
}

type inspectorExpiry struct {
	ID      string
	Payload brainstore.MemoryExpiryPayload
}

func (d *Dashboard) inspectorBrains(w http.ResponseWriter, r *http.Request) {
	s, ok := d.inspectorSession(w, r)
	if !ok {
		return
	}
	if d.Issuer == nil || d.Issuer.Store == nil {
		writeInspectorError(w, http.StatusServiceUnavailable, "unavailable", "Memory list is temporarily unavailable.")
		return
	}
	brains, err := d.Issuer.Store.Brains(r.Context(), s.AccountID)
	if err != nil {
		writeInspectorError(w, http.StatusServiceUnavailable, "unavailable", "Memory list is temporarily unavailable.")
		return
	}
	out := inspectorBrainList{Version: 1, Brains: []inspectorBrain{}}
	for _, brain := range brains {
		if brain.State != "ready" || brain.PathKey != brain.ID || !validInspectorID(brain.ID) {
			continue
		}
		name := "Project " + brain.ID[:8]
		out.Brains = append(out.Brains, inspectorBrain{ID: brain.ID, Name: name})
	}
	writeInspectorJSON(w, http.StatusOK, out)
}

func (d *Dashboard) inspectorGraph(w http.ResponseWriter, r *http.Request) {
	s, ok := d.inspectorSession(w, r)
	if !ok {
		return
	}
	filters, err := parseInspectorFilters(r)
	if err != nil {
		writeInspectorError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	brainID := r.PathValue("brainID")
	var response inspectorGraphPage
	err = d.Gateway.WithInspectorRead(r.Context(), s.AccountID, brainID, func(view hostgateway.InspectorReadView) error {
		nodes, edges, err := loadInspectorGraph(r.Context(), view)
		if err != nil {
			return err
		}
		matched := filterInspectorNodes(nodes, filters)
		start := 0
		if cursor := r.URL.Query().Get("cursor"); cursor != "" {
			state, err := decodeInspectorCursor(cursor, brainID, filters)
			if err != nil {
				return errInspectorCursor
			}
			start = sort.Search(len(matched), func(i int) bool { return inspectorOrderKey(matched[i]) > state.LastID })
		}
		end := min(start+filters.Limit, len(matched))
		pageNodes := append([]inspectorNode{}, matched[start:end]...)
		pageIDs := make(map[string]bool, len(pageNodes))
		for _, node := range pageNodes {
			pageIDs[node.ID] = true
		}
		contextNodes, pageEdges, contextTruncated := inspectorGraphNeighborhood(nodes, edges, pageIDs)
		response = inspectorGraphPage{Version: 1, BrainID: brainID, PrivateExcluded: true, Nodes: pageNodes, ContextNodes: contextNodes, ContextTruncated: contextTruncated, Edges: pageEdges, TotalMatching: len(matched)}
		if end < len(matched) && end > start {
			response.NextCursor = encodeInspectorCursor(brainID, filters, inspectorOrderKey(matched[end-1]))
		}
		return nil
	})
	if err != nil {
		writeInspectorFailure(w, err)
		return
	}
	writeInspectorJSON(w, http.StatusOK, response)
}

func (d *Dashboard) inspectorNode(w http.ResponseWriter, r *http.Request) {
	s, ok := d.inspectorSession(w, r)
	if !ok {
		return
	}
	brainID, requestedID := r.PathValue("brainID"), r.PathValue("nodeID")
	var response inspectorNodeDetail
	err := d.Gateway.WithInspectorRead(r.Context(), s.AccountID, brainID, func(view hostgateway.InspectorReadView) error {
		nodes, edges, err := loadInspectorGraph(r.Context(), view)
		if err != nil {
			return err
		}
		byID := make(map[string]inspectorNode, len(nodes))
		for _, node := range nodes {
			byID[node.ID] = node
		}
		selected, found := byID[requestedID]
		if !found {
			return errInspectorNodeNotFound
		}
		related := make(map[string]bool)
		for _, edge := range edges {
			if edge.Source == requestedID && edge.Target != requestedID {
				related[edge.Target] = true
			}
			if edge.Target == requestedID && edge.Source != requestedID {
				related[edge.Source] = true
			}
		}
		relatedIDs := make([]string, 0, len(related))
		for id := range related {
			relatedIDs = append(relatedIDs, id)
		}
		sort.Strings(relatedIDs)
		truncated := len(relatedIDs) > inspectorMaxLimit
		if truncated {
			relatedIDs = relatedIDs[:inspectorMaxLimit]
		}
		included := map[string]bool{requestedID: true}
		relatedNodes := make([]inspectorNode, 0, len(relatedIDs))
		for _, id := range relatedIDs {
			included[id] = true
			relatedNodes = append(relatedNodes, byID[id])
		}
		detailEdges := make([]inspectorEdge, 0)
		for _, edge := range edges {
			if included[edge.Source] && included[edge.Target] {
				detailEdges = append(detailEdges, edge)
			}
		}
		response = inspectorNodeDetail{Node: selected, RelatedNodes: relatedNodes, Edges: detailEdges, Truncated: truncated}
		return nil
	})
	if err != nil {
		writeInspectorFailure(w, err)
		return
	}
	writeInspectorJSON(w, http.StatusOK, response)
}

func (d *Dashboard) inspectorSession(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	cookie, err := r.Cookie("serenity_session")
	if err != nil || d.Identity == nil {
		writeInspectorError(w, http.StatusUnauthorized, "unauthorized", "Sign in to view your memory.")
		return identity.Session{}, false
	}
	session, err := d.Identity.Session(r.Context(), cookie.Value)
	if err != nil {
		writeInspectorError(w, http.StatusUnauthorized, "unauthorized", "Sign in to view your memory.")
		return identity.Session{}, false
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: cookie.Value, Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	return session, true
}

func parseInspectorFilters(r *http.Request) (inspectorFilters, error) {
	values := r.URL.Query()
	for key := range values {
		if key != "scope" && key != "year" && key != "q" && key != "limit" && key != "cursor" {
			return inspectorFilters{}, fmt.Errorf("unsupported query parameter %q", key)
		}
		if len(values[key]) != 1 {
			return inspectorFilters{}, fmt.Errorf("query parameter %q must appear once", key)
		}
	}
	filters := inspectorFilters{Scope: values.Get("scope"), Year: values.Get("year"), Query: strings.ToLower(strings.TrimSpace(values.Get("q"))), Limit: inspectorDefaultLimit}
	if filters.Scope == "" {
		filters.Scope = "all"
	}
	if filters.Scope != "all" && filters.Scope != "world" && filters.Scope != "private" {
		return inspectorFilters{}, errors.New("scope must be all, world, or private")
	}
	if filters.Year != "" && filters.Year != "unknown" {
		if len(filters.Year) != 4 {
			return inspectorFilters{}, errors.New("year must be a four-digit year or unknown")
		}
		for _, r := range filters.Year {
			if r < '0' || r > '9' {
				return inspectorFilters{}, errors.New("year must be a four-digit year or unknown")
			}
		}
		n, _ := strconv.Atoi(filters.Year)
		if n < 1 || n > 9999 {
			return inspectorFilters{}, errors.New("year is outside the supported range")
		}
	}
	if utf8.RuneCountInString(filters.Query) > inspectorMaxQueryRunes {
		return inspectorFilters{}, fmt.Errorf("search may contain at most %d characters", inspectorMaxQueryRunes)
	}
	if !utf8.ValidString(filters.Query) {
		return inspectorFilters{}, errors.New("search must be valid UTF-8")
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > inspectorMaxLimit {
			return inspectorFilters{}, fmt.Errorf("limit must be between 1 and %d", inspectorMaxLimit)
		}
		filters.Limit = limit
	}
	return filters, nil
}

var errInspectorCursor = errors.New("invalid cursor")
var errInspectorNodeNotFound = errors.New("inspector node not found")
var errInspectorDatasetTooLarge = errors.New("inspector dataset exceeds the bounded read limit")

func encodeInspectorCursor(brainID string, filters inspectorFilters, lastID string) string {
	data, _ := json.Marshal(inspectorCursor{Version: 1, BrainID: brainID, Scope: filters.Scope, Year: filters.Year, Query: filters.Query, Limit: filters.Limit, LastID: lastID})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeInspectorCursor(raw, brainID string, filters inspectorFilters) (inspectorCursor, error) {
	var cursor inspectorCursor
	if len(raw) > inspectorMaxCursor {
		return cursor, errInspectorCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor, errInspectorCursor
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.Version != 1 || cursor.BrainID != brainID || cursor.Scope != filters.Scope || cursor.Year != filters.Year || cursor.Query != filters.Query || cursor.Limit != filters.Limit || cursor.LastID == "" {
		return inspectorCursor{}, errInspectorCursor
	}
	return cursor, nil
}

func inspectorOrderKey(node inspectorNode) string { return node.Type + "\x00" + node.ID }

// inspectorGraphNeighborhood adds one deterministic hop around the paged
// core. Context nodes come from the same already-filtered eligible projection,
// but do not affect the core query count or cursor.
func inspectorGraphNeighborhood(nodes []inspectorNode, edges []inspectorEdge, core map[string]bool) ([]inspectorNode, []inspectorEdge, bool) {
	byID := make(map[string]inspectorNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	contextIDs := make(map[string]bool)
	for _, edge := range edges {
		if core[edge.Source] && !core[edge.Target] && byID[edge.Target].ID != "" {
			contextIDs[edge.Target] = true
		}
		if core[edge.Target] && !core[edge.Source] && byID[edge.Source].ID != "" {
			contextIDs[edge.Source] = true
		}
	}
	contextKeys := make([]string, 0, len(contextIDs))
	for id := range contextIDs {
		contextKeys = append(contextKeys, id)
	}
	sort.Slice(contextKeys, func(i, j int) bool {
		return inspectorOrderKey(byID[contextKeys[i]]) < inspectorOrderKey(byID[contextKeys[j]])
	})
	truncated := len(contextKeys) > inspectorMaxContext
	if truncated {
		contextKeys = contextKeys[:inspectorMaxContext]
	}
	contextNodes := make([]inspectorNode, 0, len(contextKeys))
	included := make(map[string]bool, len(core)+len(contextKeys))
	for id := range core {
		included[id] = true
	}
	for _, id := range contextKeys {
		contextNodes = append(contextNodes, byID[id])
		included[id] = true
	}
	pageEdges := make([]inspectorEdge, 0)
	for _, edge := range edges {
		if included[edge.Source] && included[edge.Target] {
			pageEdges = append(pageEdges, edge)
		}
	}
	sort.Slice(pageEdges, func(i, j int) bool { return pageEdges[i].ID < pageEdges[j].ID })
	return contextNodes, pageEdges, truncated
}

func filterInspectorNodes(nodes []inspectorNode, filters inspectorFilters) []inspectorNode {
	out := make([]inspectorNode, 0, len(nodes))
	for _, node := range nodes {
		if filters.Scope == "private" || filters.Scope == "world" && node.Scope != "world" {
			continue
		}
		if filters.Year != "" {
			date := node.CapturedAt
			if date == nil && node.Type == "entity" && node.DateKind == "earliest-linked-memory" {
				date = node.CreatedAt
			}
			if date == nil {
				if filters.Year != "unknown" {
					continue
				}
			} else if filters.Year == "unknown" || date.UTC().Format("2006") != filters.Year {
				continue
			}
		}
		if filters.Query != "" {
			searchable := strings.ToLower(strings.Join([]string{node.Type, node.Kind, node.Label, node.Text, node.Scope, node.Status}, " "))
			if !strings.Contains(searchable, filters.Query) {
				continue
			}
		}
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool { return inspectorOrderKey(out[i]) < inspectorOrderKey(out[j]) })
	return out
}

func writeInspectorJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeInspectorError(w http.ResponseWriter, status int, code, message string) {
	writeInspectorJSON(w, status, inspectorErrorEnvelope{Error: inspectorError{Code: code, Message: message}})
}

func writeInspectorFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errInspectorCursor):
		writeInspectorError(w, http.StatusBadRequest, "invalid_cursor", "The cursor is invalid or belongs to different filters.")
	case errors.Is(err, errInspectorNodeNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, hostgateway.ErrInspectorBrainUnavailable):
		writeInspectorError(w, http.StatusNotFound, "not_found", "The requested memory is unavailable.")
	case errors.Is(err, errInspectorDatasetTooLarge):
		writeInspectorError(w, http.StatusUnprocessableEntity, "dataset_too_large", "This memory is too large for the bounded inspector read.")
	default:
		writeInspectorError(w, http.StatusServiceUnavailable, "unavailable", "Memory is temporarily unavailable.")
	}
}

func validInspectorID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func loadInspectorGraph(ctx context.Context, view hostgateway.InspectorReadView) ([]inspectorNode, []inspectorEdge, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sources, facts, expiries, err := readInspectorSourceCatalog(ctx, view.Root)
	if err != nil {
		return nil, nil, err
	}
	applyInspectorExpiries(facts, expiries)

	claims, err := readInspectorClaims(ctx, view.Root)
	if err != nil {
		return nil, nil, err
	}
	entities, pageClaims, err := readInspectorEntities(ctx, view.Root)
	if err != nil {
		return nil, nil, err
	}
	for id, claim := range pageClaims {
		if prior, ok := claims[id]; ok {
			claim = mergeInspectorClaim(prior, claim)
		}
		claims[id] = claim
	}
	restrictedEntities := make(map[string]bool)
	for _, rec := range facts {
		if rec.IndexOnly || rec.Record.Payload.Visibility != brainstore.MemoryVisibilityWorld || rec.Record.Expired(view.Now) {
			if rec.Record.Payload.EntitySlug != "" {
				restrictedEntities[rec.Record.Payload.EntitySlug] = true
			}
		}
	}
	for _, claim := range claims {
		if claim.SubjectSlug != "" && (claim.Visibility == domain.VisibilityPrivate || !inspectorClaimSourceEligible(claim, sources, facts, view.Now)) {
			restrictedEntities[claim.SubjectSlug] = true
		}
	}

	nodes := make(map[string]inspectorNode)
	edges := make(map[string]inspectorEdge)
	entityDates := make(map[string]time.Time)
	entityLabels := make(map[string]string)
	entityKinds := make(map[string]string)
	addEntity := func(slug, kind, label string) string {
		if slug == "" {
			return ""
		}
		id := "entity:" + slug
		if label == "" {
			label = slug
		}
		if prior := nodes[id]; prior.ID == "" {
			entityLabels[slug] = label
			entityKinds[slug] = kind
		}
		return id
	}
	for _, rec := range facts {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if rec.IndexOnly || rec.Record.Payload.Visibility != brainstore.MemoryVisibilityWorld || rec.Record.Expired(view.Now) {
			continue
		}
		p := rec.Record.Payload
		id := "fact:" + rec.Record.SHA256
		created := p.CreatedAt.UTC()
		node := inspectorNode{Type: "fact", ID: id, Kind: string(p.Kind), Text: p.Fact, Scope: "world", Status: "active", CreatedAt: &created, CapturedAt: &created, DateKind: "captured"}
		if p.ValidUntil != nil {
			until := p.ValidUntil.UTC().Format(time.RFC3339Nano)
			node.ValidUntil = &until
		}
		if p.EntitySlug != "" {
			if !restrictedEntities[p.EntitySlug] {
				entityID := addEntity(p.EntitySlug, p.EntityType, p.EntitySlug)
				node.EntityID = entityID
				addInspectorEdge(edges, id, "about", entityID)
				if current, ok := entityDates[p.EntitySlug]; !ok || created.Before(current) {
					entityDates[p.EntitySlug] = created
				}
			}
		}
		if src, ok := sources[rec.Record.SHA256]; ok && !src.Gone && !src.Source.IndexOnly {
			sourceID := addInspectorSourceNode(nodes, edges, id, src)
			node.SourceIDs = []string{sourceID}
		}
		nodes[id] = node
	}

	for _, claim := range claims {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !inspectorClaimEligible(claim, sources, facts, view.Now) {
			continue
		}
		id := "claim:" + claim.ID
		entityID := ""
		if !restrictedEntities[claim.SubjectSlug] {
			entityID = addEntity(claim.SubjectSlug, "", claim.SubjectSlug)
		}
		confidence := claim.Confidence
		node := inspectorNode{Type: "claim", ID: id, Kind: claim.Family, Label: claim.Predicate, Text: strings.TrimSpace(claim.Predicate + " " + claim.Object), EntityID: entityID, Scope: "world", Status: string(claim.State), ObservedAt: nonzeroTime(claim.Provenance.ObservedAt), CreatedAt: nil, CapturedAt: nil, DateKind: "unknown", Confidence: &confidence}
		if claim.ValidTo != "" {
			validUntil := claim.ValidTo
			node.ValidUntil = &validUntil
		}
		if claim.Supersedes != "" {
			node.Supersedes = "claim:" + claim.Supersedes
			addInspectorEdge(edges, id, "supersedes", node.Supersedes)
		}
		if claim.SupersededBy != "" {
			node.SupersededBy = "claim:" + claim.SupersededBy
			addInspectorEdge(edges, node.SupersededBy, "supersedes", id)
		}
		if entityID != "" {
			addInspectorEdge(edges, id, "about", entityID)
		}
		if claim.Provenance.SourceSHA256 != "" {
			if src, ok := sources[claim.Provenance.SourceSHA256]; ok && !src.Gone && !src.Source.IndexOnly {
				sourceID := addInspectorSourceNode(nodes, edges, id, src)
				node.SourceIDs = []string{sourceID}
			}
		}
		nodes[id] = node
	}

	for slug, entity := range entities {
		if !restrictedEntities[slug] && entityLabels[slug] != "" {
			entityLabels[slug] = entity.label
			entityKinds[slug] = entity.kind
		}
	}
	for slug, label := range entityLabels {
		id := "entity:" + slug
		created := entityDates[slug]
		node := inspectorNode{Type: "entity", ID: id, Kind: entityKinds[slug], Label: label, Scope: "world", Status: "active", DateKind: "unknown"}
		if !created.IsZero() {
			date := created.UTC()
			node.CreatedAt = &date
			node.DateKind = "earliest-linked-memory"
		}
		nodes[id] = node
	}

	if len(nodes) > inspectorMaxNodes {
		return nil, nil, errInspectorDatasetTooLarge
	}
	nodeIDs := make([]string, 0, len(nodes))
	for id := range nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	outNodes := make([]inspectorNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node := nodes[id]
		sort.Strings(node.SourceIDs)
		outNodes = append(outNodes, node)
	}
	edgeIDs := make([]string, 0, len(edges))
	for id := range edges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	outEdges := make([]inspectorEdge, 0, len(edgeIDs))
	for _, id := range edgeIDs {
		edge := edges[id]
		if nodes[edge.Source].ID != "" && nodes[edge.Target].ID != "" {
			outEdges = append(outEdges, edge)
		}
	}
	return outNodes, outEdges, nil
}

type inspectorEntityInfo struct{ label, kind string }

func readInspectorEntities(ctx context.Context, root string) (map[string]inspectorEntityInfo, map[string]domain.Claim, error) {
	files, err := boundedInspectorTree(ctx, root, "brain/entities", inspectorMaxFiles, inspectorMaxFileBytes, inspectorMaxReadBytes)
	if err != nil {
		return nil, nil, err
	}
	entities := make(map[string]inspectorEntityInfo)
	claims := make(map[string]domain.Claim)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if filepath.Ext(path) != ".md" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		page, err := brainstore.ParseEntityBytes(data)
		if err != nil {
			return nil, nil, err
		}
		if page.Entity.Slug == "" {
			continue
		}
		entities[page.Entity.Slug] = inspectorEntityInfo{label: page.Title, kind: page.Entity.Type}
		for _, claim := range page.Claims {
			if claim.ID == "" {
				continue
			}
			claims[claim.ID] = mergeInspectorClaim(claims[claim.ID], claim)
		}
	}
	return entities, claims, nil
}

func readInspectorClaims(ctx context.Context, root string) (map[string]domain.Claim, error) {
	if _, err := boundedInspectorTree(ctx, root, "brain/claims", inspectorMaxFiles, inspectorMaxFileBytes, inspectorMaxReadBytes); err != nil {
		return nil, err
	}
	shards := brainstore.NewShardStore(root)
	slugs, err := shards.Slugs()
	if err != nil {
		return nil, err
	}
	claims := make(map[string]domain.Claim)
	for _, slug := range slugs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		families, err := shards.Families(slug)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			lines, err := shards.Lines(slug, family)
			if err != nil {
				return nil, err
			}
			for _, claim := range lines {
				if claim.ID == "" {
					continue
				}
				claims[claim.ID] = mergeInspectorClaimRevision(claims[claim.ID], claim)
				if len(claims) > inspectorMaxNodes {
					return nil, errInspectorDatasetTooLarge
				}
			}
		}
	}
	return claims, nil
}

func mergeInspectorClaim(a, b domain.Claim) domain.Claim {
	if a.ID == "" {
		return b
	}
	if a.Visibility == domain.VisibilityPrivate || b.Visibility == domain.VisibilityPrivate {
		a.Visibility = domain.VisibilityPrivate
	} else if a.Visibility == "" && b.Visibility != "" {
		a.Visibility = b.Visibility
	}
	if a.State == domain.StateRetracted || b.State == domain.StateRetracted {
		a.State = domain.StateRetracted
	} else if a.State == "" {
		a.State = b.State
	}
	if a.Provenance.SourceSHA256 == "" {
		a.Provenance.SourceSHA256 = b.Provenance.SourceSHA256
	}
	if a.Provenance.ObservedAt.IsZero() {
		a.Provenance.ObservedAt = b.Provenance.ObservedAt
	}
	if a.SupersededBy == "" {
		a.SupersededBy = b.SupersededBy
	}
	if a.Supersedes == "" {
		a.Supersedes = b.Supersedes
	}
	if a.SubjectSlug == "" {
		a.SubjectSlug = b.SubjectSlug
	}
	if a.Predicate == "" {
		a.Predicate = b.Predicate
	}
	if a.Object == "" {
		a.Object = b.Object
	}
	if a.Family == "" {
		a.Family = b.Family
	}
	if a.Confidence == 0 {
		a.Confidence = b.Confidence
	}
	if a.ValidTo == "" {
		a.ValidTo = b.ValidTo
	}
	if a.ValidFrom == "" {
		a.ValidFrom = b.ValidFrom
	}
	return a
}

// mergeInspectorClaimRevision consumes append-only shard history in order.
// Retraction and private visibility remain sticky, while ordinary state
// transitions follow the latest canonical shard line.
func mergeInspectorClaimRevision(a, b domain.Claim) domain.Claim {
	if a.ID == "" {
		return b
	}
	out := mergeInspectorClaim(a, b)
	if a.State != domain.StateRetracted && b.State != domain.StateRetracted {
		out.State = b.State
	}
	return out
}

func inspectorClaimEligible(claim domain.Claim, sources map[string]inspectorSource, facts map[string]inspectorFact, now time.Time) bool {
	if claim.ID == "" || claim.State != domain.StateActive && claim.State != domain.StateSuperseded || claim.Visibility != "" && claim.Visibility != domain.VisibilityShared {
		return false
	}
	if !claim.CurrentAt(now) {
		return false
	}
	return inspectorClaimSourceEligible(claim, sources, facts, now)
}

func inspectorClaimSourceEligible(claim domain.Claim, sources map[string]inspectorSource, facts map[string]inspectorFact, now time.Time) bool {
	sha := claim.Provenance.SourceSHA256
	if sha == "" {
		return true
	}
	src, ok := sources[sha]
	if !ok || src.Gone || src.Source.IndexOnly || src.Source.Kind == brainstore.SourceKindMemoryExpiry || src.Source.Kind == brainstore.SourceKindTombstone {
		return false
	}
	if fact, ok := facts[sha]; ok {
		return !fact.IndexOnly && fact.Record.Payload.Visibility == brainstore.MemoryVisibilityWorld && !fact.Record.Expired(now)
	}
	if src.Source.Kind == brainstore.SourceKindMemoryFact {
		return false
	}
	return true
}

func nonzeroTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	v := value.UTC()
	return &v
}

func safeInspectorKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, allowed := range []string{"person", "project", "concept", "place", "fact", "decision", "constraint", "intent", "question", "preference", "relationship", "event", "goal", "email", "file", "git_repo", "voice", "memory_fact"} {
		if value == allowed {
			return value
		}
	}
	if value == "" {
		return "entity"
	}
	return "other"
}

func addInspectorSourceNode(nodes map[string]inspectorNode, edges map[string]inspectorEdge, from string, source inspectorSource) string {
	id := "source:" + source.Source.SHA256
	if nodes[id].ID == "" {
		observed := nonzeroTime(source.Source.OccurredAt)
		kind := safeInspectorKind(source.Source.Kind)
		label := "Source"
		if kind != "other" && kind != "entity" {
			label = strings.ReplaceAll(kind, "_", " ")
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		nodes[id] = inspectorNode{Type: "source", ID: id, Kind: kind, Label: label, Scope: "world", Status: "available", ObservedAt: observed, DateKind: "observed"}
	}
	addInspectorEdge(edges, from, "evidence", id)
	return id
}

func addInspectorEdge(edges map[string]inspectorEdge, source, kind, target string) {
	if source == "" || target == "" {
		return
	}
	key := source + "\x00" + kind + "\x00" + target
	h := sha256.Sum256([]byte(key))
	id := "edge:" + hex.EncodeToString(h[:])
	edges[id] = inspectorEdge{ID: id, Type: kind, Source: source, Target: target}
}

func readInspectorSourceCatalog(ctx context.Context, root string) (map[string]inspectorSource, map[string]inspectorFact, []inspectorExpiry, error) {
	sourceRoot := filepath.Join(root, "brain", "sources")
	if err := validateInspectorPath(root, "brain/sources", true); err != nil {
		return nil, nil, nil, err
	}
	prefixes, err := inspectorReadDir(sourceRoot, 257)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]inspectorSource{}, map[string]inspectorFact{}, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	sources := make(map[string]inspectorSource)
	facts := make(map[string]inspectorFact)
	expiries := make([]inspectorExpiry, 0)
	gone := make(map[string]bool)
	var metaBytes, bodyBytes int64
	for _, prefix := range prefixes {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if prefix.Name() == ".gitkeep" && prefix.Type().IsRegular() {
			continue
		}
		if !validInspectorHexPrefix(prefix.Name()) || !prefix.IsDir() || prefix.Type()&fs.ModeSymlink != 0 {
			return nil, nil, nil, errors.New("inspector: unsafe source prefix")
		}
		prefixPath := filepath.Join(sourceRoot, prefix.Name())
		entries, err := inspectorReadDir(prefixPath, inspectorMaxFiles-len(sources)+1)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}
			if len(sources) >= inspectorMaxFiles {
				return nil, nil, nil, errInspectorDatasetTooLarge
			}
			sha := entry.Name()
			if !brainstore.ValidSourceSHA(sha) || !strings.HasPrefix(sha, prefix.Name()) || !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
				return nil, nil, nil, errors.New("inspector: unsafe source entry")
			}
			dir := filepath.Join(prefixPath, sha)
			dirEntries, err := inspectorReadDir(dir, 3)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, item := range dirEntries {
				if item.Type()&fs.ModeSymlink != 0 || item.IsDir() || item.Name() != "meta.yaml" && item.Name() != "bytes" {
					return nil, nil, nil, errors.New("inspector: unexpected source entry")
				}
			}
			metaPath := filepath.Join(dir, "meta.yaml")
			metaInfo, err := os.Lstat(metaPath)
			if err != nil {
				return nil, nil, nil, err
			}
			if !metaInfo.Mode().IsRegular() || metaInfo.Mode()&fs.ModeSymlink != 0 || metaInfo.Size() > inspectorMaxMetaFile {
				return nil, nil, nil, errors.New("inspector: unsafe source metadata")
			}
			metaBytes += metaInfo.Size()
			if metaBytes > inspectorMaxMetaBytes {
				return nil, nil, nil, errInspectorDatasetTooLarge
			}
			metaData, err := os.ReadFile(metaPath)
			if err != nil {
				return nil, nil, nil, err
			}
			var meta inspectorSourceMeta
			if err := yaml.Unmarshal(metaData, &meta); err != nil || strings.TrimSpace(meta.Kind) == "" {
				return nil, nil, nil, errors.New("inspector: invalid source metadata")
			}
			source := domain.Source{SHA256: sha, Kind: meta.Kind, URI: meta.URI, IndexOnly: meta.IndexOnly, Meta: meta.Meta}
			if meta.OccurredAt != "" {
				t, err := time.Parse(time.RFC3339Nano, meta.OccurredAt)
				if err != nil {
					return nil, nil, nil, errors.New("inspector: invalid source date")
				}
				source.OccurredAt = t.UTC()
			}
			bytesPath := filepath.Join(dir, "bytes")
			bytesInfo, bytesErr := os.Lstat(bytesPath)
			if bytesErr == nil {
				if !bytesInfo.Mode().IsRegular() || bytesInfo.Mode()&fs.ModeSymlink != 0 {
					return nil, nil, nil, errors.New("inspector: unsafe source bytes")
				}
			} else if !errors.Is(bytesErr, fs.ErrNotExist) || !meta.IndexOnly {
				return nil, nil, nil, errors.New("inspector: source bytes are unavailable")
			}
			entry := inspectorSource{Source: source}
			if meta.Kind == brainstore.SourceKindMemoryFact || meta.Kind == brainstore.SourceKindMemoryExpiry || meta.Kind == brainstore.SourceKindTombstone {
				if bytesErr != nil || bytesInfo.Size() > inspectorMaxMetaFile {
					return nil, nil, nil, errInspectorDatasetTooLarge
				}
				bodyBytes += bytesInfo.Size()
				if bodyBytes > inspectorMaxReadBytes {
					return nil, nil, nil, errInspectorDatasetTooLarge
				}
				body, err := os.ReadFile(bytesPath)
				if err != nil {
					return nil, nil, nil, err
				}
				hash := sha256.Sum256(body)
				if hex.EncodeToString(hash[:]) != sha {
					return nil, nil, nil, errors.New("inspector: canonical memory record hash mismatch")
				}
				switch meta.Kind {
				case brainstore.SourceKindMemoryFact:
					payload, err := brainstore.DecodeMemoryFact(body)
					if err != nil || meta.URI != "memory://fact" || !source.OccurredAt.Equal(payload.CreatedAt) {
						return nil, nil, nil, errors.New("inspector: invalid memory fact source")
					}
					entry.Fact = &payload
					facts[sha] = inspectorFact{Record: brainstore.MemoryFactRecord{SHA256: sha, Payload: payload}, IndexOnly: meta.IndexOnly}
				case brainstore.SourceKindMemoryExpiry:
					payload, err := brainstore.DecodeMemoryExpiry(body)
					if err != nil || meta.URI != "memory://expiry" || !source.OccurredAt.Equal(payload.ExpiredAt) {
						return nil, nil, nil, errors.New("inspector: invalid memory expiry source")
					}
					entry.Expiry = &payload
					expiries = append(expiries, inspectorExpiry{ID: sha, Payload: payload})
				case brainstore.SourceKindTombstone:
					var payload brainstore.SourceTombstonePayload
					decoder := json.NewDecoder(strings.NewReader(string(body)))
					decoder.DisallowUnknownFields()
					if decoder.Decode(&payload) != nil || payload.FormatVersion != 1 || payload.RecordType != brainstore.SourceKindTombstone || !brainstore.ValidSourceSHA(payload.TargetSHA256) || meta.URI != brainstore.SourceTombstoneURI || meta.IndexOnly {
						return nil, nil, nil, errors.New("inspector: invalid source tombstone")
					}
					canonical, _ := brainstore.EncodeSourceTombstone(payload.TargetSHA256)
					if string(canonical) != string(body) {
						return nil, nil, nil, errors.New("inspector: non-canonical source tombstone")
					}
					entry.Gone = true
					gone[payload.TargetSHA256] = true
				}
			}
			sources[sha] = entry
		}
	}
	for sha, source := range sources {
		if gone[sha] {
			source.Gone = true
			sources[sha] = source
		}
	}
	for sha, fact := range facts {
		if gone[sha] {
			fact.IndexOnly = true
			facts[sha] = fact
		}
	}
	return sources, facts, expiries, nil
}

func applyInspectorExpiries(facts map[string]inspectorFact, expiries []inspectorExpiry) {
	operations := make(map[string]string)
	for sha, fact := range facts {
		if fact.Record.Payload.OperationKey != "" {
			operations[fact.Record.Payload.OperationKey] = sha
		}
	}
	for _, expiry := range expiries {
		target := expiry.Payload.TargetSHA256
		if expiry.Payload.OperationKey != "" {
			target = operations[expiry.Payload.OperationKey]
		}
		fact, ok := facts[target]
		if !ok {
			continue
		}
		t := expiry.Payload.ExpiredAt
		if fact.Record.ExpiredAt == nil || t.Before(*fact.Record.ExpiredAt) {
			fact.Record.ExpiredAt = &t
			fact.Record.ExpirySHA256 = expiry.ID
			fact.Record.ExpiredReason = expiry.Payload.Reason
			facts[target] = fact
		}
	}
}

func validInspectorHexPrefix(value string) bool {
	if len(value) != 2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func inspectorReadDir(path string, max int) ([]os.DirEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	entries, readErr := file.ReadDir(max + 1)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(entries) > max {
		return nil, errInspectorDatasetTooLarge
	}
	return entries, nil
}

func validateInspectorPath(root, relative string, allowMissing bool) error {
	current := root
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return errors.New("inspector: unsafe canonical path")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("inspector: unsafe canonical directory")
		}
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return err
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Dir(resolved) == "" || !isInspectorDescendant(rootReal, resolved) {
		return errors.New("inspector: canonical directory escapes brain root")
	}
	return nil
}

func isInspectorDescendant(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func boundedInspectorTree(ctx context.Context, root, relative string, maxFiles int, maxFileBytes, maxTotalBytes int64) ([]string, error) {
	if err := validateInspectorPath(root, relative, true); err != nil {
		return nil, err
	}
	base := filepath.Join(root, filepath.Clean(relative))
	if _, err := os.Lstat(base); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	files := make([]string, 0)
	var total int64
	var entriesSeen int
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		entriesSeen++
		if entriesSeen > maxFiles*2 {
			return errInspectorDatasetTooLarge
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("inspector: symlink in canonical tree")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("inspector: non-regular canonical file")
		}
		if info.Size() > maxFileBytes {
			return errInspectorDatasetTooLarge
		}
		total += info.Size()
		if total > maxTotalBytes || len(files) >= maxFiles {
			return errInspectorDatasetTooLarge
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
