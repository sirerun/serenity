// Command concurrent is testhooks' regression fixture for the fixed
// dispatcher design: it hits three checkpoints from three goroutines at
// once — two armed "pause" (proving releases route to the correct phase
// even in reverse order) and one never armed at all (proving an unrelated
// checkpoint is never blocked behind the paused ones). Built with -race by
// testhooks_test.go.
package main

import (
	"fmt"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		testhooks.At("phase_a")
		fmt.Println("released phase_a")
	}()
	go func() {
		defer wg.Done()
		testhooks.At("phase_b")
		fmt.Println("released phase_b")
	}()
	go func() {
		defer wg.Done()
		testhooks.At("phase_c") // never armed: must never block on phase_a/phase_b
		fmt.Println("released phase_c")
	}()
	wg.Wait()
	fmt.Println("done")
}
