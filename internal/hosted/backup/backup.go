// Package backup creates portable control snapshots and canonical Git bundles.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

type Brain struct {
	ID    string `json:"id"`
	Empty bool   `json:"empty"`
}
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Brains    []Brain   `json:"brains"`
}

func safeID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// Create refuses to overwrite a snapshot. Upload only after this returns success.
func Create(ctx context.Context, dataDir, destination string) (err error) {
	if err = os.Mkdir(destination, 0700); err != nil {
		return err
	}
	live, err := store.Open(filepath.Join(dataDir, "control.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, live.Close()) }()
	if _, err = live.DB().ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(destination, "control.db")); err != nil {
		return fmt.Errorf("snapshot control database: %w", err)
	}
	snapshot, err := store.Open(filepath.Join(destination, "control.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, snapshot.Close()) }()
	rows, err := snapshot.DB().QueryContext(ctx, `SELECT id,path_key FROM brains WHERE state='ready' AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	var brains []Brain
	for rows.Next() {
		var id, path string
		if err = rows.Scan(&id, &path); err != nil {
			_ = rows.Close()
			return err
		}
		if id != path || !safeID(id) {
			_ = rows.Close()
			return errors.New("invalid brain path in snapshot")
		}
		brains = append(brains, Brain{ID: id})
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for i := range brains {
		brain := &brains[i]
		root := filepath.Join(dataDir, "brains", brain.ID)
		if _, err = os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
			brain.Empty = true
			continue
		} else if err != nil {
			return err
		}
		if output, e := exec.CommandContext(ctx, "git", "-C", root, "bundle", "create", filepath.Join(destination, brain.ID+".bundle"), "--all").CombinedOutput(); e != nil {
			return fmt.Errorf("bundle snapshot: %w: %s", e, output)
		}
	}
	data, err := json.MarshalIndent(Manifest{Version: 1, CreatedAt: time.Now().UTC(), Brains: brains}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destination, "manifest.json"), append(data, '\n'), 0600)
}

// Restore disables all restored sessions, credentials and accounts. An operator
// must reconcile deletion and subscription state before reactivating accounts.
// This deliberately cannot resurrect an old credential or paid entitlement.
func Restore(ctx context.Context, snapshot, destination string) (err error) {
	entries, e := os.ReadDir(destination)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if len(entries) > 0 {
		return errors.New("restore destination must be empty")
	}
	data, err := os.ReadFile(filepath.Join(snapshot, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Version != 1 {
		return errors.New("unsupported backup version")
	}
	if err = os.MkdirAll(filepath.Join(destination, "brains"), 0700); err != nil {
		return err
	}
	input, err := os.Open(filepath.Join(snapshot, "control.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, err := os.OpenFile(filepath.Join(destination, "control.db"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if err = errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(destination, "control.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	err = db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{`DELETE FROM sessions`, `DELETE FROM login_tokens`, `UPDATE accounts SET status='restore_pending',plan_id='free'`, `UPDATE subscriptions SET status='restore_pending'`} {
			if _, e := tx.ExecContext(ctx, statement); e != nil {
				return e
			}
		}
		_, e := tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=?`, store.Stamp(time.Now()))
		return e
	})
	if err != nil {
		return err
	}
	for _, brain := range manifest.Brains {
		if !safeID(brain.ID) {
			return errors.New("invalid backup brain id")
		}
		root := filepath.Join(destination, "brains", brain.ID)
		if brain.Empty {
			if err = os.Mkdir(root, 0700); err != nil {
				return err
			}
			continue
		}
		if output, e := exec.CommandContext(ctx, "git", "clone", "--", filepath.Join(snapshot, brain.ID+".bundle"), root).CombinedOutput(); e != nil {
			return fmt.Errorf("restore brain bundle: %w: %s", e, output)
		}
	}
	return nil
}
