package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestOAuthBrowserConsentMCPAndRevocation(t *testing.T) {
	for _, callback := range []string{"http://127.0.0.1:49871/callback", "https://claude.ai/api/mcp/auth_callback"} {
		t.Run(callback, func(t *testing.T) { testOAuthBrowserConsentMCPAndRevocation(t, callback) })
	}
}

func testOAuthBrowserConsentMCPAndRevocation(t *testing.T, callback string) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	origin := "http://" + ts.Listener.Addr().String()
	mail := &sender{}
	svc, err := assembleForTest(t, service.Config{DataDir: dir, PublicOrigin: origin, MaxOpen: 2, MaxInFlight: 4, AccountCap: 10}, true, db, mail, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = svc.Handler
	ts.Start()
	defer ts.Close()
	defer func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	noRedirect := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	read := func(r *http.Response) string {
		t.Helper()
		b, e := io.ReadAll(r.Body)
		if err := r.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	get := func(path string) *http.Response {
		t.Helper()
		r, e := browser.Get(origin + path)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	post := func(path string, values url.Values, client *http.Client) *http.Response {
		t.Helper()
		r, e := http.NewRequest("POST", origin+path, strings.NewReader(values.Encode()))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		resp, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return resp
	}
	extract := func(body, pattern string) string {
		t.Helper()
		v := regexp.MustCompile(pattern).FindStringSubmatch(body)
		if len(v) < 2 {
			t.Fatalf("missing %s in %s", pattern, body)
		}
		return v[1]
	}
	r, e := http.Post(origin+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"Synthetic test agent","redirect_uris":["`+callback+`"],"grant_types":["authorization_code"],"token_endpoint_auth_method":"none"}`))
	if e != nil {
		t.Fatal(e)
	}
	var client struct {
		ID string `json:"client_id"`
	}
	body := read(r)
	if r.StatusCode != 201 || json.Unmarshal([]byte(body), &client) != nil || client.ID == "" {
		t.Fatalf("registration: %d %s", r.StatusCode, body)
	}
	verifier := strings.Repeat("v", 43)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	params := url.Values{"response_type": {"code"}, "client_id": {client.ID}, "redirect_uri": {callback}, "state": {"synthetic-state"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {origin + "/mcp"}, "scope": {"memory:read memory:write"}}
	r = get("/oauth/authorize?" + params.Encode())
	body = read(r)
	if !strings.Contains(body, "Email address") {
		t.Fatal("unauthenticated consent did not require login")
	}
	r = post("/login", url.Values{"email": {"oauth-fixture@example.test"}}, browser)
	read(r)
	if r.StatusCode != 200 {
		t.Fatal("request link failed")
	}
	link, _ := url.Parse(mail.link)
	r = get(link.RequestURI())
	body = read(r)
	if !strings.Contains(body, "Connect your agent.") {
		t.Fatalf("login did not resume consent: %s", body)
	}
	csrf := extract(body, `name="csrf" value="([^"]+)"`)
	handle := extract(body, `name="handle" value="([^"]+)"`)
	brain := extract(body, `<option value="([^"]+)"`)
	consent := url.Values{"csrf": {csrf}, "handle": {handle}, "brain": {brain}, "access": {"memory:read"}, "decision": {"approve"}}
	// Cross-site, wrong CSRF and another user's brain cannot issue a code.
	bad := cloneValues(consent)
	bad.Set("csrf", "wrong")
	r = post("/oauth/consent", bad, noRedirect)
	read(r)
	if r.StatusCode != 403 {
		t.Fatal("bad CSRF accepted")
	}
	bad = cloneValues(consent)
	bad.Set("brain", "not-owned")
	r = post("/oauth/consent", bad, noRedirect)
	read(r)
	if r.StatusCode != 403 {
		t.Fatal("foreign brain accepted")
	}
	r = post("/oauth/consent", consent, noRedirect)
	read(r)
	if r.StatusCode != 303 {
		t.Fatalf("approval status %d", r.StatusCode)
	}
	target, _ := url.Parse(r.Header.Get("Location"))
	code := target.Query().Get("code")
	if code == "" || target.Query().Get("state") != "synthetic-state" {
		t.Fatal("missing bound authorization code")
	}
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ID}, "redirect_uri": {callback}, "code": {code}, "code_verifier": {verifier}, "resource": {origin + "/mcp"}}
	r = post("/oauth/token", values, browser)
	body = read(r)
	var token struct {
		Raw     string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Scope   string `json:"scope"`
	}
	if r.StatusCode != 200 || json.Unmarshal([]byte(body), &token) != nil || token.Raw == "" || token.Scope != "memory:read" {
		t.Fatalf("token exchange: %d %s", r.StatusCode, body)
	}
	if r.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("token response can be cached")
	}
	bindingRequest, err := http.NewRequest(http.MethodGet, origin+"/oauth/binding", nil)
	if err != nil {
		t.Fatal(err)
	}
	bindingRequest.Header.Set("Authorization", "Bearer "+token.Raw)
	bindingResponse, err := browser.Do(bindingRequest)
	if err != nil {
		t.Fatal(err)
	}
	bindingBody := read(bindingResponse)
	var binding struct {
		Schema          string    `json:"schema"`
		Issuer          string    `json:"issuer"`
		Resource        string    `json:"resource"`
		AccountID       string    `json:"account_id"`
		AccountState    string    `json:"account_state"`
		ProjectID       string    `json:"project_id"`
		Scopes          []string  `json:"scopes"`
		GrantID         string    `json:"grant_id"`
		ProjectState    string    `json:"project_state"`
		RevocationEpoch int       `json:"revocation_epoch"`
		ObservedAt      time.Time `json:"observed_at"`
	}
	if bindingResponse.StatusCode != http.StatusOK || json.Unmarshal([]byte(bindingBody), &binding) != nil {
		t.Fatalf("binding read: %d %s", bindingResponse.StatusCode, bindingBody)
	}
	if binding.Schema != "serenity.oauth-binding/v1" || binding.Issuer != origin || binding.Resource != origin+"/mcp" ||
		binding.AccountID == "" || binding.AccountState != "active" || binding.ProjectID != brain || binding.GrantID == "" ||
		binding.ProjectState != "ready" || binding.RevocationEpoch != 0 ||
		len(binding.Scopes) != 1 || binding.Scopes[0] != "memory:read" || binding.ObservedAt.IsZero() {
		t.Fatalf("binding is not the selected token authority: %+v", binding)
	}
	if bindingResponse.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("binding response can be cached")
	}
	foreignSelector, err := http.NewRequest(http.MethodGet, origin+"/oauth/binding?project_id=not-owned", nil)
	if err != nil {
		t.Fatal(err)
	}
	foreignSelector.Header.Set("Authorization", "Bearer "+token.Raw)
	foreignResponse, err := browser.Do(foreignSelector)
	if err != nil {
		t.Fatal(err)
	}
	read(foreignResponse)
	if foreignResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("caller-selected project was not rejected: %d", foreignResponse.StatusCode)
	}

	for _, tc := range []struct {
		name, method, suffix, auth string
		want                       int
	}{
		{"missing token despite browser session", "GET", "", "", 401},
		{"invalid token", "GET", "", "Bearer invalid", 401},
		{"refresh token", "GET", "", "Bearer " + token.Refresh, 401},
		{"post", "POST", "", "Bearer " + token.Raw, 405},
		{"head", "HEAD", "", "Bearer " + token.Raw, 405},
		{"empty query", "GET", "?", "Bearer " + token.Raw, 400},
		{"account selector", "GET", "?account_id=foreign", "Bearer " + token.Raw, 400},
	} {
		t.Run("binding/"+tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, origin+"/oauth/binding"+tc.suffix, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", tc.auth)
			res, err := browser.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			b := read(res)
			if res.StatusCode != tc.want {
				t.Fatalf("got %d, want %d: %s", res.StatusCode, tc.want, b)
			}
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable error")
			}
			if tc.want == 401 && res.Header.Get("WWW-Authenticate") == "" {
				t.Fatal("missing challenge")
			}
			if tc.want == 405 && res.Header.Get("Allow") != "GET" {
				t.Fatal("missing allowed method")
			}
			if strings.Contains(b, binding.AccountID) || strings.Contains(b, token.Raw) {
				t.Fatal("error disclosed authority")
			}
		})
	}
	for _, tc := range []struct{ name, change, restore string }{
		{"inactive account", "UPDATE accounts SET status='deleting' WHERE id=?", "UPDATE accounts SET status='active' WHERE id=?"},
		{"unready project", "UPDATE brains SET state='allocating' WHERE account_id=?", "UPDATE brains SET state='ready' WHERE account_id=?"},
		{"revoked epoch", "INSERT INTO oauth_epochs(brain_id,generation) SELECT id,1 FROM brains WHERE account_id=? ON CONFLICT(brain_id) DO UPDATE SET generation=1", "DELETE FROM oauth_epochs WHERE brain_id IN (SELECT id FROM brains WHERE account_id=?)"},
	} {
		t.Run("binding/"+tc.name, func(t *testing.T) {
			if _, err := db.DB().ExecContext(context.Background(), tc.change, binding.AccountID); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.DB().ExecContext(context.Background(), tc.restore, binding.AccountID); err != nil {
					t.Error(err)
				}
			}()
			req, err := http.NewRequest("GET", origin+"/oauth/binding", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token.Raw)
			res, err := browser.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			b := read(res)
			if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") == "" {
				t.Fatalf("invalid authority returned %d: %s", res.StatusCode, b)
			}
		})
	}
	if strings.Contains(bindingBody, token.Raw) || strings.Contains(bindingBody, token.Refresh) {
		t.Fatal("binding disclosed credential")
	}
	r = post("/oauth/token", values, browser)
	read(r)
	if r.StatusCode == 200 {
		t.Fatal("code replay accepted")
	}
	var session string
	mcp := func(frame string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", origin+"/mcp", strings.NewReader(frame))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token.Raw)
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
			req.Header.Set("MCP-Protocol-Version", "2025-11-25")
		}
		res, e := browser.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		if v := res.Header.Get("Mcp-Session-Id"); v != "" {
			session = v
		}
		return res.StatusCode, read(res)
	}
	status, body := mcp(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if status != 200 || session == "" {
		t.Fatalf("OAuth MCP initialize: %d %s", status, body)
	}
	mcp(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	_, body = mcp(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"remember","arguments":{"fact":"must not write","provenance":"synthetic test","operation_key":"oauth-readonly-denied"}}}`)
	if !strings.Contains(body, "scope_denied") {
		t.Fatalf("read-only grant allowed write: %s", body)
	}
	oldAccess := token.Raw
	if token.Refresh == "" {
		t.Fatal("missing refresh token")
	}
	r = post("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ID}, "resource": {origin + "/mcp"}, "refresh_token": {token.Refresh}}, browser)
	body = read(r)
	if r.StatusCode != 200 || json.Unmarshal([]byte(body), &token) != nil {
		t.Fatalf("refresh failed: %s", body)
	}
	// Expire the original token: this session must now use request-specific
	// refreshed authorization, not a token captured when the MCP handler opened.
	expired := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err = db.DB().Exec("UPDATE oauth_tokens SET record=json_set(record,'$.expires_at',?),expires_at=? WHERE id=?", expired, expired, store.Hash(oldAccess)); err != nil {
		t.Fatal(err)
	}
	status, body = mcp(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"recall","arguments":{"query":"synthetic marker"}}}`)
	if status != 200 || strings.Contains(body, "scope_denied") || strings.Contains(body, "invalid_params") {
		t.Fatalf("refreshed MCP session lost identity: %d %s", status, body)
	}
	snapshot := filepath.Join(dir, "oauth-snapshot")
	if err = svc.Backup(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	restoredPath := filepath.Join(dir, "restored")
	if err = backup.Restore(context.Background(), snapshot, restoredPath); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(filepath.Join(restoredPath, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"oauth_grants", "oauth_tokens", "oauth_refresh", "oauth_codes", "oauth_consents"} {
		var count int
		if err = restored.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("restored %s retained authority: %d %v", table, count, err)
		}
	}
	if err = restored.Close(); err != nil {
		t.Fatal(err)
	}
	r = get("/oauth/connections")
	body = read(r)
	if !strings.Contains(body, "Synthetic test agent") {
		t.Fatal("connection not manageable")
	}
	// Project-wide revoke invalidates the grant and any future use of its session.
	r = post("/credentials/revoke", url.Values{"csrf": {csrf}, "brain_id": {brain}}, browser)
	read(r)
	if r.StatusCode != 200 {
		t.Fatalf("revoke failed: %d", r.StatusCode)
	}
	status, _ = mcp(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	if status != 401 {
		t.Fatalf("revoked OAuth access accepted: %d", status)
	}
	bindingRequest, err = http.NewRequest(http.MethodGet, origin+"/oauth/binding", nil)
	if err != nil {
		t.Fatal(err)
	}
	bindingRequest.Header.Set("Authorization", "Bearer "+token.Raw)
	bindingResponse, err = browser.Do(bindingRequest)
	if err != nil {
		t.Fatal(err)
	}
	read(bindingResponse)
	if bindingResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked OAuth binding remained readable: %d", bindingResponse.StatusCode)
	}
	var epoch int
	if err = db.DB().QueryRowContext(context.Background(), "SELECT generation FROM oauth_epochs WHERE brain_id=?", brain).Scan(&epoch); err != nil || epoch != 1 {
		t.Fatal("revocation epoch missing")
	}
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, values := range v {
		out[k] = append([]string(nil), values...)
	}
	return out
}
