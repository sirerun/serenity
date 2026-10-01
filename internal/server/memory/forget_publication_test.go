package memory

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

func TestHostedForgetPublishesErasureBeforeHandlerReturns(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()
	remembered, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{
		Fact:       "hosted forget source boundary marker",
		Provenance: "fixture",
	}))
	if err != nil || isError {
		t.Fatalf("remember: result=%+v isError=%v err=%v", remembered, isError, err)
	}
	id := remembered.(rememberResponse).ID
	if _, err := writer.Flush(h.deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	forgotten, isError, err := h.forget(WithHostedForgetPublication(ctx), mustMarshal(t, map[string]string{"id": id}))
	if err != nil || isError || !forgotten.(forgetResponse).Expired {
		t.Fatalf("forget: result=%+v isError=%v err=%v", forgotten, isError, err)
	}
	path := "brain/sources/" + id[:2] + "/" + id
	cmd := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", "HEAD", "--", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect committed forget result: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("handler returned before erasure reached HEAD: %q", out)
	}
}

func TestLocalForgetHonorsRequestCancellationWhileWaitingForFence(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	remembered, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{Fact: "local forget cancellation fixture", Provenance: "fixture"}))
	if err != nil || isError {
		t.Fatalf("remember: result=%+v isError=%v err=%v", remembered, isError, err)
	}
	id := remembered.(rememberResponse).ID
	leave, err := h.deps.Queue.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var leaveOnce sync.Once
	releaseFence := func() { leaveOnce.Do(leave) }
	t.Cleanup(releaseFence)
	requestCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	args := mustMarshal(t, map[string]string{"id": id, "reason": "request canceled"})
	type callResult struct{ err error }
	done := make(chan callResult, 1)
	go func() {
		_, _, err := h.forget(requestCtx, args)
		done <- callResult{err: err}
	}()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("local forget error = %v, want request deadline", got.err)
		}
	case <-time.After(time.Second):
		releaseFence()
		got := <-done
		t.Fatalf("local forget ignored request cancellation; after fence release it returned %v", got.err)
	}
	if _, _, err := h.deps.Sources.Read(id); err != nil {
		t.Fatalf("canceled local forget removed source before fence release: %v", err)
	}
	releaseFence()
	if _, _, err := h.deps.Sources.Read(id); err != nil {
		t.Fatalf("canceled local forget removed source after fence release: %v", err)
	}
}
