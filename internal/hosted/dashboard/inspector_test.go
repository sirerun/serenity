package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

func TestInspectorFilterContract(t *testing.T) {
	tests := []struct {
		query string
		want  inspectorFilters
		bad   bool
	}{
		{query: "", want: inspectorFilters{Scope: "all", Type: "all", Limit: inspectorDefaultLimit}},
		{query: "scope=private&year=unknown&q=%20%20tea%20%20", want: inspectorFilters{Scope: "private", Year: "unknown", Query: "tea", Type: "all", Limit: inspectorDefaultLimit}},
		{query: "limit=100&year=2024", want: inspectorFilters{Scope: "all", Year: "2024", Type: "all", Limit: 100}},
		{query: "type=claim", want: inspectorFilters{Scope: "all", Type: "claim", Limit: inspectorDefaultLimit}},
		{query: "limit=101", bad: true},
		{query: "year=20x4", bad: true},
		{query: "type=other", bad: true},
		{query: "extra=value", bad: true},
		{query: "scope=all&scope=world", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			got, err := parseInspectorFilters(r)
			if (err != nil) != tt.bad {
				t.Fatalf("parseInspectorFilters error = %v, want bad=%v", err, tt.bad)
			}
			if err == nil && got != tt.want {
				t.Fatalf("filters = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestInspectorCursorTypeBindingAndLegacyDefaultAll(t *testing.T) {
	all, err := parseInspectorFilters(httptest.NewRequest(http.MethodGet, "/?limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":"BrainOwnerABCDEFGHIJKLMNOP","s":"all","y":"","q":"","n":2,"l":"fact:abc"}`))
	if _, err := decodeInspectorCursor(legacy, "BrainOwnerABCDEFGHIJKLMNOP", all); err != nil {
		t.Fatalf("legacy default-all cursor rejected: %v", err)
	}
	claimFilters, err := parseInspectorFilters(httptest.NewRequest(http.MethodGet, "/?type=claim&limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeInspectorCursor(legacy, "BrainOwnerABCDEFGHIJKLMNOP", claimFilters); err == nil {
		t.Fatal("legacy cursor without a type was accepted for a non-default type filter")
	}
	cursor := encodeInspectorCursor("BrainOwnerABCDEFGHIJKLMNOP", all, "fact:abc")
	if _, err := decodeInspectorCursor(cursor, "BrainOwnerABCDEFGHIJKLMNOP", claimFilters); err == nil {
		t.Fatal("cursor was accepted after the type filter changed")
	}
}

func TestInspectorYearFilterIncludesExplicitUnknownAndEntityEarliestDate(t *testing.T) {
	known := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	nodes := []inspectorNode{
		{ID: "fact:one", Type: "fact", CapturedAt: &known},
		{ID: "claim:two", Type: "claim", ObservedAt: &known},
		{ID: "entity:three", Type: "entity", CreatedAt: &known, DateKind: "earliest-linked-memory"},
		{ID: "source:four", Type: "source", ObservedAt: &known},
		{ID: "claim:unknown", Type: "claim"},
	}
	if got := filterInspectorNodes(nodes, inspectorFilters{Year: "2024"}); len(got) != 2 || got[0].ID != "entity:three" || got[1].ID != "fact:one" {
		t.Fatalf("known-year nodes = %#v", got)
	}
	// An observed date is not a captured date; those records remain unknown
	// on the capture-time axis rather than appearing in the observed year.
	if got := filterInspectorNodes(nodes, inspectorFilters{Year: "unknown"}); len(got) != 3 || got[0].ID != "claim:two" || got[1].ID != "claim:unknown" || got[2].ID != "source:four" {
		t.Fatalf("unknown-date nodes = %#v", got)
	}
}

func TestInspectorUnauthenticatedAPIReturnsJSON401WithoutRedirect(t *testing.T) {
	d := &Dashboard{}
	request := httptest.NewRequest(http.MethodGet, "/api/inspector/v1/brains", nil)
	response := httptest.NewRecorder()
	d.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("Location") != "" {
		t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content type = %q", got)
	}
	var body inspectorErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != "unauthorized" {
		t.Fatalf("JSON auth response = %q, err=%v", response.Body.String(), err)
	}
}

func TestInspectorArrayFieldsEncodeAsEmptyArrays(t *testing.T) {
	page := inspectorGraphPage{Nodes: []inspectorNode{}, ContextNodes: []inspectorNode{}, Edges: []inspectorEdge{}}
	detail := inspectorNodeDetail{RelatedNodes: []inspectorNode{}, Edges: []inspectorEdge{}}
	wantFields := map[string][]string{
		"page":   {"nodes", "contextNodes", "edges"},
		"detail": {"relatedNodes", "edges"},
		"brains": {"brains"},
	}
	for name, value := range map[string]any{"page": page, "detail": detail, "brains": inspectorBrainList{Brains: []inspectorBrain{}}} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range wantFields[name] {
			if raw := fields[key]; string(raw) != "[]" {
				t.Errorf("%s.%s = %s, want empty array", name, key, raw)
			}
		}
	}
}

func TestInspectorClaimEligibilityUsesCanonicalDatePrecisionAndFailsClosed(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	base := domain.Claim{ID: "claim-date-test", State: domain.StateActive, Visibility: domain.VisibilityShared}
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{name: "year ended", to: "2020", want: false},
		{name: "year active", to: "2027", want: true},
		{name: "month expired", to: "2026-09", want: false},
		{name: "month active", to: "2026-11", want: true},
		{name: "day expired", to: "2026-10-03", want: false},
		{name: "day active", to: "2026-10-05", want: true},
		{name: "timestamp expired", to: "2026-10-04T11:59:59Z", want: false},
		{name: "future year", from: "2027", want: false},
		{name: "future month", from: "2026-11", want: false},
		{name: "malformed end", to: "next year", want: false},
		{name: "malformed start", from: "sometime", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claim := base
			claim.ValidFrom, claim.ValidTo = tt.from, tt.to
			if got := inspectorClaimEligible(claim, map[string]inspectorSource{}, map[string]inspectorFact{}, now); got != tt.want {
				t.Fatalf("eligible = %v, want %v for from=%q to=%q", got, tt.want, tt.from, tt.to)
			}
		})
	}
}

func TestInspectorDuplicateClaimMergeCannotWeakenPrivacyRetractionOrSource(t *testing.T) {
	base := domain.Claim{
		ID: "same-claim", Visibility: domain.VisibilityPrivate, State: domain.StateSuperseded,
		Provenance: domain.Provenance{SourceSHA256: "canonical-source"},
	}
	page := domain.Claim{
		ID: "same-claim", Visibility: domain.VisibilityShared, State: domain.StateActive,
		Provenance: domain.Provenance{SourceSHA256: "page-source"},
	}
	got := mergeInspectorClaim(base, page)
	if got.Visibility != domain.VisibilityPrivate || got.State != domain.StateSuperseded || got.Provenance.SourceSHA256 != "canonical-source" {
		t.Fatalf("duplicate merge weakened authoritative shard state: %+v", got)
	}
	retracted := base
	retracted.State = domain.StateRetracted
	if got := mergeInspectorClaimRevision(retracted, page); got.State != domain.StateRetracted {
		t.Fatalf("append revision resurrected retracted claim: %+v", got)
	}
	active := page
	active.Visibility = domain.VisibilityShared
	active.State = domain.StateActive
	for name, merge := range map[string]func(domain.Claim, domain.Claim) domain.Claim{
		"duplicate":       mergeInspectorClaim,
		"append revision": mergeInspectorClaimRevision,
	} {
		t.Run(name, func(t *testing.T) {
			if got := merge(active, retracted); got.State != domain.StateRetracted {
				t.Fatalf("active claim masked a duplicate retraction: %+v", got)
			}
		})
	}
}

func TestInspectorGraphNeighborhoodIsBoundedDeterministicAndInduced(t *testing.T) {
	core := inspectorNode{Type: "fact", ID: "fact:core"}
	nodes := []inspectorNode{core}
	edges := make([]inspectorEdge, 0, inspectorMaxContext+5)
	for i := inspectorMaxContext + 4; i >= 0; i-- {
		id := fmt.Sprintf("entity:person-%03d", i)
		nodes = append(nodes, inspectorNode{Type: "entity", ID: id})
		edgeID := fmt.Sprintf("edge:%03d", i)
		edges = append(edges, inspectorEdge{ID: edgeID, Type: "about", Source: core.ID, Target: id})
	}
	contextNodes, induced, truncated := inspectorGraphNeighborhood(nodes, edges, map[string]bool{core.ID: true})
	if len(contextNodes) != inspectorMaxContext || !truncated || len(induced) != inspectorMaxContext {
		t.Fatalf("context=%d edges=%d truncated=%v", len(contextNodes), len(induced), truncated)
	}
	if contextNodes[0].ID != "entity:person-000" || contextNodes[len(contextNodes)-1].ID != "entity:person-099" {
		t.Fatalf("context ordering starts/ends at %q/%q", contextNodes[0].ID, contextNodes[len(contextNodes)-1].ID)
	}
	reversedNodes := append([]inspectorNode(nil), nodes...)
	reversedEdges := append([]inspectorEdge(nil), edges...)
	for i, j := 0, len(reversedNodes)-1; i < j; i, j = i+1, j-1 {
		reversedNodes[i], reversedNodes[j] = reversedNodes[j], reversedNodes[i]
	}
	for i, j := 0, len(reversedEdges)-1; i < j; i, j = i+1, j-1 {
		reversedEdges[i], reversedEdges[j] = reversedEdges[j], reversedEdges[i]
	}
	contextAgain, edgesAgain, truncatedAgain := inspectorGraphNeighborhood(reversedNodes, reversedEdges, map[string]bool{core.ID: true})
	if !truncatedAgain || len(contextAgain) != len(contextNodes) || len(edgesAgain) != len(induced) {
		t.Fatalf("reversed input changed neighborhood sizes or truncation")
	}
	for i := range contextNodes {
		if contextAgain[i].ID != contextNodes[i].ID {
			t.Fatalf("context order changed at %d: %s vs %s", i, contextAgain[i].ID, contextNodes[i].ID)
		}
	}
	for i := range induced {
		if edgesAgain[i].ID != induced[i].ID {
			t.Fatalf("edge order changed at %d: %s vs %s", i, edgesAgain[i].ID, induced[i].ID)
		}
	}
}

func TestInspectorGraphNeighborhoodEmptyPageHasNoContext(t *testing.T) {
	nodes := []inspectorNode{{Type: "fact", ID: "fact:one"}, {Type: "entity", ID: "entity:one"}}
	edges := []inspectorEdge{{ID: "edge:one", Source: "fact:one", Target: "entity:one"}}
	contextNodes, induced, truncated := inspectorGraphNeighborhood(nodes, edges, map[string]bool{})
	if contextNodes == nil || len(contextNodes) != 0 || len(induced) != 0 || truncated {
		t.Fatalf("empty core returned context=%v edges=%v truncated=%v", contextNodes, induced, truncated)
	}
}
