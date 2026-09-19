// Command probe is testhooks' own black-box proof fixture: it calls At() once
// at a fixed phase, then reports it returned normally. Built twice by
// testhooks_test.go, with and without -tags hostedtest, to prove the build
// tag is what gates activation, not just the presence of an env var.
package main

import (
	"fmt"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

func main() {
	testhooks.At("proof_phase")
	fmt.Println("reached")
}
