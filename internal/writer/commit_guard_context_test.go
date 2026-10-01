package writer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFlushContextCancelsWhileProviderHoldsRunLock(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	started := make(chan struct{})
	releaseProvider := make(chan struct{})
	var releaseOnce sync.Once
	releaseProviderJob := func() { releaseOnce.Do(func() { close(releaseProvider) }) }
	t.Cleanup(func() {
		releaseProviderJob()
		q.Close()
	})
	providerDone := make(chan Result, 1)
	go func() {
		providerDone <- q.Submit(Job{Render: func() ([]byte, error) {
			close(started)
			<-releaseProvider
			return nil, nil
		}})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	flushDone := make(chan error, 1)
	go func() {
		_, err := FlushContext(ctx, q, root)
		flushDone <- err
	}()
	// The exclusive checker never takes runLock, so it can enter while this
	// provider-only job is still blocked under that lock.
	checkErr, acquireErr := q.WithCommitFence(context.Background(), func(context.Context) error { return nil })
	if err := errors.Join(checkErr, acquireErr); err != nil {
		t.Fatalf("checker fence while provider blocked: %v", err)
	}
	cancel()
	select {
	case err := <-flushDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("FlushContext error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FlushContext did not cancel while waiting for provider-held runLock")
	}
	releaseProviderJob()
	if result := <-providerDone; result.Err != nil {
		t.Fatalf("provider job: %v", result.Err)
	}
}

func TestFlushContextReleasesRunLockWhileExclusiveFenceWaits(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFence()

	ctx := &flushPhaseContext{Context: context.Background(), sharedTry: make(chan struct{})}
	flushDone := make(chan error, 1)
	go func() {
		_, err := FlushContext(ctx, q, root)
		flushDone <- err
	}()
	select {
	case <-ctx.sharedTry:
	case <-time.After(time.Second):
		t.Fatal("FlushContext did not reach the shared-fence attempt while holding runLock")
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := q.runMu.lockContext(lockCtx); err != nil {
		t.Fatalf("FlushContext retained runLock while waiting for exclusive fence: %v", err)
	}
	q.runMu.unlock()
	select {
	case err := <-flushDone:
		t.Fatalf("FlushContext completed under exclusive fence: %v", err)
	default:
	}
	releaseFence()
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("FlushContext after fence release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FlushContext did not resume after exclusive fence release")
	}
}

type flushPhaseContext struct {
	context.Context
	calls     atomic.Int32
	sharedTry chan struct{}
	once      sync.Once
}

func (c *flushPhaseContext) Err() error {
	if c.calls.Add(1) == 2 {
		c.once.Do(func() { close(c.sharedTry) })
	}
	return c.Context.Err()
}
