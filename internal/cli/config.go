package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
)

// newConfigCmd is `serenity config`, the parent for config-editing
// convenience subcommands. Today it has exactly one child, set-model
// (T1.25); more subcommands attach here as they ship.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Edit serenity.yml without hand-editing the file",
	}
	cmd.AddCommand(newConfigSetModelCmd())
	return cmd
}

// newConfigSetModelCmd is `serenity config set-model <purpose> <model>`
// (plan T1.25, ADR 013 item 4, UC-048): a one-command way to rewrite a
// single pinned model in serenity.yml instead of hand-editing the file.
// It rewrites exactly the named pin -- the other two pins, and everything
// else in the file, are left untouched.
func newConfigSetModelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-model <extraction|embedding|composer> <model>",
		Short: "Rewrite one pinned model in serenity.yml",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSetModel(flagRoot, args[0], args[1], cmd.OutOrStdout())
		},
	}
}

// runConfigSetModel loads root's serenity.yml, sets the named purpose's
// pin to model, and saves it back. purpose must be exactly one of
// "extraction", "embedding", "composer"; any other value is an error and
// cfg.Save is never called, so an unknown purpose leaves the file
// byte-for-byte untouched -- no partial write.
func runConfigSetModel(root, purpose, model string, out io.Writer) error {
	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	switch purpose {
	case "extraction":
		cfg.Models.Extraction = model
	case "embedding":
		cfg.Models.Embedding = model
	case "composer":
		cfg.Models.Composer = model
	default:
		return fmt.Errorf("unknown purpose %q: must be one of extraction, embedding, composer", purpose)
	}

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("save %s: %w", config.FileName, err)
	}
	_, _ = fmt.Fprintf(out, "models.%s = %s\n", purpose, model)
	return nil
}
