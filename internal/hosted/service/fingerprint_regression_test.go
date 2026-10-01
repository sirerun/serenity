package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/service"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestCommittedRememberReplayBindsNormalizedPayload(t *testing.T) {
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
	account, err := db.CreateAccount(ctx, "fingerprint-regression@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	call := func(t testing.TB, args string) (string, bool) {
		t.Helper()
		result, callErr := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", json.RawMessage(args))
		if callErr != nil {
			return callErr.Error(), true
		}
		if len(result.Content) == 0 {
			t.Fatalf("remember returned no content: %+v", result)
		}
		return result.Content[0].Text, result.IsError
	}

	first, failed := call(t, `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case"}`)
	if failed {
		t.Fatalf("initial remember failed: %s", first)
	}
	// Defaults and explicit defaults are the same normalized request.
	defaultReplay, failed := call(t, `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","kind":"fact","visibility":"world","canonical_operation_id":"client-controlled"}`)
	if failed {
		t.Fatalf("explicit-default replay failed: %s", defaultReplay)
	}
	var original, replay struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(first), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(defaultReplay), &replay); err != nil {
		t.Fatal(err)
	}
	if original.ID == "" || replay.ID != original.ID {
		t.Fatalf("default replay IDs differ: first=%q replay=%q", original.ID, replay.ID)
	}

	for _, tc := range []struct {
		name string
		args string
	}{
		{"private visibility", `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","visibility":"private"}`},
		{"provenance", `{"fact":"A fact with stable identity","provenance":"second hand","operation_key":"fingerprint-case"}`},
		{"entity", `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","entity":"people/alex"}`},
		{"kind", `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","kind":"event"}`},
		{"ttl", `{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","ttl":"2030-01-01T00:00:00Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, failed := call(t, tc.args)
			if !failed {
				t.Fatalf("changed normalized payload replayed committed result: %s", body)
			}
			var domain struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(body), &domain); err != nil || domain.Error != "operation_conflict" {
				t.Fatalf("changed payload error=%q err=%v, want operation_conflict (body %s)", domain.Error, err, body)
			}
		})
	}

	// Validation must also run before a committed replay can bypass the handler.
	for _, args := range []string{
		`{"fact":"A fact with stable identity","provenance":"","operation_key":"fingerprint-case"}`,
		`{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"fingerprint-case","ttl":"30d"}`,
		`{"fact":"A fact with stable identity","provenance":"first hand","operation_key":"` + strings.Repeat("x", 129) + `"}`,
	} {
		if body, failed := call(t, args); !failed {
			t.Fatalf("invalid changed request replayed committed result: %s", body)
		}
	}

	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(dir, "brains", brain.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if records := projection.All(); len(records) != 1 {
		t.Fatalf("source count=%d, want exactly one", len(records))
	}
	var committed, active int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE account_id=? AND phase='committed'`, account.ID).Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT COALESCE(sum(committed),0) FROM usage_windows WHERE account_id=? AND metric='writes'`, account.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if committed != 1 || active != 1 {
		t.Fatalf("committed operations=%d writes=%d, want one each", committed, active)
	}
}
