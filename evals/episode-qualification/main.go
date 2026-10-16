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
		if _, err := fmt.Fprintln(out, "REFUSED: fixture custody or operation failed; retain hold for reconciliation"); err != nil {
			return 1
		}
		return 1
	}
	if _, err := fmt.Fprintln(out, "fixture operation completed; synthetic accounts/data retained; capture unchanged"); err != nil {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }
