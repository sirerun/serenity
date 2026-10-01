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

func TestUnkeyedHostedRememberMaterializesRelativeTTL(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := hoststore.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := assembleForTest(t, service.Config{DataDir: dir, PublicOrigin: "http://127.0.0.1", MaxOpen: 2, MaxInFlight: 4, AccountCap: 100}, true, db, &sender{}, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := db.CreateAccount(ctx, "relative-ttl@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	result, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", json.RawMessage(`{"fact":"Relative hosted expiry","provenance":"TTL regression","ttl":"30d"}`))
	finish := time.Now()
	if err != nil || result.IsError {
		t.Fatalf("unkeyed relative TTL rejected: result=%+v err=%v", result, err)
	}
	keyed, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", json.RawMessage(`{"fact":"Explicit keys keep fixed expiry","provenance":"TTL regression","operation_key":"fixed-key","ttl":"30d"}`))
	if err != nil || !keyed.IsError {
		t.Fatalf("explicit client key accepted relative TTL: result=%+v err=%v", keyed, err)
	}
	var keyedError struct {
		Error string `json:"error"`
	}
	if len(keyed.Content) == 0 || json.Unmarshal([]byte(keyed.Content[0].Text), &keyedError) != nil || keyedError.Error != "invalid_params" {
		t.Fatalf("explicit relative-TTL error=%q, want invalid_params: %+v", keyedError.Error, keyed)
	}
	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(dir, "brains", brain.ID)))
	if err != nil {
		t.Fatal(err)
	}
	records := projection.All()
	if len(records) != 1 {
		t.Fatalf("canonical records=%d, want one", len(records))
	}
	expiry := records[0].Payload.ValidUntil
	if expiry == nil || expiry.Before(start.Add(30*24*time.Hour)) || expiry.After(finish.Add(30*24*time.Hour)) {
		t.Fatalf("stored expiry=%v, want within request interval plus 30d [%s, %s]", expiry, start.Add(30*24*time.Hour), finish.Add(30*24*time.Hour))
	}
	var operationRows, committed, pending, writes int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE account_id=?`, account.ID).Scan(&operationRows); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE account_id=? AND phase='committed'`, account.ID).Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE account_id=? AND phase='pending_review'`, account.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT COALESCE(sum(committed),0) FROM usage_windows WHERE account_id=? AND metric='writes'`, account.ID).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if operationRows != 1 || committed != 1 || pending != 0 || writes != 1 {
		t.Fatalf("operation rows=%d committed=%d pending_review=%d writes=%d, want 1/1/0/1", operationRows, committed, pending, writes)
	}
}
