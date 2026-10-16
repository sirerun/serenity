// Command episode-qualification is a separate offline operator child. It is not
// linked into the hosted binary. Only inherited root custody enables execution.
package main

import (
	"fmt"
	"io"
	"os"
)

// Set to the exact reviewed source commit with -ldflags; an unpinned build refuses.
var sourceRevision string

func run(args []string, out io.Writer) int {
	if executeCustody(args) != nil {
		fmt.Fprintln(out, "REFUSED: fixture custody or operation failed; retain hold for reconciliation")
		return 1
	}
	fmt.Fprintln(out, "fixture operation completed; synthetic accounts/data retained; capture unchanged")
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }
