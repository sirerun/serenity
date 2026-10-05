package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExplorerRouteRequiresSessionAndPrivateShell(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	unauth := httptest.NewRecorder()
	f.handler.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/dashboard/explore", nil))
	if unauth.Code != http.StatusSeeOther || unauth.Header().Get("Location") != "/login" {
		t.Fatalf("signed-out route=%d %q", unauth.Code, unauth.Header().Get("Location"))
	}
	response := f.get(t, "owner", "/dashboard/explore")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "/assets/explorer/") {
		t.Fatalf("shell=%d %s", response.Code, response.Body.String())
	}
	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "connect-src 'self'") || strings.Contains(policy, "unsafe-eval") || strings.Contains(policy, "https:") {
		t.Fatalf("private CSP=%s", policy)
	}
	if strings.Contains(response.Body.String(), f.tokens["owner"]) {
		t.Fatal("session token leaked into shell")
	}
	facet := f.get(t, "owner", "/api/inspector/v1/brains/BrainOwnerABCDEFGHIJKLMNOP/facets?type=fact")
	if facet.Code != 200 {
		t.Fatalf("registered facet=%d %s", facet.Code, facet.Body.String())
	}
	public := httptest.NewRecorder()
	f.handler.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/explore/", nil))
	if public.Code != 200 || !strings.Contains(public.Body.String(), "/assets/explorer/") {
		t.Fatalf("public shell=%d", public.Code)
	}
}

// Opt-in, local non-customer acceptance server. This helper is never a
// production route and emits no credentials into the artifact receipt.
func TestExplorerBrowserFixture(t *testing.T) {
	output := os.Getenv("SERENITY_EXPLORER_FIXTURE_RECEIPT")
	if output == "" {
		t.Skip("opt-in local browser fixture")
	}
	f := newInspectorHTTPFixture(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__fixture/owner", "/__fixture/other":
			user := strings.TrimPrefix(r.URL.Path, "/__fixture/")
			http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: f.tokens[user], Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/dashboard/explore", http.StatusSeeOther)
		case "/__fixture/expire":
			http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
			w.WriteHeader(http.StatusNoContent)
		default:
			f.handler.ServeHTTP(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	receipt, _ := json.Marshal(map[string]string{"url": server.URL, "kind": "local synthetic accounts only"})
	if err := os.WriteFile(output, receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("Local synthetic browser fixture ready")
	time.Sleep(45 * time.Minute)
}
