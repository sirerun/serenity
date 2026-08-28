package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// pushFixture scaffolds a brain repo (via runInit) with a local --bare
// remote wired up as origin. It does not push -- callers commit and push
// as their scenario requires. Using a local bare repo (not a network
// remote) keeps the fixture hermetic and fast.
func pushFixture(t *testing.T) (root string) {
	t.Helper()
	tmp := t.TempDir()

	bare := filepath.Join(tmp, "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	root = filepath.Join(tmp, "work")
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("config", "user.email", "doctor-test@example.com")
	run("config", "user.name", "doctor test")
	run("remote", "add", "origin", bare)
	run("add", "-A")
	run("commit", "--quiet", "-m", "seed")

	return root
}

func currentBranch(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	branch := string(bytes.TrimSpace(out))
	if branch == "" {
		t.Fatal("no current branch")
	}
	return branch
}

// remoteRefPath is the local remote-tracking ref file doctor's last-push
// check reads the mtime of.
func remoteRefPath(root, branch string) string {
	return filepath.Join(root, ".git", "refs", "remotes", "origin", branch)
}

func TestDoctorWarnsNeverPushed(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)

	var out bytes.Buffer
	if err := runDoctor(root, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("never pushed")) {
		t.Fatalf("expected a %q warning for a repo with a remote but no push yet, got:\n%s", "never pushed", out.String())
	}
}

func TestDoctorReportsLastPushAge(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)
	branch := currentBranch(t, root)

	cmd := exec.Command("git", "push", "--quiet", "-u", "origin", branch)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push -u: %v\n%s", err, out)
	}
	if _, err := os.Stat(remoteRefPath(root, branch)); err != nil {
		t.Fatalf("push did not create the local remote-tracking ref: %v", err)
	}

	var out bytes.Buffer
	if err := runDoctor(root, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("last push")) {
		t.Fatalf("expected doctor to report %q, got:\n%s", "last push", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("never pushed")) {
		t.Fatalf("a repo pushed moments ago must not warn %q, got:\n%s", "never pushed", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("over the 24h durability floor")) {
		t.Fatalf("a fresh push must not trip the staleness warning, got:\n%s", out.String())
	}
}

func TestDoctorWarnsStalePush(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)
	branch := currentBranch(t, root)

	cmd := exec.Command("git", "push", "--quiet", "-u", "origin", branch)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push -u: %v\n%s", err, out)
	}

	// Back-date the local remote-tracking ref doctor reads the mtime of,
	// so the fixture proves the staleness path without a real 24h wait.
	stale := time.Now().Add(-30 * time.Hour)
	refPath := remoteRefPath(root, branch)
	if err := os.Chtimes(refPath, stale, stale); err != nil {
		t.Fatalf("Chtimes %s: %v", refPath, err)
	}

	var out bytes.Buffer
	if err := runDoctor(root, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("24h")) {
		t.Fatalf("expected a %q staleness warning for a 30h-old push, got:\n%s", "24h", out.String())
	}
}
