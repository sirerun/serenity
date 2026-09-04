package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/secrets"
)

// newConnectCmd is `serenity connect` (plan T4.3, T4.8). T4.8 fills in
// the real behavior -- an MCP config stanza + pre-plan hook for Claude
// Code, token never written to the config file; today only
// --rotate-token is implemented, and bare `connect` says so honestly
// rather than looking like a broken no-op.
func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect an external client (e.g. Claude Code) to the Serenity daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rotate, err := cmd.Flags().GetBool("rotate-token")
			if err != nil {
				return err
			}
			if rotate {
				return runConnectRotateToken(cmd.OutOrStdout())
			}
			return runConnectStatus(cmd.OutOrStdout())
		},
	}
	cmd.Flags().Bool("rotate-token", false, "mint a new daemon bearer token, invalidating the old one")
	return cmd
}

// runConnectRotateToken is `serenity connect --rotate-token` (RFC §14,
// ADR 010: "a leaked token is revoked by one command"). The old token
// stops authenticating on the next request; nothing else needs to
// restart, since the HTTP transport reads the token from the keychain
// per request rather than caching it.
func runConnectRotateToken(out io.Writer) error {
	if _, err := secrets.RotateDaemonToken(); err != nil {
		return fmt.Errorf("rotate daemon auth token: %w", err)
	}
	_, _ = fmt.Fprintf(out, "daemon auth token rotated (service %q); the previous token no longer authenticates\n", secrets.Service)
	return nil
}

// runConnectStatus is bare `serenity connect` with no flags.
func runConnectStatus(out io.Writer) error {
	if _, err := secrets.DaemonToken(); err != nil {
		_, _ = fmt.Fprintln(out, "no daemon auth token yet -- run `serenity init` first")
	} else {
		_, _ = fmt.Fprintf(out, "daemon auth token present in the OS keychain (service %q)\n", secrets.Service)
	}
	_, _ = fmt.Fprintln(out, "client config (MCP stanza + hook install) is not implemented yet -- see plan T4.8")
	_, _ = fmt.Fprintln(out, "available now: `serenity connect --rotate-token`")
	return nil
}
