package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunRecord is the on-disk runtime-state shape (never canonical, never
// committed to git — RFC §7.5) recording a job's most recent run and total
// run count. It lives at RecordPath(root, job) under .serenity/, the same
// runtime-state directory internal/writer's PendingRecord and
// internal/providers' index.db already use.
type RunRecord struct {
	Job      string          `json:"job"`
	LastRun  time.Time       `json:"last_run"`
	RunCount int             `json:"run_count"`
	Details  json.RawMessage `json:"details,omitempty"`
}

// RecordPath returns the runtime-state path a job's run record lives at:
// .serenity/cron/<job>.json.
func RecordPath(root, job string) string {
	return filepath.Join(root, ".serenity", "cron", job+".json")
}

// ReadRecord reads a job's run record, if one exists. A missing record
// (job never run) is reported via os.IsNotExist(err), not a zero value —
// callers that want "zero if never run" should check that explicitly.
func ReadRecord(root, job string) (RunRecord, error) {
	var rec RunRecord
	b, err := os.ReadFile(RecordPath(root, job))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return rec, fmt.Errorf("cron: read record %s: %w", job, err)
	}
	return rec, nil
}

// recordRun records a completed invocation. Detailed jobs include their actual
// computed result; a failure before this call never advances the success record.
func recordRun(root, job string, at time.Time) error {
	return recordDetails(root, job, at, nil)
}

func recordDetails(root, job string, at time.Time, details any) error {
	rec := RunRecord{Job: job, LastRun: at, RunCount: 1}
	if details != nil {
		raw, err := json.Marshal(details)
		if err != nil {
			return err
		}
		rec.Details = raw
	}
	if prev, err := ReadRecord(root, job); err == nil {
		rec.RunCount = prev.RunCount + 1
	}

	path := RecordPath(root, job)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cron: record run: %w", err)
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("cron: record run: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("cron: record run: %w", err)
	}
	return nil
}
