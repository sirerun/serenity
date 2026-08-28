package connector_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
)

// fakeConnector is a test-only Connector: Poll returns a fixed item set
// and advances the cursor once, ToSource builds a minimal domain.Source.
type fakeConnector struct {
	name    string
	items   []connector.RawItem
	next    connector.Cursor
	pollErr error
}

func (f fakeConnector) Name() string { return f.name }

func (f fakeConnector) Poll(_ context.Context, _ connector.Cursor) ([]connector.RawItem, connector.Cursor, error) {
	if f.pollErr != nil {
		return nil, nil, f.pollErr
	}
	return f.items, f.next, nil
}

func (f fakeConnector) ToSource(item connector.RawItem) (domain.Source, error) {
	return domain.Source{
		Kind:       item.Kind,
		URI:        item.URI,
		OccurredAt: item.OccurredAt,
	}, nil
}

func openTestIndex(t *testing.T) *index.SQLite {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

// TestConnectorRunWritesOneJobRow proves T1.1's acc line "a fake connector
// run writes one jobs row with status": Run against a real *index.SQLite
// leaves exactly one job row, and it records the run's terminal status.
func TestConnectorRunWritesOneJobRow(t *testing.T) {
	ctx := context.Background()
	eng := openTestIndex(t)

	fc := fakeConnector{
		name: "fake",
		items: []connector.RawItem{
			{URI: "fake://1", Kind: "file", OccurredAt: time.Now()},
			{URI: "fake://2", Kind: "file", OccurredAt: time.Now()},
		},
		next: connector.Cursor(json.RawMessage(`{"offset":2}`)),
	}

	items, next, err := connector.Run(ctx, eng, fc, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Run returned %d items, want 2", len(items))
	}
	if string(next) != `{"offset":2}` {
		t.Fatalf("Run returned cursor %s, want the advanced cursor", next)
	}

	jobs, err := eng.Jobs(ctx)
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d job rows, want exactly 1: %+v", len(jobs), jobs)
	}
	if jobs[0].Status != connector.StatusSucceeded {
		t.Fatalf("job status = %q, want %q", jobs[0].Status, connector.StatusSucceeded)
	}
	if jobs[0].Connector != "fake" {
		t.Fatalf("job connector = %q, want %q", jobs[0].Connector, "fake")
	}
}

// TestConnectorRunFailedPollRecordsFailure covers the failure branch:
// still exactly one job row, marked failed, carrying the error and the
// unadvanced cursor.
func TestConnectorRunFailedPollRecordsFailure(t *testing.T) {
	ctx := context.Background()
	eng := openTestIndex(t)

	fc := fakeConnector{name: "fake-fail", pollErr: context.DeadlineExceeded}
	initial := connector.Cursor(json.RawMessage(`{"offset":0}`))

	_, gotCursor, err := connector.Run(ctx, eng, fc, initial)
	if err == nil {
		t.Fatal("Run: want an error from a failed poll, got nil")
	}
	if string(gotCursor) != string(initial) {
		t.Fatalf("Run advanced the cursor past a failed poll: got %s, want %s", gotCursor, initial)
	}

	jobs, err := eng.Jobs(ctx)
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d job rows, want exactly 1", len(jobs))
	}
	if jobs[0].Status != connector.StatusFailed {
		t.Fatalf("job status = %q, want %q", jobs[0].Status, connector.StatusFailed)
	}
	if jobs[0].Error == "" {
		t.Fatal("job Error is empty, want the poll error text recorded")
	}
}

// TestConnectorSweepMarksInterruptedJob proves T1.1's acc line "a run
// killed mid-poll is marked interrupted by the next sweep": a job started
// but never finished (simulating a process that died mid-Poll, before it
// could call FinishJob) is flipped to "interrupted" by SweepInterrupted.
func TestConnectorSweepMarksInterruptedJob(t *testing.T) {
	ctx := context.Background()
	eng := openTestIndex(t)

	jobID, err := eng.StartJob(ctx, "fake-connector")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	// Deliberately never call FinishJob -- this is the killed-mid-poll case.

	n, err := eng.SweepInterrupted(ctx)
	if err != nil {
		t.Fatalf("SweepInterrupted: %v", err)
	}
	if n != 1 {
		t.Fatalf("SweepInterrupted swept %d jobs, want 1", n)
	}

	job, err := eng.Job(ctx, jobID)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if job.Status != index.JobInterrupted {
		t.Fatalf("job status = %q, want %q", job.Status, index.JobInterrupted)
	}
}
