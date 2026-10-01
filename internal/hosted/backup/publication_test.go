package backup

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicPublicationPreservesCompetingEmptyDestination(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	dest := filepath.Join(root, "dest")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "payload"), []byte("private snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	// A competing caller creates its destination after the library's earlier
	// absence check. The kernel operation must preserve that caller's inode.
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	err = renameNoReplace(stage, dest)
	if err == nil {
		t.Fatal("publication replaced the competing destination")
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("conflicting publication error: %v", err)
		}
	} else if !errors.Is(err, ErrAtomicPublicationUnavailable) {
		t.Fatalf("unsupported platform did not fail closed: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("destination inode replaced")
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatalf("destination modified: entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(stage, "payload")); err != nil {
		t.Fatalf("owned unpublished stage lost: %v", err)
	}
}
func TestAtomicPublicationMovesCompleteDirectory(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	dest := filepath.Join(root, "dest")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "payload"), []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	err := renameNoReplace(stage, dest)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if !errors.Is(err, ErrAtomicPublicationUnavailable) {
			t.Fatalf("unsupported publication: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stage still exists: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "payload"))
	if err != nil || string(b) != "complete" {
		t.Fatalf("published payload %q err=%v", b, err)
	}
}
