// Command episode-qualification is a separate offline operator child. It is not
// linked into the hosted binary. Fixture mutation requires inherited root custody;
// the separate metadata-only mode produces observations without mutation authority.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Set to the exact reviewed source commit with -ldflags; an unpinned build refuses.
var sourceRevision string

var (
	errAdoptionManifestChanged      = errors.New("source adoption refused: exact old manifest hash no longer matches; no adoption write; reconcile prior publication")
	errAdoptionNotPublished         = errors.New("source adoption interrupted before publication; old manifest retained; fresh custody required")
	errAdoptionPublicationUncertain = errors.New("source adoption publication may have committed; retain hold and reconcile manifest; do not replay as success")
	errAdoptionRefused              = errors.New("source adoption refused before publication; no adoption write; retain hold")
)

func run(args []string, out io.Writer) int {
	if err := executeCustody(args); err != nil {
		if code, message := adoptionOutcome(err); code != 0 {
			if _, writeErr := fmt.Fprintln(out, message); writeErr != nil {
				return 1
			}
			return code
		}
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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--fixture-custody-snapshot" {
		os.Exit(snapshotCLI(os.Args[1:], os.Stdout))
	}
	os.Exit(run(os.Args[1:], os.Stderr))
}

// Stable content-free child outcomes. A replacement proof is fresh authority,
// never evidence that a previous publication did or did not happen.
func adoptionOutcome(err error) (int, string) {
	for _, outcome := range []struct {
		err  error
		code int
	}{
		{errAdoptionRefused, 2},
		{errAdoptionManifestChanged, 3},
		{errAdoptionNotPublished, 5},
		{errAdoptionPublicationUncertain, 6},
	} {
		if errors.Is(err, outcome.err) {
			return outcome.code, outcome.err.Error()
		}
	}
	return 0, ""
}
