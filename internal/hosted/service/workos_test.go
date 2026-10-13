package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type workOSRoundTrip func(*http.Request) (*http.Response, error)

func (f workOSRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestWorkOSConfigurationFailsClosed(t *testing.T) {
	cfg := Config{WorkOSEnabled: true}
	if e := cfg.Validate(true); e == nil || !strings.Contains(e.Error(), "workos_client_id") {
		t.Fatalf("missing client ID %v", e)
	}
	cfg.WorkOSClientID = "client_test"
	if _, e := AssembleWithDependencies(context.Background(), cfg, true, nil, nil, nil, testLifecycleDependencies(t)); e == nil || !strings.Contains(e.Error(), "WorkOS") {
		t.Fatalf("missing private configuration %v", e)
	}
	cfg = Config{Bind: "127.0.0.1:8080", PublicOrigin: "http://127.0.0.1", DataDir: t.TempDir(), SecretsDir: t.TempDir(), EmbeddingModel: "fixture", EmbeddingVersion: "1", WorkOSEnabled: true, WorkOSClientID: "client_fixture"}
	if e := os.Chmod(cfg.SecretsDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(cfg.SecretsDir, "EMBEDDINGS_API_KEY"), []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := NewWithDependencies(context.Background(), cfg, true, io.Discard, testLifecycleDependencies(t)); e == nil || !strings.Contains(e.Error(), "WORKOS_API_KEY") {
		t.Fatalf("missing secret %v", e)
	}
}
func TestWorkOSAssembledJourneyCreatesOnePersonalProject(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open(filepath.Join(dir, "control.db"))
	if e != nil {
		t.Fatal(e)
	}
	cfg := Config{DataDir: dir, PublicOrigin: "http://127.0.0.1", MaxOpen: 2, MaxInFlight: 4, WorkOSEnabled: true, WorkOSClientID: "client_fixture"}
	cfg.workOS = &identity.WorkOS{ClientID: cfg.WorkOSClientID, APIKey: "fixture", HTTPClient: &http.Client{Transport: workOSRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"user":{"id":"user_fixture","email":"new@example.com","email_verified":true}}`))}, nil
	})}}
	svc, e := assembleForTest(t, cfg, true, db, &identity.DevSender{Writer: io.Discard}, deletionEmbedding{})
	if e != nil {
		_ = db.Close()
		t.Fatal(e)
	}
	start := httptest.NewRecorder()
	svc.Handler.ServeHTTP(start, httptest.NewRequest("GET", "/login/workos", nil))
	if start.Code != 303 {
		t.Fatalf("start %d %s", start.Code, start.Body.String())
	}
	u, e := url.Parse(start.Header().Get("Location"))
	if e != nil {
		t.Fatal(e)
	}
	callback := httptest.NewRequest("GET", "/login/workos/callback?state="+u.Query().Get("state")+"&code=fixture", nil)
	for _, cookie := range start.Result().Cookies() {
		callback.AddCookie(cookie)
	}
	done := httptest.NewRecorder()
	svc.Handler.ServeHTTP(done, callback)
	if done.Code != 303 || done.Header().Get("Location") != "/dashboard" {
		t.Fatalf("callback %d", done.Code)
	}
	for range 2 {
		req := httptest.NewRequest("GET", "/dashboard", nil)
		for _, cookie := range done.Result().Cookies() {
			req.AddCookie(cookie)
		}
		out := httptest.NewRecorder()
		svc.Handler.ServeHTTP(out, req)
		if out.Code != 200 || !strings.Contains(out.Body.String(), "Free plan") {
			t.Fatalf("dashboard %d %s", out.Code, out.Body.String())
		}
	}
	var count int
	if e = db.DB().QueryRow(`SELECT count(*) FROM brains WHERE is_default=1 AND state='ready'`).Scan(&count); e != nil || count != 1 {
		t.Fatalf("personal project %d %v", count, e)
	}
}
