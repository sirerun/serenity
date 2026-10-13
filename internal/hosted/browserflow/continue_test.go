package browserflow

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestContinuePolicy(t *testing.T) {
	for _, target := range []string{"javascript:alert(1)", "//evil.example/path", "/relative", "https://user:password@example.com/path", "https:///missing"} {
		rr := httptest.NewRecorder()
		Continue(rr, target)
		if rr.Code != http.StatusBadGateway {
			t.Errorf("unsafe target accepted: %q", target)
		}
	}
	rr := httptest.NewRecorder()
	Continue(rr, "https://callback.example/path?code=a%2Bb&state=%22%3Cscript%3E#fragment")
	if rr.Code != 200 || rr.Header().Get("Location") != "" || rr.Header().Get("Cache-Control") != "no-store" || rr.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("unsafe response policy")
	}
	if strings.Contains(rr.Body.String(), "<script") || !strings.Contains(rr.Body.String(), "code=a%2Bb&amp;state=%22%3Cscript%3E#fragment") {
		t.Fatal("callback encoding changed")
	}
	if !strings.Contains(rr.Header().Get("Content-Security-Policy"), "form-action 'none'") {
		t.Fatal("form policy weakened")
	}
}
func TestChromeSelfOnlyFormContinuation(t *testing.T) {
	if os.Getenv("SERENITY_PLAYWRIGHT") == "" {
		t.Skip("set SERENITY_PLAYWRIGHT for local Chromium regression")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("complete")) }))
	defer app.Close()
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "value=a%2Bb&state=%22%3Cscript%3E" {
			t.Error("callback query changed")
		}
		if r.Header.Get("Referer") != "" {
			t.Error("referrer leaked")
		}
		http.Redirect(w, r, app.URL+"/complete", http.StatusFound)
	}))
	defer callback.Close()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			Continue(w, callback.URL+"/callback?value=a%2Bb&state=%22%3Cscript%3E")
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<form method=post><button value=approve>Continue</button></form>"))
	}))
	defer host.Close()
	out, err := exec.Command("node", "testdata/handoff.cjs", host.URL, "approve", app.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("Chromium: %s: %v", out, err)
	}
}
