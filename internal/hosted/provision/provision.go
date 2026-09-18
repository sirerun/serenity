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
		return tx.QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE account_id=? AND deleted_at IS NULL AND is_default=1`, accountID).Scan(&b.ID, &b.AccountID, &b.State, &b.PathKey)
	})
	if err != nil {
		return b, fmt.Errorf("allocate brain: %w", err)
	}
	return p.finish(ctx, b)
}

func (p *Provisioner) finish(ctx context.Context, b store.Brain) (_ store.Brain, err error) {
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
		r, e := tx.ExecContext(ctx, `UPDATE brains SET state='ready',ready_at=? WHERE id=? AND account_id=? AND state='allocating'`, store.Stamp(time.Now()), b.ID, b.AccountID)
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
	return b, err
}

// Additional creates a paid-plan brain while the caller supplies its resolved limit.
func (p *Provisioner) Additional(ctx context.Context, accountID string, limit int64) (b store.Brain, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := store.ID()
	err = p.Store.Transaction(ctx, func(tx *sql.Tx) error {
		// A retry after a partial failure resumes the same allocated brain.
		e := tx.QueryRowContext(ctx, `SELECT b.id FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.account_id=? AND b.is_default=0 AND b.state='allocating' AND b.deleted_at IS NULL AND a.status='active' ORDER BY b.created_at LIMIT 1`, accountID).Scan(&id)
		if e == nil {
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var count int64
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM brains WHERE account_id=? AND deleted_at IS NULL`, accountID).Scan(&count); e != nil {
			return e
		}
		if count >= limit {
			return errors.New("brain limit reached")
		}
		r, e := tx.ExecContext(ctx, `INSERT INTO brains(id,account_id,path_key,state,is_default,created_at) SELECT ?,id,?,'allocating',0,? FROM accounts WHERE id=? AND status='active'`, id, id, store.Stamp(time.Now()), accountID)
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
	if err != nil {
		return b, err
	}
	return p.finish(ctx, store.Brain{ID: id, AccountID: accountID, PathKey: id, State: "allocating"})
}

// Recover completes every interrupted allocation before serving customers.
func (p *Provisioner) Recover(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	rows, err := p.Store.DB().QueryContext(ctx, `SELECT b.id,b.account_id,b.state,b.path_key FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.state='allocating' AND b.deleted_at IS NULL AND a.status='active'`)
	if err != nil {
		return err
	}
	var pending []store.Brain
	for rows.Next() {
		var b store.Brain
		if err = rows.Scan(&b.ID, &b.AccountID, &b.State, &b.PathKey); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, b)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	for _, b := range pending {
		if _, err = p.finish(ctx, b); err != nil {
			return err
		}
	}
	return nil
}
