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
	Job      string    `json:"job"`
	LastRun  time.Time `json:"last_run"`
	RunCount int       `json:"run_count"`
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

// recordRun writes job's run record for this invocation, incrementing
// RunCount from whatever was already on disk (0 on a first run). This is
// what makes a placeholder job idempotent: running it twice never errors,
// it just overwrites the same file with an incremented count and the new
// LastRun.
func recordRun(root, job string, at time.Time) error {
	rec := RunRecord{Job: job, LastRun: at, RunCount: 1}
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
