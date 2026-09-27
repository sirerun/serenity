package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestConnectorTrustDefaultsPerKind pins ADR 022's defaults (T24.16): the
// imap and git_repo connectors are untrusted, file is trusted, and a source
// kind no connector owns is untrusted (fail closed).
func TestConnectorTrustDefaultsPerKind(t *testing.T) {
	c := Default()
	for kind, want := range map[string]Trust{
		"email":    TrustUntrusted,
		"git_repo": TrustUntrusted,
		"file":     TrustTrusted,
		"voice":    TrustTrusted,
		"":         TrustUntrusted,
		"mystery":  TrustUntrusted,
	} {
		if got := c.SourceTrust(kind); got != want {
			t.Errorf("SourceTrust(%q) = %q, want %q", kind, got, want)
		}
	}
	for name, want := range map[string]Trust{"imap": TrustUntrusted, "git_repo": TrustUntrusted, "file": TrustTrusted, "voice": TrustTrusted, "other": TrustUntrusted} {
		if got := c.ConnectorTrust(name); got != want {
			t.Errorf("ConnectorTrust(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestConnectorTrustOverride shows connectors.<name>.trust flips the class in
// either direction; a git_repo list is trusted only when every entry says so.
func TestConnectorTrustOverride(t *testing.T) {
	c, err := Load(writeConfig(t, `version: 1
connectors:
  imap:
    account: someone@example.com
    trust: trusted
  file:
    path: /notes
    trust: untrusted
  git_repo:
    - path: /repos/one
      trust: trusted
    - path: /repos/two
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.SourceTrust("email"); got != TrustTrusted {
		t.Errorf("email = %q, want trusted", got)
	}
	if got := c.SourceTrust("file"); got != TrustUntrusted {
		t.Errorf("file = %q, want untrusted", got)
	}
	if got := c.SourceTrust("git_repo"); got != TrustUntrusted {
		t.Errorf("mixed git_repo = %q, want untrusted", got)
	}
	c, err = Load(writeConfig(t, `version: 1
connectors:
  git_repo:
    - path: /repos/one
      trust: trusted
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.SourceTrust("git_repo"); got != TrustTrusted {
		t.Errorf("all-trusted git_repo = %q, want trusted", got)
	}
}

// TestLoadRejectsUnknownConnectorTrust: a typo must not silently fall back
// to a default in either direction.
func TestLoadRejectsUnknownConnectorTrust(t *testing.T) {
	for name, body := range map[string]string{
		"map":  "version: 1\nconnectors:\n  imap:\n    account: a@example.com\n    trust: mostly\n",
		"list": "version: 1\nconnectors:\n  git_repo:\n    - path: /r\n      trust: yes-please\n",
		"type": "version: 1\nconnectors:\n  file:\n    path: /n\n    trust: [trusted]\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, body))
			if err == nil || !strings.Contains(err.Error(), "trust") {
				t.Fatalf("Load accepted an unknown trust value: err=%v", err)
			}
		})
	}
}
