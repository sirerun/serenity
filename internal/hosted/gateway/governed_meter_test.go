package gateway

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/meter"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestGovernedQuotaRecoversDurableFailureWindows(t *testing.T) {
	for _, mode := range []string{"publication", "meter_commit", "released"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			g, p, s, account, brain, root := newLifecycleExportFixture(t)
			g.Meter = &meter.Meter{Store: s}
			token, err := g.Issuer.Issue(ctx, brain, account, []string{"memory:read", "memory:write"})
			if err != nil {
				t.Fatal(err)
			}
			// Force runtime construction before fault injection.
			_, release, err := p.AcquireCanonical(ctx, brain)
			if err != nil {
				t.Fatal(err)
			}
			release()
			if mode == "publication" {
				if err = os.WriteFile(filepath.Join(root, ".git", "index.lock"), []byte("owned test lock"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				_, err = s.DB().ExecContext(ctx, "CREATE TRIGGER fail_governed_settlement BEFORE UPDATE OF status ON reservations WHEN NEW.status='committed' BEGIN SELECT RAISE(ABORT,'injected settlement crash'); END")
				if err != nil {
					t.Fatal(err)
				}
			}
			a := governedTestAction("remember", "00000000-0000-4000-8000-000000000091")
			a.WriterOwner = account
			a.Text = "durable quota failure fixture"
			req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brain, Action: a}
			code, _, _ := governedTestCall(t, g, token, req)
			if code != 503 {
				t.Fatalf("fault status=%d", code)
			}
			projection, err := brainstore.LoadGovernedMemory(brainstore.NewSourceStore(root))
			if err != nil {
				t.Fatal(err)
			}
			if len(projection.Commands) != 1 {
				t.Fatal("fault not after durable receipt")
			}
			claim := projection.Commands[a.CommandID].Response.Claims[0].ID
			var status, window string
			if err = s.DB().QueryRowContext(ctx, "SELECT status,window_key FROM reservations WHERE operation_key=?", "governed:"+brain+":"+a.CommandID).Scan(&status, &window); err != nil {
				t.Fatal(err)
			}
			if status != "open" {
				t.Fatalf("uncertain reservation released: %s", status)
			}
			if mode == "publication" {
				if err = os.Remove(filepath.Join(root, ".git", "index.lock")); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = s.DB().ExecContext(ctx, "DROP TRIGGER fail_governed_settlement"); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "released" {
				if _, err = s.DB().ExecContext(ctx, "UPDATE reservations SET status='released',created_at=?,lease_expires_at=?", time.Now().Add(-48*time.Hour).Format(time.RFC3339), time.Now().Add(-time.Hour).Format(time.RFC3339)); err != nil {
					t.Fatal(err)
				}
			}
			// Generic quota admission/GC cannot discard this original durable reservation.
			_, _ = g.Meter.Reserve(ctx, account, "recalls", 0, 1, "gc", time.Now().Add(time.Hour), "")
			r := httptest.NewRequest("GET", "/memory/v1/commands/"+a.CommandID+"?project_id="+brain+"&brain_id="+governedBrain, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			g.ServeGovernedMemory(w, r)
			var lookup brainstore.GovernedResponse
			if err = json.Unmarshal(w.Body.Bytes(), &lookup); err != nil {
				t.Fatal(err)
			}
			if lookup.Status != "unknown" || lookup.LookupAuthoritative {
				t.Fatal("unsettled receipt acknowledged")
			}
			bad := req
			bad.Action.Text = "different payload"
			if code, _, _ := governedTestCall(t, g, token, bad); code != 409 {
				t.Fatalf("wrong payload=%d", code)
			}
			for range 2 {
				code, result, body := governedTestCall(t, g, token, req)
				if code != 200 || len(result.Claims) != 1 || result.Claims[0].ID != claim {
					t.Fatalf("recovery %d %s", code, body)
				}
			}
			var used int
			if err = s.DB().QueryRowContext(ctx, "SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric='writes'", account, window).Scan(&used); err != nil {
				t.Fatal(err)
			}
			if used != 1 {
				t.Fatalf("charge=%d", used)
			}
		})
	}
}
