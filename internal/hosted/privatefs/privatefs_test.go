//go:build darwin || linux

package privatefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileRequiresPrivateStableRegularFile(t *testing.T) {
	dir := testTempDir(t)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "case.json")
	if err := os.WriteFile(file, []byte("case"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(context.Background(), dir, "case.json", 4)
	if err != nil || string(got) != "case" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if _, err := ReadFile(context.Background(), dir, "case.json", 3); err == nil {
		t.Fatal("oversize input accepted")
	}
	if _, err := ReadFile(context.Background(), dir, "../case.json", 100); err == nil {
		t.Fatal("path traversal basename accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(context.Background(), dir, "subdir", 100); err == nil {
		t.Fatal("directory opened as a regular case file")
	}
	symlink := filepath.Join(dir, "link.json")
	if err := os.Symlink(file, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(context.Background(), dir, "link.json", 100); err == nil {
		t.Fatal("symlink file accepted")
	}
	if err := os.Chmod(file, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(context.Background(), dir, "case.json", 100); err == nil {
		t.Fatal("group-readable file accepted")
	}
	if err := os.Chmod(file, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(context.Background(), dir, "case.json", 100); err == nil {
		t.Fatal("executable case file accepted")
	}
}

func TestCanceledAndUntrustedDirectoriesFailClosed(t *testing.T) {
	dir := testTempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateDirectory(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation = %v", err)
	}
	link := filepath.Join(testTempDir(t), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(context.Background(), link); err == nil {
		t.Fatal("symlink directory accepted")
	}
}

func TestWritableLeafAncestorIsRejected(t *testing.T) {
	base := testTempDir(t)
	if err := os.Chmod(base, 0777); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(base, "private")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(context.Background(), child); err == nil {
		t.Fatal("writable ancestor accepted")
	}
}

func testTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "privatefs-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
