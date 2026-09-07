package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func connectCommand(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	c := newRootCmd()
	c.SetArgs(args)
	c.SetIn(strings.NewReader(input))
	var out, stderr bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&stderr)
	err := c.Execute()
	code := 0
	if err != nil {
		code = 1
		var e *ExitError
		if errors.As(err, &e) {
			code = e.Code
		}
	}
	return out.String(), stderr.String(), code
}

// These independent regressions were run against the rejected implementation:
// blank/unverified/malformed hook inputs passed or exited 1, print was not JSON,
// and a malformed settings companion was detected only after writing MCP config.
func TestConnectClaudeContractRegressions(t *testing.T) {
	root := initBrainRepo(t)
	for name, payload := range map[string]string{
		"blank":         `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{"plan":" "}}`,
		"unverified":    `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{"plan":"Spend $800"}}`,
		"malformed":     `{`,
		"actions_fence": "{\"hook_event_name\":\"PreToolUse\",\"tool_name\":\"ExitPlanMode\",\"tool_input\":{\"plan\":\"Spend $800\\n```serenity:actions\\n[]\\n```\"}}",
	} {
		t.Run(name, func(t *testing.T) {
			out, errout, code := connectCommand(t, payload, "-C", root, "check", "--claude-hook")
			if code != 2 || out != "" || strings.TrimSpace(errout) == "" {
				t.Fatalf("need blocking exit2, empty stdout and explanation: code=%d stdout=%q stderr=%q", code, out, errout)
			}
		})
	}
	t.Run("project_selection", func(t *testing.T) {
		project := filepath.Join(t.TempDir(), "project")
		_, _, code := connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", project)
		if code != 0 {
			t.Fatal("config-dir installer unavailable")
		}
		if _, err := os.Stat(filepath.Join(project, ".claude", "settings.local.json")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("validate_companion_before_write", func(t *testing.T) {
		r := initBrainRepo(t)
		if err := os.MkdirAll(filepath.Join(r, ".claude"), 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"settings.json", "settings.local.json"} {
			if err := os.WriteFile(filepath.Join(r, ".claude", name), []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		c := newRootCmd()
		c.SetArgs([]string{"-C", r, "connect", "claude", "--config-dir", r})
		var b bytes.Buffer
		c.SetOut(&b)
		if err := c.Execute(); err == nil {
			t.Fatal("invalid companion accepted")
		}
		if _, err := os.Stat(filepath.Join(r, ".mcp.json")); !os.IsNotExist(err) {
			t.Fatalf("MCP written before validation: %v", err)
		}
	})
}

// Non-tool events have no tool_name in the real Claude hook schema.
func TestConnectClaudeUnrelatedLifecycleEvent(t *testing.T) {
	for _, event := range []string{"SessionStart", "Stop"} {
		out, stderr, code := connectCommand(t, `{"hook_event_name":"`+event+`","session_id":"test"}`, "check", "--claude-hook")
		if code != 0 || out != "" || stderr != "" {
			t.Fatalf("%s should no-op: %d %q %q", event, code, out, stderr)
		}
	}
}
