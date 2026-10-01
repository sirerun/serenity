package gateway

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

var (
	ErrDeletionJournalUnavailable              = errors.New("hosted: deletion journal is required")
	ErrDeletionRecoveryRequiresVerifiedHistory = errors.New("hosted: deletion recovery requires verified journal history")
	ErrDeletionPreflightDeclined               = errors.New("hosted: deletion preflight did not authorize cleanup")
)

func (g *Gateway) requireDeletionJournal() error {
	if g == nil || g.Journal == nil {
		return ErrDeletionJournalUnavailable
	}
	v := reflect.ValueOf(g.Journal)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return ErrDeletionJournalUnavailable
		}
	}
	return nil
}

func (g *Gateway) appendDeletion(ctx context.Context, typ contracts.DeletionSubjectType, id string, outcome contracts.DeletionOutcome) error {
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	e := contracts.DeletionEntry{SubjectType: typ, SubjectID: id, Outcome: outcome, RecordedAt: time.Now().UTC()}
	if err := e.Validate(); err != nil {
		return err
	}
	appended, err := g.Journal.AppendDeletion(ctx, e)
	if err != nil {
		return err
	}
	if err := appended.Validate(); err != nil {
		return fmt.Errorf("hosted: deletion journal returned invalid entry: %w", err)
	}
	if appended.SubjectType != e.SubjectType || appended.SubjectID != e.SubjectID || appended.Outcome != e.Outcome || appended.Watermark.IsZero() {
		return errors.New("hosted: deletion journal returned a mismatched entry")
	}
	return nil
}

// Export requires an account-owned brain and bypasses ordinary usage allowances.
func (g *Gateway) Export(ctx context.Context, account, brain string, out io.Writer) (err error) {
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
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
func (g *Gateway) DeleteBrain(ctx context.Context, account, brain string, root string) error {
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	if err := g.validateBrainForDeletion(ctx, account, brain); err != nil {
		return err
	}
	if err := g.appendDeletion(ctx, contracts.DeletionSubjectBrain, brain, contracts.DeletionIntentRequested); err != nil {
		return err
	}
	if err := g.deleteBrainUnderAccountLock(ctx, account, brain, root); err != nil {
		return err
	}
	return g.appendDeletion(ctx, contracts.DeletionSubjectBrain, brain, contracts.DeletionOutcomePurged)
}

func (g *Gateway) validateBrainForDeletion(ctx context.Context, account, brain string) error {
	var key string
	err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT path_key FROM brains WHERE id=? AND account_id=?`, brain, account).Scan(&key)
	if err != nil {
		return err
	}
	if key != brain || !validBrainPathKey(key) {
		return errors.New("invalid stored brain path")
	}
	return nil
}

func validBrainPathKey(key string) bool {
	return key != "" && key != "." && key != ".." && len(key) >= 16 && !filepath.IsAbs(key) && filepath.Clean(key) == key && filepath.Base(key) == key
}

// deleteBrainUnderAccountLock requires the caller to hold Maintenance and
// the account lock. Account deletion reuses it without recursively acquiring
// either lock while a backup writer may be waiting.
func (g *Gateway) deleteBrainUnderAccountLock(ctx context.Context, account, brain, root string) error {
	var owned hoststore.Brain
	err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE id=? AND account_id=?`, brain, account).Scan(&owned.ID, &owned.AccountID, &owned.State, &owned.PathKey)
	if err != nil {
		return err
	}
	if owned.PathKey != owned.ID || !validBrainPathKey(owned.PathKey) {
		return errors.New("invalid stored brain path")
	}
	if err = g.Pool.Drop(brain); err != nil {
		return err
	}
	err = g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		now := hoststore.Stamp(time.Now())
		if _, e := tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=? WHERE account_id=? AND brain_id=?`, now, account, brain); e != nil {
			return e
		}
		if e := hoststore.RevokeOAuth(ctx, tx, account, brain); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE brains SET state='deleted',deleted_at=? WHERE id=? AND account_id=?`, now, brain, account)
		return e
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(root, owned.PathKey))
}

// AccountDeletePreflight runs while the maintenance read fence and account
// mutex are held.
// It returns false when the account was already fully deleted and no purge is
// needed. Errors leave the operation pending for a later retry.
type AccountDeletePreflight func(context.Context, string) (proceed bool, err error)

func (g *Gateway) DeleteAccount(ctx context.Context, account, root string) error {
	return g.DeleteAccountWithPreflight(ctx, account, root, func(ctx context.Context, account string) (bool, error) {
		var status string
		var customer string
		var subscriptions bool
		err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT status,COALESCE(stripe_customer_id,''),EXISTS(SELECT 1 FROM subscriptions WHERE account_id=accounts.id) FROM accounts WHERE id=?`, account).Scan(&status, &customer, &subscriptions)
		if err != nil {
			return false, err
		}
		if status == "deleted" {
			return false, nil
		}
		if status != "active" && status != "deleting" {
			return false, errors.New("hosted: account is not deletable")
		}
		if customer != "" || subscriptions {
			return false, errors.New("hosted: billing closure requires the service deletion path")
		}
		return true, nil
	})
}

// DeleteAccountWithPreflight holds the maintenance read fence across the
// trusted service preflight (freeze and provider closure) and all account and
// brain cleanup. The per-account lock serializes the target account while
// allowing unrelated accounts to proceed. Private helpers do not reacquire
// either lock.
func (g *Gateway) DeleteAccountWithPreflight(ctx context.Context, account, root string, preflight AccountDeletePreflight) error {
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	if preflight == nil {
		return errors.New("hosted: account deletion preflight is required")
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	status, err := g.deletionAccountStatus(ctx, account)
	if err != nil {
		return err
	}
	if status == "deleted" {
		return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
	}
	if status != "active" && status != "deleting" {
		return errors.New("hosted: account is not deletable")
	}
	if err = g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionIntentRequested); err != nil {
		return err
	}
	proceed, err := preflight(ctx, account)
	if err != nil {
		return err
	}
	if !proceed {
		status, err = g.deletionAccountStatus(ctx, account)
		if err != nil {
			return err
		}
		if status == "deleted" {
			return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
		}
		return ErrDeletionPreflightDeclined
	}
	if err = g.deleteAccountUnderMaintenance(ctx, account, root); err != nil {
		return err
	}
	return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
}

func (g *Gateway) deletionAccountStatus(ctx context.Context, account string) (string, error) {
	var status string
	err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, account).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

// deleteAccountUnderMaintenance requires the caller to hold Maintenance and
// the account lock.
func (g *Gateway) deleteAccountUnderMaintenance(ctx context.Context, account, root string) error {
	if err := g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status IN ('active','deleting','deleted')`, account)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("hosted: account deletion state changed")
		}
		return nil
	}); err != nil {
		return err
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
		if err = g.appendDeletion(ctx, contracts.DeletionSubjectBrain, id, contracts.DeletionIntentRequested); err != nil {
			return err
		}
		if err = g.deleteBrainUnderAccountLock(ctx, account, id, root); err != nil {
			return err
		}
		if err = g.appendDeletion(ctx, contracts.DeletionSubjectBrain, id, contracts.DeletionOutcomePurged); err != nil {
			return err
		}
	}
	return g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var email string
		if e := tx.QueryRowContext(ctx, `SELECT email FROM accounts WHERE id=?`, account).Scan(&email); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM login_tokens WHERE email=?`, email); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM sessions WHERE account_id=?`, account); e != nil {
			return e
		}
		// Deleting the account also ends every partner link and any consent
		// still in flight (ADR 023). Partner keys were revoked with the brains.
		now := hoststore.Stamp(time.Now())
		if _, e := tx.ExecContext(ctx, `UPDATE partner_links SET status='revoked',revoked_at=?,updated_at=? WHERE account_id=? AND status='active'`, now, now, account); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `UPDATE link_requests SET status='denied',decided_at=? WHERE account_id=? AND status IN ('pending','approved')`, now, account); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleted',email='',email_hash=?,plan_id='free' WHERE id=?`, "deleted:"+account, account)
		return e
	})
}

func (g *Gateway) RecoverDeletions(ctx context.Context, root string) error {
	return ErrDeletionRecoveryRequiresVerifiedHistory
}

// RecoverDeletionsWithPreflight holds the maintenance read fence for the
// complete recovery pass and invokes the trusted service closure check under
// each account lock before that account is purged.
func (g *Gateway) RecoverDeletionsWithPreflight(ctx context.Context, root string, preflight AccountDeletePreflight) error {
	return ErrDeletionRecoveryRequiresVerifiedHistory
}

// ReplayDeletions applies every subject present in complete, verified journal
// history. The caller must obtain entries from a sealed ReadThrough admitted
// against the restored snapshot before handlers are installed. A later
// purged event does not suppress replay: the restored database may predate it.
func (g *Gateway) ReplayDeletions(ctx context.Context, root string, entries []contracts.DeletionEntry, preflight func(context.Context, string) (bool, error)) error {
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	if preflight == nil {
		return errors.New("hosted: account deletion preflight is required")
	}
	seen := make(map[string]bool)
	terminal := make(map[string]bool)
	ordered := make([]contracts.DeletionEntry, 0, len(entries))
	var previous contracts.DeletionWatermark
	for i, entry := range entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("hosted: invalid deletion replay entry %d: %w", i, err)
		}
		wm := entry.Watermark
		hash, hashErr := hex.DecodeString(wm.EntryHash)
		if wm.Generation < 1 || wm.SequenceID < 1 || len(hash) != 32 || hashErr != nil || hex.EncodeToString(hash) != wm.EntryHash || entry.RecordedAt.IsZero() {
			return fmt.Errorf("hosted: deletion replay entry %d has no assigned watermark", i)
		}
		if !previous.IsZero() && (wm.Generation < previous.Generation || (wm.Generation == previous.Generation && wm.SequenceID <= previous.SequenceID)) {
			return fmt.Errorf("hosted: deletion replay entries are not in append order")
		}
		previous = wm
		key := string(entry.SubjectType) + "\x00" + entry.SubjectID
		if !seen[key] {
			seen[key] = true
			ordered = append(ordered, entry)
		}
		if entry.Outcome == contracts.DeletionOutcomePurged {
			terminal[key] = true
		}
	}

	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	for _, entry := range ordered {
		key := string(entry.SubjectType) + "\x00" + entry.SubjectID
		if err := ctx.Err(); err != nil {
			return err
		}
		switch entry.SubjectType {
		case contracts.DeletionSubjectAccount:
			hash := sha256.Sum256([]byte(entry.SubjectID))
			lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
			lock.Lock()
			err := g.replayAccountDeletion(ctx, root, entry.SubjectID, preflight, terminal[key])
			lock.Unlock()
			if err != nil {
				return err
			}
		case contracts.DeletionSubjectBrain:
			var owner string
			err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT account_id FROM brains WHERE id=?`, entry.SubjectID).Scan(&owner)
			if errors.Is(err, sql.ErrNoRows) {
				if !terminal[key] {
					if err = g.appendDeletion(ctx, contracts.DeletionSubjectBrain, entry.SubjectID, contracts.DeletionOutcomePurged); err != nil {
						return err
					}
				}
				continue
			}
			if err != nil {
				return err
			}
			hash := sha256.Sum256([]byte(owner))
			lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
			lock.Lock()
			err = g.deleteBrainUnderAccountLock(ctx, owner, entry.SubjectID, root)
			if err == nil && !terminal[key] {
				err = g.appendDeletion(ctx, contracts.DeletionSubjectBrain, entry.SubjectID, contracts.DeletionOutcomePurged)
			}
			lock.Unlock()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *Gateway) replayAccountDeletion(ctx context.Context, root, account string, preflight AccountDeletePreflight, alreadyTerminal bool) error {
	status, err := g.deletionAccountStatus(ctx, account)
	if errors.Is(err, sql.ErrNoRows) {
		if !alreadyTerminal {
			return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
		}
		return nil
	}
	if err != nil {
		return err
	}
	proceed, err := preflight(ctx, account)
	if err != nil {
		return err
	}
	if !proceed {
		if status != "deleted" {
			return ErrDeletionPreflightDeclined
		}
		if !alreadyTerminal {
			return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
		}
		return nil
	}
	if err = g.deleteAccountUnderMaintenance(ctx, account, root); err != nil {
		return err
	}
	if !alreadyTerminal {
		return g.appendDeletion(ctx, contracts.DeletionSubjectAccount, account, contracts.DeletionOutcomePurged)
	}
	return nil
}
