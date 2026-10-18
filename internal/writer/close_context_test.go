package writer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestQueueCloseContextRetainsRunningJobUntilActualDrain(t *testing.T) {
	q := NewQueue(nil)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); q.Close() })
	finished := make(chan Result, 1)
	go func() {
		finished <- q.Submit(Job{Render: func() ([]byte, error) { close(started); <-release; return []byte("actual work completed"), nil }})
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := q.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("running close returned: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("close ignored cancellation")
	}
	select {
	case <-q.done:
		t.Fatal("cancel pretended running job drained")
	default:
	}
	if res := q.Submit(Job{Render: func() ([]byte, error) { t.Error("closed queue ran new job"); return nil, nil }}); !errors.Is(res.Err, ErrQueueClosed) {
		t.Fatalf("closed admission: %v", res.Err)
	}
	unblock()
	if res := <-finished; res.Err != nil || string(res.Bytes) != "actual work completed" {
		t.Fatalf("running job abandoned: %+v", res)
	}
	if err := q.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-q.done:
	default:
		t.Fatal("successful close did not join drain")
	}
}

func TestQueueCloseContextCancellationBeforeAdmissionDoesNotCloseQueue(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.CloseContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if res := q.Submit(Job{Render: func() ([]byte, error) { return []byte("still admitted"), nil }}); res.Err != nil {
		t.Fatal("cancelled pre-admission close changed queue", res.Err)
	}
}
