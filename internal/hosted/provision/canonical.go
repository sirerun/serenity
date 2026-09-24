package provision

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// initializeCanonical is called only for an allocating brain under ownership.
// Existing ready brains are never silently recreated. Model configuration is
// runtime-owned; this baseline deliberately contains only the cache exclusion.
func initializeCanonical(ctx context.Context, root string) error {
	git := func(args ...string) error {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("initialize canonical brain: %w: %s", err, out)
		}
		return nil
	}
	info, err := os.Lstat(filepath.Join(root, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		if err = git("init", "--initial-branch=main"); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe canonical Git directory")
	}
	// Persist the identity for later writer.Flush commits, which do not carry
	// command-local author overrides. This runs while provisioning owns the brain.
	for _, pair := range [][2]string{{"user.name", "Serenity Hosted"}, {"user.email", "hosted@serenity.sire.run"}} {
		if err = git("config", "--local", pair[0], pair[1]); err != nil {
			return err
		}
	}
	if err = git("rev-parse", "--verify", "HEAD"); err == nil {
		// A commit alone does not prove that initialization completed. Validate
		// the exact committed baseline and its working file without repairing it.
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
		committed, err := exec.CommandContext(ctx, "git", "-C", root, "show", "HEAD:.gitignore").Output()
		if err != nil {
			return fmt.Errorf("canonical baseline is not committed: %w", err)
		}
		if string(data) != ".serenity/\n" || string(committed) != string(data) {
			return errors.New("canonical baseline differs from required contents")
		}
		return nil
	}
	// Permit retry after git init but before the initial commit. A detached or
	// corrupt HEAD must not be treated as a new repository.
	if err = git("symbolic-ref", "--quiet", "HEAD"); err != nil {
		return err
	}
	path := filepath.Join(root, ".gitignore")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := f.WriteString(".serenity/\n")
		syncErr := f.Sync()
		err = errors.Join(writeErr, syncErr, f.Close())
		if err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsafe canonical baseline file")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if string(data) != ".serenity/\n" {
			return errors.New("unexpected canonical baseline contents")
		}
	} else {
		return err
	}
	if err = git("add", "--", ".gitignore"); err != nil {
		return err
	}
	// --only avoids incorporating anything already staged by another actor.
	return git("-c", "user.name=Serenity Hosted", "-c", "user.email=hosted@serenity.sire.run", "commit", "--only", "-m", "Initialize hosted brain", "--", ".gitignore")
}
