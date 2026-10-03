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

func TestResolveCommitUsesCallerRepositoryDespiteInheritedGitDir(t *testing.T) {
	root := t.TempDir()
	foreign := t.TempDir()
	initPublishRepo(t, root, "intended.txt")
	initPublishRepo(t, foreign, "foreign.txt")
	want := publishGitOutput(t, root, "rev-parse", "HEAD")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GITHUB_SHA", "")

	if got := resolveCommit(context.Background()); got != want {
		t.Fatalf("resolveCommit() = %q, want caller repository %q", got, want)
	}
}

func TestResolveCommitKeepsGITHUBSHAPreference(t *testing.T) {
	t.Setenv("GITHUB_SHA", "pinned-by-workflow")
	if got := resolveCommit(context.Background()); got != "pinned-by-workflow" {
		t.Fatalf("resolveCommit() = %q, want workflow SHA", got)
	}
}

func initPublishRepo(t *testing.T, dir, file string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Eval"}, {"config", "user.email", "eval@example.invalid"}} {
		publishGitRun(t, dir, args...)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	publishGitRun(t, dir, "add", file)
	publishGitRun(t, dir, "commit", "-qm", "fixture")
}

func publishGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func publishGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
