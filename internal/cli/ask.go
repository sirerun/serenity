package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/providers"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/embed"
)

func newAskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Compose a cited answer from the brain's accumulated claims (RFC 0001 section 11)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAsk(cmd.Context(), flagRoot, strings.Join(args, " "), cmd.OutOrStdout())
		},
	}
	return cmd
}

// runAsk is the T1.12 CLI surface over internal/compose: RFC section 11's
// composer -- a cited answer with any superseded fact's chain rendered
// alongside it, or an explicit gap statement naming the newest evidence's
// age, never a fabricated answer.
func runAsk(ctx context.Context, root, question string, out io.Writer) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	ledger := &providers.IndexSpendLedger{Eng: eng}

	r, ok, note := providers.BuildComposerRouter(cfg, ledger)
	if !ok {
		_, _ = fmt.Fprintln(out, note)
		return nil
	}

	// Query-time embedding widens which subjects chunk search surfaces
	// (compose.Composer.relevantSubjects); an unpinned or uncredentialed
	// embedding model degrades to FTS-only relevance, same fallback
	// runSearch documents for search itself -- never an error, never a
	// silent skip.
	var embedder embed.Embedder
	if er, eok, enote := providers.BuildEmbeddingRouter(cfg, ledger); eok {
		embedder = &embed.RouterEmbedder{Router: er, Pin: cfg.Models.Embedding}
	} else {
		_, _ = fmt.Fprintf(out, "%s -- widening query relevance to full-text/lexical matching only\n", enote)
	}

	c := compose.New(root, cfg, eng, embedder, r, cfg.Models.Composer)
	answer, err := c.Ask(ctx, question)
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	if answer.Gap != "" {
		_, _ = fmt.Fprintln(out, answer.Gap)
		return nil
	}
	_, _ = fmt.Fprintln(out, answer.Text)
	return nil
}
