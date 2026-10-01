package writer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestSubmitCanonicalWaitsForExclusiveFenceAndReleasesBeforeHook(t *testing.T) {
	root, _ := gitRepoFixture(t)
	path := filepath.Join(root, "canonical.md")
	var q *Queue
	hookDone := make(chan error, 1)
	q = NewQueue(func(Result) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		checkErr, acquireErr := q.WithCommitFence(ctx, func(context.Context) error { return nil })
		hookDone <- errors.Join(checkErr, acquireErr)
	})
	t.Cleanup(q.Close)

	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	started := make(chan struct{})
	result := make(chan Result, 1)
	go func() {
		result <- q.SubmitCanonical(context.Background(), Job{Path: path, Render: func() ([]byte, error) {
			close(started)
			body := []byte("canonical source\n")
			return body, os.WriteFile(path, body, 0o644)
		}})
	}()
	select {
	case <-started:
		t.Fatal("canonical Render started while exclusive fence was held")
	case <-time.After(30 * time.Millisecond):
	}
	releaseExclusive()
	select {
	case got := <-result:
		if got.Err != nil {
			t.Fatalf("SubmitCanonical: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical job did not resume after fence release")
	}
	select {
	case err := <-hookDone:
		if err != nil {
			t.Fatalf("hook ran before shared guard was released: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue hook did not run")
	}
	if !q.touchedContains(path) {
		t.Fatalf("canonical source path was not retained for the later flush: %q", path)
	}
	committed, err := Flush(q, root)
	if err != nil || !committed {
		t.Fatalf("later Flush = (%v, %v), want committed source", committed, err)
	}
}

func TestSubmitCanonicalCancellationWhileWaitingFenceSkipsRender(t *testing.T) {
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	started := false
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	got := q.SubmitCanonical(ctx, Job{Render: func() ([]byte, error) {
		started = true
		return nil, nil
	}})
	if !errors.Is(got.Err, context.DeadlineExceeded) {
		t.Fatalf("SubmitCanonical error = %v, want deadline", got.Err)
	}
	releaseExclusive()
	if started {
		t.Fatal("canceled canonical job rendered")
	}
}

func TestSubmitCanonicalAndFlushContextDoNotDeadlock(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	canonicalDone := make(chan Result, 1)
	go func() {
		canonicalDone <- q.SubmitCanonical(context.Background(), Job{Path: filepath.Join(root, "source.md"), Render: func() ([]byte, error) {
			body := []byte("source\n")
			return body, os.WriteFile(filepath.Join(root, "source.md"), body, 0o644)
		}})
	}()
	flushDone := make(chan error, 1)
	go func() {
		_, err := FlushContext(context.Background(), q, root)
		flushDone <- err
	}()
	// Let both operations reach their competing shared-guard paths while the
	// exclusive fence remains held. Releasing it must let the run-lock-first
	// Flush protocol and the guard-first canonical job make progress.
	time.Sleep(25 * time.Millisecond)
	releaseExclusive()
	select {
	case got := <-canonicalDone:
		if got.Err != nil {
			t.Fatalf("canonical job: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical job deadlocked with FlushContext")
	}
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("FlushContext: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FlushContext deadlocked with SubmitCanonical")
	}
}

func TestAfterGuardReleasesSharedFenceButPreservesFlushOrder(t *testing.T) {
	root, _ := gitRepoFixture(t)
	var q *Queue
	hookDone := make(chan error, 1)
	q = NewQueue(func(Result) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		checkErr, acquireErr := q.WithCommitFence(ctx, func(context.Context) error { return nil })
		hookDone <- errors.Join(checkErr, acquireErr)
	})
	t.Cleanup(q.Close)
	postStarted := make(chan struct{})
	postRelease := make(chan struct{})
	var releaseOnce sync.Once
	releasePost := func() { releaseOnce.Do(func() { close(postRelease) }) }
	t.Cleanup(releasePost)
	jobDone := make(chan Result, 1)
	go func() {
		jobDone <- q.SubmitCanonical(context.Background(), Job{
			Render: func() ([]byte, error) { return []byte("source\n"), nil },
			AfterGuard: func() error {
				close(postStarted)
				<-postRelease
				return nil
			},
		})
	}()
	select {
	case <-postStarted:
	case <-time.After(time.Second):
		t.Fatal("canonical job did not reach its post-guard callback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	checkErr, acquireErr := q.WithCommitFence(ctx, func(context.Context) error { return nil })
	cancel()
	if err := errors.Join(checkErr, acquireErr); err != nil {
		t.Fatalf("exclusive checker could not acquire during post-guard callback: %v", err)
	}
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelFlush()
	_, flushErr := FlushContext(flushCtx, q, root)
	if !errors.Is(flushErr, context.DeadlineExceeded) {
		t.Fatalf("FlushContext while post-guard callback held runMu = %v, want deadline", flushErr)
	}
	releasePost()
	select {
	case got := <-jobDone:
		if got.Err != nil {
			t.Fatalf("SubmitCanonical: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical job did not complete after post-guard callback released")
	}
	select {
	case err := <-hookDone:
		if err != nil {
			t.Fatalf("hook did not run after both queue locks released: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue hook did not run")
	}
}

type fenceCheckingPurger struct{ q *Queue }

func (p fenceCheckingPurger) PurgeSource(context.Context, string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	checkErr, acquireErr := p.q.WithCommitFence(ctx, func(context.Context) error { return nil })
	return errors.Join(checkErr, acquireErr)
}

func TestForgetRunsIndexPurgeAfterCanonicalGuardRelease(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources, Index: fenceCheckingPurger{q: q}}
	fact, err := w.Remember(RememberInput{
		Fact: "guarded forget fixture", Provenance: "test",
		Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	forgetDone := make(chan error, 1)
	go func() {
		_, err := w.Forget(fact.Record.SHA256, "test", time.Now().UTC())
		forgetDone <- err
	}()
	select {
	case err := <-forgetDone:
		t.Fatalf("Forget completed while exclusive fence was held: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if _, _, err := sources.Read(fact.Record.SHA256); err != nil {
		t.Fatalf("Forget mutated source while exclusive fence was held: %v", err)
	}
	releaseExclusive()
	select {
	case err := <-forgetDone:
		if err != nil {
			t.Fatalf("Forget (including post-guard index purge): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Forget did not resume after exclusive fence release")
	}
}

func TestRememberContextCancellationBeforeFenceLeavesNoSource(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = w.RememberContext(ctx, RememberInput{
		Fact: "must not land while checker owns fence", Provenance: "test",
		Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld,
	}, time.Now().UTC())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RememberContext error = %v, want deadline", err)
	}
	releaseExclusive()
	facts, err := sources.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("canceled RememberContext created %d source(s)", len(facts))
	}
}

func TestCancelRemoteOperationContextCancellationBeforeFenceLeavesNoMarker(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := w.CancelRemoteOperationContext(ctx, "0123456789abcdef", "test", time.Now().UTC()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CancelRemoteOperationContext error = %v, want deadline", err)
	}
	releaseExclusive()
	projection, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, canceled := projection.OperationCancellation("0123456789abcdef"); canceled {
		t.Fatal("canceled operation wrote its cancellation marker")
	}
}

func TestTombstoneContextCancellationBeforeFencePreservesSource(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	sources := store.NewSourceStore(root)
	src, err := sources.Write([]byte("source to preserve"), domain.Source{Kind: "email", URI: "mail://submit-canonical/test"})
	if err != nil {
		t.Fatal(err)
	}
	w := SourceTombstone{Queue: q, Sources: sources}
	releaseFence, err := q.AcquireCommitFence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseExclusive := func() { releaseOnce.Do(releaseFence) }
	t.Cleanup(releaseExclusive)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := w.TombstoneContext(ctx, src.SHA256, time.Now().UTC()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("TombstoneContext error = %v, want deadline", err)
	}
	releaseExclusive()
	if _, _, err := sources.Read(src.SHA256); err != nil {
		t.Fatalf("canceled tombstone removed source: %v", err)
	}
}
