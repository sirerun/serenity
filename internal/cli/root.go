// Package cli implements the serenity command-line surface. The CLI is
// the first conformant client of all Serenity protocols (RFC §13.1): thin
// wrappers over one engine, no privileged internal path.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is injected at release time via ldflags.
var Version = "dev"

var flagRoot string

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "serenity",
		Short:         "Claim-based personal memory and direction system",
		Long:          "Serenity ingests everything one person produces, reconciles what is true\nin a git-canonical brain repository, and serves that truth plus the\nperson's standing judgments to AI agents over open protocols.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.PersistentFlags().StringVarP(&flagRoot, "root", "C", ".", "brain repo root")
	root.AddCommand(newInitCmd(), newSyncCmd(), newExtractCmd(), newDoctorCmd(), newStatusCmd(), newCompactCmd(), newConnectorsCmd(), newSearchCmd(), newMigrateCmd(), newAskCmd(), newCheckCmd(), newConnectCmd(), newConfigCmd())
	return root
}

// Execute runs the CLI.
func Execute() error {
	return newRootCmd().Execute()
}
