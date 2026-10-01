package writer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
)

// rewriteForgottenPath removes every historical version of one path from all
// brain refs, then prunes reflogs and unreachable objects. The caller must hold
// the writer queue lock so no concurrent source write can race the rewrite.
func rewriteForgottenPath(root, relPath string) (retErr error) {
	return rewriteForgottenPathContext(context.Background(), root, relPath)
}

// rewriteForgottenPathContext bounds every Git subprocess by ctx. The
// legacy wrapper above preserves local callers' historical behavior.
func rewriteForgottenPathContext(ctx context.Context, root, relPath string) (retErr error) {
	return rewriteForgottenPathWithGit(ctx, root, relPath, gitrun.Brain(root), func(dir string) historyGit { return gitrun.Brain(dir) })
}

type historyGit interface {
	Output(context.Context, ...string) ([]byte, error)
	CombinedOutput(context.Context, ...string) ([]byte, error)
	Run(context.Context, ...string) error
	Command(context.Context, ...string) (*exec.Cmd, error)
}

func rewriteForgottenPathWithGit(ctx context.Context, root, relPath string, git historyGit, gitAt func(string) historyGit) (retErr error) {
	if ctx == nil {
		return fmt.Errorf("writer: nil history rewrite context")
	}
	if gitAt == nil {
		return fmt.Errorf("writer: nil history rewrite Git factory")
	}
	defer func() {
		if ctx.Err() != nil {
			retErr = errors.Join(retErr, ctx.Err())
		}
	}()
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || filepath.IsAbs(relPath) || relPath == ".." || strings.HasPrefix(relPath, "../") {
		return fmt.Errorf("writer: invalid history rewrite path %q", relPath)
	}
	// The path is interpolated into filter-branch's shell program. Canonical
	// source paths contain only these characters; reject anything else rather
	// than trying to quote arbitrary user-controlled shell input.
	for _, r := range relPath {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("/._-", r)
		if !allowed {
			return fmt.Errorf("writer: invalid history rewrite path %q", relPath)
		}
	}
	if _, err := git.Output(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		if _, statErr := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(statErr) {
			// A brain without a Git repository has no history to rewrite.
			return nil
		}
		return fmt.Errorf("writer: verify brain repository before history rewrite: %w", err)
	}
	historical, err := git.Output(ctx, "rev-list", "--all", "--", relPath)
	if err != nil {
		return fmt.Errorf("writer: inspect forgotten path history: %w", err)
	}
	if len(strings.TrimSpace(string(historical))) == 0 {
		// A retry after a successful rewrite must not create another force-push
		// marker or print the other-clones warning again. Reflogs can still
		// retain the path after a branch was reset or deleted, so prune them.
		if out, err := git.CombinedOutput(ctx, "rm", "--cached", "--ignore-unmatch", "-r", "--", relPath); err != nil {
			return fmt.Errorf("writer: remove forgotten path from brain index: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if err := git.Run(ctx, "reflog", "expire", "--expire=now", "--all"); err != nil {
			return fmt.Errorf("writer: expire brain reflogs: %w", err)
		}
		if err := git.Run(ctx, "gc", "--prune=now"); err != nil {
			return fmt.Errorf("writer: prune forgotten brain objects: %w", err)
		}
		return nil
	}
	marker, err := git.Output(ctx, "rev-parse", "--git-path", "serenity-history-rewrite-push")
	if err != nil {
		return fmt.Errorf("writer: locate history rewrite marker: %w", err)
	}
	markerPath := strings.TrimSpace(string(marker))
	if !filepath.IsAbs(markerPath) {
		markerPath = filepath.Join(root, markerPath)
	}
	// filter-branch --all also rewrites local remote-tracking refs. Preserve
	// the remote tip observed before that rewrite: the hook needs this exact
	// old value for force-with-lease rather than the rewritten tracking ref.
	expectedRemoteTip := "no-upstream"
	if out, err := git.Output(ctx, "rev-parse", "--verify", "@{upstream}"); err == nil {
		expectedRemoteTip = strings.TrimSpace(string(out))
	}
	// An earlier offline forget may already have rewritten the tracking ref.
	// Its marker still holds the original remote SHA, which is the only safe
	// lease for publishing both rewrites together.
	if existing, err := os.ReadFile(markerPath); err == nil {
		if previous := strings.TrimSpace(string(existing)); previous != "" {
			expectedRemoteTip = previous
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("writer: read existing history rewrite marker: %w", err)
	}
	// Remove the forgotten path from the live index too. Git's object walk
	// treats index entries as roots, so pruning history alone would leave the
	// bytes recoverable from the index until a later Flush.
	if out, err := git.CombinedOutput(ctx, "rm", "--cached", "--ignore-unmatch", "-r", "--", relPath); err != nil {
		return fmt.Errorf("writer: remove forgotten path from brain index: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		return fmt.Errorf("writer: create history rewrite marker directory: %w", err)
	}
	tempRoot, err := os.MkdirTemp("", "serenity-history-rewrite-")
	if err != nil {
		return fmt.Errorf("writer: create isolated history rewrite worktree: %w", err)
	}
	worktree := filepath.Join(tempRoot, "worktree")
	worktreeAdded := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if worktreeAdded {
			if err := git.Run(cleanupCtx, "worktree", "remove", "--force", worktree); err != nil {
				retErr = errors.Join(retErr, errors.New("writer: cleanup private history rewrite worktree failed"))
			}
		}
		if err := os.RemoveAll(tempRoot); err != nil {
			retErr = errors.Join(retErr, errors.New("writer: cleanup private history rewrite directory failed"))
		}
	}()
	if err := git.Run(ctx, "worktree", "add", "--detach", worktree, "HEAD"); err != nil {
		return fmt.Errorf("writer: create clean history rewrite worktree: %w", err)
	}
	worktreeAdded = true
	filter := "git rm --cached --ignore-unmatch -r -- '" + relPath + "'"
	filterCmd, err := gitAt(worktree).Command(ctx, "filter-branch", "--force", "--index-filter", filter, "--", "--all")
	if err != nil {
		return fmt.Errorf("writer: prepare history rewrite: %w", err)
	}
	if err := configureHistoryProcessGroup(filterCmd); err != nil {
		return fmt.Errorf("writer: configure isolated history rewrite process: %w", err)
	}
	// Git's ten-second filter-branch warning is for interactive use. This
	// operation is explicit and covered by the CLI warning and operator docs.
	filterCmd.Env = append(filterCmd.Env, "FILTER_BRANCH_SQUELCH_WARNING=1")
	if out, err := filterCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writer: rewrite forgotten path history: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// A failed filter-branch must never mark an unrevised brain for a later
	// force-with-lease push. Once refs have changed, retain the marker even if
	// pruning fails so a retry can still publish the rewritten history.
	if err := os.WriteFile(markerPath, []byte(expectedRemoteTip+"\n"), 0o600); err != nil {
		return fmt.Errorf("writer: write history rewrite marker: %w", err)
	}
	if err := git.Run(ctx, "worktree", "remove", "--force", worktree); err != nil {
		return fmt.Errorf("writer: remove history rewrite worktree: %w", err)
	}
	worktreeAdded = false
	refs, err := git.Output(ctx, "for-each-ref", "--format=%(refname)", "refs/original/")
	if err != nil {
		return fmt.Errorf("writer: list history rewrite backup refs: %w", err)
	}
	for _, ref := range strings.Fields(string(refs)) {
		if strings.HasPrefix(ref, "refs/original/") {
			if err := git.Run(ctx, "update-ref", "-d", ref); err != nil {
				return fmt.Errorf("writer: remove history rewrite backup ref %s: %w", ref, err)
			}
		}
	}
	if err := git.Run(ctx, "reflog", "expire", "--expire=now", "--all"); err != nil {
		return fmt.Errorf("writer: expire brain reflogs: %w", err)
	}
	if err := git.Run(ctx, "gc", "--prune=now"); err != nil {
		return fmt.Errorf("writer: prune forgotten brain objects: %w", err)
	}
	return nil
}
