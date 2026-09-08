package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/supersede"
)

// CompactPayload is a KindCompact disposition item's payload. Empty today:
// there is exactly one compaction scope (every shard family across every
// entity, T0.9's original global sweep) -- kept as a named struct rather
// than nil so a future scoped-compaction task (a single slug/family) has
// somewhere to add fields without changing the item's Kind or the CLI
// gate's shape.
type CompactPayload struct{}

// newCompactCmd wires `serenity compact` (RFC 0001 §7.7, T0.9/T2.9):
// moves superseded/retracted shard lines into per-family archives.
// Compaction is destructive to shard file layout, so RFC §7.7 requires it
// stay "explicit, disposition-approved... never silently" -- T0.9 shipped
// the mechanics behind a bare --confirm flag "until M2 replaces this gate
// with an approved disposition item"; this is that replacement.
//
// --propose stages a new pending KindCompact disposition item and exits;
// a human reviews and accepts it with `serenity inbox` (space/accept, or
// edit_accept) the same as any other disposition item. --item <id> then
// names that accepted item and actually runs the compaction sweep.
// Running compact with neither flag refuses, naming what to do instead.
func newCompactCmd() *cobra.Command {
	var propose bool
	var item string
	cmd := &cobra.Command{
		Use:   "compact",
		Short: "Move superseded/retracted shard lines into per-family archives (requires an accepted compact disposition item)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCompactCmd(cmd.Context(), flagRoot, propose, item, cmd.OutOrStdout(), time.Now())
		},
	}
	cmd.Flags().BoolVar(&propose, "propose", false, "stage a new pending compact disposition item and print its id, then exit -- review it with `serenity inbox` before running --item")
	cmd.Flags().StringVar(&item, "item", "", "the id of an accepted (KindCompact) disposition item authorizing this compaction pass")
	return cmd
}

// runCompactCmd opens root's derived index (same convention runInbox/
// runCapture/runStatus use) and either stages a proposal or, given an
// accepted item id, runs the actual sweep.
func runCompactCmd(ctx context.Context, root string, propose bool, itemID string, out io.Writer, now time.Time) error {
	if propose && itemID != "" {
		return fmt.Errorf("compact: --propose and --item are mutually exclusive")
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	dispStore := disposition.NewStore(eng)

	if propose {
		payload, err := json.Marshal(CompactPayload{})
		if err != nil {
			return err
		}
		item, err := dispStore.Create(ctx, disposition.KindCompact, payload, "", now)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "staged compact disposition item %s -- review it with `serenity inbox`, then run `serenity compact --item %s`\n", item.ID, item.ID)
		return nil
	}

	if itemID == "" {
		return fmt.Errorf("compact rewrites shard files on disk -- pass --item <id> naming an accepted compact disposition item (stage one first with `serenity compact --propose`)")
	}
	item, err := dispStore.Get(ctx, itemID)
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	if item.Kind != disposition.KindCompact {
		return fmt.Errorf("compact: item %s is kind %q, want %q", itemID, item.Kind, disposition.KindCompact)
	}
	if item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) {
		return fmt.Errorf("compact: item %s is not an accepted compact item (state=%q verdict=%q) -- accept it first via `serenity inbox`", itemID, item.State, item.Verdict)
	}

	result, err := supersede.ApplyAndCommitCompaction(ctx, root, dispStore, item, now)
	if err != nil {
		return err
	}
	if result.AlreadyComplete {
		_, err = fmt.Fprintf(out, "compact already complete: approved pass archived %d line(s); use a new proposal for another pass\n", result.Archived)
	} else {
		_, err = fmt.Fprintf(out, "compact complete: %d line(s) archived across %d entit(y/ies), committed\n", result.Archived, result.Entities)
	}
	return err
}
