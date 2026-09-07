package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RevisitCondition selects a timed deadline or an exact claim-state key.
type RevisitCondition struct {
	Timed bool
	After time.Time
	Key   []string
}

// RevisitRequest supplies application-owned item encoding. Callbacks run inside
// the transaction and must not perform database I/O or other side effects.
type RevisitRequest struct {
	ID          string
	Condition   RevisitCondition
	Now         time.Time
	NewItem     func(id string, now time.Time) ([]byte, error)
	Outstanding func(payload []byte) (bool, error)
}

// RevisitCheckpoint is runtime delivery state, preserved by index rebuild.
type RevisitCheckpoint struct {
	Initialized     bool      `json:"initialized"`
	Fingerprint     string    `json:"fingerprint"`
	PendingChange   bool      `json:"pending_change"`
	LastRevisitedAt time.Time `json:"last_revisited_at"`
	ItemID          string    `json:"item_id"`
	Generation      int       `json:"generation"`
}

// RevisitCheckpoint reads one sweep checkpoint by its request ID.
func (s *SQLite) RevisitCheckpoint(ctx context.Context, id string) (RevisitCheckpoint, error) {
	var checkpoint RevisitCheckpoint
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM caches WHERE id=?`, id).Scan(&data); err != nil {
		return checkpoint, fmt.Errorf("read revisit checkpoint: %w", err)
	}
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return checkpoint, fmt.Errorf("decode revisit checkpoint: %w", err)
	}
	return checkpoint, nil
}

// Revisit serializes observation and delivery in a transaction. The first SQL
// statement acquires SQLite's writer lock before any reads, including across
// independent processes, avoiding a deferred-transaction upgrade race.
func (s *SQLite) Revisit(ctx context.Context, request RevisitRequest) (bool, error) {
	if request.ID == "" || request.NewItem == nil || request.Outstanding == nil || (request.Condition.Timed == (len(request.Condition.Key) > 0)) {
		return false, errors.New("invalid revisit request")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO caches(id,payload) VALUES(?,?) ON CONFLICT(id) DO NOTHING`, request.ID, []byte(`{}`)); err != nil {
		return false, err
	}
	var data []byte
	if err = tx.QueryRowContext(ctx, `SELECT payload FROM caches WHERE id=?`, request.ID).Scan(&data); err != nil {
		return false, err
	}
	var cp RevisitCheckpoint
	if err = json.Unmarshal(data, &cp); err != nil {
		return false, fmt.Errorf("decode revisit checkpoint: %w", err)
	}
	fingerprint := ""
	due := request.Condition.Timed && !request.Now.Before(request.Condition.After)
	if len(request.Condition.Key) > 0 {
		if len(request.Condition.Key) != 3 {
			return false, errors.New("invalid revisit claim key")
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,state FROM claims WHERE slug=? AND predicate=? AND object_key=? ORDER BY id,state`, request.Condition.Key[0], request.Condition.Key[1], request.Condition.Key[2])
		if err != nil {
			return false, err
		}
		var states [][2]string
		for rows.Next() {
			var id, state string
			if err = rows.Scan(&id, &state); err != nil {
				_ = rows.Close()
				return false, err
			}
			states = append(states, [2]string{id, state})
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return false, err
		}
		if closeErr != nil {
			return false, closeErr
		}
		encoded, err := json.Marshal(states)
		if err != nil {
			return false, err
		}
		sum := sha256.Sum256(encoded)
		fingerprint = hex.EncodeToString(sum[:])
		if !cp.Initialized {
			cp.Initialized = true
			cp.Fingerprint = fingerprint
		} else if fingerprint != cp.Fingerprint {
			cp.PendingChange = true
		}
		due = cp.PendingChange
	}
	eligible := cp.LastRevisitedAt.IsZero() || !request.Now.Before(cp.LastRevisitedAt.Add(7*24*time.Hour))
	outstanding := false
	if cp.ItemID != "" {
		var itemData []byte
		if err = tx.QueryRowContext(ctx, `SELECT payload FROM disposition_items WHERE id=?`, cp.ItemID).Scan(&itemData); err != nil {
			return false, fmt.Errorf("read revisit review: %w", err)
		}
		outstanding, err = request.Outstanding(itemData)
		if err != nil {
			return false, fmt.Errorf("decode revisit review: %w", err)
		}
	}
	created := due && eligible && !outstanding
	if created {
		cp.Generation++
		cp.ItemID = fmt.Sprintf("%s:%d", request.ID, cp.Generation)
		cp.LastRevisitedAt = request.Now.UTC()
		cp.Fingerprint = fingerprint
		cp.PendingChange = false
		cp.Initialized = true
		itemData, err := request.NewItem(cp.ItemID, request.Now.UTC())
		if err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO disposition_items(id,payload) VALUES(?,?)`, cp.ItemID, itemData); err != nil {
			return false, err
		}
	}
	data, err = json.Marshal(cp)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE caches SET payload=? WHERE id=?`, data, request.ID); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return created, nil
}
