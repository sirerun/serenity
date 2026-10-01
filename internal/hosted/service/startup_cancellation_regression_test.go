package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type cancelAfterCompleteJournalRead struct {
	contracts.DeletionJournal
	cancel      context.CancelFunc
	sealedReads int
}

func (j *cancelAfterCompleteJournalRead) ReadThrough(ctx context.Context, from contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	read, err := j.DeletionJournal.ReadThrough(ctx, from)
	if err == nil && read.Sealed {
		j.sealedReads++
		if j.sealedReads == 2 {
			j.cancel()
		}
	}
	return read, err
}

func TestAssembleCancellationAfterJournalAdmissionDoesNotRecoverAllocatingBrain(t *testing.T) {
	db, cfg, accountID, brainPath := deletionFixture(t)
	if _, err := db.DB().Exec(`UPDATE brains SET state='allocating' WHERE account_id=?`, accountID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(brainPath); err != nil {
		t.Fatal(err)
	}
	deps := testLifecycleDependencies(t)
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &cancelAfterCompleteJournalRead{DeletionJournal: deps.Journal, cancel: cancel}
	deps.Journal = wrapped
	deps.Recovery = testRecoveryAdmission{journal: wrapped, from: contracts.DeletionWatermark{}}

	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
	if svc != nil {
		t.Fatal("canceled startup published a service")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("startup error=%v, want context.Canceled", err)
	}
	if wrapped.sealedReads != 2 {
		t.Fatalf("sealed reads=%d, want admitted-cut and complete-history reads", wrapped.sealedReads)
	}
	if _, err := os.Stat(brainPath); !os.IsNotExist(err) {
		t.Fatalf("startup recovered allocating brain despite cancellation: %v", err)
	}
	var state string
	if err := db.DB().QueryRow(`SELECT state FROM brains WHERE account_id=?`, accountID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "allocating" {
		t.Fatalf("canceled startup changed brain state to %q", state)
	}
}

func TestAssembleFailsClosedWhenDeletingAccountHasNoJournalIntent(t *testing.T) {
	db, cfg, accountID, brainPath := deletionFixture(t)
	if _, err := db.DB().Exec(`UPDATE accounts SET status='deleting' WHERE id=?`, accountID); err != nil {
		t.Fatal(err)
	}
	deps := testLifecycleDependencies(t)
	svc, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, deps)
	if svc != nil {
		t.Fatal("startup accepted SQL deletion without journal authority")
	}
	if !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
		t.Fatalf("startup error=%v, want ErrDeletionJournalIncomplete", err)
	}
	if _, err := os.Stat(brainPath); err != nil {
		t.Fatalf("fail-closed startup altered brain files: %v", err)
	}
	var status string
	if err := db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleting" {
		t.Fatalf("fail-closed startup changed account status to %q", status)
	}
}
