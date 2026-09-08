package writer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompactionPublicationDeletionIsNarrowAndExact(t *testing.T) {
	for _, kind := range []string{"ordinary", "entity", "base", "archive", "changed", "segment"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := "brain/claims/ava/has_balance.1.jsonl"
			switch kind {
			case "entity":
				path = "brain/entities/person/ava.md"
			case "base":
				path = "brain/claims/ava/has_balance.jsonl"
			case "archive":
				path = "brain/claims/ava/has_balance.archive.jsonl"
			}
			full := filepath.Join(root, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				t.Fatal(err)
			}
			before := []byte("original\n")
			actual := before
			if kind == "changed" {
				actual = []byte("human edit\n")
			}
			if err := os.WriteFile(full, actual, 0600); err != nil {
				t.Fatal(err)
			}
			q := NewQueue(nil)
			defer q.Close()
			changes := []FileChange{{Path: path, Before: before, After: nil}}
			var err error
			if kind == "ordinary" {
				err = PublishFiles(q, root, changes)
			} else {
				err = PublishCompactionFiles(q, root, changes)
			}
			if kind == "segment" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(full); !os.IsNotExist(err) {
					t.Fatal("segment retained")
				}
				if err := PublishCompactionFiles(q, root, changes); err != nil {
					t.Fatalf("retry deletion: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe deletion accepted")
				}
				got, err := os.ReadFile(full)
				if err != nil || string(got) != string(actual) {
					t.Fatal("refused target changed")
				}
			}
		})
	}
}

func TestFlushRetriesAlreadyStagedDeletionAndCompletedDeletion(t *testing.T) {
	root, git := gitRepoFixture(t)
	git("config", "core.hooksPath", "/dev/null")
	q := NewQueue(nil)
	defer q.Close()
	path := filepath.Join(root, "seed.txt")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	git("add", "seed.txt")
	q.MarkTouched(path)
	if committed, err := Flush(q, root); err != nil || !committed {
		t.Fatalf("staged deletion committed=%v err=%v", committed, err)
	}
	head := git("rev-parse", "HEAD")
	q.MarkTouched(path)
	if committed, err := Flush(q, root); err != nil || committed {
		t.Fatalf("completed deletion retry committed=%v err=%v", committed, err)
	}
	if git("rev-parse", "HEAD") != head {
		t.Fatal("retry duplicated commit")
	}
	// An already completed deleted path does not prevent an unrelated owned write.
	next := filepath.Join(root, "next.txt")
	writeJob(t, q, next, []byte("next"))
	q.MarkTouched(path)
	if committed, err := Flush(q, root); err != nil || !committed {
		t.Fatalf("next write committed=%v err=%v", committed, err)
	}
}
