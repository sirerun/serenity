package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/server/mcp"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "Serve MCP over standard input and output", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		stdio, err := cmd.Flags().GetBool("stdio")
		if err != nil {
			return err
		}
		if !stdio {
			return fmt.Errorf("choose --stdio to serve MCP")
		}
		server, err := mcp.New(Version, nil)
		if err != nil {
			return err
		}
		input := cmd.InOrStdin()
		closer, ok := input.(io.ReadCloser)
		if !ok {
			// In-memory readers used by embedded callers are finite. Arbitrary blocking
			// readers must expose Close so cancellation can unblock them.
			if _, finite := input.(interface{ Len() int }); !finite {
				return fmt.Errorf("MCP input must be closeable or an in-memory reader")
			}
			closer = io.NopCloser(input)
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return server.Serve(ctx, closer, cmd.OutOrStdout())
	}}
	cmd.Flags().Bool("stdio", false, "read and write newline-delimited MCP JSON-RPC")
	return cmd
}
