package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/secrets"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold a brain repository (idempotent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(flagRoot, cmd.OutOrStdout())
		},
	}
}

// runInit scaffolds the RFC §7.1 layout, seeds serenity.yml, initializes
// git with the durability floor (§7.7), and mints the daemon auth token
// into the OS keychain (§14). Safe to re-run: nothing is clobbered.
func runInit(root string, out io.Writer) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, d := range []string{
		filepath.Join("brain", "entities"),
		filepath.Join("brain", "sources"),
		filepath.Join("brain", "claims"),
		filepath.Join(".dira", "entries"),
	} {
		full := filepath.Join(root, d)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return err
		}
		keep := filepath.Join(full, ".gitkeep")
		if _, err := os.Stat(keep); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(keep, nil, 0o644); err != nil {
				return err
			}
		}
	}

	cfgPath := filepath.Join(root, config.FileName)
	if _, err := os.Stat(cfgPath); errors.Is(err, fs.ErrNotExist) {
		if err := config.Default().Save(cfgPath); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "created %s (pinned model set: none — configure providers to pin)\n", config.FileName)
	} else {
		_, _ = fmt.Fprintf(out, "%s exists, left untouched\n", config.FileName)
	}

	if err := ensureGitignore(root); err != nil {
		return err
	}

	if !isGitRepo(root) {
		if err := gitInit(root); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
		_, _ = fmt.Fprintln(out, "initialized git repository")
	}
	if installed, err := installPostCommitPush(root); err != nil {
		return err
	} else if installed {
		_, _ = fmt.Fprintln(out, "installed post-commit push hook (durability floor)")
	}
	if len(gitRemotes(root)) == 0 {
		_, _ = fmt.Fprintln(out, "WARNING: no git remote configured — the brain repo is your only copy.")
		_, _ = fmt.Fprintln(out, "         Disaster recovery is `git clone` + rebuild; add a remote and push:")
		_, _ = fmt.Fprintln(out, "         git remote add origin <url> && git push -u origin main")
	}

	_, created, err := secrets.EnsureDaemonToken()
	if err != nil {
		return fmt.Errorf("daemon auth token: OS keychain unavailable: %w", err)
	}
	if created {
		_, _ = fmt.Fprintf(out, "daemon auth token generated and stored in the OS keychain (service %q)\n", secrets.Service)
	} else {
		_, _ = fmt.Fprintln(out, "daemon auth token already present in the OS keychain")
	}

	_, _ = fmt.Fprintln(out, "brain repo ready")
	return nil
}

func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	const entry = ".serenity/"
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == entry {
			return nil
		}
	}
	content := string(b)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
