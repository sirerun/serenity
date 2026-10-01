package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestInvalidRememberReleasesOperationBeforeCanonicalEntry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "control.db"))
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
	account, err := db.CreateAccount(ctx, "canonical-validation@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"fact":"A validated canonical marker","provenance":"regression","operation_key":"validation-retry","ttl":"invalid-duration"}`)
	result, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", args)
	if err != nil || !result.IsError {
		t.Fatalf("invalid TTL accepted: %+v %v", result, err)
	}
	var phase, entered string
	if err := db.DB().QueryRowContext(ctx, `SELECT phase,COALESCE(canonical_entered_at,'') FROM operations WHERE account_id=?`, account.ID).Scan(&phase, &entered); err != nil {
		t.Fatal(err)
	}
	if phase != "released" || entered != "" {
		t.Fatalf("invalid request held quota/canonical entry: phase=%s entered=%s", phase, entered)
	}
	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(dir, "brains", brain.ID)))
	if err != nil || len(projection.All()) != 0 {
		t.Fatalf("invalid request wrote canonical source: %v", err)
	}
	// Correcting the same key must succeed, not remain stuck pending review.
	args = json.RawMessage(`{"fact":"A validated canonical marker","provenance":"regression","operation_key":"validation-retry"}`)
	result, err = svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", args)
	if err != nil || result.IsError {
		t.Fatalf("corrected retry failed: %+v %v", result, err)
	}
	var operationID string
	if err := db.DB().QueryRowContext(ctx, `SELECT id FROM operations WHERE account_id=? AND phase='committed'`, account.ID).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	projection, err = brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(dir, "brains", brain.ID)))
	if err != nil {
		t.Fatal(err)
	}
	records := projection.All()
	if len(records) != 1 || records[0].Payload.CanonicalOperationID != operationID {
		t.Fatalf("canonical source not bound to committed ledger ID: %+v", records)
	}
	var writes int
	if err := db.DB().QueryRowContext(ctx, `SELECT COALESCE(sum(committed),0) FROM usage_windows WHERE account_id=? AND metric='writes'`, account.ID).Scan(&writes); err != nil || writes != 1 {
		t.Fatalf("writes=%d err=%v, want one", writes, err)
	}
}
