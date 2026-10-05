package website

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplorerEmbeddedShellAndScripts(t *testing.T) {
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest("GET", "/explore/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/assets/explorer/") {
		t.Fatalf("explorer=%d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("explorer must not require inline script relaxation")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("same-origin API unavailable")
	}
}
