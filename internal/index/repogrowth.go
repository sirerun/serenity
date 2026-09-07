package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// RepoSizeSample is one point-in-time measurement of the canonical brain
// repo's own on-disk footprint (RFC section 16: "repo growth/month").
// Persisted as a JSON array under repoSizeSamplesID in the "caches"
// runtime table (RuntimeTables) -- runtime-only state, the same shape
// RebuildRecord already uses for "rebuild timing" -- since a size
// measurement is a fact about a run, not something the wipe-and-rebuild
// invariant needs to reproduce byte-identically. Nothing in this package
// takes the measurement itself: internal/report.MeasureRepoSize walks the
// repo. No production caller records samples yet: report is read-only.
// This API retains externally recorded samples and supports report fixtures.
type RepoSizeSample struct {
	Bytes int64     `json:"bytes"`
	At    time.Time `json:"at"`
}

const repoSizeSamplesID = "repo_growth:samples"

// maxRepoSizeSamples bounds the persisted history so the caches row
// cannot grow unboundedly across years of weekly reports -- 60 samples
// covers well over a year at a weekly cadence, RFC section 16's own
// "weekly report card" rhythm.
const maxRepoSizeSamples = 60

// RecordRepoSizeSample appends one measurement, keeping the stored history
// sorted by At and capped at maxRepoSizeSamples (oldest dropped first).
func (s *SQLite) RecordRepoSizeSample(ctx context.Context, bytes int64, at time.Time) error {
	samples, err := s.RepoSizeSamples(ctx)
	if err != nil {
		return err
	}
	samples = append(samples, RepoSizeSample{Bytes: bytes, At: at.UTC()})
	sort.Slice(samples, func(i, k int) bool { return samples[i].At.Before(samples[k].At) })
	if len(samples) > maxRepoSizeSamples {
		samples = samples[len(samples)-maxRepoSizeSamples:]
	}

	payload, err := json.Marshal(samples)
	if err != nil {
		return fmt.Errorf("index: marshal repo size samples: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO caches(id, payload) VALUES(?, ?)
		ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, repoSizeSamplesID, payload)
	if err != nil {
		return fmt.Errorf("index: record repo size sample: %w", err)
	}
	return nil
}

// RepoSizeSamples reads back every recorded measurement, oldest first. A
// brain that has never had one recorded returns an empty, non-nil slice,
// not an error.
func (s *SQLite) RepoSizeSamples(ctx context.Context) ([]RepoSizeSample, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM caches WHERE id = ?`, repoSizeSamplesID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return []RepoSizeSample{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("index: read repo size samples: %w", err)
	}
	var samples []RepoSizeSample
	if err := json.Unmarshal(payload, &samples); err != nil {
		return nil, fmt.Errorf("index: decode repo size samples: %w", err)
	}
	return samples, nil
}
