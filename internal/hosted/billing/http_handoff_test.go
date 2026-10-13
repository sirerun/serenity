package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// Billing starts from a dashboard form with form-action 'self'. A redirect
// response keeps Chrome's form navigation restrictions attached to Stripe.
func TestBillingHandoffCompletesLocalFormBeforeExternalNavigation(t *testing.T) {
	for _, endpoint := range []string{"checkout", "portal"} {
		t.Run(endpoint, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			account, err := db.CreateAccount(ctx, "handoff@example.test")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_handoff' WHERE id=?`, account.ID); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			const token = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
			if _, err = db.DB().ExecContext(ctx, `INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, store.ID(), account.ID, store.Hash(token), store.Stamp(now), store.Stamp(now.Add(time.Hour)), "handoff-csrf"); err != nil {
				t.Fatal(err)
			}
			target := "https://checkout.stripe.com/c/pay/cs_handoff"
			if endpoint == "portal" {
				target = "https://billing.stripe.com/p/session/fixture"
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /subscriptions":
					_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
				case "POST /checkout/sessions":
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("customer") != "cus_handoff" || r.Form.Get("line_items[0][price]") != "price_builder" {
						t.Error("checkout authority not bound to server-selected account/price")
					}
					_, _ = w.Write([]byte(`{"id":"cs_handoff","url":"` + target + `","status":"open"}`))
				case "POST /billing_portal/sessions":
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("customer") != "cus_handoff" {
						t.Error("portal customer mismatch")
					}
					_, _ = w.Write([]byte(`{"url":"` + target + `"}`))
				default:
					t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			t.Cleanup(provider.Close)
			svc := &Service{Store: db, Identity: &identity.Service{Store: db}, Config: Config{Origin: "https://serenity.example", BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
			form := url.Values{"csrf": {"handoff-csrf"}, "plan": {"builder"}, "return_url": {"https://attacker.invalid"}}
			r := httptest.NewRequest(http.MethodPost, "https://serenity.example/billing/"+endpoint, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Origin", svc.Config.Origin)
			r.AddCookie(&http.Cookie{Name: "serenity_session", Value: token})
			w := httptest.NewRecorder()
			svc.ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Header().Get("Location") != "" {
				t.Fatalf("form must finish locally, got status %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), target) || strings.Contains(w.Body.String(), "attacker.invalid") {
				t.Fatal("handoff did not preserve only the trusted provider target")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("handoff may cache or disclose sensitive URL")
			}
		})
	}
}
