package gitrun_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
)

// plainGit runs git outside the runner. Tests use it only to build
// fixtures and as the unhardened control that proves a hostile
// configuration is live on this git version.
func plainGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// isolateGlobalConfig keeps the operator's real ~/.gitconfig and any
// GIT_* variables in the test environment out of the fixtures.
func isolateGlobalConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "GIT_") {
			t.Setenv(k, "")
			if err := os.Unsetenv(k); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// newRepo creates a throwaway repository with one committed file that is
// modified in the worktree and one untracked file, so status, diff and
// ls-files --others all have work to do.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plainGit(t, dir, "init", "--quiet")
	plainGit(t, dir, "config", "user.name", "gitrun test")
	plainGit(t, dir, "config", "user.email", "gitrun@example.com")
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainGit(t, dir, "add", "tracked.md")
	plainGit(t, dir, "commit", "--quiet", "-m", "fixture")
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.md"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// installFsmonitor points core.fsmonitor at a script that writes marker,
// which is the SEC-H05 mechanism: git spawns the configured program on
// every index-refreshing subcommand.
func installFsmonitor(t *testing.T, dir string) (marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "fsmonitor-ran")
	script := filepath.Join(t.TempDir(), "fsmonitor.sh")
	body := fmt.Sprintf("#!/bin/sh\n: > %q\nexit 0\n", marker)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	plainGit(t, dir, "config", "core.fsmonitor", script)
	return marker
}

func markerExists(t *testing.T, marker string) bool {
	t.Helper()
	_, err := os.Stat(marker)
	if err == nil {
		return true
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return false
}

func TestQuarantineLocalBundleRestoreIgnoresHooksAndRemoteTransport(t *testing.T) {
	isolateGlobalConfig(t)
	dir := newRepo(t)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(fmt.Sprintf("#!/bin/sh\n: > %q\n", marker)), 0o755); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(os.Getenv("HOME"), ".gitconfig")
	plainGit(t, dir, "config", "--file", global, "core.hooksPath", hooks)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	bundle := filepath.Join(t.TempDir(), "brain.bundle")
	if out, err := gitrun.Quarantine(dir).CombinedOutput(context.Background(), "bundle", "create", bundle, "--all"); err != nil {
		t.Fatalf("create bundle: %v: %s", err, out)
	}
	parent := t.TempDir()
	// Control: the installed global post-checkout hook really executes.
	plainGit(t, parent, "clone", "--quiet", bundle, filepath.Join(parent, "control"))
	if !markerExists(t, marker) {
		t.Fatal("unhardened clone did not execute hostile global hook")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	plainGit(t, parent, "init", "--quiet")
	// A repository-local rewrite must not turn the bundle path into a remote.
	plainGit(t, parent, "config", "url.http://127.0.0.1:1/.insteadOf", bundle)
	t.Setenv("TMPDIR", parent)
	if out, err := gitrun.CloneBundle(context.Background(), bundle, filepath.Join(parent, "restored")); err != nil {
		t.Fatalf("restore bundle: %v: %s", err, out)
	}
	if markerExists(t, marker) {
		t.Fatal("quarantine restore executed global hook")
	}
	plainGit(t, parent, "init", "--quiet")
	plainGit(t, parent, "config", "protocol.http.allow", "always")
	if out, err := gitrun.Quarantine(parent).CombinedOutput(context.Background(), "ls-remote", "http://127.0.0.1:1/repo.git"); err == nil || !strings.Contains(string(out), "transport 'http' not allowed") {
		t.Fatalf("remote transport not refused: %v: %s", err, out)
	}
	if _, err := gitrun.Foreign(dir).Command(context.Background(), "clone", bundle, "other"); !errors.Is(err, gitrun.ErrForeignWrite) {
		t.Fatalf("foreign write restriction changed: %v", err)
	}
	if _, err := gitrun.Quarantine(parent).Command(context.Background(), "clone", bundle, "other"); err == nil {
		t.Fatal("generic quarantine clone bypassed bundle-only API")
	}
}

// TestHostileFsmonitorNeverSpawns reproduces the deep review's GIT_TRACE
// finding (SEC-H05): a repository whose configuration names a fsmonitor
// program gets that program executed by plain git, and must not get it
// executed by either runner across the index-refreshing subcommands the
// product actually runs.
func TestHostileFsmonitorNeverSpawns(t *testing.T) {
	isolateGlobalConfig(t)
	subcommands := [][]string{
		{"ls-files", "--cached", "--others", "--exclude-standard", "-z"},
		{"status", "--porcelain"},
		{"--literal-pathspecs", "status", "--porcelain", "--", "tracked.md"},
		{"diff"},
		{"diff", "--cached", "--name-only", "-z", "--no-renames"},
	}

	// Control: prove the fixture is live on this git version, otherwise a
	// green run below would be vacuous.
	control := newRepo(t)
	marker := installFsmonitor(t, control)
	plainGit(t, control, "status", "--porcelain")
	if !markerExists(t, marker) {
		t.Fatalf("control: unhardened `git status` did not spawn core.fsmonitor on %s; the fixture cannot prove anything", strings.TrimSpace(plainGit(t, control, "--version")))
	}

	runners := map[string]func(string) *gitrun.Runner{
		"Brain":   gitrun.Brain,
		"Foreign": gitrun.Foreign,
	}
	for name, construct := range runners {
		t.Run(name, func(t *testing.T) {
			dir := newRepo(t)
			marker := installFsmonitor(t, dir)
			r := construct(dir)
			for _, args := range subcommands {
				_, err := r.Output(context.Background(), args...)
				var exit *exec.ExitError
				// A non-zero exit is still a completed subcommand; only a
				// failure to run at all is a test error.
				if err != nil && !errors.As(err, &exit) {
					t.Fatalf("git %s: %v", strings.Join(args, " "), err)
				}
				if markerExists(t, marker) {
					t.Errorf("git %s under %s spawned the repository's core.fsmonitor program", strings.Join(args, " "), name)
					if err := os.Remove(marker); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

// TestBrainKeepsPostCommitHook is the UC-040 regression: the brain's
// auto-push hook installed by `serenity init` must still fire under Brain.
func TestBrainKeepsPostCommitHook(t *testing.T) {
	isolateGlobalConfig(t)
	dir := newRepo(t)
	marker := filepath.Join(t.TempDir(), "post-commit-ran")
	hook := filepath.Join(dir, ".git", "hooks", "post-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte(fmt.Sprintf("#!/bin/sh\n: > %q\n", marker)), 0o755); err != nil {
		t.Fatal(err)
	}
	r := gitrun.Brain(dir)
	ctx := context.Background()
	if out, err := r.CombinedOutput(ctx, "add", "tracked.md"); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	if out, err := r.CombinedOutput(ctx, "commit", "--quiet", "-m", "serenity: sync 1 file(s)"); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	if !markerExists(t, marker) {
		t.Fatal("post-commit hook did not run under gitrun.Brain")
	}
}

// TestForeignIgnoresRepositoryHooks: a foreign repository's hooks never
// run. post-index-change fires after any subcommand rewrites the index,
// which a status refresh of a stale index does under plain git.
func TestForeignIgnoresRepositoryHooks(t *testing.T) {
	isolateGlobalConfig(t)
	marker := filepath.Join(t.TempDir(), "post-index-change-ran")
	install := func(dir string) {
		hook := filepath.Join(dir, ".git", "hooks", "post-index-change")
		if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hook, []byte(fmt.Sprintf("#!/bin/sh\n: > %q\n", marker)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stale := func(dir string) {
		// Restore the committed content under a fresh mtime: the index's
		// stat cache is now stale for an unchanged file, which is exactly
		// what a status refresh rewrites.
		path := filepath.Join(dir, "tracked.md")
		if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(2 * time.Hour)
		if err := os.Chtimes(path, future, future); err != nil {
			t.Fatal(err)
		}
	}

	control := newRepo(t)
	install(control)
	stale(control)
	plainGit(t, control, "status", "--porcelain")
	if !markerExists(t, marker) {
		t.Skip("plain `git status` did not fire post-index-change on this git; cannot prove hook suppression")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	dir := newRepo(t)
	install(dir)
	stale(dir)
	if _, err := gitrun.Foreign(dir).Output(context.Background(), "status", "--porcelain"); err != nil {
		t.Fatal(err)
	}
	if markerExists(t, marker) {
		t.Fatal("foreign repository hook ran under gitrun.Foreign")
	}
}

func TestForeignAllowsOnlyReadOnlySubcommands(t *testing.T) {
	isolateGlobalConfig(t)
	dir := newRepo(t)
	r := gitrun.Foreign(dir)
	ctx := context.Background()
	for _, args := range [][]string{
		{"add", "tracked.md"},
		{"commit", "-m", "x"},
		{"push"},
		{"fetch"},
		{"remote", "add", "evil", "ext::sh -c id"},
		{"config", "core.fsmonitor", "x"},
		{"init"},
		{"--literal-pathspecs", "add", "tracked.md"},
	} {
		if _, err := r.Command(ctx, args...); !errors.Is(err, gitrun.ErrForeignWrite) {
			t.Errorf("git %s: want ErrForeignWrite, got %v", strings.Join(args, " "), err)
		}
	}
	for _, args := range [][]string{
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "HEAD"},
		{"log", "-1", "--format=%cI", "HEAD"},
		{"ls-files", "--cached", "--others", "--exclude-standard", "-z"},
		{"show", "HEAD:tracked.md"},
		{"--literal-pathspecs", "status", "--porcelain", "--", "tracked.md"},
	} {
		if _, err := r.Output(ctx, args...); err != nil {
			t.Errorf("git %s: %v", strings.Join(args, " "), err)
		}
	}
}

func TestCanonicalReadOnlyPinsObjectInterpretationAndRefusesWrites(t *testing.T) {
	dir := newRepo(t)
	runner := gitrun.CanonicalReadOnly(dir)
	cmd, err := runner.Command(context.Background(), "show", "HEAD:tracked.md")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, item := range cmd.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	if env["GIT_NO_REPLACE_OBJECTS"] != "1" || env["GIT_NO_LAZY_FETCH"] != "1" {
		t.Fatalf("canonical object safety env = GIT_NO_REPLACE_OBJECTS:%q GIT_NO_LAZY_FETCH:%q", env["GIT_NO_REPLACE_OBJECTS"], env["GIT_NO_LAZY_FETCH"])
	}
	if _, err := runner.Command(context.Background(), "add", "tracked.md"); !errors.Is(err, gitrun.ErrForeignWrite) {
		t.Fatalf("canonical write command error = %v, want ForeignWrite", err)
	}
}

func TestCanonicalReadOnlyIgnoresReplacementRefs(t *testing.T) {
	dir := newRepo(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	original := run("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("replacement commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "tracked.md")
	run("commit", "-m", "replacement")
	replacement := run("rev-parse", "HEAD")
	run("reset", "--hard", original)
	run("replace", original, replacement)

	ctx := context.Background()
	foreignBytes, err := gitrun.Foreign(dir).Output(ctx, "show", "HEAD:tracked.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(foreignBytes) != "replacement commit\n" {
		t.Fatalf("Foreign control did not demonstrate replacement: %q", foreignBytes)
	}
	canonicalBytes, err := gitrun.CanonicalReadOnly(dir).Output(ctx, "show", "HEAD:tracked.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(canonicalBytes) != "v1\n" {
		t.Fatalf("canonical reader followed replacement ref: %q", canonicalBytes)
	}
}

func TestReservedOptionsRejected(t *testing.T) {
	isolateGlobalConfig(t)
	dir := newRepo(t)
	ctx := context.Background()
	for _, r := range []*gitrun.Runner{gitrun.Brain(dir), gitrun.Foreign(dir)} {
		for _, args := range [][]string{
			{"-c", "core.fsmonitor=/elsewhere/x", "status"},
			{"--git-dir=/elsewhere", "status"},
			{"--git-dir", "/elsewhere", "status"},
			{"--work-tree=/elsewhere", "status"},
			{"-C", "/elsewhere", "status"},
			{"--exec-path=/elsewhere", "status"},
			{"--config-env=core.fsmonitor=X", "status"},
			{"--namespace=x", "status"},
			{},
		} {
			if _, err := r.Command(ctx, args...); !errors.Is(err, gitrun.ErrReservedOption) {
				t.Errorf("git %s: want ErrReservedOption, got %v", strings.Join(args, " "), err)
			}
		}
	}
}

// TestHardeningPrefixAndEnvironment pins the contract from ADR 018: the
// -c prefix each runner adds and the GIT_* scrub both apply.
func TestHardeningPrefixAndEnvironment(t *testing.T) {
	isolateGlobalConfig(t)
	dir := newRepo(t)
	t.Setenv("GIT_TRACE", "1")
	t.Setenv("GIT_DIR", "/elsewhere/.git")
	t.Setenv("GIT_CONFIG_GLOBAL", "/elsewhere/gitconfig")
	t.Setenv("GIT_SSH_COMMAND", "ssh -i /elsewhere/key")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("LANG", "C.UTF-8")
	t.Setenv("TMPDIR", t.TempDir())

	env := func(cmd *exec.Cmd) map[string]string {
		m := map[string]string{}
		for _, kv := range cmd.Env {
			k, v, _ := strings.Cut(kv, "=")
			m[k] = v
		}
		return m
	}
	brain, err := gitrun.Brain(dir).Command(context.Background(), "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := gitrun.Foreign(dir).Command(context.Background(), "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if brain.Dir != dir || foreign.Dir != dir {
		t.Fatalf("Dir: brain=%q foreign=%q want %q", brain.Dir, foreign.Dir, dir)
	}

	brainPrefix := []string{"git", "-c", "core.fsmonitor=false", "-c", "protocol.ext.allow=never"}
	if len(brain.Args) < len(brainPrefix) || !slices.Equal(brain.Args[:len(brainPrefix)], brainPrefix) {
		t.Errorf("Brain args %q do not start with %q", brain.Args, brainPrefix)
	}
	if !slices.Equal(brain.Args[len(brain.Args)-2:], []string{"status", "--porcelain"}) {
		t.Errorf("Brain args %q do not end with the caller's subcommand", brain.Args)
	}
	if slices.Contains(brain.Args, "core.hooksPath="+os.DevNull) {
		t.Errorf("Brain must keep repository hooks (UC-040), got %q", brain.Args)
	}
	foreignPrefix := append(slices.Clone(brainPrefix), "-c", "core.hooksPath="+os.DevNull)
	if len(foreign.Args) < len(foreignPrefix) || !slices.Equal(foreign.Args[:len(foreignPrefix)], foreignPrefix) {
		t.Errorf("Foreign args %q do not start with %q", foreign.Args, foreignPrefix)
	}

	for name, e := range map[string]map[string]string{"Brain": env(brain), "Foreign": env(foreign)} {
		for _, dropped := range []string{"GIT_TRACE", "GIT_DIR"} {
			if _, ok := e[dropped]; ok {
				t.Errorf("%s: %s leaked into the git environment", name, dropped)
			}
		}
		if e["GIT_SSH_COMMAND"] != "ssh -i /elsewhere/key" {
			t.Errorf("%s: GIT_SSH_COMMAND not preserved: %q", name, e["GIT_SSH_COMMAND"])
		}
		if e["GIT_TERMINAL_PROMPT"] != "0" {
			t.Errorf("%s: GIT_TERMINAL_PROMPT=%q, want 0", name, e["GIT_TERMINAL_PROMPT"])
		}
		for _, kept := range []string{"PATH", "HOME", "LANG", "TMPDIR"} {
			if e[kept] != os.Getenv(kept) {
				t.Errorf("%s: %s=%q, want %q", name, kept, e[kept], os.Getenv(kept))
			}
		}
	}
	if v := env(brain)["GIT_CONFIG_GLOBAL"]; v != "" {
		t.Errorf("Brain: GIT_CONFIG_GLOBAL=%q, want unset so the operator's global config applies", v)
	}
	if v := env(brain)["GIT_CONFIG_NOSYSTEM"]; v != "" {
		t.Errorf("Brain: GIT_CONFIG_NOSYSTEM=%q, want unset", v)
	}
	if v := env(foreign)["GIT_CONFIG_GLOBAL"]; v != os.DevNull {
		t.Errorf("Foreign: GIT_CONFIG_GLOBAL=%q, want %q", v, os.DevNull)
	}
	if v := env(foreign)["GIT_CONFIG_NOSYSTEM"]; v != "1" {
		t.Errorf("Foreign: GIT_CONFIG_NOSYSTEM=%q, want 1", v)
	}
}

// TestNoRawGitExecOutsideGitrun greps the six local-product packages for
// exec.Command / exec.CommandContext calls that name git and accepts them
// only inside internal/gitrun. Hosted packages migrate under their own
// file claims (ADR 018) and are covered by T24.30's module-wide test.
func TestNoRawGitExecOutsideGitrun(t *testing.T) {
	roots := []string{"../gitrun", "../cli", "../connector", "../writer", "../direction", "../supersede", "../import"}
	var offenders []string
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "exec" || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
					return true
				}
				for _, arg := range call.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING && lit.Value == `"git"` {
						pos := fset.Position(call.Pos())
						if filepath.Base(filepath.Dir(pos.Filename)) != "gitrun" {
							offenders = append(offenders, fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Line))
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("raw git exec outside internal/gitrun (ADR 018):\n  %s", strings.Join(offenders, "\n  "))
	}
}
