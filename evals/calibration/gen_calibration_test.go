//go:build ignore

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitSHAUsesCallerRepository(t *testing.T) {
	root := t.TempDir()
	foreign := t.TempDir()
	initCommittedRepo(t, root, "calibration.txt")
	initCommittedRepo(t, foreign, "foreign.txt")
	want := gitOutput(t, root, "rev-parse", "HEAD")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))

	if got := commitSHA(context.Background()); got != want {
		t.Fatalf("commitSHA() = %q, want caller repository %q", got, want)
	}
}

func TestCommitSHAMissingCommitIsUnknown(t *testing.T) {
	root := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	if got := commitSHA(context.Background()); got != "unknown" {
		t.Fatalf("commitSHA() = %q, want unknown", got)
	}
}

func initCommittedRepo(t *testing.T, dir, file string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Eval"}, {"config", "user.email", "eval@example.invalid"}} {
		gitRun(t, dir, args...)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", file)
	gitRun(t, dir, "commit", "-qm", "fixture")
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
