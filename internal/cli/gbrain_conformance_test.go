package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/secrets"
)

// TestGbrainProtocolConformance runs gbrain's own `protocol conformance`
// certifier -- github.com/dndungu/gbrain, pinned commit
// d35c9c9e441e6cfc86dd5e84b0b168c6b18ee775 (docs/protocol/MEMORY_VERBS_v1.md)
// -- against a live `serenity serve --http` endpoint over a throwaway
// fixture brain (T4.14, docs/plans/E4-m4-serve-protocols.md). This is the
// literal upstream conformance run T4.21 disclosed as its own remaining gap
// ("The literal gbrain SDK client round trip is T4.14's own job").
//
// `serve --http` runs IN-PROCESS here (the real Cobra command driven via
// runningHTTPServe, the same harness TestServeHTTPEndToEnd already
// exercises) instead of as a separately exec'd `serenity` binary. The
// daemon auth token lives in the OS keychain (RFC 0001 section 14); this
// package's TestMain (cli_test.go) swaps that for an in-memory mock via
// secrets.MockForTesting() for the whole test binary. A real `serenity`
// subprocess would bypass that mock and hit the real OS keychain, which on
// Linux is the dbus Secret Service (zalando/go-keyring) -- unavailable on a
// bare GitHub Actions ubuntu-latest runner and not set up by any other job
// in this repo. Driving the command in-process keeps the exact same
// listener/auth/session code path (internal/server, internal/server/mcp)
// live over a real TCP loopback socket, with gbrain's own TypeScript SDK
// client as the one genuine external process on the other end of the wire.
//
// Needs network access (cloning gbrain, `bun install`) and a Bun-installed
// checkout of the pin: skipped unless SERENITY_GBRAIN_CONFORMANCE=1 and
// GBRAIN_CLI_DIR are both set. Not part of the plain `go test ./...` run
// (no network there) -- the dedicated "gbrain-protocol-conformance" CI job
// (.github/workflows/ci.yml) sets both, the same separate-job convention
// internal/dira/verify-pin.sh and scripts/verify-dira-cli.sh already use
// for their own external-tool/network-requiring checks.
func TestGbrainProtocolConformance(t *testing.T) {
	if os.Getenv("SERENITY_GBRAIN_CONFORMANCE") != "1" {
		t.Skip("set SERENITY_GBRAIN_CONFORMANCE=1 and GBRAIN_CLI_DIR to run this (see the gbrain-protocol-conformance CI job)")
	}
	gbrainDir := os.Getenv("GBRAIN_CLI_DIR")
	if gbrainDir == "" {
		t.Fatal("GBRAIN_CLI_DIR must point at a `bun install`-ed checkout of dndungu/gbrain @ the pinned commit")
	}
	bunBin, err := exec.LookPath("bun")
	if err != nil {
		t.Fatalf("bun not on PATH: %v", err)
	}

	requireGit(t)
	root := pushFixture(t)

	token, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("daemon token: %v", err)
	}

	endpoint, _, cancel, done := runningHTTPServe(t, root)
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve --http exited with error on shutdown: %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Error("serve --http did not shut down")
		}
	}()

	// --synthesize exercises the synthesize case too: with no composer
	// configured on this bare fixture brain, Serenity returns the clean
	// `unavailable` protocol error, which the runner accepts as
	// conformant (the same "what CI does" posture gbrain's own self-cert
	// CI job takes with no LLM key configured).
	cmd := exec.Command(bunBin, "run", "src/cli.ts", "protocol", "conformance",
		"--target", endpoint,
		"--token", token,
		"--synthesize",
		"--json",
	)
	cmd.Dir = gbrainDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	type caseResult struct {
		Name   string `json:"name"`
		Verb   string `json:"verb"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	var report struct {
		ProtocolVersion int          `json:"protocol_version"`
		Results         []caseResult `json:"results"`
		Passed          int          `json:"passed"`
		Failed          int          `json:"failed"`
		Skipped         int          `json:"skipped"`
		OK              bool         `json:"ok"`
	}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &report); jsonErr != nil {
		t.Fatalf("gbrain protocol conformance --json: unparseable output: %v (process err=%v)\nstdout=%s\nstderr=%s", jsonErr, runErr, stdout.String(), stderr.String())
	}

	t.Logf("gbrain protocol conformance: %d passed, %d failed, %d skipped (protocol_version=%d)", report.Passed, report.Failed, report.Skipped, report.ProtocolVersion)
	for _, r := range report.Results {
		suffix := ""
		if r.Detail != "" {
			suffix = " -- " + r.Detail
		}
		t.Logf("  [%s] %s (%s)%s", strings.ToUpper(r.Status), r.Name, r.Verb, suffix)
	}

	// The acc line's three named pass criteria -- SHAPE (required fields,
	// enum validity), CONTRACT BEHAVIOR (provenance rejected, idempotent
	// forget, entity miss -> found:false), ROUND-TRIP (remember -> recall
	// by entity) -- are gbrain's own case categories
	// (conformance-fixtures.ts), interleaved in one report; gbrain has no
	// flag to run them as three separate invocations. Zero failures across
	// the whole run is what "passes the SHAPE, CONTRACT BEHAVIOR, and
	// ROUND-TRIP arms" means operationally.
	if !report.OK || report.Failed != 0 {
		t.Fatalf("gbrain protocol conformance NOT CONFORMANT: %d failed of %d cases (see per-case log above); process err=%v", report.Failed, report.Passed+report.Failed+report.Skipped, runErr)
	}
	if report.Passed == 0 {
		t.Fatal("gbrain protocol conformance reported zero passing cases -- vacuous run")
	}

	// Serenity's MCP surface exposes only the five MEMORY_VERBS core tools
	// (no gbrain-specific put_page), so the runner's entity-page seed
	// attempt fails and every requiresSeededEntity case should skip
	// honestly rather than silently not run at all. Assert the skip
	// actually happened, not just that the overall run is green.
	sawEntitySkip := false
	for _, r := range report.Results {
		if r.Verb == "entity" && r.Status == "skip" {
			sawEntitySkip = true
		}
	}
	if !sawEntitySkip {
		t.Error("expected an honest 'skip' for at least one entity-hit case (Serenity exposes no put_page tool) -- got none; either gbrain's fixture set changed or Serenity unexpectedly advertises put_page")
	}
}
