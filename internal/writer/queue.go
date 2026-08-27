// Package writer is the single serialized write path for canonical brain
// files (RFC 0001 §7.7). FenceWriter and ShardStore (internal/store) stay
// pure render/parse/append primitives; after M0 every write to a
// canonical file goes through Queue.Submit via the entry points in this
// package (Fence, Shard) so concurrent callers can never interleave
// writes to the same file. The file-first CI gate (T0.2) enforces that
// this package -- plus internal/index/rebuild.go for the derived index --
// is the only caller of the canonical writers.
package writer

import "sync"

// Job is one write submitted to the queue. Render performs the actual
// I/O (e.g. FenceWriter.WriteEntity or ShardStore.Append) and returns the
// bytes it landed, purely so callers and tests can observe the result --
// the queue itself never touches Path directly.
type Job struct {
	Path   string
	Render func() ([]byte, error)
}

// Result is delivered back to the submitter once a job has landed.
type Result struct {
	Job   Job
	Seq   uint64 // 1-based, monotonically increasing per Path
	Bytes []byte
	Err   error
}

// Queue drains every submitted job through one goroutine, so no two
// writes -- even to different files -- ever execute concurrently. That
// trivially satisfies per-file ordering: jobs for a given path always
// run strictly one at a time, in the order they were submitted.
type Queue struct {
	mu   sync.Mutex
	seq  map[string]uint64
	jobs chan submitted
	wg   sync.WaitGroup
	hook func(Result)
}

type submitted struct {
	job   Job
	seq   uint64
	reply chan Result
}

// NewQueue starts the drain goroutine. hook, if non-nil, is called from
// the drain goroutine after each job lands and before its result is
// delivered to the submitter -- tests use it to record per-path sequence
// numbers without adding fields to Job. hook must not call Submit on the
// same queue (it would deadlock the single drain goroutine).
func NewQueue(hook func(Result)) *Queue {
	q := &Queue{
		seq:  map[string]uint64{},
		jobs: make(chan submitted),
		hook: hook,
	}
	q.wg.Add(1)
	go q.drain()
	return q
}

func (q *Queue) drain() {
	defer q.wg.Done()
	for s := range q.jobs {
		b, err := s.job.Render()
		res := Result{Job: s.job, Seq: s.seq, Bytes: b, Err: err}
		if q.hook != nil {
			q.hook(res)
		}
		s.reply <- res
	}
}

// Submit enqueues a job and blocks until it has landed. The per-path
// sequence number is assigned under the same lock that orders the
// channel send, so it always matches the order jobs are handed to the
// drain goroutine, even when many goroutines submit concurrently to the
// same path.
func (q *Queue) Submit(j Job) Result {
	q.mu.Lock()
	q.seq[j.Path]++
	seq := q.seq[j.Path]
	reply := make(chan Result, 1)
	q.jobs <- submitted{job: j, seq: seq, reply: reply}
	q.mu.Unlock()
	return <-reply
}

// Close stops accepting new jobs and waits for the drain goroutine to
// finish everything already queued. A Queue is not usable after Close.
func (q *Queue) Close() {
	close(q.jobs)
	q.wg.Wait()
}
