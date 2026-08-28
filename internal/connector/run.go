package connector

import (
	"context"
	"encoding/json"
	"fmt"
)

// Job lifecycle statuses a Run reports. Kept as plain string constants
// (not a distinct named type) so they satisfy JobStore's plain-string
// signature with no conversion at the call site; internal/index mirrors
// these same literal values for its own "running"/"interrupted" states.
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// JobStore is the runtime job-bookkeeping boundary a connector run writes
// through. The jobs table is runtime-only state, never canonical repo
// content (RFC §7 preamble: "job queue ... is DB-only by design"), so it
// bypasses the canonical writer queue entirely and is written straight to
// the derived index db. Declared here with only primitive types (string,
// json.RawMessage) so this package never has to import internal/index;
// *index.SQLite (internal/index/jobs.go) implements this interface
// structurally, with no import in either direction.
type JobStore interface {
	StartJob(ctx context.Context, connector string) (jobID string, err error)
	FinishJob(ctx context.Context, jobID string, status string, cursor json.RawMessage, runErr error) error
}

// Run executes one poll cycle of c against jobs, recording exactly one
// jobs row for the run: started before Poll is called, finished with
// StatusSucceeded or StatusFailed (carrying the error text) once Poll
// returns. A process killed mid-Poll never reaches FinishJob, leaving the
// row "running" -- index.SQLite.SweepInterrupted reclaims it on the next
// sweep (RFC §10.1: "a run killed mid-poll is marked interrupted by the
// next sweep").
func Run(ctx context.Context, jobs JobStore, c Connector, cursor Cursor) ([]RawItem, Cursor, error) {
	jobID, err := jobs.StartJob(ctx, c.Name())
	if err != nil {
		return nil, cursor, fmt.Errorf("connector: start job for %s: %w", c.Name(), err)
	}

	items, next, pollErr := c.Poll(ctx, cursor)

	status := StatusSucceeded
	finalCursor := next
	if pollErr != nil {
		status = StatusFailed
		finalCursor = cursor // do not advance past a failed poll
	}
	if err := jobs.FinishJob(ctx, jobID, status, json.RawMessage(finalCursor), pollErr); err != nil {
		return items, finalCursor, fmt.Errorf("connector: finish job for %s: %w", c.Name(), err)
	}
	return items, finalCursor, pollErr
}
