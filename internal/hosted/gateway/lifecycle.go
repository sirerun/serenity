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

	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

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
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	return g.deleteBrainUnderAccountLock(ctx, account, brain, root)
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
	if owned.PathKey != owned.ID || filepath.Base(owned.PathKey) != owned.PathKey {
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
	if preflight == nil {
		return errors.New("hosted: account deletion preflight is required")
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	proceed, err := preflight(ctx, account)
	if err != nil || !proceed {
		return err
	}
	return g.deleteAccountUnderMaintenance(ctx, account, root)
}

// deleteAccountUnderMaintenance requires the caller to hold Maintenance and
// the account lock.
func (g *Gateway) deleteAccountUnderMaintenance(ctx context.Context, account, root string) error {
	if err := g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status IN ('active','deleting')`, account)
		return e
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
		if err = g.deleteBrainUnderAccountLock(ctx, account, id, root); err != nil {
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
	return g.RecoverDeletionsWithPreflight(ctx, root, func(ctx context.Context, account string) (bool, error) {
		var customer string
		var subscriptions bool
		err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(stripe_customer_id,''),EXISTS(SELECT 1 FROM subscriptions WHERE account_id=accounts.id) FROM accounts WHERE id=?`, account).Scan(&customer, &subscriptions)
		if err != nil {
			return false, err
		}
		if customer != "" || subscriptions {
			return false, errors.New("hosted: billing closure requires the service deletion path")
		}
		return true, nil
	})
}

// RecoverDeletionsWithPreflight holds the maintenance read fence for the
// complete recovery pass and invokes the trusted service closure check under
// each account lock before that account is purged.
func (g *Gateway) RecoverDeletionsWithPreflight(ctx context.Context, root string, preflight AccountDeletePreflight) error {
	if preflight == nil {
		return errors.New("hosted: account deletion preflight is required")
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	rows, err := g.Issuer.Store.DB().QueryContext(ctx, `SELECT id FROM accounts WHERE status='deleting'`)
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
		proceed, preflightErr := preflight(ctx, id)
		if preflightErr != nil {
			lock.Unlock()
			return preflightErr
		}
		if !proceed {
			lock.Unlock()
			continue
		}
		if err = g.deleteAccountUnderMaintenance(ctx, id, root); err != nil {
			lock.Unlock()
			return err
		}
		lock.Unlock()
	}
	rows, err = g.Issuer.Store.DB().QueryContext(ctx, `SELECT id,path_key FROM brains WHERE state='deleted'`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, key string
		if err = rows.Scan(&id, &key); err != nil {
			return err
		}
		if id != key || filepath.Base(key) != key || len(key) < 16 {
			return errors.New("invalid deleted brain path")
		}
		if err = os.RemoveAll(filepath.Join(root, key)); err != nil {
			return err
		}
	}
	return rows.Err()
}
