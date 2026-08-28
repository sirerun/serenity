package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	imapconn "github.com/sirerun/serenity/internal/connector/imap"
	"github.com/sirerun/serenity/internal/secrets"
)

func newConnectorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connectors",
		Short: "Manage connector authentication",
	}
	cmd.AddCommand(newConnectorsAuthCmd())
	return cmd
}

func newConnectorsAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <connector>",
		Short: "Authenticate a connector, storing its credential in the OS keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "imap":
				return runConnectorsAuthIMAP(cmd, flagRoot)
			default:
				return fmt.Errorf("unknown connector %q (supported: imap)", args[0])
			}
		},
	}
	cmd.Flags().String("email", "", "mailbox address to connect (also the IMAP username)")
	return cmd
}

// runConnectorsAuthIMAP is `serenity connectors auth imap` (RFC 0001
// §10.1, ADR 001): it reads the Gmail app password from stdin -- never
// from a flag, which would land it in shell history -- and stores it in
// the OS keychain via imapconn.StorePassword. Only the non-secret account
// address is written to serenity.yml.
func runConnectorsAuthIMAP(cmd *cobra.Command, root string) error {
	email, err := cmd.Flags().GetString("email")
	if err != nil {
		return err
	}
	if email == "" {
		return fmt.Errorf("--email is required (the Gmail address to connect)")
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "App password for %s (create one at https://myaccount.google.com/apppasswords -- requires 2-Step Verification): ", email)
	password, err := readSecretLine(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read app password: %w", err)
	}
	if password == "" {
		return fmt.Errorf("empty app password")
	}

	if err := imapconn.StorePassword(email, password); err != nil {
		return fmt.Errorf("store app password in OS keychain: %w", err)
	}

	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	if cfg.Connectors == nil {
		cfg.Connectors = map[string]any{}
	}
	// Only the account address is non-secret; the password never touches
	// serenity.yml or any other file (ADR 001: keychain only).
	cfg.Connectors["imap"] = map[string]any{"account": email}
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("save %s: %w", config.FileName, err)
	}

	_, _ = fmt.Fprintf(out, "imap connector authenticated for %s; app password stored in the OS keychain (service %q)\n", email, secrets.Service)
	return nil
}

// readSecretLine reads one line from r and trims its trailing newline.
// Not masked on a terminal -- adding a raw-mode dependency for this alone
// would cut against ADR 003's minimal-dependency stance; the connector
// guide (T1.18) recommends piping the password in from a manager rather
// than typing it at an echoing prompt.
func readSecretLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
