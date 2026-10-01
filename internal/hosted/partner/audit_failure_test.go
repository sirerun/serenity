package partner

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestAPIReportsAuditFailureWithoutChangingSuccessfulResponse(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const partnerID = "audit-fixture-partner"
	const partnerSecret = "audit-fixture-secret-01234567890123456789"
	if err := Seed(ctx, db, partnerID, "Audit Fixture", partnerSecret, "blink://linked", "active", time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DROP TABLE audit_log`); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	service := &Service{Store: db, Clock: func() time.Time { return time.Unix(1_700_000_001, 0) }}
	mutationRan := false
	handler := service.api("fixture.mutation", func(w http.ResponseWriter, _ *http.Request, c *call) {
		mutationRan = true
		c.account = "audit-fixture-account-private"
		c.detail["result"] = "committed"
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"result":"committed"}`))
	})
	request := httptest.NewRequest(http.MethodPost, "/partner/v1/fixture", nil)
	request.Header.Set("Authorization", "Partner "+partnerID+":"+partnerSecret)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !mutationRan {
		t.Fatal("mutation handler did not run")
	}
	if response.Code != http.StatusCreated || response.Body.String() != `{"result":"committed"}` {
		t.Fatalf("response = %d %q, want unchanged successful mutation response", response.Code, response.Body.String())
	}

	logText := logs.String()
	if !strings.Contains(logText, "partner audit persistence failed") {
		t.Fatalf("audit failure diagnostic missing from log: %q", logText)
	}
	for _, secret := range []string{partnerID, partnerSecret, "audit-fixture-account-private", "no such table", "audit_log"} {
		if strings.Contains(logText, secret) {
			t.Fatalf("log leaked sensitive or database detail %q: %q", secret, logText)
		}
	}
}
