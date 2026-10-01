package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestForgetOperationWireContract(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	cancel := func(key string) cancelOperationResponse {
		t.Helper()
		v, bad, err := h.cancelOperation(ctx, mustMarshal(t, map[string]string{"operation_key": key}))
		if err != nil || bad {
			t.Fatalf("cancel: %v %+v", err, v)
		}
		return v.(cancelOperationResponse)
	}
	missing := cancel("missing")
	if !missing.Canceled || missing.ID != "" || missing.Expired {
		t.Fatalf("missing cancellation: %+v", missing)
	}
	req := rememberRequest{OperationKey: "missing", Fact: "must not be created", Provenance: "test"}
	v, bad, err := h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, v, bad).Error != "operation_canceled" {
		t.Fatal("missing operation resurrected", err)
	}
	req.OperationKey = "present"
	v, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatal(err)
	}
	id := v.(rememberResponse).ID
	canceled := cancel("present")
	if !canceled.Canceled || !canceled.Expired || canceled.ID != id {
		t.Fatalf("present cancellation: %+v", canceled)
	}
	if cancel("present").Expired {
		t.Fatal("repeated cancellation reported new expiry")
	}
	v, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatal(err)
	}
	recovered := v.(rememberResponse)
	if recovered.ID != id || !recovered.Expired || recovered.SearchState != "not_eligible" {
		t.Fatalf("recovery: %+v", recovered)
	}
}

func TestCancelOperationHonorsContextWhileWaitingForCommitFence(t *testing.T) {
	h, _ := newTestHandlers(t)
	leave, err := h.deps.Queue.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var leaveOnce sync.Once
	releaseFence := func() { leaveOnce.Do(leave) }
	t.Cleanup(releaseFence)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	type callResult struct {
		bad bool
		err error
	}
	done := make(chan callResult, 1)
	args := mustMarshal(t, map[string]string{"operation_key": "ctx-cancel-fence-01"})
	go func() {
		_, bad, err := h.cancelOperation(ctx, args)
		done <- callResult{bad: bad, err: err}
	}()

	var got callResult
	select {
	case got = <-done:
	case <-time.After(time.Second):
		releaseFence()
		got = <-done
		projection, loadErr := store.LoadMemoryProjection(h.deps.Sources)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		_, markerWritten := projection.OperationCancellation("ctx-cancel-fence-01")
		t.Fatalf("cancel handler ignored request cancellation while waiting for fence: after fence release err=%v bad=%v markerWritten=%v", got.err, got.bad, markerWritten)
	}
	if !errors.Is(got.err, context.DeadlineExceeded) || got.bad {
		t.Fatalf("cancel handler result = bad:%v err:%v, want request deadline error", got.bad, got.err)
	}
	projection, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, markerWritten := projection.OperationCancellation("ctx-cancel-fence-01"); markerWritten {
		t.Fatal("canceled cancel handler wrote an operation marker")
	}
	releaseFence()
	projection, err = store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, markerWritten := projection.OperationCancellation("ctx-cancel-fence-01"); markerWritten {
		t.Fatal("canceled cancel handler wrote an operation marker after fence release")
	}
}
