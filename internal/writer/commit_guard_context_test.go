package writer

import (
	"context"
	"errors"
	"runtime"
	"sync"
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

	// Occupy runLock first so the flush attempt is known to be queued on it.
	if err := q.runMu.lockContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.runMu.unlock)
	flushDone := make(chan error, 1)
	go func() {
		_, err := FlushContext(context.Background(), q, root)
		flushDone <- err
	}()
	deadline := time.After(time.Second)
	for {
		q.runMu.mu.Lock()
		waiters := q.runMu.waiters
		q.runMu.mu.Unlock()
		if waiters > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("FlushContext never queued for the occupied runLock")
		default:
			runtime.Gosched()
		}
	}
	q.runMu.unlock()

	deadline = time.After(time.Second)
	for {
		q.runMu.mu.Lock()
		held, waiters := q.runMu.held, q.runMu.waiters
		q.runMu.mu.Unlock()
		if !held && waiters == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("FlushContext retained runLock while waiting for exclusive fence")
		default:
			runtime.Gosched()
		}
	}
	select {
	case err := <-flushDone:
		t.Fatalf("FlushContext completed under exclusive fence: %v", err)
	default:
	}
	releaseFence()
	releaseFence = func() {}
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("FlushContext after fence release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FlushContext did not resume after exclusive fence release")
	}
}
