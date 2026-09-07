package index

import (
	"context"
	"errors"
	"fmt"
)

// AppendEvent inserts one events row, keyed by id (internal/events.Store's
// own random event id, not the cursor -- the cursor lives inside the
// marshaled payload the same way internal/disposition.Item's State/Verdict
// do, so this table's generic (id, payload) schema shell (RuntimeTables,
// T4.12 "claims" it the way T1.7's spend_ledger and M2's disposition_items
// did) needs no migration). Events are append-only -- a colliding id is a
// caller bug (internal/events.Store generates ids with crypto/rand, never
// content-derived), so this is a plain INSERT, never an upsert, and
// surfaces a collision as an error instead of silently overwriting a prior
// event.
func (s *SQLite) AppendEvent(ctx context.Context, id string, payload []byte) error {
	if id == "" {
		return errors.New("index: append event: empty id")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO events(id, payload) VALUES(?, ?)`, id, payload); err != nil {
		return fmt.Errorf("index: append event: %w", err)
	}
	return nil
}

// EventPayloads lists every events row's raw payload, in no particular
// order -- internal/events.Store decodes each payload's embedded Cursor
// field and sorts/filters by it, the same convention
// internal/disposition.Store.List uses for CreatedAt.
func (s *SQLite) EventPayloads(ctx context.Context) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM events`)
	if err != nil {
		return nil, fmt.Errorf("index: list events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("index: list events: %w", err)
		}
		out = append(out, payload)
	}
	return out, rows.Err()
}
