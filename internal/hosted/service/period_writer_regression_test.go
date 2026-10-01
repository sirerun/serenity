package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/service"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestHostedWriterRetryKeyIsScopedToQuotaPeriod(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := hoststore.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.Assemble(service.Config{DataDir: dir, PublicOrigin: "http://127.0.0.1", MaxOpen: 2, MaxInFlight: 4, AccountCap: 100}, true, db, &sender{}, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := db.CreateAccount(ctx, "period-writer@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	call := func(month time.Month, fact string) string {
		t.Helper()
		svc.Gateway.Meter.Clock = func() time.Time { return time.Date(2026, month, 15, 12, 0, 0, 0, time.UTC) }
		args, err := json.Marshal(map[string]string{"fact": fact, "provenance": "period regression", "operation_key": "monthly-key"})
		if err != nil {
			t.Fatal(err)
		}
		result, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", args)
		if err != nil || result.IsError {
			t.Fatalf("remember in %s failed: result=%+v err=%v", month, result, err)
		}
		var body struct {
			ID string `json:"id"`
		}
		if len(result.Content) == 0 || json.Unmarshal([]byte(result.Content[0].Text), &body) != nil || body.ID == "" {
			t.Fatalf("missing fact identity: %+v", result)
		}
		return body.ID
	}
	first := call(time.September, "September decision")
	second := call(time.October, "October decision")
	if first == second {
		t.Fatal("new period replayed prior fact")
	}
	if retry := call(time.October, "October decision"); retry != second {
		t.Fatalf("same-period retry ID=%q want %q", retry, second)
	}
	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(dir, "brains", brain.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if records := projection.All(); len(records) != 2 {
		t.Fatalf("canonical record count=%d want2", len(records))
	}
	for _, record := range projection.All() {
		var clientKey, phase, period string
		if err := db.DB().QueryRowContext(ctx, `SELECT client_key,phase,quota_period FROM operations WHERE id=?`, record.Payload.CanonicalOperationID).Scan(&clientKey, &phase, &period); err != nil {
			t.Fatal(err)
		}
		if record.Payload.OperationKey != record.Payload.CanonicalOperationID || clientKey != "monthly-key" || phase != "committed" {
			t.Fatalf("canonical and client key scopes differ: record=%+v client=%q phase=%q period=%q", record.Payload, clientKey, phase, period)
		}
	}
	for _, period := range []string{"2026-09", "2026-10"} {
		var writes int
		if err := db.DB().QueryRowContext(ctx, `SELECT committed FROM usage_windows WHERE account_id=? AND metric='writes' AND window_key=?`, account.ID, period).Scan(&writes); err != nil {
			t.Fatal(err)
		}
		if writes != 1 {
			t.Fatalf("period %s charged%d writes want1", period, writes)
		}
	}
}
