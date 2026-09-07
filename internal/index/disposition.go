package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PutDispositionItem inserts or updates one disposition_items row, keyed by
// id. Items are mutable -- state advances pending -> deferred/parked ->
// disposed as Dispose calls land (internal/disposition.Store) -- so this is
// an upsert, unlike AppendDispositionHistory below. The table itself is the
// generic (id, payload) schema shell T0.10 seeded (RuntimeTables, T2.1
// "claims" it per the plan without a schema migration, the same way T1.1's
// jobs and T1.7's spend_ledger did).
func (s *SQLite) PutDispositionItem(ctx context.Context, id string, payload []byte) error {
	if id == "" {
		return errors.New("index: put disposition item: empty id")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO disposition_items(id, payload) VALUES(?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, id, payload); err != nil {
		return fmt.Errorf("index: put disposition item: %w", err)
	}
	return nil
}

// DispositionItem reads back one disposition_items row by id. A missing row
// is (nil, false, nil), never an error -- callers (internal/disposition.Store)
// distinguish "not found" from a real storage failure this way, the same
// convention HasVector uses.
func (s *SQLite) DispositionItem(ctx context.Context, id string) ([]byte, bool, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM disposition_items WHERE id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("index: get disposition item: %w", err)
	}
	return payload, true, nil
}

// DispositionItems lists every disposition_items row's raw payload, in no
// particular order -- internal/disposition.Store decodes and sorts these.
func (s *SQLite) DispositionItems(ctx context.Context) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM disposition_items`)
	if err != nil {
		return nil, fmt.Errorf("index: list disposition items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("index: list disposition items: %w", err)
		}
		out = append(out, payload)
	}
	return out, rows.Err()
}

// AppendDispositionHistory inserts one disposition_history row. History is
// append-only (RFC 0001 §8.2: "every disposition is recorded ... disposition
// history is the training signal for the earned-automation ladder") -- ids
// are freshly random per event (internal/disposition.Store), so a colliding
// id is a caller bug, not a legitimate update; this is a plain INSERT,
// never an upsert, and surfaces a collision as an error instead of silently
// overwriting a prior disposition event.
func (s *SQLite) AppendDispositionHistory(ctx context.Context, id string, payload []byte) error {
	if id == "" {
		return errors.New("index: append disposition history: empty id")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO disposition_history(id, payload) VALUES(?, ?)`, id, payload); err != nil {
		return fmt.Errorf("index: append disposition history: %w", err)
	}
	return nil
}

// DispositionHistory lists every disposition_history row's raw payload,
// every item's history pooled together -- internal/disposition.Store
// filters by item id and sorts by occurred_at.
func (s *SQLite) DispositionHistory(ctx context.Context) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM disposition_history`)
	if err != nil {
		return nil, fmt.Errorf("index: list disposition history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("index: list disposition history: %w", err)
		}
		out = append(out, payload)
	}
	return out, rows.Err()
}
