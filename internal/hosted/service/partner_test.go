package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/server/mcp"
)

const blinkSecret = "blink-partner-secret-0123456789abcdef0123456789"

type partnerEnv struct {
	t      *testing.T
	svc    *service.Service
	server *httptest.Server
	db     *store.Store
	mail   *sender
	dir    string
}

func newPartnerEnv(t *testing.T, mutate func(*service.Config)) *partnerEnv {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "brains"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := service.Config{DataDir: dir, MaxOpen: 2, MaxInFlight: 8, PublicOrigin: "http://127.0.0.1", AccountCap: 100}
	if mutate != nil {
		mutate(&cfg)
	}
	mail := &sender{}
	svc, err := service.Assemble(cfg, true, db, mail, embedding{})
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	env := &partnerEnv{t: t, svc: svc, server: httptest.NewServer(svc.Handler), db: db, mail: mail, dir: dir}
	t.Cleanup(func() {
		env.server.Close()
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	env.seed("blink", "Blink", blinkSecret, "blink://serenity-linked", "")
	return env
}

// seed exercises the admin unix-socket route with a secret file.
func (e *partnerEnv) seed(id, name, secret, prefix, status string) int {
	e.t.Helper()
	path := filepath.Join(e.t.TempDir(), "partner-"+id)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		e.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"id": id, "display_name": name, "secret_file": path, "redirect_prefix": prefix, "status": status})
	w := httptest.NewRecorder()
	e.svc.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/partners", bytes.NewReader(body)))
	return w.Code
}

func (e *partnerEnv) call(method, path, auth string, payload any) (int, map[string]any) {
	e.t.Helper()
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	r, err := http.NewRequest(method, e.server.URL+path, body)
	if err != nil {
		e.t.Fatal(err)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err = json.Unmarshal(raw, &out); err != nil {
			e.t.Fatalf("%s %s: non-JSON body %q", method, path, raw)
		}
	}
	return resp.StatusCode, out
}

func (e *partnerEnv) blink(method, path string, payload any) (int, map[string]any) {
	return e.call(method, path, "Partner blink:"+blinkSecret, payload)
}

func (e *partnerEnv) ensure(email string) map[string]any {
	e.t.Helper()
	status, out := e.blink(http.MethodPost, "/partner/v1/accounts:ensure", map[string]any{"email": email, "email_verified": true})
	if status != 200 {
		e.t.Fatalf("ensure %s: %d %v", email, status, out)
	}
	return out
}

func (e *partnerEnv) issue(account string) (string, string) {
	e.t.Helper()
	status, out := e.blink(http.MethodPost, "/partner/v1/credentials", map[string]any{"account_id": account})
	if status != 201 {
		e.t.Fatalf("credentials: %d %v", status, out)
	}
	return out["credential_id"].(string), out["key"].(string)
}

// browser signs in with the magic link. The returned client does not follow
// redirects, so custom-scheme returns are observable.
func (e *partnerEnv) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *partnerEnv) get(c *http.Client, path string) (*http.Response, string) {
	e.t.Helper()
	resp, err := c.Get(e.server.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

// login completes the magic link flow and returns where consume redirected.
func (e *partnerEnv) login(c *http.Client, email string) string {
	e.t.Helper()
	resp, err := c.PostForm(e.server.URL+"/login", url.Values{"email": {email}})
	if err != nil {
		e.t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		e.t.Fatalf("login %d", resp.StatusCode)
	}
	link, _ := url.Parse(e.mail.link)
	resp, _ = e.get(c, link.RequestURI())
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("consume %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func (e *partnerEnv) post(c *http.Client, path string, form url.Values, origin bool) *http.Response {
	e.t.Helper()
	r, _ := http.NewRequest(http.MethodPost, e.server.URL+path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin {
		r.Header.Set("Origin", "http://127.0.0.1")
	}
	resp, err := c.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func csrfOf(t *testing.T, body string) string {
	t.Helper()
	m := csrfPattern.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatalf("missing CSRF: %s", body)
	}
	return m[1]
}

// consent drives the browser flow for a link request and returns the
// redirect the partner app receives.
func (e *partnerEnv) consent(c *http.Client, consentURL, email, decision string) *url.URL {
	e.t.Helper()
	u, _ := url.Parse(consentURL)
	resp, _ := e.get(c, u.RequestURI())
	if resp.StatusCode == http.StatusSeeOther {
		if resp.Header.Get("Location") != "/login" {
			e.t.Fatalf("unsigned consent redirected to %s", resp.Header.Get("Location"))
		}
		if next := e.login(c, email); next != u.RequestURI() {
			e.t.Fatalf("login did not resume consent: %s", next)
		}
	}
	resp, body := e.get(c, u.RequestURI())
	if resp.StatusCode != 200 || !strings.Contains(body, "Allow Blink to read and write your Serenity memory?") {
		e.t.Fatalf("consent page %d %s", resp.StatusCode, body)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' blink://serenity-linked") {
		e.t.Fatalf("consent CSP does not allow the return URL: %s", csp)
	}
	form := url.Values{"csrf": {csrfOf(e.t, body)}, "request": {u.Query().Get("request")}, "decision": {decision}}
	if resp := e.post(c, "/partner/consent", form, false); resp.StatusCode != http.StatusForbidden {
		e.t.Fatalf("consent without Origin %d", resp.StatusCode)
	}
	resp = e.post(c, "/partner/consent", form, true)
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("consent decision %d", resp.StatusCode)
	}
	target, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	return target
}

func (e *partnerEnv) mcpSession(token string) string {
	e.t.Helper()
	status, session, body := e.mcp(token, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "partner-test", "version": "1"}}})
	if status != 200 || session == "" {
		e.t.Fatalf("initialize %d %s", status, body)
	}
	status, _, body = e.mcp(token, session, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if status != 202 && status != 200 {
		e.t.Fatalf("initialized %d %s", status, body)
	}
	return session
}

func (e *partnerEnv) mcp(token, session string, payload any) (int, string, []byte) {
	e.t.Helper()
	data, _ := json.Marshal(payload)
	r, _ := http.NewRequest(http.MethodPost, e.server.URL+"/mcp", bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		r.Header.Set(mcp.SessionIDHeader, session)
		r.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, resp.Header.Get(mcp.SessionIDHeader), body
}

// tool runs one tool call and returns the tool's text payload.
func (e *partnerEnv) tool(token, session, name string, args map[string]any) (string, bool) {
	e.t.Helper()
	status, _, body := e.mcp(token, session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if status != 200 {
		e.t.Fatalf("tool %s %d %s", name, status, body)
	}
	var out struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Result.Content) == 0 {
		e.t.Fatalf("tool %s body %s", name, body)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

func (e *partnerEnv) used(account, window, metric string) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.DB().QueryRow(`SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric=?),0)`, account, window, metric).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestPartnerAuthenticationAuditAndRateLimit(t *testing.T) {
	env := newPartnerEnv(t, nil)
	if code := env.seed("bad", "Bad", "short", "blink://x", ""); code != http.StatusBadRequest {
		t.Fatalf("short secret seeded: %d", code)
	}
	if code := env.seed("evil", "Evil", blinkSecret, "http://evil.example", ""); code != http.StatusBadRequest {
		t.Fatalf("http redirect prefix seeded: %d", code)
	}
	body := map[string]any{"email": "auth@example.com", "email_verified": true}
	for _, auth := range []string{"", "Bearer " + blinkSecret, "Partner blink", "Partner blink:wrong-secret", "Partner nobody:" + blinkSecret, "Partner BLINK:" + blinkSecret} {
		status, out := env.call(http.MethodPost, "/partner/v1/accounts:ensure", auth, body)
		if status != 401 || out["error"] != "unauthorized" || out["message"] == nil {
			t.Fatalf("auth %q: %d %v", auth, status, out)
		}
	}
	var accounts int
	if err := env.db.DB().QueryRow(`SELECT count(*) FROM accounts`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("unauthenticated call created accounts: %d %v", accounts, err)
	}
	// Rotation replaces the digest; the old secret stops working.
	if code := env.seed("blink", "Blink", blinkSecret+"-rotated", "blink://serenity-linked", ""); code != http.StatusNoContent {
		t.Fatalf("rotate %d", code)
	}
	if status, _ := env.blink(http.MethodPost, "/partner/v1/accounts:ensure", body); status != 401 {
		t.Fatalf("old secret accepted after rotation: %d", status)
	}
	if status, _ := env.call(http.MethodPost, "/partner/v1/accounts:ensure", "Partner blink:"+blinkSecret+"-rotated", body); status != 200 {
		t.Fatalf("rotated secret rejected: %d", status)
	}
	if code := env.seed("blink", "Blink", blinkSecret, "blink://serenity-linked", "disabled"); code != http.StatusNoContent {
		t.Fatalf("disable %d", code)
	}
	if status, _ := env.blink(http.MethodPost, "/partner/v1/accounts:ensure", body); status != 401 {
		t.Fatalf("disabled partner accepted: %d", status)
	}
	env.seed("blink", "Blink", blinkSecret, "blink://serenity-linked", "")
	out := env.ensure("auth@example.com")
	var actor, action, detail string
	if err := env.db.DB().QueryRow(`SELECT actor,action,detail FROM audit_log WHERE account_id=? ORDER BY id DESC LIMIT 1`, out["account_id"]).Scan(&actor, &action, &detail); err != nil {
		t.Fatal(err)
	}
	if actor != "partner:blink" || action != "partner.accounts.ensure" || !strings.Contains(detail, `"status":200`) {
		t.Fatalf("audit row %s %s %s", actor, action, detail)
	}
	// 600 requests per minute per partner; the ensure above used some.
	limited := false
	for i := 0; i < 600; i++ {
		status, out := env.blink(http.MethodDelete, "/partner/v1/credentials/missing", nil)
		if status == http.StatusTooManyRequests {
			if out["error"] != "rate_limited" {
				t.Fatalf("rate limit body %v", out)
			}
			limited = true
			break
		}
		if status != 404 {
			t.Fatalf("revoke unknown %d %v", status, out)
		}
	}
	if !limited {
		t.Fatal("partner rate limit never applied")
	}
	var audited int
	if err := env.db.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE actor='partner:blink'`).Scan(&audited); err != nil || audited < 590 {
		t.Fatalf("audit rows %d %v", audited, err)
	}
}

func TestPartnerEnsureCreatesLinksAndIsExemptFromPublicCaps(t *testing.T) {
	env := newPartnerEnv(t, func(c *service.Config) {
		c.AccountCap = 1
		c.PartnerAccountCap = 2
		c.RegistrationMode = contracts.RegistrationInviteOnly
		c.InviteAllowlist = []string{"public@example.com"}
	})
	for _, payload := range []map[string]any{{"email": "x@example.com"}, {"email": "x@example.com", "email_verified": false}, {"email": "not an email", "email_verified": true}} {
		status, out := env.blink(http.MethodPost, "/partner/v1/accounts:ensure", payload)
		if status != 400 {
			t.Fatalf("ensure %v: %d %v", payload, status, out)
		}
	}
	out := env.ensure("  New.User@Example.com ")
	if out["created"] != true || out["linked"] != true {
		t.Fatalf("new account %v", out)
	}
	account := out["account_id"].(string)
	var creator, email, status string
	var customer any
	if err := env.db.DB().QueryRow(`SELECT created_by_partner,email,status,stripe_customer_id FROM accounts WHERE id=? AND email_hash=?`, account, store.Hash("new.user@example.com")).Scan(&creator, &email, &status, &customer); err != nil {
		t.Fatal(err)
	}
	if creator != "blink" || email != "new.user@example.com" || status != "active" || customer != nil {
		t.Fatalf("account row %s %s %s %v", creator, email, status, customer)
	}
	brains, err := env.db.Brains(context.Background(), account)
	if err != nil || len(brains) != 1 || brains[0].State != "ready" {
		t.Fatalf("default brain %+v %v", brains, err)
	}
	again := env.ensure("new.user@example.com")
	if again["account_id"] != account || again["created"] != false || again["linked"] != true {
		t.Fatalf("idempotent ensure %v", again)
	}
	env.ensure("second@example.com")
	if status, out := env.blink(http.MethodPost, "/partner/v1/accounts:ensure", map[string]any{"email": "third@example.com", "email_verified": true}); status != 503 || out["error"] != "capacity_reached" {
		t.Fatalf("partner cap %d %v", status, out)
	}
	// Partner accounts do not consume the public AccountCap of 1.
	if next := env.login(env.browser(), "public@example.com"); next != "/dashboard" {
		t.Fatalf("public signup blocked: %s", next)
	}
	// The user can claim the partner-created account with the magic link,
	// even though invite_only would refuse a new registration.
	if next := env.login(env.browser(), "new.user@example.com"); next != "/dashboard" {
		t.Fatalf("partner-created account not claimable: %s", next)
	}
}

func TestPartnerExistingAccountRequiresOwnerConsent(t *testing.T) {
	env := newPartnerEnv(t, nil)
	if code := env.seed("other", "Other", blinkSecret+"-other", "https://other.example/cb", ""); code != http.StatusNoContent {
		t.Fatalf("seed other %d", code)
	}
	existing, err := env.db.CreateAccount(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	out := env.ensure("owner@example.com")
	if out["account_id"] != existing.ID || out["created"] != false || out["linked"] != false {
		t.Fatalf("existing account %v", out)
	}
	if status, out := env.blink(http.MethodPost, "/partner/v1/credentials", map[string]any{"account_id": existing.ID}); status != 404 || out["error"] != "not_linked" {
		t.Fatalf("credentials before consent %d %v", status, out)
	}
	for _, bad := range []string{"blink://serenity-linked.evil", "https://evil.example/cb", "blink://serenity-linkedx", "blink://serenity-linked#frag", ""} {
		if status, out := env.blink(http.MethodPost, "/partner/v1/link-requests", map[string]any{"account_id": existing.ID, "return_url": bad}); status != 400 || out["error"] != "invalid_return_url" {
			t.Fatalf("return_url %q: %d %v", bad, status, out)
		}
	}
	if status, _ := env.blink(http.MethodPost, "/partner/v1/link-requests", map[string]any{"account_id": "missing", "return_url": "blink://serenity-linked"}); status != 404 {
		t.Fatalf("unknown account link request %d", status)
	}
	newRequest := func() map[string]any {
		status, out := env.blink(http.MethodPost, "/partner/v1/link-requests", map[string]any{"account_id": existing.ID, "return_url": "blink://serenity-linked?flow=1"})
		if status != 201 || out["link_request_id"] == "" || !strings.HasPrefix(out["consent_url"].(string), env.svc.Partner.Origin+"/partner/consent?request=") {
			t.Fatalf("link request %d %v", status, out)
		}
		expires, err := time.Parse(time.RFC3339, out["expires_at"].(string))
		if err != nil || time.Until(expires) > 15*time.Minute+time.Second || time.Until(expires) < 14*time.Minute {
			t.Fatalf("expires_at %v %v", out["expires_at"], err)
		}
		return out
	}

	// Deny first.
	denied := newRequest()
	browser := env.browser()
	target := env.consent(browser, denied["consent_url"].(string), "owner@example.com", "deny")
	if target.Scheme != "blink" || target.Host != "serenity-linked" || target.Query().Get("error") != "denied" || target.Query().Get("state") != denied["link_request_id"] || target.Query().Get("code") != "" {
		t.Fatalf("deny redirect %s", target)
	}
	if status, _ := env.blink(http.MethodPost, "/partner/v1/link-requests/"+denied["link_request_id"].(string)+":redeem", map[string]any{"code": "anything"}); status != 409 {
		t.Fatalf("denied request redeemed: %d", status)
	}

	// Allow, then redeem exactly once.
	allowed := newRequest()
	target = env.consent(browser, allowed["consent_url"].(string), "owner@example.com", "allow")
	code := target.Query().Get("code")
	if target.Query().Get("flow") != "1" || target.Query().Get("state") != allowed["link_request_id"] || len(code) != 43 {
		t.Fatalf("allow redirect %s", target)
	}
	redeem := "/partner/v1/link-requests/" + allowed["link_request_id"].(string) + ":redeem"
	if status, _ := env.call(http.MethodPost, redeem, "Partner other:"+blinkSecret+"-other", map[string]any{"code": code}); status != 404 {
		t.Fatalf("other partner redeemed Blink's request: %d", status)
	}
	if status, out := env.blink(http.MethodPost, redeem, map[string]any{"code": code + "x"}); status != 400 || out["error"] != "invalid_code" {
		t.Fatalf("wrong code %d %v", status, out)
	}
	if status, out := env.blink(http.MethodPost, redeem, map[string]any{"code": code}); status != 200 || out["account_id"] != existing.ID || out["linked"] != true {
		t.Fatalf("redeem %d %v", status, out)
	}
	if status, out := env.blink(http.MethodPost, redeem, map[string]any{"code": code}); status != 409 {
		t.Fatalf("second redeem %d %v", status, out)
	}
	if out := env.ensure("owner@example.com"); out["linked"] != true || out["created"] != false {
		t.Fatalf("ensure after consent %v", out)
	}
	if status, out := env.blink(http.MethodPost, "/partner/v1/link-requests", map[string]any{"account_id": existing.ID, "return_url": "blink://serenity-linked"}); status != 409 || out["error"] != "already_linked" {
		t.Fatalf("link request while linked %d %v", status, out)
	}
	env.issue(existing.ID)

	// An approved code expires 15 minutes after the request was created.
	if status, _ := env.blink(http.MethodDelete, "/partner/v1/links/"+existing.ID, nil); status != 204 {
		t.Fatalf("unlink %d", status)
	}
	late := newRequest()
	target = env.consent(browser, late["consent_url"].(string), "owner@example.com", "allow")
	env.svc.Partner.Clock = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if status, out := env.blink(http.MethodPost, "/partner/v1/link-requests/"+late["link_request_id"].(string)+":redeem", map[string]any{"code": target.Query().Get("code")}); status != 410 || out["error"] != "expired" {
		t.Fatalf("expired redeem %d %v", status, out)
	}
	env.svc.Partner.Clock = nil

	// A different signed-in user can never approve the owner's request.
	stranger := newRequest()
	intruder := env.browser()
	env.login(intruder, "intruder@example.com")
	u, _ := url.Parse(stranger["consent_url"].(string))
	resp, body := env.get(intruder, u.RequestURI())
	if resp.StatusCode != http.StatusForbidden || strings.Contains(body, `value="allow"`) {
		t.Fatalf("stranger consent page %d %s", resp.StatusCode, body)
	}
	form := url.Values{"csrf": {csrfOf(t, body)}, "request": {stranger["link_request_id"].(string)}, "decision": {"allow"}}
	if resp := env.post(intruder, "/partner/consent", form, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("stranger approval %d", resp.StatusCode)
	}
}

func TestPartnerKeysRevokeIndependentlyAndUnlinkKeepsAccount(t *testing.T) {
	env := newPartnerEnv(t, nil)
	ctx := context.Background()
	if code := env.seed("other", "Other", blinkSecret+"-other", "https://other.example/cb", ""); code != http.StatusNoContent {
		t.Fatalf("seed other %d", code)
	}
	account := env.ensure("keys@example.com")["account_id"].(string)
	brains, err := env.db.Brains(ctx, account)
	if err != nil || len(brains) != 1 {
		t.Fatal(brains, err)
	}
	brain := brains[0].ID
	// The owner's own manual key and an OAuth grant on the same brain.
	manual, err := env.svc.Gateway.Issuer.Issue(ctx, brain, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	grant := `{"id":"grant-1","subject":"` + account + `","binding":"` + brain + `.0","client_id":"claude"}`
	if _, err = env.db.DB().Exec(`INSERT INTO oauth_grants(id,record,expires_at) VALUES('grant-1',?,?)`, grant, store.Stamp(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	issuer := env.svc.Gateway.Issuer
	firstID, first := env.issue(account)
	binding, err := issuer.Verify(ctx, first)
	if err != nil || binding.PartnerID != "blink" || binding.BrainID != brain || binding.CredentialID != firstID {
		t.Fatalf("partner binding %+v %v", binding, err)
	}
	secondID, second := env.issue(account)
	if _, err = issuer.Verify(ctx, first); !errors.Is(err, credential.ErrRevoked) {
		t.Fatalf("rotated partner key still valid: %v", err)
	}
	if secondID == firstID {
		t.Fatal("rotation reused credential id")
	}
	session := env.mcpSession(second)
	if text, isErr := env.tool(second, session, "remember", map[string]any{"fact": "Blink remembers the teal kite", "provenance": "partner test", "operation_key": "blink-1"}); isErr {
		t.Fatalf("partner remember %s", text)
	}
	if status, _ := env.call(http.MethodDelete, "/partner/v1/credentials/"+secondID, "Partner other:"+blinkSecret+"-other", nil); status != 404 {
		t.Fatalf("other partner revoked Blink's key: %d", status)
	}
	if status, _ := env.blink(http.MethodDelete, "/partner/v1/credentials/"+secondID, nil); status != 204 {
		t.Fatalf("revoke key %d", status)
	}
	if _, err = issuer.Verify(ctx, second); !errors.Is(err, credential.ErrRevoked) {
		t.Fatalf("revoked partner key %v", err)
	}
	if status, _, _ := env.mcp(second, session, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list"}); status != 401 {
		t.Fatalf("revoked key session %d", status)
	}
	assertOthersIntact := func(stage string) {
		t.Helper()
		if b, err := issuer.Verify(ctx, manual); err != nil || b.PartnerID != "" {
			t.Fatalf("%s: manual key %+v %v", stage, b, err)
		}
		var grants, epochs int
		if err := env.db.DB().QueryRow(`SELECT (SELECT count(*) FROM oauth_grants WHERE id='grant-1'),(SELECT count(*) FROM oauth_epochs WHERE brain_id=?)`, brain).Scan(&grants, &epochs); err != nil || grants != 1 || epochs != 0 {
			t.Fatalf("%s: OAuth grant/epoch disturbed grants=%d epochs=%d %v", stage, grants, epochs, err)
		}
	}
	assertOthersIntact("per-key revoke")
	manualSession := env.mcpSession(manual)
	if text, isErr := env.tool(manual, manualSession, "recall", map[string]any{"query": "teal kite"}); isErr || !strings.Contains(text, "teal kite") {
		t.Fatalf("manual key recall %s", text)
	}

	_, third := env.issue(account)
	if status, _ := env.blink(http.MethodDelete, "/partner/v1/links/"+account, nil); status != 204 {
		t.Fatalf("unlink %d", status)
	}
	if status, out := env.blink(http.MethodDelete, "/partner/v1/links/"+account, nil); status != 404 || out["error"] != "not_found" {
		t.Fatalf("second unlink %d %v", status, out)
	}
	if _, err = issuer.Verify(ctx, third); !errors.Is(err, credential.ErrRevoked) {
		t.Fatalf("unlink left partner key live: %v", err)
	}
	assertOthersIntact("unlink")
	var status string
	if err = env.db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, account).Scan(&status); err != nil || status != "active" {
		t.Fatalf("unlink touched account: %s %v", status, err)
	}
	if b, err := env.db.BrainByID(ctx, account, brain); err != nil || b.State != "ready" {
		t.Fatalf("unlink touched brain: %+v %v", b, err)
	}
	if _, err = os.Stat(filepath.Join(env.dir, "brains", brain)); err != nil {
		t.Fatalf("brain directory removed: %v", err)
	}
	if text, isErr := env.tool(manual, manualSession, "recall", map[string]any{"query": "teal kite"}); isErr || !strings.Contains(text, "teal kite") {
		t.Fatalf("memory lost after unlink: %s", text)
	}
	if code, out := env.blink(http.MethodPost, "/partner/v1/credentials", map[string]any{"account_id": account}); code != 404 || out["error"] != "not_linked" {
		t.Fatalf("credentials after unlink %d %v", code, out)
	}
	if out := env.ensure("keys@example.com"); out["linked"] != false || out["created"] != false {
		t.Fatalf("unlinked partner-created account relinked without consent: %v", out)
	}
}

func TestDashboardShowsAndDisconnectsPartner(t *testing.T) {
	env := newPartnerEnv(t, nil)
	ctx := context.Background()
	account := env.ensure("dash@example.com")["account_id"].(string)
	_, key := env.issue(account)
	browser := env.browser()
	env.login(browser, "dash@example.com")
	resp, body := env.get(browser, "/dashboard/connections")
	if resp.StatusCode != 200 || !strings.Contains(body, "Connected apps") || !strings.Contains(body, "Disconnect Blink") {
		t.Fatalf("connections page %d %s", resp.StatusCode, body)
	}
	form := url.Values{"csrf": {csrfOf(t, body)}, "partner_id": {"blink"}}
	if resp := env.post(browser, "/connections/partners/disconnect", form, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disconnect %d", resp.StatusCode)
	}
	if _, err := env.svc.Gateway.Issuer.Verify(ctx, key); !errors.Is(err, credential.ErrRevoked) {
		t.Fatalf("partner key after owner disconnect: %v", err)
	}
	_, body = env.get(browser, "/dashboard/connections")
	if strings.Contains(body, "Disconnect Blink") {
		t.Fatal("disconnected partner still listed")
	}
	if brains, err := env.db.Brains(ctx, account); err != nil || len(brains) != 1 {
		t.Fatalf("disconnect removed memory %+v %v", brains, err)
	}
	if code, _ := env.blink(http.MethodPost, "/partner/v1/credentials", map[string]any{"account_id": account}); code != 404 {
		t.Fatalf("credentials after owner disconnect %d", code)
	}

	// Account deletion ends partner links.
	other := env.ensure("gone@example.com")["account_id"].(string)
	if err := env.svc.Gateway.DeleteAccount(ctx, other, filepath.Join(env.dir, "brains")); err != nil {
		t.Fatal(err)
	}
	var linkStatus string
	if err := env.db.DB().QueryRow(`SELECT status FROM partner_links WHERE account_id=?`, other).Scan(&linkStatus); err != nil || linkStatus != "revoked" {
		t.Fatalf("deleted account link %s %v", linkStatus, err)
	}
}

func TestPartnerMeteringSplitsBuckets(t *testing.T) {
	env := newPartnerEnv(t, nil)
	ctx := context.Background()
	account := env.ensure("meter@example.com")["account_id"].(string)
	brains, _ := env.db.Brains(ctx, account)
	manual, err := env.svc.Gateway.Issuer.Issue(ctx, brains[0].ID, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	_, key := env.issue(account)
	now := time.Now().UTC()
	accountWindow := now.Format("2006-01")
	partnerWindow := meter.PartnerWindow("blink", now)
	partnerSession := env.mcpSession(key)
	manualSession := env.mcpSession(manual)
	n := 0
	remember := func(token, session string) (string, bool) {
		n++
		return env.tool(token, session, "remember", map[string]any{"fact": "Metering fact number " + strings.Repeat("x", n), "provenance": "meter test", "operation_key": "meter-" + strings.Repeat("k", n)})
	}
	expect := func(stage string, accountWrites, partnerWrites int64) {
		t.Helper()
		if got := env.used(account, accountWindow, "writes"); got != accountWrites {
			t.Fatalf("%s: account writes %d want %d", stage, got, accountWrites)
		}
		if got := env.used(account, partnerWindow, "writes"); got != partnerWrites {
			t.Fatalf("%s: partner writes %d want %d", stage, got, partnerWrites)
		}
	}
	for _, bad := range []map[string]any{{"tier": "gold"}, {"tier": "pro", "expires_at": "tomorrow"}} {
		if status, _ := env.blink(http.MethodPut, "/partner/v1/entitlements/"+account, bad); status != 400 {
			t.Fatalf("entitlement %v accepted: %d", bad, status)
		}
	}
	if status, _ := env.blink(http.MethodPut, "/partner/v1/entitlements/not-linked", map[string]any{"tier": "pro"}); status != 404 {
		t.Fatalf("entitlement for unlinked account %d", status)
	}

	// Free (no entitlement): partner traffic counts against the account.
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatal(text)
	}
	expect("free", 1, 0)
	status, usage := env.blink(http.MethodGet, "/partner/v1/accounts/"+account+"/usage", nil)
	if status != 200 || usage["partner"] != nil || usage["account"].(map[string]any)["writes"].(float64) != 1 {
		t.Fatalf("free usage %d %v", status, usage)
	}

	// Pro: partner traffic uses the partner bucket; other clients do not.
	if status, out := env.blink(http.MethodPut, "/partner/v1/entitlements/"+account, map[string]any{"tier": "pro", "expires_at": now.Add(24 * time.Hour).Format(time.RFC3339)}); status != 200 || out["tier"] != "pro" {
		t.Fatalf("set pro %d %v", status, out)
	}
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatal(text)
	}
	expect("pro partner", 1, 1)
	if text, isErr := env.tool(key, partnerSession, "recall", map[string]any{"query": "Metering"}); isErr {
		t.Fatal(text)
	}
	if got := env.used(account, partnerWindow, "recalls"); got != 1 {
		t.Fatalf("partner recalls %d", got)
	}
	if text, isErr := remember(manual, manualSession); isErr {
		t.Fatal(text)
	}
	expect("pro manual", 2, 1)
	status, usage = env.blink(http.MethodGet, "/partner/v1/accounts/"+account+"/usage", nil)
	partnerUsage, _ := usage["partner"].(map[string]any)
	if status != 200 || partnerUsage == nil || partnerUsage["tier"] != "pro" || partnerUsage["writes"].(float64) != 1 || partnerUsage["limits"].(map[string]any)["writes"].(float64) != 5000 {
		t.Fatalf("pro usage %d %v", status, usage)
	}
	if acct := usage["account"].(map[string]any); acct["plan"] != "free" || acct["writes"].(float64) != 2 || acct["memories"].(float64) != 3 || acct["limits"].(map[string]any)["writes"].(float64) != 500 {
		t.Fatalf("account usage %v", acct)
	}

	// An exhausted account plan refuses other clients with bucket "account"
	// while partner-pro traffic continues in its own bucket.
	// Write quotas are admitted by the operation ledger, so exhaust a bucket
	// with a committed operation charged to that bucket's quota period.
	exhaust := func(window string, units int) {
		t.Helper()
		stamp := store.Stamp(time.Now())
		if _, err := env.db.DB().Exec(`INSERT INTO operations(id,account_id,brain_id,deltas_json,quota_period,phase,source,lease_expires_at,created_at) VALUES(?,?,?,?,?,'committed','test.exhaust',?,?)`, store.ID(), account, brains[0].ID, `[{"metric":"writes","units":`+strconv.Itoa(units)+`}]`, window, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	exhaust(accountWindow, 500)
	text, isErr := remember(manual, manualSession)
	if !isErr || !strings.Contains(text, `"error":"limit_exceeded"`) || !strings.Contains(text, `"bucket":"account"`) {
		t.Fatalf("account bucket limit %s", text)
	}
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatalf("partner-pro write blocked by account plan: %s", text)
	}
	expect("account exhausted", 2, 2)
	exhaust(partnerWindow, 5000)
	text, isErr = remember(key, partnerSession)
	if !isErr || !strings.Contains(text, `"error":"limit_exceeded"`) || !strings.Contains(text, `"bucket":"partner"`) {
		t.Fatalf("partner bucket limit %s", text)
	}

	// Expired pro and explicit free fall back to the account plan.
	if _, err = env.db.DB().Exec(`DELETE FROM operations WHERE source='test.exhaust'`); err != nil {
		t.Fatal(err)
	}
	if _, err = env.db.DB().Exec(`UPDATE usage_windows SET committed=0 WHERE account_id=?`, account); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.blink(http.MethodPut, "/partner/v1/entitlements/"+account, map[string]any{"tier": "pro", "expires_at": now.Add(-time.Hour).Format(time.RFC3339)}); status != 200 {
		t.Fatal("set expired pro")
	}
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatal(text)
	}
	expect("expired pro", 1, 0)
	if status, _ := env.blink(http.MethodPut, "/partner/v1/entitlements/"+account, map[string]any{"tier": "free"}); status != 200 {
		t.Fatal("set free")
	}
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatal(text)
	}
	expect("free again", 2, 0)
	// Comp grants: pro without an end date stays live.
	if status, out := env.blink(http.MethodPut, "/partner/v1/entitlements/"+account, map[string]any{"tier": "pro"}); status != 200 || out["expires_at"] != nil {
		t.Fatalf("open-ended pro %d %v", status, out)
	}
	if text, isErr := remember(key, partnerSession); isErr {
		t.Fatal(text)
	}
	expect("open-ended pro", 2, 1)
}
