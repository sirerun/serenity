package writer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSubmitAndFlushPublishesBeforeHookAndLeavesGuards(t *testing.T) {
	root, run := gitRepoFixture(t)
	path := filepath.Join(root, "canonical.md")
	var q *Queue
	var hookMu sync.Mutex
	var hookErr error
	q = NewQueue(func(result Result) {
		if result.Err != nil {
			hookMu.Lock()
			hookErr = result.Err
			hookMu.Unlock()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, acquireErr := q.WithCommitFence(ctx, func(context.Context) error {
			committed := strings.TrimSpace(run("show", "HEAD:canonical.md"))
			if committed != "canonical fact" {
				return errors.New("hook observed source before flush")
			}
			return nil
		})
		hookMu.Lock()
		defer hookMu.Unlock()
		hookErr = acquireErr
	})
	defer q.Close()

	got := q.SubmitAndFlush(context.Background(), root, Job{
		Path: path,
		Render: func() ([]byte, error) {
			body := []byte("canonical fact\n")
			return body, os.WriteFile(path, body, 0o644)
		},
	})
	if got.Result.Err != nil || !got.Committed {
		t.Fatalf("SubmitAndFlush = %+v; want successful new commit", got)
	}
	if got.Result.Job.Path != path || got.Result.Seq != 1 {
		t.Fatalf("result identity = %+v, want path %q sequence 1", got.Result, path)
	}
	hookMu.Lock()
	defer hookMu.Unlock()
	if hookErr != nil {
		t.Fatalf("hook ran before guards released or flush completed: %v", hookErr)
	}
}

func TestSubmitAndFlushPreservesUnrelatedDirtyPaths(t *testing.T) {
	root, run := gitRepoFixture(t)
	if err := os.WriteFile(filepath.Join(root, "human-staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "human-staged.txt")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("human unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(root, "managed.txt")
	q := NewQueue(nil)
	defer q.Close()
	got := q.SubmitAndFlush(context.Background(), root, Job{
		Path: managed,
		Render: func() ([]byte, error) {
			body := []byte("managed\n")
			return body, os.WriteFile(managed, body, 0o644)
		},
	})
	if got.Result.Err != nil || !got.Committed {
		t.Fatalf("SubmitAndFlush = %+v; want committed managed path", got)
	}
	if paths := strings.Fields(run("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")); len(paths) != 1 || paths[0] != "managed.txt" {
		t.Fatalf("inline commit paths = %v, want only managed.txt", paths)
	}
	if staged := strings.TrimSpace(run("diff", "--cached", "--name-only")); staged != "human-staged.txt" {
		t.Fatalf("staged human path after flush = %q, want human-staged.txt", staged)
	}
	if unstaged := strings.TrimSpace(run("diff", "--name-only")); unstaged != "seed.txt" {
		t.Fatalf("unstaged human path after flush = %q, want seed.txt", unstaged)
	}
}

func TestSubmitAndFlushRequeuesTouchedPathsAfterCommitFailure(t *testing.T) {
	root, run := gitRepoFixture(t)
	hooks := filepath.Join(root, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run("config", "core.hooksPath", hooks)
	path := filepath.Join(root, "canonical.md")
	q := NewQueue(nil)
	defer q.Close()
	got := q.SubmitAndFlush(context.Background(), root, Job{
		Path: path,
		Render: func() ([]byte, error) {
			body := []byte("durable retry\n")
			return body, os.WriteFile(path, body, 0o644)
		},
	})
	if got.Result.Err == nil || got.Committed {
		t.Fatalf("SubmitAndFlush = %+v; want failed commit and no commit claim", got)
	}
	if !q.touchedContains(path) {
		t.Fatalf("failed commit did not restore touched path %q", path)
	}
	run("config", "core.hooksPath", "/dev/null")
	committed, err := Flush(q, root)
	if err != nil || !committed {
		t.Fatalf("retry Flush = (%v, %v), want successful commit", committed, err)
	}
	if body := strings.TrimSpace(run("show", "HEAD:canonical.md")); body != "durable retry" {
		t.Fatalf("committed body = %q", body)
	}
}

func TestSubmitAndFlushHonorsContextCancellation(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := q.SubmitAndFlush(ctx, root, Job{Render: func() ([]byte, error) {
		t.Fatal("canceled submission must not render")
		return nil, nil
	}})
	if !errors.Is(got.Result.Err, context.Canceled) || got.Committed {
		t.Fatalf("canceled SubmitAndFlush = %+v", got)
	}
}

func TestCommitPathsContextHonorsCancellation(t *testing.T) {
	root, run := gitRepoFixture(t)
	path := filepath.Join(root, "canceled.md")
	if err := os.WriteFile(path, []byte("must remain uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	committed, err := commitPathsContext(ctx, root, []string{path}, "serenity: canceled")
	if err == nil || committed {
		t.Fatalf("commitPathsContext(canceled) = (%v, %v), want error and no commit", committed, err)
	}
	if status := run("status", "--short"); !strings.Contains(status, "?? canceled.md") {
		t.Fatalf("canceled git add changed the index: status %q", status)
	}
}

func TestWaitingExclusiveFenceBlocksNewSharedCommitSections(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	leave, err := q.EnterCommit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fenceEntered := make(chan struct{})
	releaseFence := make(chan struct{})
	fenceDone := make(chan error, 1)
	go func() {
		checkErr, acquireErr := q.WithCommitFence(context.Background(), func(context.Context) error {
			close(fenceEntered)
			<-releaseFence
			return nil
		})
		fenceDone <- errors.Join(checkErr, acquireErr)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		q.commit.mu.Lock()
		waiting := q.commit.waitingWriter > 0
		q.commit.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exclusive fence never registered as waiting")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := q.EnterCommit(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared entrant while fence waited: err=%v, want deadline", err)
	}
	leave()
	select {
	case <-fenceEntered:
	case <-time.After(time.Second):
		t.Fatal("fence did not acquire after existing writer left")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel2()
	if _, err := q.EnterCommit(ctx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared entrant during fence: err=%v, want deadline", err)
	}
	close(releaseFence)
	if err := <-fenceDone; err != nil {
		t.Fatalf("fence result: %v", err)
	}
	leave2, err := q.EnterCommit(context.Background())
	if err != nil {
		t.Fatalf("shared entrant after fence: %v", err)
	}
	leave2()
}

func TestPlainQueueJobDoesNotBlockExclusiveCommitFence(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	started := make(chan struct{})
	releaseJob := make(chan struct{})
	jobDone := make(chan Result, 1)
	go func() {
		jobDone <- q.Submit(Job{Render: func() ([]byte, error) {
			close(started)
			<-releaseJob
			return nil, nil
		}})
	}()
	<-started
	fenceDone := make(chan error, 1)
	go func() {
		checkErr, acquireErr := q.WithCommitFence(context.Background(), func(context.Context) error { return nil })
		fenceDone <- errors.Join(checkErr, acquireErr)
	}()
	select {
	case err := <-fenceDone:
		if err != nil {
			t.Fatalf("fence: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("plain provider-style queue job blocked exclusive commit fence")
	}
	close(releaseJob)
	if result := <-jobDone; result.Err != nil {
		t.Fatalf("plain queue job: %v", result.Err)
	}
}

func TestWithCommitFenceSeparatesAcquisitionAndCheckerErrors(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()
	want := errors.New("checker failed")
	checkErr, acquireErr := q.WithCommitFence(context.Background(), func(context.Context) error { return want })
	if !errors.Is(checkErr, want) || acquireErr != nil {
		t.Fatalf("checker failure = (%v, %v)", checkErr, acquireErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checkErr, acquireErr = q.WithCommitFence(ctx, func(context.Context) error {
		t.Fatal("checker must not run when fence acquisition is canceled")
		return nil
	})
	if checkErr != nil || !errors.Is(acquireErr, context.Canceled) {
		t.Fatalf("acquisition failure = (%v, %v)", checkErr, acquireErr)
	}
}

func (q *Queue) touchedContains(path string) bool {
	q.touchedMu.Lock()
	defer q.touchedMu.Unlock()
	return q.touched[path]
}
