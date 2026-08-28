package writer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepoFixture initializes a real git repo with one seed commit, so
// Flush's git plumbing (add/diff/commit) has real history to operate
// against.
func gitRepoFixture(t *testing.T) (root string, run func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root = t.TempDir()

	run = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "--quiet")
	run("config", "user.email", "flush-test@example.com")
	run("config", "user.name", "flush test")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "seed.txt")
	run("commit", "--quiet", "-m", "seed")
	return root, run
}

// writeJob submits a job that writes the given content to path through
// the queue, so the queue's touched-path tracking sees it exactly as
// writer.Fence/writer.Shard would.
func writeJob(t *testing.T, q *Queue, path string, content []byte) {
	t.Helper()
	res := q.Submit(Job{
		Path: path,
		Render: func() ([]byte, error) {
			return content, os.WriteFile(path, content, 0o644)
		},
	})
	if res.Err != nil {
		t.Fatalf("submit %s: %v", path, res.Err)
	}
}

func TestQueueFlushCommitsWithSerenityPrefix(t *testing.T) {
	root, run := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()

	writeJob(t, q, filepath.Join(root, "queue-managed.md"), []byte("machine content\n"))

	committed, err := Flush(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("expected Flush to report a commit was made")
	}

	subject := strings.TrimSpace(run("log", "-1", "--format=%s"))
	if !strings.HasPrefix(subject, "serenity:") {
		t.Fatalf("commit subject %q does not start with the required serenity: prefix", subject)
	}
}

func TestQueueFlushNoopWhenNothingTouched(t *testing.T) {
	root, run := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()

	before := strings.TrimSpace(run("rev-parse", "HEAD"))

	committed, err := Flush(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if committed {
		t.Fatal("expected Flush to be a noop when the queue wrote nothing")
	}
	after := strings.TrimSpace(run("rev-parse", "HEAD"))
	if before != after {
		t.Fatalf("noop Flush must not create a commit: HEAD moved from %s to %s", before, after)
	}
}

// TestQueueFlushScopesToTouchedPaths is the negative-space fixture for
// §7.7's "the human's file state is truth": a human edit dirty in the
// working tree but never touched by the queue must not be swept into a
// daemon commit by a naive `git add -A`/`.`.
func TestQueueFlushScopesToTouchedPaths(t *testing.T) {
	root, run := gitRepoFixture(t)

	untouched := filepath.Join(root, "human-edit-untouched.md")
	if err := os.WriteFile(untouched, []byte("first draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "human-edit-untouched.md")
	run("commit", "--quiet", "-m", "seed the human-authored file")
	// Now dirty it, uncommitted -- exactly like a human mid-edit when the
	// daemon's flush timer fires.
	if err := os.WriteFile(untouched, []byte("human is mid-edit, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := NewQueue(nil)
	defer q.Close()
	writeJob(t, q, filepath.Join(root, "queue-managed.md"), []byte("machine content\n"))

	committed, err := Flush(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("expected Flush to report a commit was made")
	}

	// `git show --stat` lists every path the commit touched.
	stat := run("show", "--stat", "--format=", "HEAD")
	if strings.Contains(stat, "human-edit-untouched.md") {
		t.Fatalf("flush commit swept in a file the queue never touched:\n%s", stat)
	}

	status := run("status", "--porcelain")
	if !strings.Contains(status, "human-edit-untouched.md") {
		t.Fatalf("expected the human's uncommitted edit to remain dirty and untouched, status:\n%s", status)
	}
}
