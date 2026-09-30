package writer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRewriteForgottenPathWithThousandFacts(t *testing.T) {
	root, git := gitRepoFixture(t)
	for i := 0; i < 1000; i++ {
		path := filepath.Join(root, "brain", "sources", fmt.Sprintf("%04d", i), "bytes")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(fmt.Sprintf("fact %d\n", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "brain/sources")
	git("commit", "--quiet", "-m", "one thousand facts")
	start := time.Now()
	if err := rewriteForgottenPath(root, "brain/sources/0500"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	t.Logf("history rewrite with 1000 facts: %s", elapsed)
	if elapsed >= 5*time.Second {
		t.Fatalf("history rewrite exceeded five seconds: %s", elapsed)
	}
	if history := strings.TrimSpace(git("rev-list", "--all", "--", "brain/sources/0500")); history != "" {
		t.Fatalf("forgotten source remains in history: %s", history)
	}
}

func TestFailedHistoryRewriteDoesNotMarkForcePush(t *testing.T) {
	root, git := gitRepoFixture(t)
	relPath := filepath.Join("brain", "sources", "failed", "bytes")
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", relPath)
	git("commit", "--quiet", "-m", "fact to forget")
	actualGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	shim := "#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = filter-branch ]; then exit 7; fi\ndone\nexec \"" + actualGit + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := rewriteForgottenPath(root, filepath.ToSlash(filepath.Dir(relPath))); err == nil {
		t.Fatal("injected filter-branch failure was not reported")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "serenity-history-rewrite-push")); !os.IsNotExist(err) {
		t.Fatalf("failed rewrite left a force-push marker: %v", err)
	}
}

func TestRewrittenHistoryPushHonorsRemoteLease(t *testing.T) {
	root, git := gitRepoFixture(t)
	relPath := filepath.Join("brain", "sources", "remote", "bytes")
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("remote secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", relPath)
	git("commit", "--quiet", "-m", "fact to forget")
	remote := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", "--quiet", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare remote: %v: %s", err, out)
	}
	git("remote", "add", "origin", remote)
	git("push", "--quiet", "-u", "origin", "HEAD")
	oldRemoteTip := strings.TrimSpace(git("rev-parse", "@{upstream}"))
	if err := rewriteForgottenPath(root, filepath.ToSlash(filepath.Dir(relPath))); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(root, ".git", "serenity-history-rewrite-push"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(marker)) != oldRemoteTip {
		t.Fatalf("force-push marker should retain pre-rewrite remote tip: got %q want %q", marker, oldRemoteTip)
	}
	branch := strings.TrimSpace(git("symbolic-ref", "--short", "HEAD"))
	cmd = exec.Command("git", "push", "--force-with-lease=refs/heads/"+branch+":"+oldRemoteTip, "--quiet")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("push rewritten history with remote lease: %v: %s", err, out)
	}
	cmd = exec.Command("git", "--git-dir", remote, "rev-list", "--all", "--", filepath.ToSlash(filepath.Dir(relPath)))
	if out, err := cmd.CombinedOutput(); err != nil || len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("remote retained forgotten source history: err=%v output=%s", err, out)
	}
}

func TestRewriteForgottenPathRemovesHistoricalBlobs(t *testing.T) {
	root, git := gitRepoFixture(t)
	path := filepath.Join("brain", "sources", "ab", strings.Repeat("a", 64), "bytes")
	absPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		t.Fatal(err)
	}
	const oldBody = "historical forgotten fact secret"
	if err := os.WriteFile(absPath, []byte(oldBody), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", path)
	git("commit", "--quiet", "-m", "remember fact")
	firstCommit := strings.TrimSpace(git("rev-parse", "HEAD"))
	firstBlob := strings.TrimSpace(git("rev-parse", "HEAD:"+filepath.ToSlash(path)))
	if err := os.WriteFile(absPath, []byte("changed fact body"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", path)
	git("commit", "--quiet", "-m", "update fact")
	secondCommit := strings.TrimSpace(git("rev-parse", "HEAD"))
	secondBlob := strings.TrimSpace(git("rev-parse", "HEAD:"+filepath.ToSlash(path)))

	if err := rewriteForgottenPath(root, filepath.ToSlash(path)); err != nil {
		t.Fatal(err)
	}
	if history := strings.TrimSpace(git("rev-list", "--all", "--", filepath.ToSlash(path))); history != "" {
		t.Fatalf("forgotten path remains in history: %s", history)
	}
	for _, commit := range []string{firstCommit, secondCommit} {
		if out := git("reflog", "--all", "--format=%H"); strings.Contains(out, commit) {
			t.Fatalf("old commit %s remains in reflog: %s", commit, out)
		}
	}
	for _, blob := range []string{firstBlob, secondBlob} {
		cmd := exec.Command("git", "cat-file", "-e", blob)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("forgotten blob %s still exists: %s", blob, out)
		}
	}
	if counts := git("count-objects", "-v"); !strings.Contains(counts, "count: 0") {
		t.Fatalf("objects remain after history rewrite and gc:\n%s", counts)
	}
	marker := strings.TrimSpace(git("rev-parse", "--git-path", "serenity-history-rewrite-push"))
	if !filepath.IsAbs(marker) {
		marker = filepath.Join(root, marker)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("history rewrite push marker missing: %v", err)
	}
}
