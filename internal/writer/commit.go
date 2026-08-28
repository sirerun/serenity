package writer

import (
	"fmt"
	"os/exec"
)

// Flush commits every path the queue has written since the last Flush,
// scoped to exactly those paths (`git add --` with the touched-path list,
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

	if out, err := runGit(root, append([]string{"add", "--"}, paths...)...); err != nil {
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

func runGit(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.CombinedOutput()
}
