package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newForgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forget <id>",
		Short: "Forget a memory fact through the MEMORY_VERBS service",
		Long: `Forget removes a memory fact and rewrites the brain's Git history so the
fact's source path and objects are removed from local refs and reflogs. The
next post-commit push uses --force-with-lease; other clones must re-clone.

Run this operation with the MCP MEMORY_VERBS v1 forget tool exposed by
"serenity serve --stdio" or "serenity serve --http". This command does not
perform the forget operation itself.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return errors.New("serenity forget requires the MCP MEMORY_VERBS v1 forget tool; connect to a running Serenity server and call forget")
		},
	}
}
