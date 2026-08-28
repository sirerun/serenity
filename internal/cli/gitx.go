package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Small git helpers. The daemon-side writer queue and GitOps port arrive
// with the ingest engine (M1+); init/doctor only need these probes.

func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

func gitInit(root string) error {
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = root
	return cmd.Run()
}

func gitRemotes(root string) []string {
	cmd := exec.Command("git", "remote")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(out))
	return fields
}

// gitUnpushed returns the number of commits ahead of upstream, or -1 when
// there is no upstream to compare against.
func gitUnpushed(root string) int {
	cmd := exec.Command("git", "rev-list", "--count", "@{u}..HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// gitLastPushTime reports when this branch was last pushed, approximated
// by the mtime of its local remote-tracking ref (`.git/refs/remotes/<r>/<b>`)
// -- git creates or refreshes that loose ref file on every successful push,
// including the first `push -u`. ok is false when there is no upstream
// configured at all, i.e. this branch has never been pushed anywhere.
func gitLastPushTime(root string) (t time.Time, ok bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, false
	}
	upstream := strings.TrimSpace(string(out)) // e.g. "origin/main"
	remote, branch, found := strings.Cut(upstream, "/")
	if !found {
		return time.Time{}, false
	}
	fi, err := os.Stat(filepath.Join(root, ".git", "refs", "remotes", remote, branch))
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}

// installPostCommitPush writes the durability-floor hook (§7.7): push
// after every commit, warn loudly on failure. Existing hooks are never
// overwritten.
func installPostCommitPush(root string) (installed bool, err error) {
	hooksDir := filepath.Join(root, ".git", "hooks")
	if _, err := os.Stat(hooksDir); err != nil {
		return false, nil // not a git repo or unusual layout; doctor will flag
	}
	hookPath := filepath.Join(hooksDir, "post-commit")
	if _, err := os.Stat(hookPath); err == nil {
		return false, nil
	}
	script := `#!/bin/sh
# serenity durability floor (RFC 0001 §7.7): "no DB backups" assumes a
# healthy remote — push after every commit and warn loudly on failure.
git push --quiet 2>/dev/null || echo "serenity: warning: post-commit push failed (no remote, offline, or rejected)" >&2
`
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		return false, err
	}
	return true, nil
}
