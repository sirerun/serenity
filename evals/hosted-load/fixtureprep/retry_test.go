package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

func TestRetryOwnedWaitsForARelease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "brain")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	held, err := writer.AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = held.Close()
	}()
	var retries atomic.Int64
	var got *os.File
	err = retryOwned(context.Background(), 5*time.Second, &retries, func() (e error) { got, e = writer.AcquireBrain(root); return e })
	if err != nil {
		t.Fatalf("the lock was released after 300 ms, so a 5 s window must succeed: %v", err)
	}
	_ = got.Close()
	if retries.Load() == 0 {
		t.Fatal("the wait must be counted")
	}
}

func TestRetryOwnedGivesUpAfterTheWindowAndOnlyForOwnership(t *testing.T) {
	root := filepath.Join(t.TempDir(), "brain")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	held, err := writer.AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	var retries atomic.Int64
	err = retryOwned(context.Background(), 200*time.Millisecond, &retries, func() error { _, e := writer.AcquireBrain(root); return e })
	if !errors.Is(err, writer.ErrBrainOwned) {
		t.Fatalf("a lock held past the window must still fail with ErrBrainOwned, got %v", err)
	}
	boom := errors.New("some other failure")
	calls := 0
	err = retryOwned(context.Background(), 5*time.Second, &retries, func() error { calls++; return boom })
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("only ErrBrainOwned is retried: %d calls, %v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = retryOwned(ctx, 5*time.Second, &retries, func() error { _, e := writer.AcquireBrain(root); return e }); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context must stop the wait, got %v", err)
	}
}
