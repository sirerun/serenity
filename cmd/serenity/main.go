package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/sirerun/serenity/internal/cli"
)

func main() {
	err := cli.Execute()
	if err == nil {
		return
	}

	// A command that has already printed its answer (e.g. `serenity check`'s
	// verdict, ADR 010) signals its exit code via *cli.ExitError instead of a
	// bare error -- that is not a process failure, so it skips the "error: "
	// line below.
	var exitErr *cli.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}

	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
