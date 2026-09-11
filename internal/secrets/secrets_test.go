package secrets

import "testing"

func TestMain(m *testing.M) {
	MockForTesting() // never touch the real OS keychain from tests
	m.Run()
}

// TestProfileTokensAreIndependent proves RFC-BRAIN-AUTH-02's core keychain
// property: two different profile names never resolve to the same token,
// and neither profile's provisioning or rotation touches the other's
// account or the legacy shared one.
func TestProfileTokensAreIndependent(t *testing.T) {
	legacyBefore, _, err := EnsureDaemonToken()
	if err != nil {
		t.Fatalf("EnsureDaemonToken: %v", err)
	}

	tokenA, created, err := EnsureProfileDaemonToken("profile-a")
	if err != nil {
		t.Fatalf("EnsureProfileDaemonToken(a): %v", err)
	}
	if !created {
		t.Fatal("expected profile-a to be freshly created")
	}
	tokenB, created, err := EnsureProfileDaemonToken("profile-b")
	if err != nil {
		t.Fatalf("EnsureProfileDaemonToken(b): %v", err)
	}
	if !created {
		t.Fatal("expected profile-b to be freshly created")
	}
	if tokenA == tokenB {
		t.Fatal("two different profiles minted the identical token")
	}
	if tokenA == legacyBefore || tokenB == legacyBefore {
		t.Fatal("a profile token copied the legacy shared token's value")
	}

	// Re-provisioning is idempotent: it must not silently re-mint.
	again, created, err := EnsureProfileDaemonToken("profile-a")
	if err != nil {
		t.Fatalf("EnsureProfileDaemonToken(a) again: %v", err)
	}
	if created || again != tokenA {
		t.Fatal("re-provisioning an existing profile changed its token")
	}

	// Rotating A must not touch B or the legacy slot.
	rotatedA, err := RotateProfileDaemonToken("profile-a")
	if err != nil {
		t.Fatalf("RotateProfileDaemonToken(a): %v", err)
	}
	if rotatedA == tokenA {
		t.Fatal("rotation did not change profile-a's token")
	}
	stillB, err := ProfileDaemonToken("profile-b")
	if err != nil {
		t.Fatalf("ProfileDaemonToken(b) after rotating a: %v", err)
	}
	if stillB != tokenB {
		t.Fatal("rotating profile-a changed profile-b's token")
	}
	legacyAfter, err := DaemonToken()
	if err != nil {
		t.Fatalf("DaemonToken after profile rotation: %v", err)
	}
	if legacyAfter != legacyBefore {
		t.Fatal("rotating a profile changed the legacy shared token")
	}
}

// TestProfileDaemonTokenAbsentIsErrNotFound proves the fail-closed contract:
// an unprovisioned profile reports ErrNotFound exactly like the legacy slot
// does when absent, so a caller (serve --http) can refuse identically
// rather than needing a second code path or a fallback.
func TestProfileDaemonTokenAbsentIsErrNotFound(t *testing.T) {
	if _, err := ProfileDaemonToken("never-provisioned"); err == nil {
		t.Fatal("expected an error for an unprovisioned profile")
	} else if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
