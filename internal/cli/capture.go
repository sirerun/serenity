package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

// newCaptureCmd wires `serenity capture`, the CLI surface over DISPOSITION
// v1's capture (RFC 0001 §8.2/§10.1, plan T2.20, UC-034): "zero-friction
// ingress; returns a staged distill item id." Routing a staged capture to
// one of its four destinations (claim-batch/precept-draft/note/trash,
// internal/disposition.RouteDistill) is a disposition-queue operation, not
// a capture-time one -- it is exposed once the CLI inbox (T2.5) or a
// protocol client (T4.4) can list and dispose of pending items generally,
// not here.
func newCaptureCmd() *cobra.Command {
	var audioRef, hint string
	cmd := &cobra.Command{
		Use:   "capture [text]",
		Short: "Stage a zero-friction capture into the distill queue (DISPOSITION v1 capture)",
		Long: "capture stages free text or a reference to an audio recording as a\n" +
			"pending distill item, returning its id. Nothing is written to the brain\n" +
			"repo yet -- a later disposition (claim-batch, precept-draft, note, or\n" +
			"trash) decides what, if anything, becomes durable.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if len(args) == 1 {
				text = args[0]
			}
			return runCapture(cmd.Context(), flagRoot, text, audioRef, hint, cmd.OutOrStdout(), time.Now())
		},
	}
	cmd.Flags().StringVar(&audioRef, "audio", "", "reference to an audio recording (mutually exclusive with neither -- a caption may accompany an audio ref)")
	cmd.Flags().StringVar(&hint, "hint", "", "optional hint for how the capture should eventually be routed")
	return cmd
}

// runCapture opens root's derived index (creating it if needed -- runtime
// state, RFC §7.5, not a brain write) and stages one distill item via
// internal/disposition.Capture.
func runCapture(ctx context.Context, root, text, audioRef, hint string, out io.Writer, now time.Time) error {
	if _, err := config.Load(filepath.Join(root, config.FileName)); err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	store := disposition.NewStore(eng)
	item, err := disposition.Capture(ctx, store, text, audioRef, hint, now)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "staged distill item %s\n", item.ID)
	return nil
}
