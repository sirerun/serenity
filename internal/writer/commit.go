package writer

import (
	"fmt"
	"os/exec"
	"strings"
)

// Flush commits every path the queue has written since the last Flush,
// scoped to exactly those paths (`git add` with the touched-path list,
// never `-A` or `.`) so a human edit sitting dirty elsewhere in the
// working tree is never swept into a daemon commit -- "the human's file
// state is truth" (RFC 0001 §7.7). The commit subject carries the
// `serenity:` prefix §7.7 mandates so daemon commits are distinguishable
// from human ones in the log.
//
// Flush is a noop -- (false, nil), no commit created -- when nothing was
// touched, and also when every touched path renders byte-identical to
// what's already committed (the writers themselves skip no-op disk
// writes, but a resubmitted identical write still marks the path
// touched).
func Flush(q *Queue, root string) (committed bool, err error) {
	paths := q.takeTouched()
	if len(paths) == 0 {
		return false, nil
	}

	// Paths are fed to `git add --pathspec-from-file=-` over stdin, one
	// per line, rather than appended to argv (`git add -- path1 path2
	// ...`): argv has an OS-enforced ceiling (ARG_MAX) that a single
	// sync of a real mailbox blows straight through -- reproduced during
	// T1.23's real Gmail ingest (11,087 new sources in one sync => 2
	// paths each => "fork/exec /usr/bin/git: argument list too long",
	// the whole sync aborting with every new source's bytes already
	// safely written to disk by SourceStore.Write but never committed).
	// Reading pathspecs from stdin has no such limit regardless of how
	// many paths one Flush touches. Every path this package ever queues
	// is a plain content-addressed store path (sha256 dirs, "meta.yaml",
	// "bytes", brain/ fence and shard files) -- none can contain a
	// newline, so newline-delimited is safe without the NUL-delimited
	// (`--pathspec-file-nul`) variant.
	if out, err := runGitStdin(root, strings.Join(paths, "\n"), "add", "--pathspec-from-file=-"); err != nil {
		return false, fmt.Errorf("git add: %w: %s", err, out)
	}

	// git diff --cached --quiet exits 0 when nothing is staged (e.g. the
	// touched paths ended up unchanged) and 1 when something is -- a
	// clean way to tell "nothing to commit" from a real command failure.
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		return false, nil
	} else if _, isExit := err.(*exec.ExitError); !isExit {
		return false, fmt.Errorf("git diff --cached: %w", err)
	}

	msg := fmt.Sprintf("serenity: sync %d file(s)", len(paths))
	if out, err := runGit(root, "commit", "--quiet", "-m", msg); err != nil {
		return false, fmt.Errorf("git commit: %w: %s", err, out)
	}
	return true, nil
}

// CommitPath stages and commits exactly one already-on-disk path with the
// given message -- for a caller that needs a targeted commit outside the
// touched-path bookkeeping Flush's own queue tracks (T2.4: formalizing an
// accepted dirty-tree-guard human edit as its own commit, before any
// further machine write against the same path can proceed -- the guard
// (dirtytree.go) treats "uncommitted" and "not yet reviewed" as the same
// thing, so a path stays paused until it is actually committed, review or
// not). Unlike Flush's generated "serenity: sync N file(s)" subject,
// message is used verbatim -- the caller is expected to say what the
// commit represents. A no-op (false, nil), exactly like Flush's, when the
// path has nothing staged to commit (already clean, or called twice).
func CommitPath(root, path, message string) (bool, error) {
	if out, err := runGit(root, "add", "--", path); err != nil {
		return false, fmt.Errorf("git add: %w: %s", err, out)
	}
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		return false, nil
	} else if _, isExit := err.(*exec.ExitError); !isExit {
		return false, fmt.Errorf("git diff --cached: %w", err)
	}
	if out, err := runGit(root, "commit", "--quiet", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w: %s", err, out)
	}
	return true, nil
}

func runGit(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.CombinedOutput()
}

// runGitStdin is runGit plus a stdin pipe, for the one git invocation
// (`add --pathspec-from-file=-`) that takes its argument list over stdin
// instead of argv.
func runGitStdin(root, stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}
