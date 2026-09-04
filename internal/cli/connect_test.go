package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/secrets"
)

// TestConnectRotateTokenInvalidatesOldToken proves T4.3's acc line:
// `serenity connect --rotate-token` invalidates the old token. secrets
// itself is exercised end to end (TestMain mocks the keyring, never the
// real OS keychain); internal/server has the matching HTTP-level proof
// that the old token then gets 401 and the new one gets 200.
func TestConnectRotateTokenInvalidatesOldToken(t *testing.T) {
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	oldToken, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("DaemonToken: %v", err)
	}

	var out bytes.Buffer
	if err := runConnectRotateToken(&out); err != nil {
		t.Fatalf("runConnectRotateToken: %v", err)
	}
	if !strings.Contains(out.String(), "rotated") {
		t.Fatalf("expected rotation confirmation, got: %q", out.String())
	}

	newToken, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("DaemonToken after rotation: %v", err)
	}
	if newToken == oldToken {
		t.Fatal("token unchanged after --rotate-token")
	}
}

// TestConnectNeverWritesTokenToDisk is the executable form of the acc
// line "a grep of the repo and .serenity/ finds no token bytes anywhere
// on disk": the token lives only in the (mocked) OS keychain. Runs
// init, rotates the token, then greps every file under the brain repo
// root -- which is where .serenity/ would live -- for both token values.
func TestConnectNeverWritesTokenToDisk(t *testing.T) {
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	initialToken, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("DaemonToken: %v", err)
	}

	var out bytes.Buffer
	if err := runConnectRotateToken(&out); err != nil {
		t.Fatalf("runConnectRotateToken: %v", err)
	}
	rotatedToken, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("DaemonToken after rotation: %v", err)
	}

	grepTreeFor(t, root, initialToken)
	grepTreeFor(t, root, rotatedToken)
}

// TestConnectStatusHonestAboutWhatIsBuilt proves bare `serenity connect`
// (no flags) reports the daemon token's presence and does not claim the
// T4.8 client-config behavior it doesn't implement yet.
func TestConnectStatusHonestAboutWhatIsBuilt(t *testing.T) {
	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runConnectStatus(&out); err != nil {
		t.Fatalf("runConnectStatus: %v", err)
	}
	if !strings.Contains(out.String(), "daemon auth token present") {
		t.Fatalf("expected token-presence report, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "not implemented yet") {
		t.Fatalf("expected honest not-implemented note, got: %q", out.String())
	}
}
