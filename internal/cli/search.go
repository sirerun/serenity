package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/providers"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/search"
)

func newSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Hybrid search over the brain repo (RRF-fused vector + full-text, deduplicated)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd.Context(), flagRoot, strings.Join(args, " "), limit, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum results to return")
	return cmd
}

// runSearch is the T1.11 CLI surface over internal/search: RRF-fused
// vector+FTS ranking through the 4 dedup layers (RFC §10.1, §16).
//
// No provider-from-config wiring exists yet to build a live embed.Embedder
// (that lands with real extraction, T1.15) -- a pinned embedding model
// with no way to call it degrades to full-text-only search, stated
// plainly, rather than erroring or silently pretending to search vectors.
// This mirrors runExtract's precedent for an unpinned/unreachable model.
// searchResults runs the exact search runSearch prints, returning the raw
// ranked results plus the human-readable degrade note runSearch would
// otherwise print directly. Split out so a drift test (T4.9, CLI vs
// protocol) can invoke the identical code path `search` runs and compare
// its output against MEMORY_VERBS's recall verb (internal/server/memory)
// without reimplementing runSearch's own logic -- a genuine drift test has
// to exercise the real CLI function, not a hand-rolled duplicate of it.
func searchResults(ctx context.Context, root, query string, limit int) ([]search.Result, string, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return nil, "", fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = eng.Close() }()

	var note string
	if cfg.Models.Embedding == "" || cfg.Models.Embedding == "none@v0" {
		note = "no embedding model pinned; running full-text-only search"
	} else {
		note = fmt.Sprintf("embedding model %s pinned; live embedding calls land with real extraction wiring (T1.15) -- running full-text-only search", cfg.Models.Embedding)
	}

	results, err := search.Search(ctx, eng, nil, query, limit, search.Options{})
	if err != nil {
		return nil, "", err
	}
	return results, note, nil
}

func runSearch(ctx context.Context, root, query string, limit int, out io.Writer) error {
	results, note, err := searchResults(ctx, root, query, limit)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, note)
	if len(results) == 0 {
		_, _ = fmt.Fprintln(out, "no results")
		return nil
	}
	for i, r := range results {
		_, _ = fmt.Fprintf(out, "%2d. %-24s score=%.4f  %s\n", i+1, r.ChunkRef, r.RRFScore, truncateText(r.Text, 80))
	}
	return nil
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
