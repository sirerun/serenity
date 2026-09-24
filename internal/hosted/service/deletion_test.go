package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type appendFailureJournal struct {
	inner    contracts.DeletionJournal
	fail     bool
	failCall int
	calls    int
	reads    int
}

func (j *appendFailureJournal) AppendDeletion(ctx context.Context, entry contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	j.calls++
	if j.fail || j.calls == j.failCall {
		return contracts.DeletionEntry{}, errors.New("simulated journal outage")
	}
	return j.inner.AppendDeletion(ctx, entry)
}

func (j *appendFailureJournal) ReadThrough(ctx context.Context, from contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	j.reads++
	return j.inner.ReadThrough(ctx, from)
}

func (j *appendFailureJournal) Seal(ctx context.Context, generation int64) (contracts.DeletionWatermark, error) {
	return j.inner.Seal(ctx, generation)
}

func TestDeletionJournalFailureLeavesAccountRestrictedAndRecoveryCompletes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	brainsRoot := filepath.Join(dir, "brains")
	if err := os.MkdirAll(brainsRoot, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.Assemble(service.Config{
		DataDir: dir, MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1", AccountCap: 10,
	}, true, db, &sender{}, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()

	account, err := db.CreateAccount(ctx, "pending-delete@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brainID := store.ID()
	if _, err = db.InsertBrain(ctx, account.ID, brainID, brainID, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	brainDir := filepath.Join(brainsRoot, brainID)
	if err = os.MkdirAll(brainDir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(brainDir, "canonical-memory")
	if err = os.WriteFile(marker, []byte("must remain until journal accepts intent"), 0600); err != nil {
		t.Fatal(err)
	}

	failing := &appendFailureJournal{inner: svc.Journal, fail: true}
	svc.Gateway.Journal = failing
	if err = svc.Gateway.DeleteAccount(ctx, account.ID, brainsRoot); err == nil {
		t.Fatal("deletion succeeded while the intent journal was unavailable")
	}
	var status, brainState string
	if err = db.DB().QueryRowContext(ctx, "SELECT status FROM accounts WHERE id=?", account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRowContext(ctx, "SELECT state FROM brains WHERE id=?", brainID).Scan(&brainState); err != nil {
		t.Fatal(err)
	}
	if status != "deleting" || brainState != "ready" {
		t.Fatalf("failed intent left account=%q brain=%q; want deleting/ready", status, brainState)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatalf("brain purged before intent was durable: %v", err)
	}
	if _, err = svc.Journal.ReadThrough(ctx, contracts.DeletionWatermark{}); err != nil {
		t.Fatalf("fake journal should still be readable: %v", err)
	}

	failing.fail = false
	if err = svc.Gateway.RecoverDeletions(ctx, brainsRoot); err != nil {
		t.Fatalf("retry recovery: %v", err)
	}
	if err = db.DB().QueryRowContext(ctx, "SELECT status FROM accounts WHERE id=?", account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err = db.DB().QueryRowContext(ctx, "SELECT state FROM brains WHERE id=?", brainID).Scan(&brainState); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || brainState != "deleted" {
		t.Fatalf("recovery left account=%q brain=%q; want deleted/deleted", status, brainState)
	}
	if _, err = os.Stat(brainDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery retained brain data: %v", err)
	}
	read, err := svc.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[contracts.DeletionOutcome]bool{}
	for _, entry := range read.Entries {
		if seen[entry.SubjectID] == nil {
			seen[entry.SubjectID] = map[contracts.DeletionOutcome]bool{}
		}
		seen[entry.SubjectID][entry.Outcome] = true
	}
	for _, id := range []string{account.ID, brainID} {
		if !seen[id][contracts.DeletionIntentRequested] || !seen[id][contracts.DeletionOutcomePurged] {
			t.Fatalf("journal lacks complete lifecycle for %s: %v", id, seen[id])
		}
	}
	if len(read.Entries) != 4 {
		t.Fatalf("retry duplicated deletion journal entries: got %d, want 4", len(read.Entries))
	}
}

func TestRecoveryRepairsMissingAccountOutcomeAfterFinalAppendFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	brainsRoot := filepath.Join(dir, "brains")
	if err := os.MkdirAll(brainsRoot, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.Assemble(service.Config{
		DataDir: dir, MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1", AccountCap: 10,
	}, true, db, &sender{}, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()

	account, err := db.CreateAccount(ctx, "final-outcome@example.test")
	if err != nil {
		t.Fatal(err)
	}
	failing := &appendFailureJournal{inner: svc.Journal, failCall: 2}
	svc.Gateway.Journal = failing
	if err = svc.Gateway.DeleteAccount(ctx, account.ID, brainsRoot); err == nil {
		t.Fatal("deletion succeeded despite failing to append its final outcome")
	}
	var status string
	if err = db.DB().QueryRowContext(ctx, "SELECT status FROM accounts WHERE id=?", account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("account status=%q after final append failure, want deleted", status)
	}
	read, err := svc.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Entries) != 1 || read.Entries[0].Outcome != contracts.DeletionIntentRequested {
		t.Fatalf("journal before recovery = %#v, want one durable intent", read.Entries)
	}
	for i := 0; i < 4; i++ {
		other, createErr := db.CreateAccount(ctx, fmt.Sprintf("recovery-%d@example.test", i))
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, err = db.DB().ExecContext(ctx, "UPDATE accounts SET status='deleted' WHERE id=?", other.ID); err != nil {
			t.Fatal(err)
		}
	}

	failing.failCall = 0
	failing.reads = 0
	if err = svc.Gateway.RecoverDeletions(ctx, brainsRoot); err != nil {
		t.Fatalf("recover final outcome: %v", err)
	}
	if failing.reads != 1 {
		t.Fatalf("recovery verified the journal %d times for 5 accounts, want one full read", failing.reads)
	}
	read, err = svc.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Entries) != 10 || read.Entries[1].Outcome != contracts.DeletionOutcomePurged {
		t.Fatalf("journal after recovery has %d entries, want five intent/outcome pairs", len(read.Entries))
	}
	failing.reads = 0
	if err = svc.Gateway.RecoverDeletions(ctx, brainsRoot); err != nil {
		t.Fatalf("idempotent recovery: %v", err)
	}
	if failing.reads != 1 {
		t.Fatalf("idempotent recovery verified the journal %d times, want one full read", failing.reads)
	}
	read, err = svc.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Entries) != 10 {
		t.Fatalf("idempotent recovery duplicated entries: got %d, want 10", len(read.Entries))
	}
}
