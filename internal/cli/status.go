package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show brain repo status: pins, index counts, durability",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			cfg, err := config.Load(filepath.Join(flagRoot, config.FileName))
			if err != nil {
				return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
			}
			_, _ = fmt.Fprintf(out, "models    embedding=%s extraction=%s\n", cfg.Models.Embedding, cfg.Models.Extraction)
			_, _ = fmt.Fprintf(out, "engine    %s\n", cfg.Index.Engine)
			eng, err := openIndex(flagRoot)
			if err != nil {
				return err
			}
			defer func() { _ = eng.Close() }()
			return printStats(cmd.Context(), eng, out)
		},
	}
}
