package writer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	q.runMu.Lock()
	defer q.runMu.Unlock()
	paths := q.takeTouched()
	if len(paths) == 0 {
		return false, nil
	}
	defer func() {
		if err != nil {
			for _, path := range paths {
				q.MarkTouched(path)
			}
		}
	}()
	return commitPaths(root, paths, fmt.Sprintf("serenity: sync %d file(s)", len(paths)))
}

// commitPaths commits only the queue's exact files, preserving unrelated staged
// changes as well as unstaged edits. NUL pathspecs avoid argv limits and quoting.
func commitPaths(root string, paths []string, message string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	owned := make(map[string]bool, len(paths))
	rels := make([]string, 0, len(paths))
	missing := map[string]bool{}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			// Writers may return paths prefixed with a relative brain root;
			// callers may also supply paths relative to the brain itself.
			fromCWD, err := filepath.Abs(path)
			if err != nil {
				return false, err
			}
			within, err := filepath.Rel(absRoot, fromCWD)
			if err == nil && within != ".." && !strings.HasPrefix(within, ".."+string(filepath.Separator)) {
				path = fromCWD
			} else {
				path = filepath.Join(absRoot, path)
			}
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false, fmt.Errorf("writer: touched path outside brain: %q", path)
		}
		info, statErr := os.Lstat(path)
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return false, statErr
		}
		if statErr == nil && info.IsDir() {
			return false, fmt.Errorf("writer: touched path must name an exact file: %q", path)
		}
		rel = filepath.ToSlash(rel)
		owned[rel] = true
		missing[rel] = errors.Is(statErr, fs.ErrNotExist)
		rels = append(rels, rel)
	}

	// A previous failed commit may already have staged a deletion. Such a path
	// no longer exists in either the worktree or index, so git add rejects it.
	// Add missing paths only while they are still tracked in the index.
	var addRels []string
	var tracked map[string]bool
	for _, rel := range rels {
		if missing[rel] {
			if tracked == nil {
				raw, err := runGit(root, "ls-files", "-z", "--cached")
				if err != nil {
					return false, fmt.Errorf("git indexed paths: %w: %s", err, raw)
				}
				tracked = map[string]bool{}
				for _, path := range strings.Split(string(raw), "\x00") {
					tracked[path] = true
				}
			}
			if !tracked[rel] {
				continue
			}
		}
		addRels = append(addRels, rel)
	}
	if len(addRels) > 0 {
		pathspec := strings.Join(addRels, "\x00") + "\x00"
		if out, err := runGitStdin(root, pathspec, "--literal-pathspecs", "add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return false, fmt.Errorf("git add: %w: %s", err, out)
		}
	}
	staged, err := runGit(root, "diff", "--cached", "--name-only", "-z", "--no-renames")
	if err != nil {
		return false, fmt.Errorf("git diff staged paths: %w: %s", err, staged)
	}
	var changedRels []string
	for _, path := range strings.Split(string(staged), "\x00") {
		if owned[path] {
			changedRels = append(changedRels, path)
		}
	}
	if len(changedRels) == 0 {
		return false, nil
	}
	pathspec := strings.Join(changedRels, "\x00") + "\x00"
	if out, err := runGitStdin(root, pathspec, "--literal-pathspecs", "commit", "--only", "--quiet", "-m", message, "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
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
	return commitPaths(root, []string{path}, message)
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
