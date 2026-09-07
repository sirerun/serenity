package writer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestMemoryFlushRetriesOnlyExactOwnedFiles(t *testing.T) {
	root, git := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	input := RememberInput{Fact: "keep my attribution", Provenance: "author", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	result, err := w.Remember(input, now)
	if err != nil {
		t.Fatal(err)
	}
	owned := sources.DirFor(result.Record.SHA256)
	intruder := filepath.Join(owned, "human-note.txt")
	if err := os.WriteFile(intruder, []byte("human draft"), 0644); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(root, "unrelated.txt")
	if err := os.WriteFile(staged, []byte("already staged human edit"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "unrelated.txt")
	hooks := filepath.Join(root, ".git", "review-hooks")
	if err := os.Mkdir(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	git("config", "core.hooksPath", hooks)
	before := git("rev-parse", "HEAD")
	if _, err := Flush(q, root); err == nil {
		t.Fatal("forced commit failure was not reported")
	}
	if git("rev-parse", "HEAD") != before {
		t.Fatal("failed commit changed history")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	committed, err := Flush(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("retry lost canonical touched paths")
	}
	files := strings.Fields(git("show", "--pretty=format:", "--name-only", "HEAD"))
	if len(files) != 2 || !strings.HasSuffix(files[0], "/bytes") || !strings.HasSuffix(files[1], "/meta.yaml") {
		t.Fatalf("commit swept unintended paths: %v", files)
	}
	if strings.TrimSpace(git("diff", "--cached", "--name-only")) != "unrelated.txt" {
		t.Fatal("unrelated staged state was not preserved")
	}
	rel, _ := filepath.Rel(root, intruder)
	if git("ls-files", "--", rel) != "" {
		t.Fatal("human file beneath source directory was committed")
	}
	// A new process reconstructs its obligation on duplicate retry without making
	// another canonical record, even when the original queue no longer exists.
	q.Close()
	q2 := NewQueue(nil)
	defer q2.Close()
	w.Queue = q2
	again, err := w.Remember(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted || again.Record.SHA256 != result.Record.SHA256 {
		t.Fatal("duplicate retry allocated another fact")
	}
	if committed, err := Flush(q2, root); err != nil || committed {
		t.Fatalf("committed duplicate should be no-op: committed=%v error=%v", committed, err)
	}
}

func TestMemoryQueueCloseJoinsWritesAndRejectsFurtherJobs(t *testing.T) {
	q := NewQueue(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	submitted := make(chan Result, 1)
	go func() {
		submitted <- q.Submit(Job{Render: func() ([]byte, error) { close(entered); <-release; return []byte("durable"), nil }})
	}()
	<-entered
	closed := make(chan struct{})
	go func() { q.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned before in-flight write finished")
	default:
	}
	close(release)
	if r := <-submitted; r.Err != nil || string(r.Bytes) != "durable" {
		t.Fatalf("in-flight write failed: %+v", r)
	}
	<-closed
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); q.Close() }()
	}
	wg.Wait()
	called := false
	r := q.Submit(Job{Render: func() ([]byte, error) { called = true; return nil, nil }})
	if !errors.Is(r.Err, ErrQueueClosed) || called {
		t.Fatal("closed queue accepted another write")
	}
}
