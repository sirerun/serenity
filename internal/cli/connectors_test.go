package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	imapconn "github.com/sirerun/serenity/internal/connector/imap"
)

// authIMAPCmd builds a throwaway command carrying the --email flag and
// wired stdin/stdout, the same shape newConnectorsAuthCmd's RunE sees --
// matching how the rest of this package tests CLI logic by calling the
// run* function directly rather than through cobra's Execute (see
// TestInitScaffoldsBrainRepo et al.).
func authIMAPCmd(t *testing.T, email, stdin string, out *bytes.Buffer) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("email", "", "")
	if email != "" {
		if err := cmd.Flags().Set("email", email); err != nil {
			t.Fatalf("set --email: %v", err)
		}
	}
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(out)
	return cmd
}

// grepTreeFor asserts substr appears in no file under root -- the
// executable form of T1.4's acc line "a grep of the repo and .serenity/
// finds no secret".
func grepTreeFor(t *testing.T, root, substr string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(b), substr) {
			t.Errorf("secret found on disk at %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// TestConnectorsAuthIMAPStoresPasswordInKeychain proves T1.4's acc line:
// `serenity connectors auth imap` stores the app password in the
// keychain and a grep of the repo and .serenity/ finds no secret.
func TestConnectorsAuthIMAPStoresPasswordInKeychain(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	const email = "alice@example.com"
	const password = "correct horse battery staple app password"

	var out bytes.Buffer
	cmd := authIMAPCmd(t, email, password+"\n", &out)
	if err := runConnectorsAuthIMAP(cmd, root); err != nil {
		t.Fatalf("runConnectorsAuthIMAP: %v", err)
	}

	got, err := imapconn.StoredPassword(email)
	if err != nil {
		t.Fatalf("StoredPassword: %v", err)
	}
	if got != password {
		t.Fatalf("stored password = %q, want %q", got, password)
	}

	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	imapCfg, ok := cfg.Connectors["imap"].(map[string]any)
	if !ok {
		t.Fatalf("serenity.yml connectors.imap = %#v, want a map with account", cfg.Connectors["imap"])
	}
	if imapCfg["account"] != email {
		t.Fatalf("serenity.yml connectors.imap.account = %v, want %q", imapCfg["account"], email)
	}

	// Exercise the .serenity/ runtime dir too, matching the acc line's
	// exact "repo and .serenity/" wording.
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0o755); err != nil {
		t.Fatal(err)
	}
	grepTreeFor(t, root, password)
}

func TestConnectorsAuthIMAPRequiresEmail(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	cmd := authIMAPCmd(t, "", "some-password\n", &out)
	if err := runConnectorsAuthIMAP(cmd, root); err == nil {
		t.Fatal("want an error when --email is missing, got nil")
	}
}

func TestConnectorsAuthIMAPRejectsEmptyPassword(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := authIMAPCmd(t, "alice@example.com", "\n", &out)
	if err := runConnectorsAuthIMAP(cmd, root); err == nil {
		t.Fatal("want an error on an empty app password, got nil")
	}
}

// TestConnectorsAuthIMAPReAuthOverwrites proves re-auth is one command
// (RFC 0001 §10.1): running auth again with a new password replaces the
// old one in the keychain rather than erroring or stacking entries.
func TestConnectorsAuthIMAPReAuthOverwrites(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}
	const email = "alice@example.com"

	var out1 bytes.Buffer
	if err := runConnectorsAuthIMAP(authIMAPCmd(t, email, "first-password\n", &out1), root); err != nil {
		t.Fatalf("first auth: %v", err)
	}
	var out2 bytes.Buffer
	if err := runConnectorsAuthIMAP(authIMAPCmd(t, email, "second-password\n", &out2), root); err != nil {
		t.Fatalf("re-auth: %v", err)
	}

	got, err := imapconn.StoredPassword(email)
	if err != nil {
		t.Fatalf("StoredPassword: %v", err)
	}
	if got != "second-password" {
		t.Fatalf("stored password = %q, want the re-auth's %q", got, "second-password")
	}
}
