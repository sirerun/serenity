package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/store"
)

// newCompactCmd wires the explicit, opt-in compaction verb (RFC §7.7).
// Compaction is destructive to shard file layout (it moves dead lines into
// an archive shard) so it stays gated behind --confirm until M2 replaces
// this gate with an approved disposition item (T2.9).
func newCompactCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "compact",
		Short: "Move superseded/retracted shard lines into per-family archives (requires --confirm)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCompact(flagRoot, confirm, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm the compaction pass (required — until M2 gates this behind an approved disposition item)")
	return cmd
}

// runCompact walks every shard family and archives its dead (superseded or
// retracted) lines, leaving only resolved heads in the live shard.
// ShardStore.Compact (internal/store/shard.go) does the actual file work
// and is already correct and tested — this is purely the explicit-gate CLI
// surface RFC §7.7 requires.
func runCompact(root string, confirm bool, out io.Writer) error {
	if !confirm {
		return fmt.Errorf("compact rewrites shard files on disk — re-run with --confirm (M2 will replace this gate with an approved disposition item)")
	}

	ss := store.NewShardStore(root)
	slugs, err := ss.Slugs()
	if err != nil {
		return err
	}

	var total int
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return err
		}
		for _, family := range families {
			moved, err := ss.Compact(slug, family)
			if err != nil {
				return fmt.Errorf("compact %s/%s: %w", slug, family, err)
			}
			if moved > 0 {
				_, _ = fmt.Fprintf(out, "compacted %s/%s: %d line(s) archived\n", slug, family, moved)
			}
			total += moved
		}
	}
	_, _ = fmt.Fprintf(out, "compact complete: %d line(s) archived across %d entit(y/ies)\n", total, len(slugs))
	return nil
}
