package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/secrets"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check brain repo health: config, durability, keychain, index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(flagRoot, cmd.OutOrStdout())
		},
	}
}

func runDoctor(root string, out io.Writer) error {
	ok := func(format string, a ...any) { fmt.Fprintf(out, "ok    "+format+"\n", a...) }
	warn := func(format string, a ...any) { fmt.Fprintf(out, "WARN  "+format+"\n", a...) }

	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ok("serenity.yml parses (index engine %s, %d predicate families)", cfg.Index.Engine, len(cfg.Families))
	ok("pinned model set: embedding %s, extraction %s", cfg.Models.Embedding, cfg.Models.Extraction)

	// Durability floor (§7.7): a healthy remote is the backup story.
	if !isGitRepo(root) {
		warn("not a git repository — no history, no durability; run `serenity init`")
	} else if remotes := gitRemotes(root); len(remotes) == 0 {
		warn("no git remote — a lost disk is lost data; add a remote and push")
	} else if n := gitUnpushed(root); n < 0 {
		warn("remote %s configured but no upstream tracking — `git push -u`", remotes[0])
	} else if n > 0 {
		warn("%d commit(s) not yet pushed to the remote", n)
	} else {
		ok("git remote configured and fully pushed")
	}

	if _, err := secrets.DaemonToken(); err != nil {
		warn("daemon auth token missing from OS keychain — run `serenity init`")
	} else {
		ok("daemon auth token present in OS keychain")
	}

	dbPath := filepath.Join(root, ".serenity", "index.db")
	if fi, err := os.Stat(dbPath); err != nil {
		warn("derived index absent — run `serenity sync` (it rebuilds from the repo)")
	} else {
		ok("derived index present (%d bytes; rebuildable, never canonical)", fi.Size())
	}
	return nil
}
