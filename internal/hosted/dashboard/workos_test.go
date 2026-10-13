package dashboard

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type authTransport func(*http.Request) (*http.Response, error)

func (f authTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func newWorkOSDashboard(t *testing.T) *Dashboard {
	t.Helper()
	db, e := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Dashboard{Dev: true, Origin: "http://localhost", Identity: &identity.Service{Store: db, Origin: "http://localhost", WorkOS: &identity.WorkOS{ClientID: "client_test", APIKey: "fixture", HTTPClient: &http.Client{Transport: authTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"user":{"id":"user_test","email":"person@example.com","email_verified":true}}`))}, nil
	})}}}}
}
func TestWorkOSCallbackResumesExistingConsent(t *testing.T) {
	for _, kind := range []string{"dashboard", "oauth", "partner"} {
		t.Run(kind, func(t *testing.T) {
			d := newWorkOSDashboard(t)
			h := d.Handler()
			start := httptest.NewRecorder()
			h.ServeHTTP(start, httptest.NewRequest("GET", "/login/workos", nil))
			if start.Code != 303 {
				t.Fatalf("start %d", start.Code)
			}
			location, e := url.Parse(start.Header().Get("Location"))
			if e != nil {
				t.Fatal(e)
			}
			var stateCookie *http.Cookie
			for _, c := range start.Result().Cookies() {
				if c.Name == workOSStateCookie {
					stateCookie = c
				}
			}
			if stateCookie == nil || !stateCookie.HttpOnly || stateCookie.SameSite != http.SameSiteLaxMode || stateCookie.Path != "/" {
				t.Fatal("state cookie missing/broken")
			}
			req := httptest.NewRequest("GET", "/login/workos/callback?state="+url.QueryEscape(location.Query().Get("state"))+"&code=fixture", nil)
			req.AddCookie(stateCookie)
			want := "/dashboard"
			if kind == "oauth" {
				req.AddCookie(&http.Cookie{Name: "serenity_oauth_resume", Value: "request-handle"})
				want = "/oauth/authorize?request=request-handle"
			}
			if kind == "partner" {
				req.AddCookie(&http.Cookie{Name: "serenity_partner_resume", Value: "request-handle"})
				want = "/partner/consent?request=request-handle"
			}
			out := httptest.NewRecorder()
			h.ServeHTTP(out, req)
			if out.Code != 303 || out.Header().Get("Location") != want {
				t.Fatalf("callback %d %s want %s body %s", out.Code, out.Header().Get("Location"), want, out.Body.String())
			}
			issued := false
			for _, c := range out.Result().Cookies() {
				if c.Name == "serenity_session" && c.Value != "" {
					issued = true
					if _, e = d.Identity.Session(context.Background(), c.Value); e != nil {
						t.Fatal(e)
					}
				}
			}
			if !issued {
				t.Fatal("missing local session")
			}
			replay := httptest.NewRecorder()
			h.ServeHTTP(replay, req)
			if replay.Code != 400 {
				t.Fatal("callback replay accepted")
			}
		})
	}
}
func TestWorkOSLinkRequiresCSRFAndSafeExternalContinuation(t *testing.T) {
	d := newWorkOSDashboard(t)
	state, _, e := d.Identity.BeginWorkOS(context.Background(), "", "fixture")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := d.Identity.CompleteWorkOS(context.Background(), state, state, "code", "")
	if e != nil {
		t.Fatal(e)
	}
	session, e := d.Identity.Session(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, csrf := range []string{"wrong", session.CSRF} {
		req := httptest.NewRequest("POST", "/login/workos/link", strings.NewReader(url.Values{"csrf": {csrf}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "serenity_session", Value: raw})
		out := httptest.NewRecorder()
		d.Handler().ServeHTTP(out, req)
		if csrf == "wrong" {
			if out.Code != 403 {
				t.Fatalf("CSRF accepted %d", out.Code)
			}
			continue
		}
		if out.Code != 200 || out.Header().Get("Location") != "" || !strings.Contains(out.Body.String(), "https://api.workos.com/user_management/authorize?") {
			t.Fatalf("link did not use continuation: %d %s", out.Code, out.Body.String())
		}
	}
}
func TestWorkOSDisabledKeepsMagicLinkAndNoExternalOption(t *testing.T) {
	d := newWorkOSDashboard(t)
	d.Identity.WorkOS = nil
	out := httptest.NewRecorder()
	d.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/login", nil))
	if out.Code != 200 || !strings.Contains(out.Body.String(), "Send sign-in link") || strings.Contains(out.Body.String(), "More sign-in options") {
		t.Fatal("disabled login changed")
	}
}
func TestWorkOSCallbackDoesNotReflectProviderErrors(t *testing.T) {
	d := newWorkOSDashboard(t)
	state, _, e := d.Identity.BeginWorkOS(context.Background(), "", "fixture")
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("GET", "/login/workos/callback?state="+state+"&error=secret-provider-error&code=ignored", nil)
	req.AddCookie(&http.Cookie{Name: workOSStateCookie, Value: state})
	out := httptest.NewRecorder()
	d.Handler().ServeHTTP(out, req)
	if out.Code != 400 || strings.Contains(out.Body.String(), "secret-provider-error") || !strings.Contains(out.Body.String(), "Serenity email link") {
		t.Fatal("provider error leaked or no recovery")
	}
}
