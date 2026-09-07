package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/cron"
)

// newCronCmd wires `serenity cron <job>` (ADR 006, plan T2.19): run one
// scheduled job to completion and exit. internal/cron owns the actual job
// registry and bodies; this is purely the CLI surface, matching every
// other verb in this package.
func newCronCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "cron <job>",
		Short:     fmt.Sprintf("Run one scheduled job to completion and exit (%s)", strings.Join(cron.Names(), ", ")),
		Args:      cobra.ExactArgs(1),
		ValidArgs: cron.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCron(cmd.Context(), flagRoot, args[0], cmd.OutOrStdout())
		},
	}
}

// runCron runs job against root with the production clock and reports the
// outcome to out. Errors (including an unknown job name) propagate
// unchanged from internal/cron.Run, so `serenity cron bogus` exits non-zero
// with a message naming every valid job.
func runCron(ctx context.Context, root, job string, out io.Writer) error {
	if job == "revisit" {
		result, err := cron.RunRevisit(ctx, root, cron.RealClock)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "cron revisit: ok (created=%d unsupported=%d)\n", result.Created, result.Unsupported)
		return err
	}
	if err := cron.Run(ctx, job, root, cron.RealClock); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "cron %s: ok\n", job)
	return nil
}
