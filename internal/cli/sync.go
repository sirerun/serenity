package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Rebuild the derived index from canonical repo bytes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSync(cmd.Context(), flagRoot, cmd.OutOrStdout())
		},
	}
}

func newExtractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extract [all]",
		Short: "Run extraction over ingested sources",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExtract(cmd.Context(), flagRoot, cmd.OutOrStdout())
		},
	}
}

// runSync is the disaster-recovery path (§7): wipe-safe, deterministic,
// rebuilds every derived row from the repo.
func runSync(ctx context.Context, root string, out io.Writer) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := openIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	return printStats(ctx, eng, out)
}

// runExtract runs model extraction over sources. With no extraction model
// pinned (models.extraction: none@v0) there is nothing to extract with —
// stated explicitly, never a silent pass — and the claim index is still
// re-derived from canonical files so `sync && extract all` is the full
// rebuild the RFC names.
func runExtract(ctx context.Context, root string, out io.Writer) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	if cfg.Models.Extraction == "" || cfg.Models.Extraction == "none@v0" {
		_, _ = fmt.Fprintln(out, "no extraction model pinned (models.extraction: none@v0); model extraction skipped")
	} else {
		_, _ = fmt.Fprintf(out, "extraction model %s pinned; model extraction lands in M1 — skipped\n", cfg.Models.Extraction)
	}
	eng, err := openIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "re-derived claim index from canonical files")
	return printStats(ctx, eng, out)
}

// rebuildTimed runs index.Rebuild and records its wall-clock duration via
// eng.RecordRebuildTiming (RFC section 16 "rebuild timing", plan T1.17) --
// `serenity status` reads this back. Timing lives outside Rebuild itself
// so a bare Rebuild call (as internal/index's own invariant tests use)
// never writes into the caches runtime table.
func rebuildTimed(ctx context.Context, root string, cfg *config.Config, eng *index.SQLite) error {
	start := time.Now()
	if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
		return err
	}
	return eng.RecordRebuildTiming(ctx, time.Since(start), time.Now())
}

func openIndex(root string) (*index.SQLite, error) {
	dir := filepath.Join(root, ".serenity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return index.Open(filepath.Join(dir, "index.db"))
}

func printStats(ctx context.Context, eng index.Engine, out io.Writer) error {
	stats, err := eng.Stats(ctx)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "%-10s %d\n", k, stats[k])
	}
	return nil
}
