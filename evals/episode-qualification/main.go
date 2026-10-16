// Command episode-qualification is a deliberately disabled offline operator entry
// point. The fixture engine is not reachable until the existing maintenance
// authority contract is supplied and independently reviewed. It is never linked
// into the hosted service.
package main

import (
	"fmt"
	"io"
	"os"
)

func run(out io.Writer) int {
	fmt.Fprintln(out, "REFUSED: offline maintenance authority integration unavailable; no files opened or changed")
	return 1
}

func main() { os.Exit(run(os.Stderr)) }
