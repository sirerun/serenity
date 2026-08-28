package index

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func openJobsTestDB(t *testing.T) *SQLite {
	t.Helper()
	eng, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

func TestStartJobThenJobRoundTrips(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	id, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if id == "" {
		t.Fatal("StartJob returned an empty id")
	}

	j, err := eng.Job(ctx, id)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if j.Connector != "connector-a" {
		t.Fatalf("Connector = %q, want %q", j.Connector, "connector-a")
	}
	if j.Status != JobRunning {
		t.Fatalf("Status = %q, want %q", j.Status, JobRunning)
	}
	if j.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero")
	}
}

func TestStartJobTwiceProducesDistinctIDs(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	id1, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	id2, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("two StartJob calls produced the same id %q", id1)
	}
}

func TestFinishJobSuccessSetsStatusCursorAndFinishedAt(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	id, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	cursor := json.RawMessage(`{"offset":42}`)
	if err := eng.FinishJob(ctx, id, JobSucceeded, cursor, nil); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}

	j, err := eng.Job(ctx, id)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if j.Status != JobSucceeded {
		t.Fatalf("Status = %q, want %q", j.Status, JobSucceeded)
	}
	if string(j.Cursor) != string(cursor) {
		t.Fatalf("Cursor = %s, want %s", j.Cursor, cursor)
	}
	if j.FinishedAt.IsZero() {
		t.Fatal("FinishedAt is zero after FinishJob")
	}
	if j.Error != "" {
		t.Fatalf("Error = %q, want empty on success", j.Error)
	}
}

func TestFinishJobFailureRecordsErrorText(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	id, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	runErr := errors.New("boom")
	if err := eng.FinishJob(ctx, id, JobFailed, nil, runErr); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}

	j, err := eng.Job(ctx, id)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if j.Status != JobFailed {
		t.Fatalf("Status = %q, want %q", j.Status, JobFailed)
	}
	if j.Error != "boom" {
		t.Fatalf("Error = %q, want %q", j.Error, "boom")
	}
}

func TestFinishJobUnknownIDReturnsErrJobNotFound(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	err := eng.FinishJob(ctx, "does-not-exist", JobSucceeded, nil, nil)
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("FinishJob on unknown id: got %v, want ErrJobNotFound", err)
	}
}

func TestJobUnknownIDReturnsErrJobNotFound(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	_, err := eng.Job(ctx, "does-not-exist")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("Job on unknown id: got %v, want ErrJobNotFound", err)
	}
}

func TestJobsListsMostRecentFirst(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	id1, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if err := eng.FinishJob(ctx, id1, JobSucceeded, nil, nil); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
	id2, err := eng.StartJob(ctx, "connector-b")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	jobs, err := eng.Jobs(ctx)
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].ID != id2 || jobs[1].ID != id1 {
		t.Fatalf("Jobs order = [%s, %s], want most-recently-started first [%s, %s]",
			jobs[0].ID, jobs[1].ID, id2, id1)
	}
}

func TestSweepInterruptedLeavesTerminalJobsAlone(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	running, err := eng.StartJob(ctx, "connector-a")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	succeeded, err := eng.StartJob(ctx, "connector-b")
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if err := eng.FinishJob(ctx, succeeded, JobSucceeded, nil, nil); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}

	n, err := eng.SweepInterrupted(ctx)
	if err != nil {
		t.Fatalf("SweepInterrupted: %v", err)
	}
	if n != 1 {
		t.Fatalf("SweepInterrupted swept %d jobs, want 1", n)
	}

	gotRunning, err := eng.Job(ctx, running)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if gotRunning.Status != JobInterrupted {
		t.Fatalf("running job Status = %q, want %q", gotRunning.Status, JobInterrupted)
	}

	gotSucceeded, err := eng.Job(ctx, succeeded)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if gotSucceeded.Status != JobSucceeded {
		t.Fatalf("succeeded job Status changed to %q, want untouched %q", gotSucceeded.Status, JobSucceeded)
	}
}

func TestSweepInterruptedOnEmptyJobsIsNoop(t *testing.T) {
	ctx := context.Background()
	eng := openJobsTestDB(t)

	n, err := eng.SweepInterrupted(ctx)
	if err != nil {
		t.Fatalf("SweepInterrupted: %v", err)
	}
	if n != 0 {
		t.Fatalf("SweepInterrupted on empty jobs table swept %d, want 0", n)
	}
}
