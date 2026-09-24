package billing_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/dashboard"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestLegacyCancelAccountRejectsPendingClosure(t *testing.T) {
	for _, returnedStatus := range []string{"active", "canceled"} {
		t.Run(returnedStatus, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			a, err := db.CreateAccount(ctx, "legacy@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_legacy',plan_id='builder' WHERE id=?`, a.ID); err != nil {
				t.Fatal(err)
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /subscriptions":
					_, _ = fmt.Fprint(w, `{"data":[{"id":"sub_legacy","customer":"cus_legacy","status":"active","items":{"data":[{"price":{"id":"price_builder"}}]}}],"has_more":false}`)
				case "DELETE /subscriptions/sub_legacy":
					_, _ = fmt.Fprintf(w, `{"id":"sub_legacy","customer":"cus_legacy","status":%q}`, returnedStatus)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			t.Cleanup(provider.Close)
			s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}}
			if err = s.CancelAccount(ctx, a.ID); !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("legacy adapter reported successful cancellation while closure is pending: %v", err)
			}

			now := time.Now()
			if _, err = db.DB().ExecContext(ctx, `INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, "session_fixture", a.ID, store.Hash(strings.Repeat("s", 43)), store.Stamp(now), store.Stamp(now.Add(time.Hour)), "csrf_fixture"); err != nil {
				t.Fatal(err)
			}
			idService := &identity.Service{Store: db}
			d := &dashboard.Dashboard{BillingService: s, Identity: idService, Gateway: &gateway.Gateway{Issuer: &credential.Issuer{Store: db}}, Provision: &provision.Provisioner{Store: db, BrainsRoot: t.TempDir()}, Origin: "https://fixture.example", Dev: true}
			req := httptest.NewRequest(http.MethodPost, "/account/delete", strings.NewReader(url.Values{"confirm": {"DELETE"}, "csrf": {"csrf_fixture"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", d.Origin)
			req.AddCookie(&http.Cookie{Name: "serenity_session", Value: strings.Repeat("s", 43)})
			response := httptest.NewRecorder()
			d.Handler().ServeHTTP(response, req)
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "cancellation is pending") {
				t.Fatalf("delete response=%d %s", response.Code, response.Body.String())
			}
			var accountStatus string
			if err = db.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, a.ID).Scan(&accountStatus); err != nil || accountStatus != "active" {
				t.Fatalf("account status=%s err=%v", accountStatus, err)
			}
			if _, err = idService.Session(ctx, strings.Repeat("s", 43)); err != nil {
				t.Fatalf("session lost during pending cancellation: %v", err)
			}
			var plan string
			if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "builder" {
				t.Fatalf("pending closure changed plan: %s %v", plan, err)
			}
		})
	}
}
