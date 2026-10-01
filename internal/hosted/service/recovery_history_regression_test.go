package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

func TestRecoveryReplaysIntentBeforeSnapshotWatermark(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	ctx := context.Background()
	if _, err := db.DB().Exec("UPDATE accounts SET status='restore_pending' WHERE id=?", accountID); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	prior, err := deletion.NewFilesystemJournal(root, "prior", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := prior.AppendDeletion(ctx, contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: accountID, Outcome: contracts.DeletionIntentRequested})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = prior.Seal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	active, err := deletion.NewFilesystemJournal(root, "adopted-test-writer", 2, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	deps := LifecycleDependencies{Journal: active, BuildSHA: testLifecycleBuildSHA, Recovery: testRecoveryAdmission{journal: active, from: intent.Watermark}}
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Gateway.Close(); _ = svc.Pool.Close() })
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pre-cut intent left restored files: %v", err)
	}
	var status string
	if err = db.DB().QueryRow("SELECT status FROM accounts WHERE id=?", accountID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("replayed account status=%q", status)
	}
	read, err := active.ReadThrough(ctx, intent.Watermark)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range read.Entries {
		if e.SubjectType == contracts.DeletionSubjectAccount && e.SubjectID == accountID && e.Outcome == contracts.DeletionOutcomePurged {
			found = true
		}
	}
	if !found {
		t.Fatal("completed recovery lacks durable account purge outcome")
	}
}
func TestRecoveryRejectsUnsealedOrMismatchedHistory(t *testing.T) {
	for _, badHash := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsealed", true: "wrong snapshot hash"}[badHash], func(t *testing.T) {
			db, cfg, accountID, path := deletionFixture(t)
			ctx := context.Background()
			j, err := deletion.NewFilesystemJournal(t.TempDir(), "prior", 1, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := j.AppendDeletion(ctx, contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: accountID, Outcome: contracts.DeletionIntentRequested})
			if err != nil {
				t.Fatal(err)
			}
			from := entry.Watermark
			want := contracts.ErrDeletionJournalIncomplete
			if badHash {
				from.EntryHash = strings.Repeat("f", 64)
				want = contracts.ErrDeletionJournalHistoryMismatch
			}
			deps := LifecycleDependencies{Journal: j, BuildSHA: testLifecycleBuildSHA, Recovery: testRecoveryAdmission{journal: j, from: from}}
			if svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps); !errors.Is(err, want) || svc != nil {
				t.Fatalf("unsafe history admitted: service=%v err=%v", svc, err)
			}
			if _, err = os.Stat(path); err != nil {
				t.Fatalf("rejected startup altered files: %v", err)
			}
			var status string
			if err = db.DB().QueryRow("SELECT status FROM accounts WHERE id=?", accountID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "active" {
				t.Fatalf("rejected startup altered status=%q", status)
			}
		})
	}
}
