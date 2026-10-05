package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	brainstore "github.com/sirerun/serenity/internal/store"
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
	sources := brainstore.NewSourceStore(f.brains["owner"])
	for i := 0; i < 220; i++ {
		_, err := sources.WriteMemoryFact(brainstore.MemoryFactPayload{FormatVersion: brainstore.MemoryFactFormatVersion, LegacyID: int64(100 + i), Fact: fmt.Sprintf("Synthetic browser memory %03d <svg onload=alert(1)>", i), Provenance: "non-customer browser qualification", Kind: brainstore.MemoryFactKindFact, Visibility: brainstore.MemoryVisibilityWorld, CreatedAt: time.Date(2020+i%6, time.January, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{}, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__fixture/owner", "/__fixture/other":
			user := strings.TrimPrefix(r.URL.Path, "/__fixture/")
			http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: f.tokens[user], Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/dashboard/explore", http.StatusSeeOther)
		case "/__fixture/expire":
			http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
			w.WriteHeader(http.StatusNoContent)
		case "/__fixture/stop":
			select {
			case stop <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusOK)
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
	select {
	case <-stop:
	case <-time.After(45 * time.Minute):
		t.Fatal("local browser fixture exceeded its bounded lifetime")
	}
}
