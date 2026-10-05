package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
)

func inspectorFacetsHTTPHandler(f *inspectorHTTPFixture) http.Handler {
	issuer := &credential.Issuer{Store: f.store}
	d := &Dashboard{
		Identity: &identity.Service{Store: f.store},
		Issuer:   issuer,
		Gateway:  &gateway.Gateway{Issuer: issuer, Pool: f.pool},
		Dev:      true,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/inspector/v1/brains/{brainID}/facets", d.inspectorFacets)
	return mux
}

func inspectorFacetGet(f *inspectorHTTPFixture, handler http.Handler, user, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token := f.tokens[user]; token != "" {
		req.AddCookie(&http.Cookie{Name: "serenity_session", Value: token})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestInspectorFacetsHTTPAuthOwnershipCountsAndYearSemantics(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	handler := inspectorFacetsHTTPHandler(f)
	path := "/api/inspector/v1/brains/" + "BrainOwnerABCDEFGHIJKLMNOP" + "/facets"
	before := inspectorTreeDigest(t, f.brains["owner"])

	unauthorized := inspectorFacetGet(f, handler, "", path)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized facets status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	foreign := inspectorFacetGet(f, handler, "other", path)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign brain facets status=%d body=%s", foreign.Code, foreign.Body.String())
	}

	for _, query := range []string{"", "?year=unknown", "?year=2025"} {
		response := inspectorFacetGet(f, handler, "owner", path+query)
		if response.Code != http.StatusOK {
			t.Fatalf("facets %q status=%d body=%s", query, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("facets cache-control = %q", response.Header().Get("Cache-Control"))
		}
		var got inspectorFacetsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Version != 1 || got.BrainID != "BrainOwnerABCDEFGHIJKLMNOP" || !got.PrivateExcluded || got.TotalMatching != 6 {
			t.Fatalf("facets %q = %#v", query, got)
		}
		if len(got.Years) != 2 || got.Years[0] != (inspectorYearCount{Year: "2025", Count: 3}) || got.Years[1] != (inspectorYearCount{Year: "unknown", Count: 3}) {
			t.Fatalf("facets years %q = %#v, selected year must not constrain counts", query, got.Years)
		}
		if got.TypeCounts["fact"] != 3 || got.TypeCounts["claim"] != 0 || got.TypeCounts["entity"] != 0 || got.TypeCounts["source"] != 3 {
			t.Fatalf("facets type counts %q = %#v", query, got.TypeCounts)
		}
	}
	filtered := inspectorFacetGet(f, handler, "owner", path+"?type=claim&year=2025&q=sentinel")
	var got inspectorFacetsResponse
	if filtered.Code != http.StatusOK || json.Unmarshal(filtered.Body.Bytes(), &got) != nil || got.TotalMatching != 0 || got.TypeCounts["claim"] != 0 {
		t.Fatalf("filtered facets status=%d body=%s", filtered.Code, filtered.Body.String())
	}
	if after := inspectorTreeDigest(t, f.brains["owner"]); after != before {
		t.Fatal("facets GET changed canonical brain files")
	}
	if calls := f.embed.calls.Load(); calls != 0 {
		t.Fatalf("facets GET made %d embedding/model calls", calls)
	}
}

func TestInspectorGraphHTTPTypeFilterAndCursorBinding(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	brainID := "BrainOwnerABCDEFGHIJKLMNOP"
	first := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?type=fact&limit=2&q=sentinel")
	var firstPage inspectorGraphPage
	if first.Code != http.StatusOK || json.Unmarshal(first.Body.Bytes(), &firstPage) != nil || len(firstPage.Nodes) != 2 || firstPage.TotalMatching != 3 || firstPage.NextCursor == "" {
		t.Fatalf("typed first page status=%d body=%s", first.Code, first.Body.String())
	}
	second := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?type=fact&limit=2&q=sentinel&cursor="+firstPage.NextCursor)
	var secondPage inspectorGraphPage
	if second.Code != http.StatusOK || json.Unmarshal(second.Body.Bytes(), &secondPage) != nil || len(secondPage.Nodes) != 1 || secondPage.Nodes[0].Type != "fact" {
		t.Fatalf("typed second page status=%d body=%s", second.Code, second.Body.String())
	}
	changedType := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?type=all&limit=2&q=sentinel&cursor="+firstPage.NextCursor)
	if changedType.Code != http.StatusBadRequest {
		t.Fatalf("cursor reused after type changed: status=%d body=%s", changedType.Code, changedType.Body.String())
	}
	claims := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?type=claim&q=sentinel")
	var claimPage inspectorGraphPage
	if claims.Code != http.StatusOK || json.Unmarshal(claims.Body.Bytes(), &claimPage) != nil || len(claimPage.Nodes) != 0 || claimPage.TotalMatching != 0 {
		t.Fatalf("claim filter status=%d body=%s", claims.Code, claims.Body.String())
	}
}
