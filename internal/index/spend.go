package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// SpendRow is one spend_ledger row as `serenity status` (T1.17) reads it
// back. Declared with primitive types only, independent of
// internal/router's SpendEntry -- the same asymmetric-dependency shape
// jobs.go uses for connector.JobStore (see connector.go), so internal/
// index never has to import internal/router just to read what a Router
// call recorded (RFC section 16). Field names mirror SpendEntry
// one-for-one so a future SpendLedger adapter over RecordSpend is a
// straight field copy.
type SpendRow struct {
	ID           string    `json:"id"`
	TaskClass    string    `json:"task_class"`
	Tier         string    `json:"tier"`
	Provider     string    `json:"provider"`
	ModelVersion string    `json:"model_version"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// RecordSpend appends one spend_ledger row. Exported so whichever
// subsystem first holds a live index.Engine handle in production (see
// internal/router/ledger.go's SpendLedger doc) can persist SpendEntry
// rows here via a thin adapter, and so tests can seed known fixture data
// for `serenity status`'s "spend to date" field.
func (s *SQLite) RecordSpend(ctx context.Context, row SpendRow) error {
	if row.ID == "" {
		return errors.New("index: record spend: empty id")
	}
	payload, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("index: marshal spend row: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO spend_ledger(id, payload) VALUES(?, ?)`, row.ID, payload); err != nil {
		return fmt.Errorf("index: record spend: %w", err)
	}
	return nil
}

// SpendRows lists every spend_ledger row, oldest first -- `serenity
// status` (T1.17) sums these for spend-to-date.
func (s *SQLite) SpendRows(ctx context.Context) ([]SpendRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM spend_ledger`)
	if err != nil {
		return nil, fmt.Errorf("index: list spend rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SpendRow
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("index: list spend rows: %w", err)
		}
		var r SpendRow
		if err := json.Unmarshal(payload, &r); err != nil {
			return nil, fmt.Errorf("index: decode spend row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, k int) bool { return out[i].OccurredAt.Before(out[k].OccurredAt) })
	return out, nil
}
