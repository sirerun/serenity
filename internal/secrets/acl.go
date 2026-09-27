package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// securityPath is macOS's keychain command-line tool, the same binary
// go-keyring drives.
const securityPath = "/usr/bin/security"

// base64Prefix is go-keyring's marker for a base64-encoded secret; writing
// it lets keyring.Get decode items this package creates (SEC-L08).
const base64Prefix = "go-keyring-base64:"

// trustedExecutable is the absolute, symlink-resolved path of the running
// serenity binary: the one application a keychain item's access control
// list trusts.
func trustedExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("secrets: locate executable for keychain ACL: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("secrets: resolve executable for keychain ACL: %w", err)
	}
	return resolved, nil
}

// addGenericPasswordCommand renders the `security -i` line that creates a
// generic-password item whose access control list trusts only the given
// executable (-T). Without -T the creating application, /usr/bin/security,
// is trusted, and any same-user process can read the item through it with
// no prompt (SEC-L08). No -U: an existing item keeps its old ACL on update,
// so callers delete it first. keychain empty means the default keychain.
// The line goes to stdin so the secret never appears in the process list.
func addGenericPasswordCommand(keychain, service, account, secret, trusted string) string {
	encoded := base64Prefix + base64.StdEncoding.EncodeToString([]byte(secret))
	var b strings.Builder
	fmt.Fprintf(&b, "add-generic-password -s %s -a %s -T %s -w %s",
		quote(service), quote(account), quote(trusted), quote(encoded))
	if keychain != "" {
		b.WriteString(" " + quote(keychain))
	}
	b.WriteString("\n")
	return b.String()
}

// quote single-quotes s for the shell-style word splitting `security -i`
// applies to each input line.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// addGenericPassword replaces any existing macOS keychain item for
// service/account in keychain ("" for the default keychain) with one whose
// ACL trusts only the trusted executable.
func addGenericPassword(keychain, service, account, secret, trusted string) error {
	line := addGenericPasswordCommand(keychain, service, account, secret, trusted)
	if len(line) > 4096 {
		return keyring.ErrSetDataTooBig
	}
	if err := deleteGenericPassword(keychain, service, account); err != nil {
		return err
	}
	cmd := exec.Command(securityPath, "-i")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return err
	}
	_, werr := io.WriteString(stdin, line)
	cerr := stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("secrets: add keychain item: %w: %s", err, strings.TrimSpace(out.String()))
	}
	// security -i exits 0 even when a line fails; its output reports it.
	if msg := strings.TrimSpace(out.String()); strings.Contains(msg, "security:") || strings.Contains(msg, "rror") {
		return fmt.Errorf("secrets: add keychain item: %s", msg)
	}
	return errors.Join(werr, cerr)
}

// deleteGenericPassword removes service/account from keychain, treating an
// absent item as success.
func deleteGenericPassword(keychain, service, account string) error {
	args := []string{"delete-generic-password", "-s", service, "-a", account}
	if keychain != "" {
		args = append(args, keychain)
	}
	out, err := exec.Command(securityPath, args...).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("secrets: replace keychain item: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
