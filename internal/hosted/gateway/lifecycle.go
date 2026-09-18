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
	owned, err := g.Issuer.Store.BrainByID(ctx, account, brain)
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
		_, e := tx.ExecContext(ctx, `UPDATE brains SET state='deleted',deleted_at=? WHERE id=? AND account_id=?`, now, brain, account)
		return e
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(root, owned.PathKey))
}

func (g *Gateway) DeleteAccount(ctx context.Context, account, root string) error {
	if err := g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status IN ('active','deleting')`, account)
		return e
	}); err != nil {
		return err
	}
	brains, err := g.Issuer.Store.Brains(ctx, account)
	if err != nil {
		return err
	}
	for _, brain := range brains {
		if err = g.DeleteBrain(ctx, account, brain.ID, root); err != nil {
			return err
		}
	}
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
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
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET status='deleted',email='',email_hash=?,plan_id='free' WHERE id=?`, "deleted:"+account, account)
		return e
	})
}

func (g *Gateway) RecoverDeletions(ctx context.Context, root string) error {
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
		if err = g.DeleteAccount(ctx, id, root); err != nil {
			return err
		}
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
