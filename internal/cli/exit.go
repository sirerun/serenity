package cli

import "fmt"

// ExitError is returned by a command's RunE when the command has already
// written everything relevant to stdout and needs a specific process exit
// code other than the generic failure code 1 -- ADR 010's `serenity check`
// contract (exit 0 for pass/no_applicable_constraints, 2 for violated, 1
// for unverified or any other error) is the first CLI verb in this repo
// that needs a code cobra's default RunE-error path does not produce.
//
// cmd/serenity/main.go checks for this type via errors.As and exits with
// Code directly, skipping the "error: " line it prints for a genuine
// failure: a check verdict, however it comes out, is not a process error --
// it is the answer the caller asked for, already on stdout.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}
