package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestChromeConsentHandoff(t *testing.T) {
	if os.Getenv("SERENITY_PLAYWRIGHT") == "" {
		t.Skip("set SERENITY_PLAYWRIGHT for local Chromium regression")
	}
	for _, decision := range []string{"approve", "deny"} {
		t.Run(decision, func(t *testing.T) {
			adapter, account, _ := lifecycleStore(t)
			ctx := ctxOf(t)
			now := time.Now()
			_, err := adapter.DB.DB().Exec("INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)", store.ID(), account.ID, store.Hash(strings.Repeat("s", 43)), store.Stamp(now), store.Stamp(now.Add(time.Hour)), "fixture-csrf")
			if err != nil {
				t.Fatal(err)
			}
			var visits, exchanges atomic.Int32
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("Completed")) }))
			defer app.Close()
			var h *Hosted
			verifier := strings.Repeat("v", 43)
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				visits.Add(1)
				if r.URL.Query().Get("state") != "fixture-state" {
					t.Error("state changed")
				}
				if r.Header.Get("Referer") != "" {
					t.Error("referrer disclosed")
				}
				if decision == "deny" {
					if r.URL.Query().Get("error") != "access_denied" || r.URL.Query().Get("code") != "" {
						t.Error("incorrect denial")
					}
				} else {
					v := url.Values{"grant_type": {"authorization_code"}, "client_id": {"browser-fixture"}, "redirect_uri": {"http://" + r.Host + "/callback"}, "code": {r.URL.Query().Get("code")}, "code_verifier": {verifier}, "resource": {h.Origin + "/mcp"}}
					for attempt := 0; attempt < 2; attempt++ {
						req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(v.Encode()))
						req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
						rr := httptest.NewRecorder()
						h.Server.TokenHandler().ServeHTTP(rr, req)
						if attempt == 0 {
							if rr.Code != 200 {
								t.Errorf("exchange status %d", rr.Code)
							} else {
								exchanges.Add(1)
							}
						} else if rr.Code == 200 {
							t.Error("code replay accepted")
						}
					}
				}
				http.Redirect(w, r, app.URL+"/complete", http.StatusFound)
			}))
			defer callback.Close()
			server := httptest.NewUnstartedServer(nil)
			origin := "http://" + server.Listener.Addr().String()
			h, err = New(adapter.DB, &identity.Service{Store: adapter.DB}, &provision.Provisioner{Store: adapter.DB, BrainsRoot: t.TempDir()}, origin, true)
			if err != nil {
				t.Fatal(err)
			}
			server.Config.Handler = h.Handler()
			server.Start()
			defer server.Close()
			if err = h.Store.CreateClient(ctx, mcpoauth.Client{ID: "browser-fixture", Name: "Browser fixture", RedirectURIs: []string{callback.URL + "/callback"}, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(verifier))
			v := url.Values{"response_type": {"code"}, "client_id": {"browser-fixture"}, "redirect_uri": {callback.URL + "/callback"}, "state": {"fixture-state"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"}, "resource": {origin + "/mcp"}, "scope": {"memory:read"}}
			cmd := exec.Command("node", "../browserflow/testdata/handoff.cjs", origin+"/oauth/authorize?"+v.Encode(), decision, app.URL)
			out, e := cmd.CombinedOutput()
			t.Logf("Chromium: %s; callback visits=%d exchanges=%d", out, visits.Load(), exchanges.Load())
			if e != nil {
				t.Fatal(e)
			}
			if visits.Load() != 1 {
				t.Error("callback count")
			}
			want := int32(0)
			if decision == "approve" {
				want = 1
			}
			if exchanges.Load() != want {
				t.Error("exchange count")
			}
			if n := count(t, adapter.DB.DB(), "oauth_consents"); n != 0 {
				t.Error("consent not consumed")
			}
		})
	}
}
