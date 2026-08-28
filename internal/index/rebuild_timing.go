package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RebuildRecord is the timing of the most recent Rebuild call (RFC
// section 16: "rebuild timing"). Persisted as a single row keyed
// rebuildRecordID in the "caches" runtime table (RuntimeTables) --
// runtime-only state, never canonical, exactly like the jobs and
// spend_ledger rows it sits alongside. Rebuild itself never writes this
// (it would double-count against the runtime-state-preserved-across-
// Rebuild invariant tests in runtime_test.go); callers that drive a
// rebuild (runSync, runExtract in internal/cli) measure the call and
// record it explicitly.
type RebuildRecord struct {
	Duration time.Duration `json:"duration_ns"`
	At       time.Time     `json:"at"`
}

const rebuildRecordID = "rebuild:last"

// ErrNoRebuildRecord means Rebuild has never completed against this
// index -- a fresh brain that has only run `serenity init`, for example.
var ErrNoRebuildRecord = errors.New("index: no rebuild record")

// RecordRebuildTiming persists dur/at as the most recent rebuild's
// timing, overwriting whatever was recorded before.
func (s *SQLite) RecordRebuildTiming(ctx context.Context, dur time.Duration, at time.Time) error {
	payload, err := json.Marshal(RebuildRecord{Duration: dur, At: at})
	if err != nil {
		return fmt.Errorf("index: marshal rebuild record: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO caches(id, payload) VALUES(?, ?)
		ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, rebuildRecordID, payload)
	if err != nil {
		return fmt.Errorf("index: record rebuild timing: %w", err)
	}
	return nil
}

// LastRebuildTiming reads back the most recent rebuild's timing.
func (s *SQLite) LastRebuildTiming(ctx context.Context) (RebuildRecord, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM caches WHERE id = ?`, rebuildRecordID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return RebuildRecord{}, ErrNoRebuildRecord
	}
	if err != nil {
		return RebuildRecord{}, fmt.Errorf("index: read rebuild timing: %w", err)
	}
	var r RebuildRecord
	if err := json.Unmarshal(payload, &r); err != nil {
		return RebuildRecord{}, fmt.Errorf("index: decode rebuild timing: %w", err)
	}
	return r, nil
}
