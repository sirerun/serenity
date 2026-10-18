package pool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

func assertLifecycleCancelled(t *testing.T, call func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := call(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lifecycle cancellation: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("lifecycle retained caller after cancellation")
	}
}

func assertClosingRuntimeUnavailable(t *testing.T, p *Pool, id string) {
	t.Helper()
	for name, acquire := range map[string]func(context.Context, string) (*Runtime, func(), error){"ordinary": p.Acquire, "canonical": p.AcquireCanonical, "existing": p.AcquireExisting, "inspector": p.AcquireExistingForRead} {
		r, release, err := acquire(context.Background(), id)
		if release != nil {
			release()
		}
		if r != nil || !errors.Is(err, ErrRuntimeClosing) {
			t.Fatalf("%s admitted closing runtime: %v", name, err)
		}
	}
}

func TestDropContextRetainsOwnershipDuringRunningQueueJob(t *testing.T) {
	p, id, _ := newReconcilerTestPool(t, 2, 4)
	r, releaseRuntime, err := p.Acquire(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	started, releaseJob := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(releaseJob) }) }
	t.Cleanup(unblock)
	done := make(chan writer.Result, 1)
	go func() {
		done <- r.Queue.Submit(writer.Job{Render: func() ([]byte, error) { close(started); <-releaseJob; return nil, nil }})
	}()
	<-started
	releaseRuntime()
	assertLifecycleCancelled(t, func(ctx context.Context) error { return p.DropContext(ctx, id) })
	assertClosingRuntimeUnavailable(t, p, id)
	owner, err := writer.AcquireBrain(r.Root)
	if owner != nil {
		_ = owner.Close()
	}
	if !errors.Is(err, writer.ErrBrainOwned) {
		t.Fatalf("cancelled drop released writer while work runs: %v", err)
	}
	unblock()
	if res := <-done; res.Err != nil {
		t.Fatal(res.Err)
	}
	if err = p.DropContext(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	owner, err = writer.AcquireBrain(r.Root)
	if err != nil {
		t.Fatal("completed drop retained writer", err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, release, err := p.AcquireCanonical(context.Background(), id)
	if err != nil {
		t.Fatal("completed drop could not reopen", err)
	}
	if reopened == r {
		t.Fatal("reopened a closed queue")
	}
	release()
}

func TestDropContextCommitFenceCancellationRetainsClosedQueueAndOwner(t *testing.T) {
	p, id, _ := newReconcilerTestPool(t, 2, 4)
	r, releaseRuntime, err := p.Acquire(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	releaseFence, err := r.Queue.AcquireCommitFence(context.Background())
	if err != nil {
		releaseRuntime()
		t.Fatal(err)
	}
	var once sync.Once
	unblock := func() { once.Do(releaseFence) }
	t.Cleanup(unblock)
	releaseRuntime()
	assertLifecycleCancelled(t, func(ctx context.Context) error { return p.DropContext(ctx, id) })
	assertClosingRuntimeUnavailable(t, p, id)
	owner, err := writer.AcquireBrain(r.Root)
	if owner != nil {
		_ = owner.Close()
	}
	if !errors.Is(err, writer.ErrBrainOwned) {
		t.Fatalf("flush cancellation released owner: %v", err)
	}
	if result := r.Queue.Submit(writer.Job{Render: func() ([]byte, error) { t.Error("closing queue ran job"); return nil, nil }}); !errors.Is(result.Err, writer.ErrQueueClosed) {
		t.Fatal("drained queue reopened admission", result.Err)
	}
	unblock()
	if err = p.DropContext(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

func TestFlushAllContextCancelsAtRunLockAndCommitFence(t *testing.T) {
	for _, phase := range []string{"run_lock", "commit_fence"} {
		t.Run(phase, func(t *testing.T) {
			p, id, _ := newReconcilerTestPool(t, 2, 4)
			r, releaseRuntime, err := p.Acquire(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			var unblock func()
			var jobDone chan writer.Result
			if phase == "run_lock" {
				started, releaseJob := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock = func() { once.Do(func() { close(releaseJob) }) }
				jobDone = make(chan writer.Result, 1)
				go func() {
					jobDone <- r.Queue.Submit(writer.Job{Render: func() ([]byte, error) { close(started); <-releaseJob; return nil, nil }})
				}()
				<-started
			} else {
				releaseFence, e := r.Queue.AcquireCommitFence(context.Background())
				if e != nil {
					releaseRuntime()
					t.Fatal(e)
				}
				var once sync.Once
				unblock = func() { once.Do(releaseFence) }
			}
			t.Cleanup(unblock)
			releaseRuntime()
			assertLifecycleCancelled(t, p.FlushAllContext)
			unblock()
			if jobDone != nil {
				if result := <-jobDone; result.Err != nil {
					t.Fatal(result.Err)
				}
			}
			if err = p.FlushAllContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			// Flush cancellation alone does not close a healthy queue/runtime.
			_, release, err := p.AcquireExisting(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			release()
		})
	}
}

func TestPoolLifecycleMutexWaitHonorsContext(t *testing.T) {
	p, id, _ := newReconcilerTestPool(t, 1, 1)
	for _, action := range []string{"flush", "drop"} {
		t.Run(action, func(t *testing.T) {
			p.mu.Lock()
			defer p.mu.Unlock()
			assertLifecycleCancelled(t, func(ctx context.Context) error {
				if action == "flush" {
					return p.FlushAllContext(ctx)
				}
				return p.DropContext(ctx, id)
			})
		})
	}
}
