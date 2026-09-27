package oauth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// SEC-H02: an unauthenticated flood from a handful of addresses must not
// switch off the OAuth authorization server for every tenant.

// floodHost builds a Hosted over a fresh control database holding one live
// grant with a refresh token, so a real refresh_token grant can be exercised
// through the public handler.
func floodHost(t *testing.T) (*Hosted, string) {
	t.Helper()
	adapter, a, brain := lifecycleStore(t)
	now := time.Now().UTC()
	rawRefresh := strings.Repeat("r", 43)
	grant := mcpoauth.GrantRecord{ID: "flood-family", ClientID: "flood-client", Subject: a.ID, Binding: brain + ".0", Resource: "https://memory.example/mcp", Scopes: []string{"memory:read"}, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	token := mcpoauth.TokenRecord{ID: store.Hash("flood-access"), GrantID: grant.ID, ClientID: grant.ClientID, Subject: grant.Subject, Binding: grant.Binding, Resource: grant.Resource, Scopes: grant.Scopes, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	refresh := mcpoauth.RefreshRecord{ID: store.Hash(rawRefresh), GrantID: grant.ID, ClientID: grant.ClientID, Generation: 1, CreatedAt: now, ExpiresAt: grant.ExpiresAt}
	if err := adapter.CreateGrant(ctxOf(t), mcpoauth.GrantIssue{Grant: grant, Token: token, Refresh: &refresh}); err != nil {
		t.Fatal(err)
	}
	h, err := New(adapter.DB, nil, nil, "https://memory.example", true)
	if err != nil {
		t.Fatal(err)
	}
	return h, rawRefresh
}

func requestFrom(t *testing.T, handler http.Handler, method, target, ip string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if form == nil {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	host := ip
	if strings.Contains(ip, ":") {
		host = "[" + ip + "]"
	}
	r.RemoteAddr = host + ":40000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestSECH02SeventeenAddressFloodLeavesRefreshAdmitted(t *testing.T) {
	h, rawRefresh := floodHost(t)
	handler := h.Handler()
	// Seventeen distinct /24 prefixes from the RFC 2544 benchmarking block
	// each send the full per-key allowance within one minute.
	for i := range 17 {
		for range 120 {
			w := requestFrom(t, handler, "GET", "/oauth/authorize", fmt.Sprintf("198.18.%d.7", i), nil)
			if w.Code == http.StatusTooManyRequests {
				t.Fatalf("prefix %d refused inside its own allowance", i)
			}
		}
	}
	// A refresh_token grant for a known grant from an eighteenth address must
	// still be admitted: the global window no longer covers /oauth/token.
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rawRefresh}, "client_id": {"flood-client"}}
	w := requestFrom(t, handler, "POST", "/oauth/token", "198.18.200.9", form)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh from 18th address: status %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"access_token"`) {
		t.Fatalf("refresh from 18th address returned no access token: %s", w.Body.String())
	}
	// The eighteenth prefix's own authorize is judged by its own bucket, not by
	// the seventeen attackers' consumption.
	w = requestFrom(t, handler, "GET", "/oauth/authorize", "198.18.200.9", nil)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("authorize from an untouched prefix refused: %d", w.Code)
	}
	// Any single flooding prefix remains refused for the rest of the window.
	w = requestFrom(t, handler, "GET", "/oauth/authorize", "198.18.3.250", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("flooding /24 admitted a 121st request: %d", w.Code)
	}
}

func TestSECH02IPv6RotationWithinPrefixIsOneKey(t *testing.T) {
	h, _ := floodHost(t)
	handler := h.Handler()
	admitted := 0
	// Rotate through 130 distinct /64 subnets (and hosts) inside one /56 of the
	// documentation prefix. They must share a single limiter key.
	for i := range 130 {
		ip := fmt.Sprintf("2001:db8:0:%x::%x", i, i+1)
		w := requestFrom(t, handler, "GET", "/oauth/authorize", ip, nil)
		if w.Code != http.StatusTooManyRequests {
			admitted++
		}
	}
	if admitted != 120 {
		t.Fatalf("admitted %d requests from one /56; want exactly the per-key allowance of 120", admitted)
	}
	// A neighbouring /56 is a separate key and is not affected.
	w := requestFrom(t, handler, "GET", "/oauth/authorize", "2001:db8:0:100::1", nil)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("neighbouring /56 refused: %d", w.Code)
	}
}

func consentFor(client string, n int, expires time.Time) mcpoauth.ConsentRecord {
	now := time.Now().UTC()
	return mcpoauth.ConsentRecord{ID: store.Hash(fmt.Sprintf("%s-%d", client, n)), ClientID: client, RedirectURI: "https://client.example/callback", CodeChallenge: strings.Repeat("c", 43), Scopes: []string{"memory:read"}, CreatedAt: now, ExpiresAt: expires}
}

func TestSECH02ConsentCapIsPerClient(t *testing.T) {
	adapter, _, _ := lifecycleStore(t)
	ctx := context.Background()
	live := time.Now().UTC().Add(20 * time.Minute)
	for i := range 500 {
		if err := adapter.CreateConsent(ctx, consentFor("client-a", i, live)); err != nil {
			t.Fatalf("consent %d for client-a: %v", i, err)
		}
	}
	if err := adapter.CreateConsent(ctx, consentFor("client-a", 500, live)); err == nil {
		t.Fatal("client-a exceeded 500 live consents")
	}
	if err := adapter.CreateConsent(ctx, consentFor("client-b", 0, live)); err != nil {
		t.Fatalf("client-b blocked by client-a's consents: %v", err)
	}
	// Expired consents of the same client are evicted on insert, freeing room.
	if _, err := adapter.DB.DB().ExecContext(ctx, "UPDATE oauth_consents SET expires_at=? WHERE json_extract(record,'$.client_id')='client-a'", store.Stamp(time.Now().Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := adapter.CreateConsent(ctx, consentFor("client-a", 501, live)); err != nil {
		t.Fatalf("client-a still refused after its consents expired: %v", err)
	}
	if n := countWhere(t, adapter.DB.DB(), "oauth_consents", "json_extract(record,'$.client_id')='client-a'"); n != 1 {
		t.Fatalf("expired client-a consents not evicted: %d rows", n)
	}
}

func TestSECH02TableCapNoLongerAppliesToConsents(t *testing.T) {
	adapter, _, _ := lifecycleStore(t)
	ctx := context.Background()
	live := store.Stamp(time.Now().UTC().Add(20 * time.Minute))
	// 10,050 live consents spread over 201 clients, each under its own cap.
	err := adapter.DB.Transaction(ctx, func(tx *sql.Tx) error {
		for c := range 201 {
			for i := range 50 {
				rec := consentFor(fmt.Sprintf("bulk-%d", c), i, time.Now().Add(20*time.Minute))
				if _, err := tx.ExecContext(ctx, "INSERT INTO oauth_consents(id,record,expires_at) VALUES(?,?,?)", rec.ID, fmt.Sprintf(`{"id":%q,"client_id":%q}`, rec.ID, rec.ClientID), live); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.CreateConsent(ctx, consentFor("fresh-client", 0, time.Now().UTC().Add(20*time.Minute))); err != nil {
		t.Fatalf("authorize for another client blocked by table volume: %v", err)
	}
}

func countWhere(t *testing.T, db *sql.DB, table, where string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table + " WHERE " + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPrefixKeyMasksV4To24AndV6To56(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"198.18.7.9", "198.18.7.0"},
		{"198.18.7.250", "198.18.7.0"},
		{"198.18.8.1", "198.18.8.0"},
		{"2001:db8:0:1::1", "2001:db8::"},
		{"2001:db8:0:ff:ffff:ffff:ffff:ffff", "2001:db8::"},
		{"2001:db8:0:100::1", "2001:db8:0:100::"},
		{"::ffff:198.18.7.9", "198.18.7.0"},
		{"not-an-address", "not-an-address"},
	} {
		if got := prefixKey(tc.in); got != tc.want {
			t.Errorf("prefixKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestLimiterPerKeyIsEvaluatedBeforeGlobal(t *testing.T) {
	l := newLimiter(2, 3, time.Minute)
	base := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return base }
	// Key a exhausts its own allowance; the third refusal must not have
	// charged the global window.
	if !l.allow("a") || !l.allow("a") || l.allow("a") {
		t.Fatal("per-key allowance of 2 not enforced")
	}
	if l.global.count != 2 {
		t.Fatalf("refused per-key request charged the global window: %d", l.global.count)
	}
	// One more request from another key fills the global ceiling of 3.
	if !l.allow("b") {
		t.Fatal("key b refused within both allowances")
	}
	// A saturated global window refuses without charging the key.
	if l.allow("c") {
		t.Fatal("global ceiling of 3 not enforced")
	}
	if l.keys["c"].count != 0 {
		t.Fatalf("globally refused request charged key c: %d", l.keys["c"].count)
	}
	// After the window both reset.
	l.now = func() time.Time { return base.Add(time.Minute) }
	if !l.allow("a") || !l.allow("c") {
		t.Fatal("windows did not reset after one minute")
	}
}

func TestLimiterZeroLimitsDisableThatWindow(t *testing.T) {
	global := newLimiter(0, 2, time.Minute)
	if !global.allow("a") || !global.allow("b") || global.allow("c") {
		t.Fatal("global-only limiter did not enforce its ceiling across keys")
	}
	if len(global.keys) != 0 {
		t.Fatal("global-only limiter tracked keys")
	}
	perKey := newLimiter(1, 0, time.Minute)
	if !perKey.allow("a") || perKey.allow("a") || !perKey.allow("b") {
		t.Fatal("per-key-only limiter did not isolate keys")
	}
}

func TestLimiterSweepDropsElapsedKeysAndBoundsMemory(t *testing.T) {
	l := newLimiter(1, 0, time.Minute)
	base := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return base }
	for i := range 10 {
		l.allow(fmt.Sprint("k", i))
	}
	l.now = func() time.Time { return base.Add(time.Minute) }
	l.allow("fresh")
	if len(l.keys) != 1 {
		t.Fatalf("elapsed keys survived the sweep: %d", len(l.keys))
	}
	l.keys = make(map[string]*rateWindow, maxTrackedKeys+1)
	for i := range maxTrackedKeys + 1 {
		l.keys[fmt.Sprint(i)] = &rateWindow{start: base.Add(time.Minute), count: 1}
	}
	if !l.allow("0") {
		t.Fatal("over-capacity table was not reset")
	}
	if len(l.keys) != 1 {
		t.Fatalf("over-capacity table not reset: %d keys", len(l.keys))
	}
}

func TestSECH02GlobalCeilingCoversOnlyRegisterAndAuthorize(t *testing.T) {
	h, rawRefresh := floodHost(t)
	h.limits.StateCreating = 30
	handler := h.Handler()
	// Thirty distinct prefixes each send one authorize; the global ceiling is
	// now full for register and authorize.
	for i := range 30 {
		if w := requestFrom(t, handler, "GET", "/oauth/authorize", fmt.Sprintf("198.18.%d.1", i), nil); w.Code == http.StatusTooManyRequests {
			t.Fatalf("authorize %d refused under the ceiling", i)
		}
	}
	if w := requestFrom(t, handler, "GET", "/oauth/authorize", "198.18.200.1", nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("authorize admitted past the global ceiling: %d", w.Code)
	}
	if w := requestFrom(t, handler, "POST", "/oauth/register", "198.18.201.1", url.Values{"redirect_uris": {"https://client.example/cb"}}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("register admitted past the global ceiling: %d", w.Code)
	}
	// Token, revoke and the session-authenticated paths are outside it.
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rawRefresh}, "client_id": {"flood-client"}}
	w := requestFrom(t, handler, "POST", "/oauth/token", "198.18.202.1", form)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh refused by the state-creating ceiling: %d %s", w.Code, w.Body.String())
	}
	if w := requestFrom(t, handler, "POST", "/oauth/revoke", "198.18.203.1", url.Values{"token": {"x"}, "client_id": {"flood-client"}}); w.Code == http.StatusTooManyRequests {
		t.Fatal("revoke refused by the state-creating ceiling")
	}
	if w := requestFrom(t, handler, "POST", "/oauth/consent", "198.18.204.1", url.Values{}); w.Code == http.StatusTooManyRequests {
		t.Fatal("consent refused by the state-creating ceiling")
	}
}

func TestSECH02GlobalCeilingIsAtLeastTenTimesPerKey(t *testing.T) {
	l := defaultRateLimits()
	for name, perKey := range map[string]int{"PerPrefix": l.PerPrefix, "Register": l.Register, "Token": l.Token} {
		if l.StateCreating < 10*perKey {
			t.Errorf("StateCreating %d is below 10x %s %d", l.StateCreating, name, perKey)
		}
	}
}
