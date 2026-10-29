package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConnectHostedPlans(t *testing.T) {
	for _, host := range []string{"codex", "claude-code"} {
		for _, write := range []bool{false, true} {
			t.Run(host+map[bool]string{false: "/read", true: "/write"}[write], func(t *testing.T) {
				args := []string{"connect", "hosted", "--host", host}
				if write {
					args = append(args, "--write-scope")
				}
				out, stderr, code := connectCommand(t, "", args...)
				if code != 0 {
					t.Fatalf("code=%d stderr=%s", code, stderr)
				}
				var plan hostedConnectionPlan
				if err := json.Unmarshal([]byte(out), &plan); err != nil {
					t.Fatal(err)
				}
				if plan.Host != host || plan.Endpoint != "https://serenity.sire.run/mcp" || plan.Schema != "serenity.hosted-connection-plan/v1" {
					t.Fatalf("wrong connection target: %+v", plan)
				}
				if plan.Binding != "unverified" || plan.Recall != "unverified" || plan.Hooks != "unqualified" || plan.AutomaticCapture != "disabled" {
					t.Fatalf("plan must not imply qualification: %+v", plan)
				}
				scopes := []string{"memory:read"}
				if write {
					scopes = append(scopes, "memory:write")
				}
				if !reflect.DeepEqual(plan.RequestedScopes, scopes) {
					t.Fatalf("wrong scopes: %v", plan.RequestedScopes)
				}
				if len(plan.Steps) != 4 || plan.Steps[0].Purpose != "preflight" || plan.Steps[3].Purpose != "acceptance" {
					t.Fatalf("missing guarded progression: %+v", plan.Steps)
				}
				if host == "codex" {
					wantScopes := "memory:read"
					if write {
						wantScopes += ",memory:write"
					}
					if !reflect.DeepEqual(plan.Steps[1].Argv, []string{"codex", "mcp", "add", "serenity", "--url", hostedMCPEndpoint}) || !reflect.DeepEqual(plan.Steps[2].Argv, []string{"codex", "mcp", "login", "serenity", "--scopes", wantScopes}) {
						t.Fatalf("unsupported native commands: %+v", plan.Steps)
					}
				} else if !reflect.DeepEqual(plan.Steps[1].Argv, []string{"claude", "mcp", "add", "--transport", "http", "--scope", "local", "serenity", hostedMCPEndpoint}) || len(plan.Steps[2].Argv) != 0 || plan.Steps[2].Action == "" {
					t.Fatalf("Claude authentication must use its native UI: %+v", plan.Steps)
				}
			})
		}
	}
}

func TestConnectHostedRejectsInvalidInvocation(t *testing.T) {
	for _, args := range [][]string{
		{"connect", "hosted"},
		{"connect", "hosted", "--host", "cursor"},
		{"connect", "hosted", "--host", "codex", "unexpected"},
		{"connect", "hosted", "--host", "codex", "--url", "https://example.com/mcp"},
	} {
		out, _, code := connectCommand(t, "", args...)
		if code == 0 || out != "" {
			t.Fatalf("invalid invocation %v: code=%d stdout=%q", args, code, out)
		}
	}
}

func TestConnectHostedDoesNotTouchBrainOrHostConfiguration(t *testing.T) {
	command, _, err := newRootCmd().Find([]string{"connect", "hosted"})
	if err != nil || commandOwnsWriter(command) {
		t.Fatalf("connection planning must not acquire a writer: %v", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, ".codex")
	if err := os.Mkdir(config, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "config.toml")
	original := []byte("malformed config must remain untouched\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	brain := filepath.Join(home, "absent-brain")
	for _, host := range []string{"codex", "claude-code"} {
		_, stderr, code := connectCommand(t, "", "-C", brain, "connect", "hosted", "--host", host)
		if code != 0 {
			t.Fatalf("plan unexpectedly requires a brain: %s", stderr)
		}
	}
	if _, err := os.Stat(brain); !os.IsNotExist(err) {
		t.Fatalf("plan touched brain: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("plan changed host config: %q, %v", got, err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".codex" {
		t.Fatalf("unexpected home changes: %v %v", entries, err)
	}
}
