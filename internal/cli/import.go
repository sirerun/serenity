package cli

import (
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/import/gbrain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var source string
	cmd := &cobra.Command{Use: "import --from-gbrain <repo>", Short: "Import gbrain markdown pages with provenance and review flags", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if source == "" {
			return fmt.Errorf("import: --from-gbrain is required")
		}
		cfg, err := config.Load(filepath.Join(flagRoot, config.FileName))
		if err != nil {
			return err
		}
		result, err := gbrain.Import(cmd.Context(), source, flagRoot, cfg)
		if err != nil {
			return err
		}
		eng, err := providers.OpenIndex(flagRoot)
		if err != nil {
			return err
		}
		defer func() { _ = eng.Close() }()
		if err := rebuildTimed(cmd.Context(), flagRoot, cfg, eng); err != nil {
			return fmt.Errorf("import: canonical pages committed; rebuild failed: %w", err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "imported %d page(s), %d claim(s); %d identical page(s) skipped; semantic translations require review\n", result.Pages, result.Claims, result.Skipped)
		return err
	}}
	cmd.Flags().StringVar(&source, "from-gbrain", "", "gbrain repository checkout to read")
	return cmd
}
