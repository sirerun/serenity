package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestCIMDAndDCRHTTPAuthorization(t *testing.T) {
	for _, tc := range []struct{ mode, scope string }{{"cimd", "memory:read"}, {"cimd", "memory:read memory:write"}, {"dcr", "memory:read"}, {"dcr", "memory:read memory:write"}} {
		t.Run(tc.mode+"/"+tc.scope, func(t *testing.T) {
			mode := tc.mode
			adapter, account, brain := lifecycleStore(t)
			h, err := New(adapter.DB, nil, nil, "https://memory.example", true)
			if err != nil {
				t.Fatal(err)
			}
			resolver := NewCIMDResolver()
			resolver.http = &http.Client{Transport: metadataRoundTrip(func(*http.Request) (*http.Response, error) {
				return metadataResponse(metadataBody(t, map[string]any{"scope": "openid memory:read memory:write"})), nil
			})}
			h.Store.Metadata = resolver
			callback := "https://client.example/callback"
			id := testMetadataID
			handler := http.NewServeMux()
			handler.Handle("/oauth/register", h.Server.RegisterHandler())
			handler.Handle("/oauth/token", h.Server.TokenHandler())
			var handle string
			handler.Handle("/oauth/authorize", h.Server.AuthorizeHandler(func(w http.ResponseWriter, r *http.Request, v string, request mcpoauth.ConsentRequest) {
				handle = v
				w.WriteHeader(200)
			}))
			ts := httptest.NewServer(handler)
			defer ts.Close()
			post := func(path string, v url.Values) (int, map[string]any) {
				t.Helper()
				resp, e := http.PostForm(ts.URL+path, v)
				if e != nil {
					t.Fatal(e)
				}
				defer func() { _ = resp.Body.Close() }()
				var body map[string]any
				if e = json.NewDecoder(resp.Body).Decode(&body); e != nil {
					t.Fatal(e)
				}
				return resp.StatusCode, body
			}
			if mode == "dcr" {
				resp, e := http.Post(ts.URL+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"Gumloop fixture","redirect_uris":["`+callback+`"]}`))
				if e != nil {
					t.Fatal(e)
				}
				var body map[string]any
				e = json.NewDecoder(resp.Body).Decode(&body)
				_ = resp.Body.Close()
				if e != nil || resp.StatusCode != 201 {
					t.Fatalf("registration %d %v", resp.StatusCode, e)
				}
				id = body["client_id"].(string)
			}
			verifier := strings.Repeat("v", 43)
			digest := sha256.Sum256([]byte(verifier))
			q := url.Values{"response_type": {"code"}, "client_id": {id}, "redirect_uri": {callback}, "state": {"fixture"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"}, "resource": {"https://memory.example/mcp"}, "scope": {"memory:read memory:write"}}
			q.Set("scope", "admin")
			noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			badResponse, badErr := noRedirect.Get(ts.URL + "/oauth/authorize?" + q.Encode())
			if badErr != nil {
				t.Fatal(badErr)
			}
			_ = badResponse.Body.Close()
			badTarget, parseErr := url.Parse(badResponse.Header.Get("Location"))
			if badResponse.StatusCode != http.StatusFound || parseErr != nil || badTarget.Query().Get("error") != "invalid_scope" || handle != "" {
				t.Fatalf("unsupported scope response: status=%d parse=%v error=%q consent=%v", badResponse.StatusCode, parseErr, badTarget.Query().Get("error"), handle != "")
			}
			q.Set("scope", "memory:read memory:write")
			resp, e := http.Get(ts.URL + "/oauth/authorize?" + q.Encode())
			if e != nil {
				t.Fatal(e)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 200 || handle == "" {
				t.Fatalf("authorize=%d handle missing=%v", resp.StatusCode, handle == "")
			}
			target, e := h.Server.Approve(context.Background(), handle, mcpoauth.Approval{Subject: account.ID, Binding: brain + ".0", Scopes: strings.Fields(tc.scope)})
			if e != nil {
				t.Fatal(e)
			}
			u, e := url.Parse(target)
			if e != nil {
				t.Fatal(e)
			}
			code, body := post("/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "redirect_uri": {callback}, "code": {u.Query().Get("code")}, "code_verifier": {verifier}, "resource": {"https://memory.example/mcp"}})
			if code != 200 || body["scope"] != tc.scope {
				t.Fatalf("exchange %d %v", code, body)
			}
			raw, _ := body["access_token"].(string)
			if _, err := h.Verify(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
			refresh, _ := body["refresh_token"].(string)
			if refresh == "" {
				t.Fatal("missing refresh")
			}
			code, body = post("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {id}, "refresh_token": {refresh}, "resource": {"https://memory.example/mcp"}})
			if code != 200 || body["scope"] != tc.scope {
				t.Fatalf("refresh %d %v", code, body)
			}
			if mode == "cimd" {
				var n int
				if err := adapter.DB.DB().QueryRow("SELECT count(*) FROM oauth_clients").Scan(&n); err != nil || n != 0 {
					t.Fatalf("CIMD persisted DCR registration: %d %v", n, err)
				}
			}
		})
	}
}

func TestCIMDMetadataPreservesDiscovery(t *testing.T) {
	adapter, _, _ := lifecycleStore(t)
	h, err := New(adapter.DB, nil, nil, "https://memory.example", true)
	if err != nil {
		t.Fatal(err)
	}
	old, new := httptest.NewRecorder(), httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.Server.MetadataHandler().ServeHTTP(old, req)
	h.MetadataHandler().ServeHTTP(new, req)
	var a, b map[string]any
	if json.Unmarshal(old.Body.Bytes(), &a) != nil || json.Unmarshal(new.Body.Bytes(), &b) != nil {
		t.Fatal("bad metadata")
	}
	if b["client_id_metadata_document_supported"] != true {
		t.Fatal("CIMD not advertised")
	}
	delete(b, "client_id_metadata_document_supported")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("DCR discovery changed: %v vs %v", a, b)
	}
}

func TestConsentApprovalFailuresAreDistinct(t *testing.T) {
	adapter, account, brain := lifecycleStore(t)
	h, err := New(adapter.DB, nil, nil, "https://memory.example", true)
	if err != nil {
		t.Fatal(err)
	}
	client := mcpoauth.Client{ID: "fixture", Name: "Gumloop fixture", RedirectURIs: []string{"https://api.gumstack.com/auth/callback"}, CreatedAt: time.Now()}
	if err := h.Store.CreateClient(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(strings.Repeat("v", 43)))
	create := func(handle string, expires time.Time) {
		t.Helper()
		err := h.Store.CreateConsent(context.Background(), mcpoauth.ConsentRecord{ID: store.Hash(handle), ClientID: client.ID, RedirectURI: client.RedirectURIs[0], Resource: "https://memory.example/mcp", Scopes: []string{"memory:read"}, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CreatedAt: time.Now(), ExpiresAt: expires})
		if err != nil {
			t.Fatal(err)
		}
	}
	approve := func(handle string, scopes []string) error {
		_, err := h.Server.Approve(context.Background(), handle, mcpoauth.Approval{Subject: account.ID, Binding: brain + ".0", Scopes: scopes})
		return err
	}
	create("expired", time.Now().Add(-time.Minute))
	expired := approve("expired", []string{"memory:read"})
	if !errors.Is(expired, mcpoauth.ErrExpired) {
		t.Fatalf("expiry %v", expired)
	}
	create("live", time.Now().Add(time.Minute))
	scopeErr := approve("live", []string{"memory:read", "memory:write"})
	if !errors.Is(scopeErr, mcpoauth.ErrInvalid) {
		t.Fatalf("scope %v", scopeErr)
	}
	if err := approve("live", []string{"memory:read"}); err != nil {
		t.Fatal("bad narrowing consumed request", err)
	}
	replay := approve("live", []string{"memory:read"})
	if !errors.Is(replay, mcpoauth.ErrNotFound) {
		t.Fatalf("replay %v", replay)
	}
	for _, tc := range []struct {
		err    error
		status int
		text   string
	}{{expired, 400, "expired"}, {scopeErr, 400, "selected access"}, {replay, 400, "already used"}, {errors.New("database unavailable private-details"), 503, "temporarily unavailable"}} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeApprovalError(w, tc.err) }))
		resp, err := http.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ts.Close()
		if resp.StatusCode != tc.status || !strings.Contains(string(body), tc.text) || strings.Contains(string(body), "private-details") {
			t.Fatalf("status %d body %s", resp.StatusCode, body)
		}
	}
}
