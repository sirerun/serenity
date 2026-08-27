package writer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestQueueOrderingProperty is the M0 acceptance property test for T0.3
// (docs/plans/E0-m0-residuals.md, ADR 004): 8 goroutines x 200 jobs over a
// small set of overlapping paths must prove no interleaving per file,
// total per-file order, and every job landed.
func TestQueueOrderingProperty(t *testing.T) {
	const workers = 8
	const perWorker = 200
	const wantTotal = workers * perWorker

	// Deliberately fewer paths than goroutines so every path is hit by
	// several goroutines concurrently -- "overlapping files".
	paths := []string{"a.md", "b.md", "c.md"}

	active := map[string]*int32{}
	for _, p := range paths {
		n := int32(0)
		active[p] = &n
	}

	var seenMu sync.Mutex
	seen := map[string][]uint64{} // path -> seq numbers in the order the drain goroutine committed them

	q := NewQueue(func(r Result) {
		seenMu.Lock()
		seen[r.Job.Path] = append(seen[r.Job.Path], r.Seq)
		seenMu.Unlock()
	})
	defer q.Close()

	var landed int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				p := paths[(w+i)%len(paths)]
				counter := active[p]
				res := q.Submit(Job{
					Path: p,
					Render: func() ([]byte, error) {
						// A single drain goroutine should make it
						// impossible for two renders of the same path
						// to ever be in flight at once. Detect a
						// violation directly rather than trusting the
						// design.
						if n := atomic.AddInt32(counter, 1); n != 1 {
							t.Errorf("interleaving on %s: %d renders in flight", p, n)
						}
						time.Sleep(time.Microsecond)
						atomic.AddInt32(counter, -1)
						return []byte(p), nil
					},
				})
				if res.Err != nil {
					t.Errorf("job for %s failed: %v", p, res.Err)
				}
				atomic.AddInt64(&landed, 1)
			}
		}(w)
	}
	wg.Wait()

	if landed != wantTotal {
		t.Fatalf("landed = %d jobs, want %d", landed, wantTotal)
	}

	total := 0
	for _, p := range paths {
		seqs := seen[p]
		total += len(seqs)
		for i, s := range seqs {
			if want := uint64(i + 1); s != want {
				t.Fatalf("path %s: sequence out of order at position %d: got %d want %d (full: %v)", p, i, s, want, seqs)
			}
		}
	}
	if total != wantTotal {
		t.Fatalf("hook observed %d jobs across %d paths, want %d", total, len(paths), wantTotal)
	}
}
