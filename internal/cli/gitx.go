package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
)

// Small git helpers. The daemon-side writer queue and GitOps port arrive
// with the ingest engine (M1+); init/doctor only need these probes.

func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

func gitInit(root string) error {
	return gitrun.Brain(root).Run(context.Background(), "init", "--quiet")
}

func gitRemotes(root string) []string {
	out, err := gitrun.Brain(root).Output(context.Background(), "remote")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(out))
	return fields
}

// gitUnpushed returns the number of commits ahead of upstream, or -1 when
// there is no upstream to compare against.
func gitUnpushed(root string) int {
	out, err := gitrun.Brain(root).Output(context.Background(), "rev-list", "--count", "@{u}..HEAD")
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
	out, err := gitrun.Brain(root).Output(context.Background(), "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
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

// Exact hook written by Serenity before history-rewrite pushes were added.
// Only this known version may be upgraded; user-owned hooks are preserved.
const legacyPostCommitPush = `#!/bin/sh
# serenity durability floor (RFC 0001 §7.7): "no DB backups" assumes a
# healthy remote — push after every commit and warn loudly on failure.
git push --quiet 2>/dev/null || echo "serenity: warning: post-commit push failed (no remote, offline, or rejected)" >&2
`

// installPostCommitPush installs or upgrades Serenity's durability-floor hook.
// A custom hook is never overwritten.
func installPostCommitPush(root string) (installed bool, err error) {
	hooksDir := filepath.Join(root, ".git", "hooks")
	if _, err := os.Stat(hooksDir); err != nil {
		return false, nil // not a git repo or unusual layout; doctor will flag
	}
	hookPath := filepath.Join(hooksDir, "post-commit")
	script := `#!/bin/sh
# serenity durability floor; forget may rewrite this brain's source history.
marker=$(git rev-parse --git-path serenity-history-rewrite-push 2>/dev/null) || marker=
if [ -n "$marker" ] && [ -f "$marker" ]; then
  expected=$(cat "$marker")
  branch=$(git symbolic-ref --quiet --short HEAD 2>/dev/null) || branch=
  merge_ref=$(git config --get "branch.$branch.merge" 2>/dev/null) || merge_ref=
  case "$merge_ref" in
    refs/heads/*)
      if printf '%s' "$expected" | grep -Eq '^[0-9a-f]{40,64}$'; then
        git push --force-with-lease="$merge_ref:$expected" --quiet 2>/dev/null
      else
        git push --force-with-lease --quiet 2>/dev/null
      fi
      ;;
    *) git push --force-with-lease --quiet 2>/dev/null ;;
  esac
  if [ "$?" -eq 0 ]; then
    rm -f -- "$marker"
    echo "serenity: warning: brain history was rewritten; other clones must re-clone" >&2
  else
    echo "serenity: warning: post-commit history rewrite push failed (no remote, offline, or rejected)" >&2
  fi
else
  git push --quiet 2>/dev/null || echo "serenity: warning: post-commit push failed (no remote, offline, or rejected)" >&2
fi
`
	existing, err := os.ReadFile(hookPath)
	if err == nil {
		if string(existing) == script || string(existing) != legacyPostCommitPush {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	// Rename within the hooks directory avoids leaving a truncated executable
	// hook if a process is interrupted during migration.
	temp, err := os.CreateTemp(hooksDir, ".serenity-post-commit-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := temp.WriteString(script); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Chmod(0o755); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temp.Name(), hookPath); err != nil {
		return false, err
	}
	return true, nil
}
