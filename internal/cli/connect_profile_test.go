package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/secrets"
)

// TestConnectProvisionProfileIsFreshAndIndependent proves provisioning two
// different profiles mints two different tokens, neither equal to the
// legacy shared one, and re-provisioning is idempotent (never re-mints).
func TestConnectProvisionProfileIsFreshAndIndependent(t *testing.T) {
	legacy, _, err := secrets.EnsureDaemonToken()
	if err != nil {
		t.Fatalf("EnsureDaemonToken: %v", err)
	}

	var outA bytes.Buffer
	if err := runConnectProvisionProfile("conn-profile-a", &outA); err != nil {
		t.Fatalf("provision a: %v", err)
	}
	if !strings.Contains(outA.String(), "provisioned") {
		t.Fatalf("expected provisioning confirmation, got %q", outA.String())
	}
	tokenA, err := secrets.ProfileDaemonToken("conn-profile-a")
	if err != nil {
		t.Fatalf("ProfileDaemonToken(a): %v", err)
	}
	if tokenA == legacy {
		t.Fatal("profile token equals legacy shared token")
	}

	var outB bytes.Buffer
	if err := runConnectProvisionProfile("conn-profile-b", &outB); err != nil {
		t.Fatalf("provision b: %v", err)
	}
	tokenB, err := secrets.ProfileDaemonToken("conn-profile-b")
	if err != nil {
		t.Fatalf("ProfileDaemonToken(b): %v", err)
	}
	if tokenA == tokenB {
		t.Fatal("two profiles minted the identical token")
	}

	var outAgain bytes.Buffer
	if err := runConnectProvisionProfile("conn-profile-a", &outAgain); err != nil {
		t.Fatalf("provision a again: %v", err)
	}
	if !strings.Contains(outAgain.String(), "already provisioned") {
		t.Fatalf("expected idempotent no-op message, got %q", outAgain.String())
	}
	if again, err := secrets.ProfileDaemonToken("conn-profile-a"); err != nil || again != tokenA {
		t.Fatal("re-provisioning changed the token")
	}
	for _, s := range []string{outA.String(), outB.String(), outAgain.String()} {
		if strings.Contains(s, tokenA) || strings.Contains(s, tokenB) || strings.Contains(s, legacy) {
			t.Fatal("token value leaked into CLI output")
		}
	}
}

// TestConnectRotateProfileScoped proves rotation is scoped to exactly the
// named profile: the legacy token and a second, unrelated profile are
// unaffected.
func TestConnectRotateProfileScoped(t *testing.T) {
	legacyBefore, _, err := secrets.EnsureDaemonToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := secrets.EnsureProfileDaemonToken("rot-profile-a"); err != nil {
		t.Fatal(err)
	}
	tokenBBefore, _, err := secrets.EnsureProfileDaemonToken("rot-profile-b")
	if err != nil {
		t.Fatal(err)
	}
	tokenABefore, err := secrets.ProfileDaemonToken("rot-profile-a")
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runConnectRotateProfile("rot-profile-a", &out); err != nil {
		t.Fatalf("rotate a: %v", err)
	}
	if !strings.Contains(out.String(), "rotated") {
		t.Fatalf("expected rotation confirmation, got %q", out.String())
	}
	if strings.Contains(out.String(), tokenABefore) {
		t.Fatal("rotation output leaked the old token")
	}

	tokenAAfter, err := secrets.ProfileDaemonToken("rot-profile-a")
	if err != nil {
		t.Fatal(err)
	}
	if tokenAAfter == tokenABefore {
		t.Fatal("rotation did not change profile a's token")
	}
	if tokenBAfter, err := secrets.ProfileDaemonToken("rot-profile-b"); err != nil || tokenBAfter != tokenBBefore {
		t.Fatal("rotating profile a changed profile b's token")
	}
	if legacyAfter, err := secrets.DaemonToken(); err != nil || legacyAfter != legacyBefore {
		t.Fatal("rotating a profile changed the legacy shared token")
	}
}

// TestConnectProfileStatusIsReadOnly proves bare `connect
// --credential-profile NAME` (no --provision-token/--rotate-token) never
// mints a token as a side effect of checking one -- the contract's "keep
// bare connect/status read-only."
func TestConnectProfileStatusIsReadOnly(t *testing.T) {
	var out bytes.Buffer
	if err := runConnectProfileStatus("status-only-profile", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no token yet") {
		t.Fatalf("expected absent-token report, got %q", out.String())
	}
	if _, err := secrets.ProfileDaemonToken("status-only-profile"); err == nil {
		t.Fatal("status check provisioned a token as a side effect")
	}
	if !strings.Contains(out.String(), "not proof of a stored per-brain binding") {
		t.Fatal("status output must disclose profiles are not persistent per-brain bindings")
	}

	if _, _, err := secrets.EnsureProfileDaemonToken("status-only-profile"); err != nil {
		t.Fatal(err)
	}
	var out2 bytes.Buffer
	if err := runConnectProfileStatus("status-only-profile", &out2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2.String(), "has a token") {
		t.Fatalf("expected present-token report, got %q", out2.String())
	}
}

// TestConnectProfileFlagConflicts exercises the full CLI (cobra flag
// parsing) for the invalid combinations the contract names: --provision-token
// without --credential-profile, and --provision-token with --rotate-token
// together.
func TestConnectProfileFlagConflicts(t *testing.T) {
	if _, _, code := connectCommand(t, "", "connect", "--provision-token"); code == 0 {
		t.Fatal("--provision-token without --credential-profile must fail")
	}
	if _, _, code := connectCommand(t, "", "connect", "--credential-profile", "conflict-profile", "--provision-token", "--rotate-token"); code == 0 {
		t.Fatal("--provision-token and --rotate-token together must fail")
	}
	if _, _, code := connectCommand(t, "", "connect", "--credential-profile", "Not Valid"); code == 0 {
		t.Fatal("malformed profile name must fail closed")
	}
	if _, _, code := connectCommand(t, "", "connect", "--credential-profile", ""); code == 0 {
		t.Fatal("explicit empty profile name must fail closed")
	}
}

// TestConnectBareStatusUnaffectedByProfileFlag proves the legacy path
// (--credential-profile entirely absent) is exactly the pre-existing
// behavior: bare `connect` still reports the legacy token, not a profile.
func TestConnectBareStatusUnaffectedByProfileFlag(t *testing.T) {
	if _, _, err := secrets.EnsureDaemonToken(); err != nil {
		t.Fatal(err)
	}
	out, _, code := connectCommand(t, "", "connect")
	if code != 0 {
		t.Fatalf("bare connect failed: %d %s", code, out)
	}
	if !strings.Contains(out, "daemon auth token present") {
		t.Fatalf("bare connect did not report the legacy token: %q", out)
	}
	if strings.Contains(out, "credential profile") {
		t.Fatalf("bare connect must not mention profiles at all: %q", out)
	}
}
