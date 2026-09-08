package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishFilesResumesPartialTransition(t *testing.T) {
	root := t.TempDir()
	a, b := "brain/entities/topic/a.md", "brain/claims/a/balance.jsonl"
	put := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Simulates death after the first atomic rename but before the second.
	put(a, "new page")
	q := NewQueue(nil)
	defer q.Close()
	changes := []FileChange{{Path: a, Before: []byte("old page"), After: []byte("new page")}, {Path: b, After: []byte("new shard")}}
	for i := 0; i < 2; i++ {
		if err := PublishFiles(q, root, changes); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range changes {
		raw, err := os.ReadFile(filepath.Join(root, change.Path))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(change.After) {
			t.Fatalf("unexpected result %s", change.Path)
		}
	}
	if len(q.takeTouched()) != 2 {
		t.Fatal("resumed files were not both marked for commit")
	}
}

func TestPublishFilesPreflightsAllHumanEdits(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "brain/entities/topic")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("human revision"), 0644); err != nil {
		t.Fatal(err)
	}
	q := NewQueue(nil)
	defer q.Close()
	err := PublishFiles(q, root, []FileChange{{Path: "brain/entities/topic/a.md", After: []byte("new")}, {Path: "brain/entities/topic/b.md", Before: []byte("old"), After: []byte("new")}})
	if err == nil || !strings.Contains(err.Error(), "changed since approval") {
		t.Fatalf("expected human edit guard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.md")); !os.IsNotExist(err) {
		t.Fatal("wrote first path before checking second")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "b.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "human revision" {
		t.Fatal("overwrote human")
	}
}

func TestPublishFilesRejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{"../outside", ".serenity/config", "brain/entities/../../outside", "brain/entities/topic/link.md", "brain/entities/link/a.md"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "brain/entities/topic"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "target"), []byte("private bytes"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(root, "brain/entities/topic/link.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "brain/entities/link")); err != nil {
				t.Fatal(err)
			}
			q := NewQueue(nil)
			defer q.Close()
			if err := PublishFiles(q, root, []FileChange{{Path: path, After: []byte("replacement")}}); err == nil {
				t.Fatal("unsafe path accepted")
			}
			raw, err := os.ReadFile(filepath.Join(outside, "target"))
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != "private bytes" {
				t.Fatal("changed external bytes")
			}
		})
	}
}

func TestMergeClaimFencesPreservesUnmanagedBytes(t *testing.T) {
	original := []byte("---\ncustom-owner: local\n---\n# Handwritten heading\n\nHuman introduction.\n<!-- serenity:summary:begin -->\nMy exact summary.\n<!-- serenity:summary:end -->\n<!-- serenity:claims:begin -->\nold claim\n<!-- serenity:claims:end -->\nHuman middle.\n<!-- serenity:timeline:begin -->\nHuman timeline.\n<!-- serenity:timeline:end -->\nHuman conclusion.\n")
	fresh := []byte("<!-- serenity:claims:begin -->\nnew claim\n<!-- serenity:claims:end -->\n<!-- serenity:metadata:begin -->\n{}\n<!-- serenity:metadata:end -->")
	got, err := MergeClaimFences(original, fresh)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Replace(string(original), "old claim", "new claim", 1)
	expected = strings.Replace(expected, "<!-- serenity:timeline:end -->", "<!-- serenity:timeline:end -->\n\n<!-- serenity:metadata:begin -->\n{}\n<!-- serenity:metadata:end -->", 1)
	if string(got) != expected {
		t.Fatalf("changed unmanaged bytes:\n%s", got)
	}
}
