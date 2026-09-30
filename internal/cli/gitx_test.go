package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostCommitPushUsesForceWithLeaseOnlyAfterHistoryRewrite(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(realGit, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "--quiet")
	run("config", "user.email", "gitx-test@example.com")
	run("config", "user.name", "gitx test")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "seed")
	run("commit", "--quiet", "-m", "seed")
	if installed, err := installPostCommitPush(root); err != nil || !installed {
		t.Fatalf("install hook: installed=%v err=%v", installed, err)
	}
	bin := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "push-calls")
	fakeGit := "#!/bin/sh\nif [ \"$1\" = rev-parse ]; then exec \"$REAL_GIT\" \"$@\"; fi\nprintf '%s\\n' \"$*\" >> \"$FAKE_GIT_LOG\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "post-commit")
	hook, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	hook = append([]byte("PATH='"+bin+"':\"$PATH\"\n"), hook...)
	if err := os.WriteFile(hookPath, hook, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("FAKE_GIT_LOG", logPath)

	marker := filepath.Join(root, ".git", "serenity-history-rewrite-push")
	if err := os.WriteFile(marker, []byte("rewrite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "first"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "first")
	cmd := exec.Command(realGit, "commit", "-m", "forget commit")
	cmd.Dir = root
	firstOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rewrite commit: %v\n%s", err, firstOutput)
	}
	if strings.Count(string(firstOutput), "other clones must re-clone") != 1 {
		t.Fatalf("expected one clone warning, got:\n%s", firstOutput)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("rewrite marker remains after successful force push: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "second"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "second")
	cmd = exec.Command(realGit, "commit", "-m", "ordinary commit")
	cmd.Dir = root
	secondOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ordinary commit: %v\n%s", err, secondOutput)
	}
	if strings.Contains(string(secondOutput), "other clones must re-clone") {
		t.Fatalf("clone warning repeated without marker:\n%s", secondOutput)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(calls))
	if strings.Join(lines, " ") != "push --force-with-lease --quiet push --quiet" {
		t.Fatalf("unexpected push calls: %q", calls)
	}
}

func TestForgetHelpExplainsHistoryRewrite(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"forget", "--help"})
	var out strings.Builder
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rewrite", "--force-with-lease", "other clones must re-clone", "MCP MEMORY_VERBS"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("forget help missing %q:\n%s", want, out.String())
		}
	}
}
