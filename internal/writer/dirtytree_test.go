package writer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// gitRepo initializes root as a git repo with a fixed local identity, so
// commits succeed even on machines/CI runners with no global git config.
func gitRepo(t *testing.T, root string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Dirty Tree Test")
}

// gitCommitAll stages and commits every change in root -- test shorthand
// for "a human's edit becomes the new committed baseline".
func gitCommitAll(t *testing.T, root, message string) {
	t.Helper()
	add := exec.Command("git", "add", "-A")
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commit := exec.Command("git", "commit", "--quiet", "-m", message)
	commit.Dir = root
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// handEdit overwrites path directly, bypassing the writer entirely --
// simulating a human editing a canonical file (§7.2).
func handEdit(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDirtyTreeGuardNewFileNotDirty proves the guard never blocks a
// path git has no committed history for -- a first write to a brand-new
// page, and a second overwrite before that page is ever committed, both
// go straight through untouched (T0.4 acc: "new file not dirty").
func TestDirtyTreeGuardNewFileNotDirty(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)

	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	defer q.Close()

	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."
	path, rendered, err := Fence(q, fw, p)
	if err != nil {
		t.Fatalf("first write to a brand-new page must not be paused: %v", err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, rendered) {
		t.Fatalf("disk content does not match the first write")
	}

	// Still untracked (never committed) -- a second, different write must
	// also go through, not be treated as a conflict with itself.
	p2 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p2.Summary = "Runs engineering at Acme; promoted to VP."
	if _, _, err := Fence(q, fw, p2); err != nil {
		t.Fatalf("second write to a still-untracked page must not be paused: %v", err)
	}
	disk2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fw.RenderEntity(p2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk2, want) {
		t.Fatalf("second write did not land:\n--- got ---\n%s\n--- want ---\n%s", disk2, want)
	}
}

// TestDirtyTreeGuardFencePauses proves a committed fence page that a
// human has since hand-edited pauses the next machine write instead of
// racing it: the file on disk is left exactly as the human left it, and
// the write returns ErrDirtyTree.
func TestDirtyTreeGuardFencePauses(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)

	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	defer q.Close()

	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."
	path, _, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "seed alice-tan page")

	humanContent := []byte("a human edit that does not match any machine render\n")
	handEdit(t, path, humanContent)

	p2 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p2.Summary = "Runs engineering at Acme; promoted to VP."
	_, rendered, err := Fence(q, fw, p2)
	if !errors.Is(err, ErrDirtyTree) {
		t.Fatalf("Fence over a hand-edited committed page: err = %v, want ErrDirtyTree", err)
	}
	if rendered != nil {
		t.Fatalf("rendered = %q, want nil on a paused write", rendered)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, humanContent) {
		t.Fatalf("paused write must not touch the file:\n--- got ---\n%s\n--- want (human edit) ---\n%s", disk, humanContent)
	}

	if _, err := os.Stat(PendingPath(root, "alice-tan")); err != nil {
		t.Fatalf("pending record not written: %v", err)
	}
}

// TestDirtyTreeGuardShardPauses is TestDirtyTreeGuardFencePauses's twin
// for the shard entry point: a hand-edited, committed shard file pauses
// the next append rather than being raced.
func TestDirtyTreeGuardShardPauses(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)

	ss := store.NewShardStore(root)
	q := NewQueue(nil)
	defer q.Close()

	c := domain.Claim{
		SubjectSlug: "checking-acct", Predicate: "has_balance", Family: "has_balance",
		Object: "1200.00 usd", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}
	path, _, err := Shard(q, ss, c)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "seed checking-acct shard")

	humanContent := []byte(`{"id":"deadbeef","subject_slug":"checking-acct"}` + "\n")
	handEdit(t, path, humanContent)

	c2 := c
	c2.Object = "1300.00 usd"
	c2.Provenance.ObservedAt = c.Provenance.ObservedAt.Add(time.Hour)
	_, line, err := Shard(q, ss, c2)
	if !errors.Is(err, ErrDirtyTree) {
		t.Fatalf("Shard over a hand-edited committed shard: err = %v, want ErrDirtyTree", err)
	}
	if line != nil {
		t.Fatalf("line = %q, want nil on a paused write", line)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, humanContent) {
		t.Fatalf("paused write must not touch the file:\n--- got ---\n%s\n--- want (human edit) ---\n%s", disk, humanContent)
	}

	if _, err := os.Stat(PendingPath(root, "checking-acct-has_balance")); err != nil {
		t.Fatalf("pending record not written: %v", err)
	}
}

// TestDirtyTreeGuardPendingRecordBothSides asserts the pending record a
// paused write leaves behind actually holds both sides of the conflict
// (T0.4 acc), not just a marker file.
func TestDirtyTreeGuardPendingRecordBothSides(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)

	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	defer q.Close()

	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."
	path, _, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "seed alice-tan page")

	humanContent := []byte("a human edit that does not match any machine render\n")
	handEdit(t, path, humanContent)

	p2 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p2.Summary = "Runs engineering at Acme; promoted to VP."
	wantMachine, err := fw.RenderEntity(p2)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Fence(q, fw, p2); !errors.Is(err, ErrDirtyTree) {
		t.Fatalf("expected ErrDirtyTree, got %v", err)
	}

	raw, err := os.ReadFile(PendingPath(root, "alice-tan"))
	if err != nil {
		t.Fatalf("read pending record: %v", err)
	}
	var rec PendingRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("pending record is not valid JSON: %v\n%s", err, raw)
	}
	if rec.Path != path {
		t.Fatalf("rec.Path = %q, want %q", rec.Path, path)
	}
	if rec.Human != string(humanContent) {
		t.Fatalf("rec.Human = %q, want the on-disk human edit %q", rec.Human, humanContent)
	}
	if rec.Machine != string(wantMachine) {
		t.Fatalf("rec.Machine = %q, want the paused machine render %q", rec.Machine, wantMachine)
	}
	if rec.DetectedAt == "" {
		t.Fatal("rec.DetectedAt is empty")
	}
}

// TestDirtyTreeGuardResumeAfterClear proves a paused write is not
// stranded: once the conflict is resolved (the human's edit is
// committed, folding it into the tree's clean state) and the pending
// record is cleared, the next machine write to that path lands normally
// (T0.4 acc: "clearing it resumes the write").
func TestDirtyTreeGuardResumeAfterClear(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)

	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	defer q.Close()

	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."
	path, _, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "seed alice-tan page")

	humanContent := []byte("a human edit that does not match any machine render\n")
	handEdit(t, path, humanContent)

	p2 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p2.Summary = "Runs engineering at Acme; promoted to VP."
	if _, _, err := Fence(q, fw, p2); !errors.Is(err, ErrDirtyTree) {
		t.Fatalf("expected the first retry to pause, got %v", err)
	}

	// Resolve: the human's edit becomes the new committed baseline, and
	// the pending record is cleared (what M2's disposition import will
	// eventually automate).
	gitCommitAll(t, root, "human accepts their edit")
	pendingFile := PendingPath(root, "alice-tan")
	if err := os.Remove(pendingFile); err != nil {
		t.Fatalf("clear pending record: %v", err)
	}

	p3 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p3.Summary = "Runs engineering at Acme; promoted to VP; relocated to NYC."
	_, rendered, err := Fence(q, fw, p3)
	if err != nil {
		t.Fatalf("write after clearing the conflict must resume: %v", err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, rendered) {
		t.Fatalf("resumed write did not land:\n--- disk ---\n%s\n--- rendered ---\n%s", disk, rendered)
	}
	want, err := fw.RenderEntity(p3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, want) {
		t.Fatalf("resumed write does not match the fresh machine render:\n--- disk ---\n%s\n--- want ---\n%s", disk, want)
	}

	if _, err := os.Stat(pendingFile); !os.IsNotExist(err) {
		t.Fatalf("pending record should stay cleared after a successful resume, stat err = %v", err)
	}
}
