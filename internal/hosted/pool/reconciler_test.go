package pool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/writer"
)

type reconcilerTestEmbedder struct{}

func (reconcilerTestEmbedder) ModelVersion() string { return "reconciler-test@v1" }
func (reconcilerTestEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{0.25, 0.5}, nil
}

func newReconcilerTestPool(t *testing.T, maxOpen, maxInFlight int) (*Pool, string, string) {
	t.Helper()
	isolateCanonicalTestGit(t)
	brainsRoot := t.TempDir()
	brainID := "b123456789abcdef"
	otherBrainID := "b234567890abcdef"
	for _, id := range []string{brainID, otherBrainID} {
		brainRoot := filepath.Join(brainsRoot, id)
		if err := os.Mkdir(brainRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		initializeCanonicalTestGit(t, brainRoot)
	}
	p, err := New(Config{
		MaxOpen:     maxOpen,
		MaxInFlight: maxInFlight,
		IdleTimeout: time.Hour,
		BrainsRoot:  brainsRoot,
		Embedder:    reconcilerTestEmbedder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("close pool: %v", err)
		}
	})
	return p, brainID, otherBrainID
}

func initializeCanonicalTestGit(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"init", "--initial-branch=main"},
		{"add", "--", ".gitignore"},
		{"-c", "user.name=Serenity Hosted", "-c", "user.email=hosted@serenity.sire.run", "commit", "--only", "-m", "Initialize hosted brain", "--", ".gitignore"},
	}
	for _, args := range commands {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("prepare canonical Git fixture with %v: %v: %s", args, err, output)
		}
	}
}

func TestPoolBrainsRootReturnsConfiguredImmutableRoot(t *testing.T) {
	root := t.TempDir()
	p, err := New(Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: root, Embedder: reconcilerTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.BrainsRoot(); got != root {
		t.Fatalf("BrainsRoot()=%q, want %q", got, root)
	}
	var nilPool *Pool
	if got := nilPool.BrainsRoot(); got != "" {
		t.Fatalf("nil pool BrainsRoot()=%q, want empty", got)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

type observedDoneContext struct {
	context.Context
	calls     atomic.Int32
	threshold int32
	observed  chan struct{}
	once      sync.Once
}

func newObservedDoneContext(ctx context.Context, threshold int32) *observedDoneContext {
	return &observedDoneContext{Context: ctx, threshold: threshold, observed: make(chan struct{})}
}

func (c *observedDoneContext) Done() <-chan struct{} {
	if c.calls.Add(1) >= c.threshold {
		c.once.Do(func() { close(c.observed) })
	}
	return c.Context.Done()
}

func TestReconcilerLeasesExactRuntimeAcrossCheckAndCommit(t *testing.T) {
	ctx := context.Background()
	p, brainID, otherBrainID := newReconcilerTestPool(t, 1, 4)
	reconciler := NewReconciler(p)
	rec := contracts.OperationRecord{ID: "reconcile-operation", BrainID: brainID, Source: "gateway.remember"}

	verdict, err := reconciler.Check(ctx, rec)
	if !errors.Is(err, contracts.ErrBrainNotQuiescent) || verdict.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("check without fence = %+v, %v; want fail-closed Unknown", verdict, err)
	}

	releaseFence, err := reconciler.Fence(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseFence)

	runtime, releaseRuntime, err := p.Acquire(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseRuntime)
	if got := reconciler.held[brainID]; got == nil || got.runtime != runtime {
		t.Fatal("fence and checker do not retain the exact leased Runtime")
	}

	if err := p.Drop(brainID); !errors.Is(err, ErrCapacity) {
		t.Fatalf("Drop while reconciliation fence is held = %v; want ErrCapacity", err)
	}
	if _, _, err := p.Acquire(ctx, otherBrainID); !errors.Is(err, ErrCapacity) {
		t.Fatalf("eviction while reconciliation fence is held = %v; want ErrCapacity", err)
	}

	verdict, err = reconciler.Check(ctx, rec)
	if err != nil || verdict.Outcome != contracts.CanonicalUnknown || verdict.Ref != "working_tree_absence_unverified" {
		t.Fatalf("fenced canonical check = %+v, %v; want conservative Unknown", verdict, err)
	}
	wrongBrain := rec
	wrongBrain.BrainID = otherBrainID
	if verdict, err = reconciler.Check(ctx, wrongBrain); !errors.Is(err, contracts.ErrBrainNotQuiescent) || verdict.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("check for an unfenced brain = %+v, %v; want fail-closed Unknown", verdict, err)
	}

	ready := newObservedDoneContext(ctx, 4)
	rendered := make(chan struct{})
	submitted := make(chan writer.Result, 1)
	go func() {
		submitted <- runtime.queue.SubmitCanonical(ready, writer.Job{Render: func() ([]byte, error) {
			close(rendered)
			return nil, nil
		}})
	}()
	select {
	case <-ready.observed:
	case <-time.After(3 * time.Second):
		t.Fatal("canonical submission did not reach the queue's fenced commit wait")
	}
	select {
	case <-rendered:
		t.Fatal("canonical Render ran while reconciliation fence was held")
	default:
	}

	releaseFence()
	select {
	case result := <-submitted:
		if result.Err != nil {
			t.Fatalf("canonical submission after release: %v", result.Err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canonical submission did not resume after reconciliation release")
	}
	select {
	case <-rendered:
	default:
		t.Fatal("canonical Render did not run after fence release")
	}

	verdict, err = reconciler.Check(ctx, rec)
	if !errors.Is(err, contracts.ErrBrainNotQuiescent) || verdict.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("check after release = %+v, %v; want fail-closed Unknown", verdict, err)
	}
	releaseRuntime()
	if err := p.Drop(brainID); err != nil {
		t.Fatalf("Drop after all leases released: %v", err)
	}
}

func TestReconcilerCancellationReleasesAcquiredRuntimeLease(t *testing.T) {
	p, brainID, _ := newReconcilerTestPool(t, 1, 2)
	reconciler := NewReconciler(p)
	runtime, releaseRuntime, err := p.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	leaveCommit, err := runtime.EnterCommit(context.Background(), brainID)
	if err != nil {
		releaseRuntime()
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type fenceResult struct {
		release func()
		err     error
	}
	observed := newObservedDoneContext(ctx, 1)
	result := make(chan fenceResult, 1)
	go func() {
		release, err := reconciler.Fence(observed, brainID)
		result <- fenceResult{release: release, err: err}
	}()
	completed := false
	var lateRelease func()
	t.Cleanup(func() {
		cancel()
		leaveCommit()
		if lateRelease != nil {
			lateRelease()
		}
		if !completed {
			select {
			case outcome := <-result:
				if outcome.release != nil {
					outcome.release()
				}
			case <-time.After(3 * time.Second):
				t.Error("canceled fence goroutine did not settle during cleanup")
			}
		}
		releaseRuntime()
	})
	select {
	case <-observed.observed:
	case <-time.After(3 * time.Second):
		t.Fatal("exclusive reconciliation fence did not wait for the open commit section")
	}
	cancel()
	select {
	case outcome := <-result:
		completed = true
		lateRelease = outcome.release
		if !errors.Is(outcome.err, context.Canceled) || !errors.Is(outcome.err, contracts.ErrBrainNotQuiescent) {
			t.Fatalf("canceled reconciliation fence = %v; want cancellation and ErrBrainNotQuiescent", outcome.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled reconciliation fence did not return")
	}

	p.mu.Lock()
	users := p.open[brainID].users
	p.mu.Unlock()
	if users != 1 {
		t.Fatalf("runtime lease users after canceled fence = %d; want only the original lease", users)
	}
	leaveCommit()
	releaseRuntime()
	completed = true
	if err := p.Drop(brainID); err != nil {
		t.Fatalf("Drop after canceled fence cleanup: %v", err)
	}
}

func TestReconcilerMapsPoolCapacityToNotQuiescent(t *testing.T) {
	p, brainID, _ := newReconcilerTestPool(t, 1, 1)
	reconciler := NewReconciler(p)
	_, release, err := p.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	if _, err = reconciler.Fence(context.Background(), brainID); !errors.Is(err, contracts.ErrBrainNotQuiescent) || !errors.Is(err, ErrCapacity) {
		t.Fatalf("fence at pool capacity = %v; want both ErrBrainNotQuiescent and ErrCapacity", err)
	}
	release()
}

func TestReconcilerCheckHonorsCancellation(t *testing.T) {
	p, brainID, _ := newReconcilerTestPool(t, 1, 2)
	reconciler := NewReconciler(p)
	release, err := reconciler.Fence(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	verdict, err := reconciler.Check(ctx, contracts.OperationRecord{ID: "cancel-check", BrainID: brainID, Source: "gateway.remember"})
	if !errors.Is(err, context.Canceled) || verdict.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("canceled canonical check = %+v, %v; want Unknown and context.Canceled", verdict, err)
	}
}

func TestExistingReconcilerNeverOpensOrReopensColdRuntime(t *testing.T) {
	p, brainID, _ := newReconcilerTestPool(t, 1, 4)
	r := NewExistingReconciler(p)
	ctx := context.Background()
	if err := os.RemoveAll(filepath.Join(p.BrainsRoot(), brainID, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Fence(ctx, brainID); !errors.Is(err, contracts.ErrBrainNotQuiescent) {
		t.Fatalf("cold runtime admitted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(p.BrainsRoot(), brainID, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cold brain initialized: %v", err)
	}
	initializeCanonicalTestGit(t, filepath.Join(p.BrainsRoot(), brainID))
	runtime, leave, err := p.Acquire(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	leave()
	release, err := r.Fence(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	if r.held[brainID].runtime != runtime {
		t.Fatal("existing adapter substituted runtime")
	}
	if err = p.Drop(brainID); !errors.Is(err, ErrCapacity) {
		t.Fatalf("held runtime dropped: %v", err)
	}
	release()
	if err = p.Drop(brainID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Fence(ctx, brainID); !errors.Is(err, contracts.ErrBrainNotQuiescent) {
		t.Fatalf("dropped runtime reopened: %v", err)
	}
	if p.open[brainID] != nil {
		t.Fatal("existing adapter recreated dropped runtime")
	}
}
