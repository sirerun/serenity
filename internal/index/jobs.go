package index

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Job lifecycle states (RFC §10.1: "every run is a job row"). Plain
// untyped string constants -- not a named type -- so *SQLite's StartJob/
// FinishJob methods satisfy internal/connector's JobStore interface
// (declared with bare `string`) without either package importing the
// other.
const (
	JobRunning     = "running"
	JobSucceeded   = "succeeded"
	JobFailed      = "failed"
	JobInterrupted = "interrupted"
)

// ErrJobNotFound is returned by Job/FinishJob when no row matches the id.
var ErrJobNotFound = errors.New("index: job not found")

// Job is one connector-run job row, marshaled whole into the "jobs"
// runtime table's existing payload column (see RuntimeTables and
// migrate's schema-shell loop in sqlite.go) -- the shell's flat
// (id, payload) shape is left untouched so later milestones can claim
// their own runtime tables (spend_ledger, disposition_items) the same
// way, with no migration of this one.
type Job struct {
	ID         string          `json:"id"`
	Connector  string          `json:"connector"`
	Status     string          `json:"status"`
	Cursor     json.RawMessage `json:"cursor,omitempty"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at,omitzero"`
	Error      string          `json:"error,omitempty"`
}

// newJobID returns a random 32-hex-char id. Job ids are not content-
// derived (unlike claim ids) -- crypto/rand keeps internal/index stdlib-
// only per ADR 003, which reserves third-party dependencies for the
// connector/provider edge.
func newJobID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("index: generate job id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// StartJob records a new running job for connector and returns its id.
func (s *SQLite) StartJob(ctx context.Context, connector string) (string, error) {
	id, err := newJobID()
	if err != nil {
		return "", err
	}
	j := Job{
		ID:        id,
		Connector: connector,
		Status:    JobRunning,
		StartedAt: s.clock.Now().UTC(),
	}
	payload, err := json.Marshal(j)
	if err != nil {
		return "", fmt.Errorf("index: marshal job: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO jobs(id, payload) VALUES(?, ?)`, j.ID, payload); err != nil {
		return "", fmt.Errorf("index: start job: %w", err)
	}
	return j.ID, nil
}

// FinishJob records a job's terminal status and, when cursor is non-nil,
// its resume position. A run that never calls FinishJob (killed
// mid-Poll) leaves its row "running" -- SweepInterrupted reclaims it on
// the next sweep.
func (s *SQLite) FinishJob(ctx context.Context, jobID string, status string, cursor json.RawMessage, runErr error) error {
	j, err := s.Job(ctx, jobID)
	if err != nil {
		return err
	}
	j.Status = status
	j.FinishedAt = s.clock.Now().UTC()
	if cursor != nil {
		j.Cursor = cursor
	}
	if runErr != nil {
		j.Error = runErr.Error()
	}
	payload, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("index: marshal job: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET payload = ? WHERE id = ?`, payload, jobID)
	if err != nil {
		return fmt.Errorf("index: finish job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("index: finish job: %w", err)
	}
	if n == 0 {
		return ErrJobNotFound
	}
	return nil
}

// Job reads back one job row by id.
func (s *SQLite) Job(ctx context.Context, jobID string) (Job, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM jobs WHERE id = ?`, jobID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("index: read job: %w", err)
	}
	var j Job
	if err := json.Unmarshal(payload, &j); err != nil {
		return Job{}, fmt.Errorf("index: decode job %s: %w", jobID, err)
	}
	return j, nil
}

// Jobs lists every job row, most recently started first -- `serenity
// status` (T1.17) reads this for ingest lag / connector health.
func (s *SQLite) Jobs(ctx context.Context) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM jobs`)
	if err != nil {
		return nil, fmt.Errorf("index: list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Job
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("index: list jobs: %w", err)
		}
		var j Job
		if err := json.Unmarshal(payload, &j); err != nil {
			return nil, fmt.Errorf("index: decode job: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt.After(out[k].StartedAt) })
	return out, nil
}

// SweepInterrupted marks every job still "running" as "interrupted". In
// this single-process CLI model, a job can only still be "running" when
// the process that started it died before calling FinishJob -- there is
// no legitimate reason for a "running" row to survive to the next sweep.
// Callers run this once before starting new jobs (e.g. at the top of
// `serenity sync`).
func (s *SQLite) SweepInterrupted(ctx context.Context) (int, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, j := range jobs {
		if j.Status != JobRunning {
			continue
		}
		if err := s.FinishJob(ctx, j.ID, JobInterrupted, nil, nil); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
