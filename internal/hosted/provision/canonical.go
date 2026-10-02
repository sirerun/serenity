package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/gitrun"
)

const maxCanonicalHeadBytes = 128

// initializeCanonical is called only for an allocating brain under ownership.
// Existing ready brains are never silently recreated. Model configuration is
// runtime-owned; this baseline deliberately contains only the cache exclusion.
func initializeCanonical(ctx context.Context, root string) error {
	brainGit := gitrun.Brain(root)
	canonicalGit := gitrun.CanonicalReadOnly(root)
	write := func(args ...string) error {
		out, err := brainGit.CombinedOutput(ctx, args...)
		if err != nil {
			return fmt.Errorf("initialize canonical brain: %w: %s", err, out)
		}
		return nil
	}

	gitDir := filepath.Join(root, ".git")
	info, err := os.Lstat(gitDir)
	if errors.Is(err, os.ErrNotExist) {
		if err = write("init", "--initial-branch=main"); err != nil {
			return err
		}
		info, err = os.Lstat(gitDir)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe canonical Git directory")
	}

	_, headErr := canonicalGit.Output(ctx, "rev-parse", "--verify", "HEAD")
	if headErr == nil {
		if err = validateCommittedCanonical(ctx, canonicalGit, root); err != nil {
			return err
		}
		return persistCanonicalIdentity(write)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = validateUnbornMainHead(ctx, canonicalGit, gitDir, info); err != nil {
		return fmt.Errorf("canonical HEAD is not an unborn main branch: %w", err)
	}

	if err = persistCanonicalIdentity(write); err != nil {
		return err
	}
	if err = ensureCanonicalIgnore(root); err != nil {
		return err
	}
	if err = write("add", "--", ".gitignore"); err != nil {
		return err
	}
	cmd, err := brainGit.Command(ctx, "commit", "--only", "-m", "Initialize hosted brain", "--", ".gitignore")
	if err != nil {
		return fmt.Errorf("initialize canonical brain: %w", err)
	}
	cmd.Env = append(cmd.Env,
		"GIT_AUTHOR_NAME=Serenity Hosted",
		"GIT_AUTHOR_EMAIL=hosted@serenity.sire.run",
		"GIT_COMMITTER_NAME=Serenity Hosted",
		"GIT_COMMITTER_EMAIL=hosted@serenity.sire.run",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initialize canonical brain: %w: %s", err, out)
	}
	return nil
}

func validateCommittedCanonical(ctx context.Context, runner *gitrun.Runner, root string) error {
	path := filepath.Join(root, ".gitignore")
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("canonical baseline missing: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("unsafe canonical baseline file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	committed, err := runner.Output(ctx, "show", "HEAD:.gitignore")
	if err != nil {
		return fmt.Errorf("canonical baseline is not committed: %w", err)
	}
	if string(data) != ".serenity/\n" || string(committed) != string(data) {
		return errors.New("canonical baseline differs from required contents")
	}
	return nil
}

func validateUnbornMainHead(ctx context.Context, runner *gitrun.Runner, gitDir string, knownGitDir os.FileInfo) error {
	currentGitDir, err := os.Lstat(gitDir)
	if err != nil || !currentGitDir.IsDir() || currentGitDir.Mode()&os.ModeSymlink != 0 || !os.SameFile(knownGitDir, currentGitDir) {
		return errors.New("canonical Git directory changed during HEAD validation")
	}
	path := filepath.Join(gitDir, "HEAD")
	before, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect canonical HEAD: %w", err)
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > maxCanonicalHeadBytes {
		return errors.New("unsafe canonical HEAD file")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open canonical HEAD: %w", err)
	}
	opened, statErr := f.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return errors.Join(errors.New("canonical HEAD changed before descriptor read"), statErr, f.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxCanonicalHeadBytes+1))
	afterDescriptor, afterStatErr := f.Stat()
	closeErr := f.Close()
	afterPath, pathErr := os.Lstat(path)
	afterGitDir, gitDirErr := os.Lstat(gitDir)
	if err = errors.Join(readErr, afterStatErr, closeErr, pathErr, gitDirErr); err != nil {
		return fmt.Errorf("read canonical HEAD: %w", err)
	}
	if len(data) > maxCanonicalHeadBytes || !os.SameFile(opened, afterDescriptor) || opened.Size() != afterDescriptor.Size() || !opened.ModTime().Equal(afterDescriptor.ModTime()) || !os.SameFile(opened, afterPath) || afterPath.Size() != afterDescriptor.Size() || !afterPath.ModTime().Equal(afterDescriptor.ModTime()) || !afterPath.Mode().IsRegular() || afterPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(knownGitDir, afterGitDir) || !afterGitDir.IsDir() || afterGitDir.Mode()&os.ModeSymlink != 0 {
		return errors.New("canonical HEAD changed during descriptor read")
	}
	if string(data) != "ref: refs/heads/main\n" {
		return errors.New("canonical HEAD is not the expected main symbolic reference")
	}
	refs, err := runner.CombinedOutput(ctx, "for-each-ref", "--format=%(refname)", "refs/heads/main")
	if err != nil {
		return fmt.Errorf("inspect canonical main reference: %w: %s", err, refs)
	}
	if len(refs) != 0 {
		return fmt.Errorf("canonical main reference is not absent: %s", refs)
	}
	return nil
}

func ensureCanonicalIgnore(root string) error {
	path := filepath.Join(root, ".gitignore")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := f.WriteString(".serenity/\n")
		syncErr := f.Sync()
		if err = errors.Join(writeErr, syncErr, f.Close()); err != nil {
			return err
		}
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("unsafe canonical baseline file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != ".serenity/\n" {
		return errors.New("unexpected canonical baseline contents")
	}
	return nil
}

func persistCanonicalIdentity(write func(...string) error) error {
	for _, pair := range [][2]string{{"user.name", "Serenity Hosted"}, {"user.email", "hosted@serenity.sire.run"}} {
		if err := write("config", "--local", pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}
