package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/pool"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/providers"
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

type journalTestEmbedder struct{}

func (journalTestEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0}, nil
}
func (journalTestEmbedder) ModelVersion() string { return "journal-test@v1" }

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

func TestReplayOrphanBrainRemovesCanonicalGitAndDerivedIndexBeforePurge(t *testing.T) {
	ctx := context.Background()
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root := filepath.Join(t.TempDir(), "brains")
	brainID := "0123456789abcdef0123456789abcdef"
	brainDir := filepath.Join(root, brainID)
	sourceDir := filepath.Join(brainDir, "sources", strings.Repeat("b", 64))
	if err = os.MkdirAll(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(sourceDir, "bytes"), []byte("canonical source"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, e := exec.Command("git", "-C", brainDir, "init", "-q").CombinedOutput(); e != nil {
		t.Fatalf("git init: %v: %s", e, out)
	}
	if out, e := exec.Command("git", "-C", brainDir, "config", "user.name", "Test").CombinedOutput(); e != nil {
		t.Fatalf("git config name: %v: %s", e, out)
	}
	if out, e := exec.Command("git", "-C", brainDir, "config", "user.email", "test@example.test").CombinedOutput(); e != nil {
		t.Fatalf("git config email: %v: %s", e, out)
	}
	if out, e := exec.Command("git", "-C", brainDir, "add", "sources").CombinedOutput(); e != nil {
		t.Fatalf("git add source: %v: %s", e, out)
	}
	if out, e := exec.Command("git", "-C", brainDir, "commit", "-qm", "source fixture").CombinedOutput(); e != nil {
		t.Fatalf("git commit: %v: %s", e, out)
	}
	if out, e := exec.Command("git", "-C", brainDir, "rev-list", "--all").CombinedOutput(); e != nil || len(strings.TrimSpace(string(out))) == 0 {
		t.Fatalf("source fixture has no retained Git commit: %v: %s", e, out)
	}
	idx, err := providers.OpenIndex(brainDir)
	if err != nil {
		t.Fatal(err)
	}
	chunkRef := "source:" + strings.Repeat("b", 64) + ":0"
	if err = idx.InsertChunk(ctx, chunkRef, "source/test", "unique tombstone marker", strings.Repeat("b", 64), "text"); err != nil {
		t.Fatal(err)
	}
	if err = idx.UpsertVector(ctx, chunkRef, "journal-test@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.SearchFTS(ctx, "tombstone", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("fixture FTS row missing before purge: hits=%d err=%v", len(hits), err)
	}
	hasVector, err := idx.HasVector(ctx, chunkRef, "journal-test@v1")
	if err != nil || !hasVector {
		t.Fatalf("fixture vector row missing before purge: present=%v err=%v", hasVector, err)
	}
	if err = idx.Close(); err != nil {
		t.Fatal(err)
	}
	brainPool, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: root, Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = brainPool.Close() })
	journal := &recordingDeletionJournal{}
	journal.onAppend = func(entry contracts.DeletionEntry) {
		if entry.Outcome != contracts.DeletionOutcomePurged {
			return
		}
		if _, e := os.Lstat(brainDir); !errors.Is(e, os.ErrNotExist) {
			t.Fatalf("purged outcome appended before whole brain tree disappeared: %v", e)
		}
	}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Pool: brainPool, Journal: journal}
	entry := contracts.DeletionEntry{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("c", 64)}, SubjectType: contracts.DeletionSubjectBrain, SubjectID: brainID, Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()}
	if err = g.ReplayDeletions(ctx, root, []contracts.DeletionEntry{entry}, func(context.Context, string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 || journal.entries[0].Outcome != contracts.DeletionOutcomePurged {
		t.Fatalf("outcome entries = %#v", journal.entries)
	}
	if _, err = os.Lstat(brainDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("brain tree remains after replay: %v", err)
	}
}

func TestReplayFailsClosedForUnsafeBrainExternalReference(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root := filepath.Join(t.TempDir(), "brains")
	brainID := "fedcba9876543210"
	brainDir := filepath.Join(root, brainID)
	if err = os.MkdirAll(brainDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(brainDir, ".git"), []byte("gitdir: /external/common/repository"), 0600); err != nil {
		t.Fatal(err)
	}
	brainPool, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: root, Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = brainPool.Close() })
	journal := &recordingDeletionJournal{}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Pool: brainPool, Journal: journal}
	entry := contracts.DeletionEntry{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("d", 64)}, SubjectType: contracts.DeletionSubjectBrain, SubjectID: brainID, Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()}
	if err = g.ReplayDeletions(context.Background(), root, []contracts.DeletionEntry{entry}, func(context.Context, string) (bool, error) { return true, nil }); !errors.Is(err, ErrDeletionSubjectStateUnknown) {
		t.Fatalf("ReplayDeletions with external Git reference = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatalf("unsafe tree received terminal outcome: %#v", journal.entries)
	}
	if _, err = os.Stat(filepath.Join(brainDir, ".git")); err != nil {
		t.Fatalf("unsafe tree was removed: %v", err)
	}
}

func TestValidateBrainTreeRejectsAnyExternalCoreWorktreeValue(t *testing.T) {
	root := t.TempDir()
	brain := "b123456789abcdef"
	brainDir := filepath.Join(root, brain)
	if err := os.MkdirAll(brainDir, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", brainDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	for _, worktree := range []string{"/external/brain", brainDir} {
		if out, err := exec.Command("git", "-C", brainDir, "config", "--local", "--add", "core.worktree", worktree).CombinedOutput(); err != nil {
			t.Fatalf("git config core.worktree: %v: %s", err, out)
		}
	}
	if _, _, err := validateBrainTree(context.Background(), root, brain); !errors.Is(err, ErrDeletionSubjectStateUnknown) {
		t.Fatalf("validateBrainTree with hidden external worktree = %v", err)
	}
}

func TestValidateBrainTreeRejectsMalformedGitConfig(t *testing.T) {
	root := t.TempDir()
	brain := "b123456789abcdef"
	brainDir := filepath.Join(root, brain)
	if err := os.MkdirAll(brainDir, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", brainDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	configPath := filepath.Join(brainDir, ".git", "config")
	if err := os.WriteFile(configPath, []byte("not valid config = ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateBrainTree(context.Background(), root, brain); err == nil {
		t.Fatal("validateBrainTree accepted malformed Git config")
	}
}

func TestReplayDoesNotAcknowledgeAbsentAccountWithoutClosureProof(t *testing.T) {
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	journal := &recordingDeletionJournal{}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}, Journal: journal}
	entries := []contracts.DeletionEntry{
		{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("e", 64)}, SubjectType: contracts.DeletionSubjectAccount, SubjectID: "missing-account-1234", Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()},
		{Watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 2, EntryHash: strings.Repeat("f", 64)}, SubjectType: contracts.DeletionSubjectAccount, SubjectID: "missing-account-1234", Outcome: contracts.DeletionOutcomePurged, RecordedAt: time.Now().UTC()},
	}
	err = g.ReplayDeletions(context.Background(), t.TempDir(), entries, func(context.Context, string) (bool, error) {
		t.Fatal("provider preflight called without account row")
		return false, nil
	})
	if !errors.Is(err, ErrDeletionSubjectStateUnknown) {
		t.Fatalf("ReplayDeletions with missing account = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatalf("missing account received fabricated outcome: %#v", journal.entries)
	}
}

func TestBootstrapCannotOpenBrainWhileAccountDeletionOwnsFence(t *testing.T) {
	ctx := context.Background()
	db, err := hoststore.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(ctx, "bootstrap-race@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brainID := "abcdef0123456789abcdef0123456789"
	if _, err = db.DB().Exec(`INSERT INTO brains(id,account_id,path_key,state,created_at) VALUES(?,?,?,'ready',?)`, brainID, account.ID, brainID, hoststore.Stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "brains")
	brainDir := filepath.Join(root, brainID)
	if err = os.MkdirAll(brainDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(brainDir, "retained.txt"), []byte("private brain"), 0600); err != nil {
		t.Fatal(err)
	}
	brainPool, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: root, Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = brainPool.Close() })
	binding := credential.Binding{AccountID: account.ID, BrainID: brainID, CredentialID: "oauth:test", Generation: 1}
	var verifies atomic.Int32
	initialVerified := make(chan struct{})
	issuer := &credential.Issuer{Store: db, OAuthVerify: func(context.Context, string) (credential.Binding, error) {
		if verifies.Add(1) == 1 {
			close(initialVerified)
			return binding, nil
		}
		return credential.Binding{}, credential.ErrInvalidCredential
	}}
	journal := &recordingDeletionJournal{}
	g := &Gateway{Issuer: issuer, Pool: brainPool, Journal: journal}
	preflightEntered := make(chan struct{})
	releasePreflight := make(chan struct{})
	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- g.DeleteAccountWithPreflight(ctx, account.ID, root, func(context.Context, string) (bool, error) {
			close(preflightEntered)
			<-releasePreflight
			return true, nil
		})
	}()
	select {
	case <-preflightEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("account deletion did not reach preflight")
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	request.Header.Set("Authorization", "Bearer stale-oauth")
	response := httptest.NewRecorder()
	httpDone := make(chan struct{})
	go func() {
		g.ServeHTTP(response, request)
		close(httpDone)
	}()
	select {
	case <-initialVerified:
	case <-time.After(5 * time.Second):
		close(releasePreflight)
		t.Fatal("bootstrap request did not pass its initial credential check")
	}
	configPath := filepath.Join(brainDir, config.FileName)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(configPath); statErr == nil {
			close(releasePreflight)
			t.Fatal("bootstrap created runtime configuration while deletion held the account fence")
		}
		runtime.Gosched()
	}
	close(releasePreflight)
	select {
	case err = <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("account deletion did not finish")
	}
	select {
	case <-httpDone:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap request did not finish after deletion")
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("bootstrap response status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if verifies.Load() != 2 {
		t.Fatalf("credential verified %d times, want initial plus locked revalidation", verifies.Load())
	}
	if _, err = os.Lstat(brainDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted brain directory was reopened or retained: %v", err)
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
