// Package writer is the single serialized write path for canonical brain
// files (RFC 0001 §7.7). FenceWriter and ShardStore (internal/store) stay
// pure render/parse/append primitives; after M0 every write to a
// canonical file goes through Queue.Submit via the entry points in this
// package (Fence, Shard) so concurrent callers can never interleave
// writes to the same file. The file-first CI gate (T0.2) enforces that
// this package -- plus internal/index/rebuild.go for the derived index --
// is the only caller of the canonical writers.
package writer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
)

// Job is one write submitted to the queue. Render performs the actual
// I/O (e.g. FenceWriter.WriteEntity or ShardStore.Append) and returns the
// bytes it landed, purely so callers and tests can observe the result --
// the queue itself never touches Path directly.
//
// Kind is an optional label naming the write (e.g. "fence", "shard",
// "memory-fact") for diagnostics only: it appears in the PanicError the
// queue returns when Render panics. It never affects ordering. When it is
// empty the queue falls back to Path, and then to Render's own symbol, so
// an unlabelled job is still locatable from the error alone.
type Job struct {
	Kind   string
	Path   string
	Render func() ([]byte, error)
}

// kind is the diagnostic name used in a PanicError: Kind, else Path, else
// the Render function's symbol (a closure inside the submitting function,
// which names the entry point that built the job).
func (j Job) kind() string {
	if j.Kind != "" {
		return j.Kind
	}
	if j.Path != "" {
		return j.Path
	}
	if j.Render == nil {
		return "unnamed"
	}
	if fn := runtime.FuncForPC(reflect.ValueOf(j.Render).Pointer()); fn != nil {
		return fn.Name()
	}
	return "unnamed"
}

// PanicError is the error a job's submitter receives when its Render
// panicked (CON-06). The drain goroutine recovers the panic so one bad
// write can never terminate the process -- on hosted that is every tenant
// at once -- and returns it here instead, carrying the job kind, the panic
// value and the stack captured at the recover site. Unwrap exposes Value
// when it is itself an error, so errors.Is/As keep working through it.
type PanicError struct {
	Kind  string
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("writer: job %s panicked: %v", e.Kind, e.Value)
}

func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Result is delivered back to the submitter once a job has landed.
type Result struct {
	Job   Job
	Seq   uint64 // 1-based, monotonically increasing per Path
	Bytes []byte
	Err   error
}

// SubmitAndFlushResult combines the ordinary job outcome with the inline
// publication result. Result.Err is nil only when both Render and the
// requested flush succeeded; Committed reports whether Git created a new
// commit (a successful no-op has Committed=false).
type SubmitAndFlushResult struct {
	Result    Result
	Committed bool
}

// Queue drains every submitted job through one goroutine, so no two
// writes -- even to different files -- ever execute concurrently. That
// trivially satisfies per-file ordering: jobs for a given path always
// run strictly one at a time, in the order they were submitted.
type Queue struct {
	mu     sync.Mutex
	runMu  sync.Mutex    // serializes complete writes with git publication
	commit commitGate    // excludes hosted canonical checkers from source+flush sections
	submit chan struct{} // serializes ordered sends without holding mu while they block
	closed bool
	seq    map[string]uint64
	jobs   chan submitted
	wg     sync.WaitGroup
	hook   func(Result)

	// touchedMu guards paths independently of mu and the ordered-submit token.
	touchedMu sync.Mutex
	touched   map[string]bool // paths written since the last Flush (§7.7 daemon commits)
}

type submitted struct {
	job       Job
	seq       uint64
	flushRoot string
	flushCtx  context.Context
	flush     bool
	state     *submissionState
	reply     chan submitResponse
}

type submitResponse struct {
	result    Result
	committed bool
}

type submissionState struct {
	mu       sync.Mutex
	started  bool
	canceled bool
}

func (s *submissionState) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.canceled {
		return false
	}
	s.started = true
	return true
}

func (s *submissionState) cancelBeforeStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return false
	}
	s.canceled = true
	return true
}

// NewQueue starts the drain goroutine. hook, if non-nil, is called from
// the drain goroutine after each job lands and before its result is
// delivered to the submitter -- tests use it to record per-path sequence
// numbers without adding fields to Job. hook must not call Submit on the
// same queue (it would deadlock the single drain goroutine).
func NewQueue(hook func(Result)) *Queue {
	q := &Queue{
		seq:     map[string]uint64{},
		touched: map[string]bool{},
		jobs:    make(chan submitted),
		submit:  make(chan struct{}, 1),
		hook:    hook,
	}
	q.submit <- struct{}{}
	q.wg.Add(1)
	go q.drain()
	return q
}

func (q *Queue) drain() {
	defer q.wg.Done()
	for s := range q.jobs {
		if s.flush && !s.state.begin() {
			res := Result{Job: s.job, Seq: s.seq, Err: s.flushCtx.Err()}
			if res.Err == nil {
				res.Err = context.Canceled
			}
			if q.hook != nil {
				q.hook(res)
			}
			s.reply <- submitResponse{result: res}
			continue
		}
		var committed bool
		var leaveCommit func()
		var err error
		if s.flush {
			leaveCommit, err = q.EnterCommit(s.flushCtx)
		}
		if err != nil {
			res := Result{Job: s.job, Seq: s.seq, Err: err}
			if q.hook != nil {
				q.hook(res)
			}
			s.reply <- submitResponse{result: res}
			continue
		}
		q.runMu.Lock()
		var b []byte
		if err == nil {
			b, err = run(s.job)
		}
		if err == nil && s.job.Path != "" {
			q.touchedMu.Lock()
			q.touched[s.job.Path] = true
			q.touchedMu.Unlock()
		}
		if err == nil && s.flush {
			committed, err = flushTouchedLocked(s.flushCtx, q, s.flushRoot)
		}
		res := Result{Job: s.job, Seq: s.seq, Bytes: b, Err: err}
		q.runMu.Unlock()
		if leaveCommit != nil {
			leaveCommit()
		}
		if q.hook != nil {
			q.hook(res)
		}
		s.reply <- submitResponse{result: res, committed: committed}
	}
}

// run executes one job's Render on the drain goroutine, converting a panic
// into a *PanicError so the goroutine -- and with it the process -- survives
// and the queue moves on to the next job. It is a plain function rather
// than inline in drain so the deferred recover scopes exactly one Render:
// runMu, the touched set and the reply channel are all handled by drain
// after this returns, on the normal path and the recovered path alike.
func run(j Job) (b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			b = nil
			err = &PanicError{Kind: j.kind(), Value: r, Stack: debug.Stack()}
		}
	}()
	return j.Render()
}

// Submit enqueues a job and blocks until it has landed. The per-path
// sequence number is assigned under the same lock that orders the
// channel send, so it always matches the order jobs are handed to the
// drain goroutine, even when many goroutines submit concurrently to the
// same path.
func (q *Queue) Submit(j Job) Result {
	<-q.submit
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		q.submit <- struct{}{}
		return Result{Job: j, Err: ErrQueueClosed}
	}
	if j.Render == nil {
		q.mu.Unlock()
		q.submit <- struct{}{}
		return Result{Job: j, Err: errors.New("writer: missing render callback")}
	}
	q.seq[j.Path]++
	seq := q.seq[j.Path]
	reply := make(chan submitResponse, 1)
	q.mu.Unlock()
	q.jobs <- submitted{job: j, seq: seq, reply: reply}
	q.submit <- struct{}{}
	return (<-reply).result
}

// SubmitAndFlush runs Render and publishes all touched paths before releasing
// the queue's run lock. It acquires the per-queue shared commit section before
// runMu, for the complete Render+flush interval. Do not wrap this call in
// EnterCommit: the guard is not recursive. Ordinary Submit intentionally does
// not acquire the commit guard; callers must route canonical mutation paths
// explicitly. Cancellation before the drain starts the job removes it without
// rendering. Once started, the call waits for a settled result while the
// context is propagated to the render closure and Git commands; closures must
// honor cancellation to keep that boundary bounded.
func (q *Queue) SubmitAndFlush(ctx context.Context, root string, j Job) SubmitAndFlushResult {
	if ctx == nil {
		return SubmitAndFlushResult{Result: Result{Job: j, Err: ErrNilCommitContext}}
	}
	select {
	case <-q.submit:
	case <-ctx.Done():
		return SubmitAndFlushResult{Result: Result{Job: j, Err: ctx.Err()}}
	}
	queued := false
	defer func() {
		if !queued {
			q.submit <- struct{}{}
		}
	}()
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return SubmitAndFlushResult{Result: Result{Job: j, Err: ErrQueueClosed}}
	}
	if j.Render == nil {
		q.mu.Unlock()
		return SubmitAndFlushResult{Result: Result{Job: j, Err: errors.New("writer: missing render callback")}}
	}
	key := j.Path
	q.seq[key]++
	seq := q.seq[key]
	reply := make(chan submitResponse, 1)
	state := &submissionState{}
	sub := submitted{job: j, seq: seq, flushRoot: root, flushCtx: ctx, flush: true, state: state, reply: reply}
	q.mu.Unlock()
	select {
	case q.jobs <- sub:
		queued = true
		q.submit <- struct{}{}
	case <-ctx.Done():
		q.mu.Lock()
		q.seq[key]--
		q.mu.Unlock()
		return SubmitAndFlushResult{Result: Result{Job: j, Seq: seq, Err: ctx.Err()}}
	}
	select {
	case response := <-reply:
		return SubmitAndFlushResult{Result: response.result, Committed: response.committed}
	case <-ctx.Done():
		if state.cancelBeforeStart() {
			return SubmitAndFlushResult{Result: Result{Job: j, Seq: seq, Err: ctx.Err()}}
		}
		response := <-reply
		return SubmitAndFlushResult{Result: response.result, Committed: response.committed}
	}
}

// takeTouched returns every path successfully written since the last call
// (or since the queue was created), and resets the set -- Flush uses this
// to scope its `git add` to exactly what the queue wrote, never a human
// edit sitting elsewhere in the working tree (§7.7).
func (q *Queue) takeTouched() []string {
	q.touchedMu.Lock()
	defer q.touchedMu.Unlock()
	if len(q.touched) == 0 {
		return nil
	}
	paths := make([]string, 0, len(q.touched))
	for p := range q.touched {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	q.touched = map[string]bool{}
	return paths
}

// MarkTouched records path as an additional touched path from inside a
// Job's Render callback -- safe without its own synchronization beyond
// touchedMu (already required for every other touched-set access) because
// Submit guarantees Render only ever runs on the single drain goroutine, so
// two Renders can never call this concurrently. Exists for a Render that
// writes more than one file, or whose final path is not knowable until
// Render itself runs -- e.g. internal/writer's MemoryFact entry point,
// where the content-addressed write's own path depends on a legacy id
// Render allocates under this same single-writer guarantee (T4.20); Job.Path
// alone cannot name it up front, so that Job leaves Path empty and calls
// this instead.
func (q *Queue) MarkTouched(path string) {
	if path == "" {
		return
	}
	q.touchedMu.Lock()
	q.touched[path] = true
	q.touchedMu.Unlock()
}

// Close stops accepting new jobs and waits for the drain goroutine to
// finish everything already queued. A Queue is not usable after Close.
var ErrQueueClosed = errors.New("writer: queue closed")

func (q *Queue) Close() {
	<-q.submit
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.jobs)
	}
	q.mu.Unlock()
	q.submit <- struct{}{}
	q.wg.Wait()
}
