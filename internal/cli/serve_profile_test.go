package cli

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/secrets"
)

// TestServeHTTPStdioProfileConflict proves --credential-profile with
// --stdio is rejected rather than silently ignored: stdio has no
// bearer-token authentication to select a credential for.
func TestServeHTTPStdioProfileConflict(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", t.TempDir(), "serve", "--stdio", "--credential-profile", "stdio-conflict"})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--stdio") {
		t.Fatalf("expected a --stdio/--credential-profile conflict error, got err=%v out=%q", err, out.String())
	}
}

// TestServeHTTPMalformedProfileRefused proves a malformed name is refused
// before the daemon ever starts listening.
func TestServeHTTPMalformedProfileRefused(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", t.TempDir(), "serve", "--http", "--credential-profile", "Not Valid"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("malformed profile name must be refused")
	}
	if strings.Contains(out.String(), "http://") {
		t.Fatal("daemon must not have started listening")
	}
}

// TestServeHTTPUnprovisionedProfileFailsClosed proves a valid but never
// provisioned profile is a hard refusal, never a silent fall back to the
// legacy shared token.
func TestServeHTTPUnprovisionedProfileFailsClosed(t *testing.T) {
	legacyToken, _, err := secrets.EnsureDaemonToken()
	if err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", t.TempDir(), "serve", "--http", "--credential-profile", "never-provisioned-serve"})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("unprovisioned profile must be refused, not silently served")
	}
	if strings.Contains(out.String(), "http://") {
		t.Fatal("daemon must not have started listening for an unprovisioned profile")
	}
	if strings.Contains(out.String()+stderr.String(), legacyToken) {
		t.Fatal("refusal output leaked the legacy token")
	}
}

// runningHTTPServeArgs is runningHTTPServe (serve_http_test.go) generalized
// to accept arbitrary extra serve flags, so profile-scoped variants can
// reuse the exact same real-listener/poll-for-endpoint pattern.
func runningHTTPServeArgs(t *testing.T, root string, extra ...string) (addr string, stdout *syncBuffer, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cmd := newRootCmd()
	args := append([]string{"-C", root, "serve", "--http"}, extra...)
	cmd.SetArgs(args)
	out := &syncBuffer{}
	cmd.SetOut(out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	ctx, cancelFn := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- cmd.ExecuteContext(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	var line string
	for time.Now().Before(deadline) {
		line = out.String()
		if strings.Contains(line, "http://") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(line, "http://") {
		cancelFn()
		t.Fatalf("serve --http never printed its bound endpoint; stdout=%q stderr=%q", line, stderr.String())
	}
	idx := strings.Index(line, "http://")
	endpoint := strings.TrimSpace(line[idx:])
	endpoint = strings.SplitN(endpoint, "\n", 2)[0]
	// The printed line is ".../mcp"; healthz needs the bare origin.
	endpoint = strings.TrimSuffix(endpoint, "/mcp")
	return endpoint, out, cancelFn, errCh
}

func healthz(t *testing.T, addr, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, addr+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestServeHTTPProfilesCrossRejectAndScopeRotation is the in-process
// (mocked keyring), fast regression counterpart to the separate real
// stock-CLI-process proof: two --credential-profile daemons, each on its
// own throwaway loopback port, must reject each other's token and remain
// independent under rotation. The real, separate-OS-process, real-OS-
// keychain version of this exact property is proven by
// outbox/brain-auth/evidence/stock-cli-process-proof.sh, not by this test.
func TestServeHTTPProfilesCrossRejectAndScopeRotation(t *testing.T) {
	if _, _, err := secrets.EnsureProfileDaemonToken("cross-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secrets.EnsureProfileDaemonToken("cross-b"); err != nil {
		t.Fatal(err)
	}
	tokenA, err := secrets.ProfileDaemonToken("cross-a")
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := secrets.ProfileDaemonToken("cross-b")
	if err != nil {
		t.Fatal(err)
	}

	addrA, _, cancelA, doneA := runningHTTPServeArgs(t, t.TempDir(), "--credential-profile", "cross-a")
	defer func() {
		cancelA()
		<-doneA
	}()
	addrB, _, cancelB, doneB := runningHTTPServeArgs(t, t.TempDir(), "--credential-profile", "cross-b")
	defer func() {
		cancelB()
		<-doneB
	}()

	if code := healthz(t, addrA, tokenA); code != http.StatusOK {
		t.Fatalf("A's own token against A: got %d, want 200", code)
	}
	if code := healthz(t, addrB, tokenB); code != http.StatusOK {
		t.Fatalf("B's own token against B: got %d, want 200", code)
	}
	if code := healthz(t, addrA, tokenB); code != http.StatusUnauthorized {
		t.Fatalf("B's token against A: got %d, want 401", code)
	}
	if code := healthz(t, addrB, tokenA); code != http.StatusUnauthorized {
		t.Fatalf("A's token against B: got %d, want 401", code)
	}

	newTokenA, err := secrets.RotateProfileDaemonToken("cross-a")
	if err != nil {
		t.Fatal(err)
	}
	// TokenSource is read per request (no restart) -- rotation must take
	// effect on the very next request against the already-running daemon.
	if code := healthz(t, addrA, tokenA); code != http.StatusUnauthorized {
		t.Fatalf("A's old token after rotation: got %d, want 401", code)
	}
	if code := healthz(t, addrA, newTokenA); code != http.StatusOK {
		t.Fatalf("A's new token after rotation: got %d, want 200", code)
	}
	if code := healthz(t, addrB, tokenB); code != http.StatusOK {
		t.Fatalf("B unaffected by A's rotation: got %d, want 200", code)
	}
}
