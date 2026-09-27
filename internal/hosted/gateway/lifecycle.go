package gateway

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/hosted/testhooks"
	brainstore "github.com/sirerun/serenity/internal/store"
)

// Export requires an account-owned brain and bypasses ordinary usage allowances.
func (g *Gateway) Export(ctx context.Context, account, brain string, out io.Writer) (err error) {
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	if _, err = g.Issuer.Store.BrainByID(ctx, account, brain); err != nil {
		return err
	}
	runtime, release, err := g.Pool.Acquire(ctx, brain)
	if err != nil {
		return err
	}
	defer release()
	runtime.Mutations.Lock()
	defer runtime.Mutations.Unlock()
	temp, err := os.MkdirTemp("", "serenity-export-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(temp)) }()
	if err = runtime.Flush(); err != nil {
		return err
	}
	bundle := filepath.Join(temp, "brain.bundle")
	if output, e := exec.CommandContext(ctx, "git", "-C", runtime.Root, "bundle", "create", bundle, "--all").CombinedOutput(); e != nil {
		return fmt.Errorf("bundle brain: %w: %s", e, output)
	}
	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(runtime.Root))
	if err != nil {
		return err
	}
	facts := []brainstore.MemoryFactPayload{}
	for _, r := range projection.All() {
		if !r.Expired(time.Now()) {
			facts = append(facts, r.Payload)
		}
	}
	archive := zip.NewWriter(out)
	defer func() { err = errors.Join(err, archive.Close()) }()
	entry, err := archive.Create("facts.json")
	if err != nil {
		return err
	}
	if err = json.NewEncoder(entry).Encode(facts); err != nil {
		return err
	}
	entry, err = archive.Create("brain.bundle")
	if err != nil {
		return err
	}
	file, err := os.Open(bundle)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	_, err = io.Copy(entry, file)
	return err
}

var errDeletionJournalUnavailable = errors.New("hosted/gateway: independent deletion journal is required")

func (g *Gateway) DeleteBrain(ctx context.Context, account, brain string, root string) error {
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	return g.deleteBrainLocked(ctx, account, brain, root)
}

func (g *Gateway) deleteBrainLocked(ctx context.Context, account, brain string, root string) error {
	return g.deleteBrainWithJournalState(ctx, account, brain, root, nil)
}

func (g *Gateway) deleteBrainWithJournalState(ctx context.Context, account, brain string, root string, state *deletionEntrySet) error {
	if g.Journal == nil {
		return errDeletionJournalUnavailable
	}
	var owned hoststore.Brain
	err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE id=? AND account_id=?`, brain, account).Scan(&owned.ID, &owned.AccountID, &owned.State, &owned.PathKey)
	if err != nil {
		return err
	}
	if owned.PathKey != owned.ID || filepath.Base(owned.PathKey) != owned.PathKey {
		return errors.New("invalid stored brain path")
	}
	err = g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		now := hoststore.Stamp(time.Now())
		if _, e := tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=? WHERE account_id=? AND brain_id=?`, now, account, brain); e != nil {
			return e
		}
		if e := hoststore.RevokeOAuth(ctx, tx, account, brain); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE brains SET state='deleted',deleted_at=COALESCE(deleted_at,?) WHERE id=? AND account_id=?`, now, brain, account)
		return e
	})
	if err != nil {
		return err
	}
	if err = g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectBrain, brain, contracts.DeletionIntentRequested, state); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseDeletionJournaled)
	if err = g.Pool.Drop(brain); err != nil {
		return err
	}
	if err = os.RemoveAll(filepath.Join(root, owned.PathKey)); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseDeletionPurged)
	return g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectBrain, brain, contracts.DeletionOutcomePurged, state)
}

func (g *Gateway) DeleteAccount(ctx context.Context, account, root string) error {
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	if g.Journal == nil {
		return errDeletionJournalUnavailable
	}
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	return g.deleteAccountLocked(ctx, account, root, nil)
}

func (g *Gateway) deleteAccountLocked(ctx context.Context, account, root string, state *deletionEntrySet) error {
	var status, email string
	if err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT status,email FROM accounts WHERE id=?`, account).Scan(&status, &email); err != nil {
		return err
	}
	if status == "deleted" {
		if err := g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionIntentRequested, state); err != nil {
			return err
		}
		return g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged, state)
	}
	if err := g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status IN ('active','deleting')`, account)
		return e
	}); err != nil {
		return err
	}
	if err := g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionIntentRequested, state); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseDeletionJournaled)
	if g.BillingRequired && g.BillingCloser == nil {
		return errors.New("hosted/gateway: billing closure is required but not configured")
	}
	if g.BillingCloser != nil {
		result, err := g.BillingCloser.CloseBillingAccount(ctx, account)
		if err != nil {
			return err
		}
		if result.Status != contracts.CloseStatusClosed {
			return fmt.Errorf("hosted/gateway: billing closure remains pending: %s", result.PendingReason)
		}
	}
	rows, err := g.Issuer.Store.DB().QueryContext(ctx, `SELECT id FROM brains WHERE account_id=?`, account)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, id := range ids {
		if err = g.deleteBrainWithJournalState(ctx, account, id, root, state); err != nil {
			return err
		}
	}
	if err = g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `DELETE FROM login_tokens WHERE email=?`, email); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM sessions WHERE account_id=?`, account); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleted',email='',email_hash=?,plan_id='free' WHERE id=?`, "deleted:"+account, account)
		return e
	}); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseDeletionPurged)
	return g.ensureDeletionEntryWithState(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged, state)
}

func (g *Gateway) ensureDeletionEntry(ctx context.Context, subjectType contracts.DeletionSubjectType, subjectID string, outcome contracts.DeletionOutcome) error {
	if g.Journal == nil {
		return errDeletionJournalUnavailable
	}
	read, err := g.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		return fmt.Errorf("hosted/gateway: verify deletion journal before append: %w", err)
	}
	state := newDeletionEntrySet(read.Entries)
	return g.ensureDeletionEntryWithState(ctx, subjectType, subjectID, outcome, state)
}

func (g *Gateway) ensureDeletionEntryWithState(ctx context.Context, subjectType contracts.DeletionSubjectType, subjectID string, outcome contracts.DeletionOutcome, state *deletionEntrySet) error {
	if g.Journal == nil {
		return errDeletionJournalUnavailable
	}
	if state == nil {
		return g.ensureDeletionEntry(ctx, subjectType, subjectID, outcome)
	}
	if state.contains(subjectType, subjectID, outcome) {
		return nil
	}
	if _, err := g.Journal.AppendDeletion(ctx, contracts.DeletionEntry{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Outcome:     outcome,
	}); err != nil {
		return fmt.Errorf("hosted/gateway: append deletion journal entry: %w", err)
	}
	state.add(subjectType, subjectID, outcome)
	return nil
}

type deletionEntryKey struct {
	subjectType contracts.DeletionSubjectType
	subjectID   string
	outcome     contracts.DeletionOutcome
}

type deletionEntrySet map[deletionEntryKey]struct{}

func newDeletionEntrySet(entries []contracts.DeletionEntry) *deletionEntrySet {
	set := make(deletionEntrySet, len(entries))
	for _, entry := range entries {
		set.add(entry.SubjectType, entry.SubjectID, entry.Outcome)
	}
	return &set
}

func (s *deletionEntrySet) add(subjectType contracts.DeletionSubjectType, subjectID string, outcome contracts.DeletionOutcome) {
	(*s)[deletionEntryKey{subjectType: subjectType, subjectID: subjectID, outcome: outcome}] = struct{}{}
}

func (s *deletionEntrySet) contains(subjectType contracts.DeletionSubjectType, subjectID string, outcome contracts.DeletionOutcome) bool {
	if _, ok := (*s)[deletionEntryKey{subjectType: subjectType, subjectID: subjectID, outcome: outcome}]; ok {
		return true
	}
	if outcome == contracts.DeletionIntentRequested {
		_, ok := (*s)[deletionEntryKey{subjectType: subjectType, subjectID: subjectID, outcome: contracts.DeletionOutcomePurged}]
		return ok
	}
	return false
}

func (g *Gateway) RecoverDeletions(ctx context.Context, root string) error {
	if g.Journal == nil {
		return errDeletionJournalUnavailable
	}
	journal, err := g.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		return fmt.Errorf("hosted/gateway: verify deletion journal before recovery: %w", err)
	}
	state := newDeletionEntrySet(journal.Entries)
	rows, err := g.Issuer.Store.DB().QueryContext(ctx, `SELECT id FROM accounts WHERE status IN ('deleting','deleted')`)
	if err != nil {
		return err
	}
	var accounts []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		accounts = append(accounts, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, id := range accounts {
		hash := sha256.Sum256([]byte(id))
		lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
		lock.Lock()
		err = g.deleteAccountLocked(ctx, id, root, state)
		lock.Unlock()
		if err != nil {
			return err
		}
	}
	rows, err = g.Issuer.Store.DB().QueryContext(ctx, `SELECT id,account_id FROM brains WHERE state='deleted'`)
	if err != nil {
		return err
	}
	type deletedBrain struct{ id, account string }
	var deleted []deletedBrain
	for rows.Next() {
		var brain deletedBrain
		if err = rows.Scan(&brain.id, &brain.account); err != nil {
			_ = rows.Close()
			return err
		}
		deleted = append(deleted, brain)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, brain := range deleted {
		hash := sha256.Sum256([]byte(brain.account))
		lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
		lock.Lock()
		err = g.deleteBrainWithJournalState(ctx, brain.account, brain.id, root, state)
		lock.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}
