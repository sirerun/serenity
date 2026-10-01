package gateway

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
)

type recordingDeletionJournal struct {
	entries   []contracts.DeletionEntry
	appendErr error
	onAppend  func(contracts.DeletionEntry)
}

func (j *recordingDeletionJournal) AppendDeletion(_ context.Context, entry contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	if j.appendErr != nil {
		return contracts.DeletionEntry{}, j.appendErr
	}
	entry.Watermark = contracts.DeletionWatermark{Generation: 1, SequenceID: int64(len(j.entries) + 1), EntryHash: strings.Repeat("a", 64)}
	j.entries = append(j.entries, entry)
	if j.onAppend != nil {
		j.onAppend(entry)
	}
	return entry, nil
}

func (j *recordingDeletionJournal) ReadThrough(context.Context, contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	return contracts.DeletionRead{}, errors.New("unexpected ReadThrough")
}

func (j *recordingDeletionJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return contracts.DeletionWatermark{}, errors.New("unexpected Seal")
}

type nilDeletionJournal struct{}

func (*nilDeletionJournal) AppendDeletion(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, nil
}
func (*nilDeletionJournal) ReadThrough(context.Context, contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	return contracts.DeletionRead{}, nil
}
func (*nilDeletionJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return contracts.DeletionWatermark{}, nil
}

func TestDestructiveLifecycleRequiresJournalBeforeCallbacksOrStoreAccess(t *testing.T) {
	called := false
	g := &Gateway{}
	if err := g.DeleteBrain(context.Background(), "acct", "brain", t.TempDir()); !errors.Is(err, ErrDeletionJournalUnavailable) {
		t.Fatalf("DeleteBrain without journal = %v", err)
	}
	if err := g.DeleteAccountWithPreflight(context.Background(), "acct", t.TempDir(), func(context.Context, string) (bool, error) {
		called = true
		return true, nil
	}); !errors.Is(err, ErrDeletionJournalUnavailable) {
		t.Fatalf("DeleteAccount without journal = %v", err)
	}
	if called {
		t.Fatal("preflight called without a journal")
	}
	if err := g.RecoverDeletions(context.Background(), t.TempDir()); !errors.Is(err, ErrDeletionRecoveryRequiresVerifiedHistory) {
		t.Fatalf("legacy RecoverDeletions = %v", err)
	}
	var typedNil *nilDeletionJournal
	g.Journal = typedNil
	if err := g.DeleteBrain(context.Background(), "acct", "brain", t.TempDir()); !errors.Is(err, ErrDeletionJournalUnavailable) {
		t.Fatalf("DeleteBrain with typed-nil journal = %v", err)
	}
}

func TestAccountDeletionJournalsIntentBeforePreflightAndOutcomeAfterSQL(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(context.Background(), "journal@example.test")
	if err != nil {
		t.Fatal(err)
	}
	journal := &recordingDeletionJournal{}
	journal.onAppend = func(entry contracts.DeletionEntry) {
		if entry.Outcome != contracts.DeletionOutcomePurged {
			return
		}
		var status string
		if e := db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, account.ID).Scan(&status); e != nil {
			t.Fatal(e)
		}
		if status != "deleted" {
			t.Fatalf("account outcome appended before SQL cleanup: %q", status)
		}
	}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Journal: journal}
	preflightCalled := false
	err = g.DeleteAccountWithPreflight(context.Background(), account.ID, t.TempDir(), func(_ context.Context, id string) (bool, error) {
		preflightCalled = true
		if id != account.ID {
			t.Fatalf("preflight account = %q", id)
		}
		if len(journal.entries) != 1 || journal.entries[0].Outcome != contracts.DeletionIntentRequested {
			t.Fatalf("journal at preflight = %#v", journal.entries)
		}
		var status string
		if e := db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, id).Scan(&status); e != nil {
			t.Fatal(e)
		}
		if status != "active" {
			t.Fatalf("account changed before preflight: %q", status)
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preflightCalled {
		t.Fatal("preflight was not called")
	}
	if len(journal.entries) != 2 || journal.entries[0].Outcome != contracts.DeletionIntentRequested || journal.entries[1].Outcome != contracts.DeletionOutcomePurged {
		t.Fatalf("journal entries = %#v", journal.entries)
	}
	var status string
	if err = db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("account status = %q", status)
	}
}

func TestDeclinedPreflightLeavesDurableRequestWithoutPurge(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(context.Background(), "journal-declined@example.test")
	if err != nil {
		t.Fatal(err)
	}
	journal := &recordingDeletionJournal{}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Journal: journal}
	if err = g.DeleteAccountWithPreflight(context.Background(), account.ID, t.TempDir(), func(context.Context, string) (bool, error) {
		return false, nil
	}); !errors.Is(err, ErrDeletionPreflightDeclined) {
		t.Fatalf("DeleteAccountWithPreflight = %v", err)
	}
	if len(journal.entries) != 1 || journal.entries[0].Outcome != contracts.DeletionIntentRequested {
		t.Fatalf("journal entries after declined preflight = %#v", journal.entries)
	}
	var status string
	if err = db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("account status after declined preflight = %q", status)
	}
}

func TestReplayDoesNotIgnoreSubjectWithTerminalEvent(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(context.Background(), "journal-replay@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`UPDATE accounts SET status='deleted' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	journal := &recordingDeletionJournal{}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Journal: journal}
	entries := []contracts.DeletionEntry{
		{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("1", 64)}, SubjectType: contracts.DeletionSubjectAccount, SubjectID: account.ID, Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()},
		{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 2, EntryHash: strings.Repeat("2", 64)}, SubjectType: contracts.DeletionSubjectAccount, SubjectID: account.ID, Outcome: contracts.DeletionOutcomePurged, RecordedAt: time.Now().UTC()},
	}
	preflightCalled := false
	err = g.ReplayDeletions(context.Background(), t.TempDir(), entries, func(context.Context, string) (bool, error) {
		preflightCalled = true
		return false, nil // The local row is already deleted and closure is complete.
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preflightCalled {
		t.Fatal("terminal purged event suppressed replay of its subject")
	}
	if len(journal.entries) != 0 {
		t.Fatalf("replay appended duplicate outcome: %#v", journal.entries)
	}
}

func TestAccountDeletionAppendFailurePreventsMutation(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(context.Background(), "journal-fail@example.test")
	if err != nil {
		t.Fatal(err)
	}
	writeErr := errors.New("journal unavailable")
	called := false
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Journal: &recordingDeletionJournal{appendErr: writeErr}}
	if err = g.DeleteAccountWithPreflight(context.Background(), account.ID, t.TempDir(), func(context.Context, string) (bool, error) {
		called = true
		return true, nil
	}); !errors.Is(err, writeErr) {
		t.Fatalf("DeleteAccountWithPreflight = %v", err)
	}
	if called {
		t.Fatal("preflight called after request append failed")
	}
	var status string
	if err = db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("account status after append failure = %q", status)
	}
}

func TestReplayRejectsUnassignedWatermarkBeforeAnyMutation(t *testing.T) {
	journal := &recordingDeletionJournal{}
	g := &Gateway{Journal: journal}
	entry := contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "acct", Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()}
	if err := g.ReplayDeletions(context.Background(), t.TempDir(), []contracts.DeletionEntry{entry}, func(context.Context, string) (bool, error) {
		t.Fatal("preflight called with unassigned watermark")
		return false, nil
	}); err == nil {
		t.Fatal("ReplayDeletions accepted an unassigned watermark")
	}
	if len(journal.entries) != 0 {
		t.Fatalf("journal mutated during rejected replay: %#v", journal.entries)
	}
}
