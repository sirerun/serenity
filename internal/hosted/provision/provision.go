// Package provision recovers durable default-brain allocation.
package provision

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

type Provisioner struct {
	Store      *store.Store
	BrainsRoot string
	mu         sync.Mutex
}

func (p *Provisioner) Provision(ctx context.Context, accountID string) (b store.Brain, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	err = p.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var status string
		if e := tx.QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); e != nil {
			return e
		}
		if status != "active" {
			return store.ErrNotFound
		}
		id := store.ID()
		_, e := tx.ExecContext(ctx, `INSERT INTO brains(id,account_id,path_key,state,created_at) VALUES(?,?,?,'allocating',?) ON CONFLICT DO NOTHING`, id, accountID, id, store.Stamp(time.Now()))
		if e != nil {
			return e
		}
		return tx.QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE account_id=? AND deleted_at IS NULL`, accountID).Scan(&b.ID, &b.AccountID, &b.State, &b.PathKey)
	})
	if err != nil {
		return b, fmt.Errorf("allocate brain: %w", err)
	}
	if b.State == "ready" {
		return b, nil
	}
	if b.PathKey != b.ID || filepath.Base(b.PathKey) != b.PathKey {
		return b, errors.New("invalid stored brain path")
	}
	path := filepath.Join(p.BrainsRoot, b.PathKey)
	if err = os.MkdirAll(path, 0700); err != nil {
		return b, fmt.Errorf("create brain directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return b, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return b, errors.New("unsafe brain directory")
	}
	err = p.Store.Transaction(ctx, func(tx *sql.Tx) error {
		r, e := tx.ExecContext(ctx, `UPDATE brains SET state='ready',ready_at=? WHERE id=? AND account_id=? AND state='allocating'`, store.Stamp(time.Now()), b.ID, accountID)
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return store.ErrNotFound
		}
		return nil
	})
	if err == nil {
		b.State = "ready"
	}
	return
}
