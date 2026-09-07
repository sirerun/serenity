package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
// (no flags) reports the daemon token's presence and points at the real
// `connect claude` subcommand -- T4.8 shipped it, so the T4.3-era "not
// implemented yet" wording would now be a false claim rather than an
// honest one.
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
	if !strings.Contains(out.String(), "serenity connect claude") {
		t.Fatalf("expected a pointer to `serenity connect claude`, got: %q", out.String())
	}
}

// Independent replacement assertions follow repair-red.log's six reproduced
// failures. Exact test names are retained, but acceptance comes from real
// commands, generated paths and observed behavior rather than names alone.
func readFileT(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func writeConfigT(t *testing.T, p, s string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
}
func readConfigT(t *testing.T, p string) map[string]any {
	t.Helper()
	var v map[string]any
	if e := json.Unmarshal([]byte(readFileT(t, p)), &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func installClaudeT(t *testing.T, root, project string) {
	t.Helper()
	out, errout, code := connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", project)
	if code != 0 {
		t.Fatalf("install exit%d: %s %s", code, out, errout)
	}
}
func hookPayloadT(t *testing.T, tool, plan string) string {
	t.Helper()
	b, e := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": map[string]any{"plan": plan}, "cwd": "/wrong-brain"})
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func installedHookT(t *testing.T, project string) string {
	t.Helper()
	d := readConfigT(t, filepath.Join(project, ".claude", "settings.local.json"))
	entry := d["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if entry["matcher"] != "^ExitPlanMode$" {
		t.Fatal(entry)
	}
	return entry["hooks"].([]any)[0].(map[string]any)["command"].(string)
}
func runInstalledHookT(t *testing.T, bin, command, input string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	c.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	c.Stdin = strings.NewReader(input)
	var out, stderr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &stderr
	e := c.Run()
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	code := 0
	if e != nil {
		var x *exec.ExitError
		if !errors.As(e, &x) {
			t.Fatal(e)
		}
		code = x.ExitCode()
	}
	return out.String(), stderr.String(), code
}

func TestConnectClaudeInstall(t *testing.T) {
	root := initBrainRepo(t)
	project := t.TempDir()
	installClaudeT(t, root, project)
	if _, e := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(e) {
		t.Fatal("wrote brain instead of project")
	}
	doc := readConfigT(t, filepath.Join(project, ".mcp.json"))
	entry := doc["mcpServers"].(map[string]any)["serenity"].(map[string]any)
	if entry["command"] != "serenity" || !reflect.DeepEqual(entry["args"], []any{"-C", root, "serve", "--stdio"}) {
		t.Fatal(entry)
	}
	_ = installedHookT(t, project)
	bin := buildSerenityBinary(t)
	args := []string{}
	for _, v := range entry["args"].([]any) {
		args = append(args, v.(string))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	c.Dir = project
	c.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"connect-test","version":"1"}}}` + "\n")
	b, e := c.Output()
	if e != nil {
		t.Fatalf("configured MCP handshake: %v %s", e, b)
	}
	var response map[string]any
	if e = json.Unmarshal(b, &response); e != nil {
		t.Fatal(e)
	}
	if response["jsonrpc"] != "2.0" || response["id"] != float64(1) || response["result"] == nil || response["error"] != nil {
		t.Fatal(response)
	}
}
func TestConnectClaudeIdempotent(t *testing.T) {
	root := initBrainRepo(t)
	p := t.TempDir()
	installClaudeT(t, root, p)
	for _, rel := range []string{".mcp.json", ".claude/settings.local.json"} {
		path := filepath.Join(p, rel)
		before := readFileT(t, path)
		old := time.Unix(100, 0)
		if e := os.Chtimes(path, old, old); e != nil {
			t.Fatal(e)
		}
		installClaudeT(t, root, p)
		fi, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		if readFileT(t, path) != before || !fi.ModTime().Equal(old) {
			t.Fatal("rerun changed bytes/mtime", rel)
		}
	}
}
func TestConnectClaudePrint(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(t.TempDir(), "absent")
	out, _, code := connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", p, "--print")
	if code != 0 {
		t.Fatal(code)
	}
	var doc any
	if e := json.Unmarshal([]byte(out), &doc); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("print created directory")
	}
	installClaudeT(t, root, p)
	if !reflect.DeepEqual(doc, readConfigT(t, filepath.Join(p, ".mcp.json"))) {
		t.Fatal("printed stanza differs")
	}
	before := readFileT(t, filepath.Join(p, ".claude/settings.local.json"))
	connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", p, "--print")
	if before != readFileT(t, filepath.Join(p, ".claude/settings.local.json")) {
		t.Fatal("print changed existing file")
	}
}
func TestConnectClaudeNoToken(t *testing.T) {
	secrets.MockForTesting()
	p := t.TempDir()
	root := t.TempDir()
	installClaudeT(t, root, p)
	out, stderr, code := connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", p, "--print")
	if code != 0 {
		t.Fatal("stdio setup needs no token")
	}
	for _, s := range []string{out, stderr, readFileT(t, filepath.Join(p, ".mcp.json")), readFileT(t, filepath.Join(p, ".claude/settings.local.json"))} {
		for _, bad := range []string{"Authorization", "Bearer", "\"env\""} {
			if strings.Contains(s, bad) {
				t.Fatal("credential material", s)
			}
		}
	}
	token, _, e := secrets.EnsureDaemonToken()
	if e != nil {
		t.Fatal(e)
	}
	installClaudeT(t, root, p)
	grepTreeFor(t, p, token)
	out, stderr, _ = connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", p, "--print")
	if strings.Contains(out+stderr, token) {
		t.Fatal("token printed")
	}
}
func TestConnectClaudePreserve(t *testing.T) {
	root := t.TempDir()
	p := t.TempDir()
	writeConfigT(t, filepath.Join(p, ".mcp.json"), `{"other":9007199254740993,"mcpServers":{"other":{"command":"other"}}}`)
	writeConfigT(t, filepath.Join(p, ".claude/settings.local.json"), `{"permissions":{"deny":["Bash(rm *)"]},"hooks":{"Stop":[],"PreToolUse":[{"matcher":"^ExitPlanMode$","hooks":[{"type":"command","command":"other-check"}]}]}}`)
	installClaudeT(t, root, p)
	s := readFileT(t, filepath.Join(p, ".mcp.json"))
	if !strings.Contains(s, "9007199254740993") {
		t.Fatal("number lost precision")
	}
	d := readConfigT(t, filepath.Join(p, ".claude/settings.local.json"))
	if d["permissions"] == nil || len(d["hooks"].(map[string]any)["PreToolUse"].([]any)) != 2 {
		t.Fatal(d)
	}
	before := readFileT(t, filepath.Join(p, ".claude/settings.local.json"))
	installClaudeT(t, root, p)
	if before != readFileT(t, filepath.Join(p, ".claude/settings.local.json")) {
		t.Fatal("duplicate hook")
	}
}
func TestConnectClaudeConflict(t *testing.T) {
	for _, which := range []string{"mcp", "hook"} {
		t.Run(which, func(t *testing.T) {
			p := t.TempDir()
			root := t.TempDir()
			m := filepath.Join(p, ".mcp.json")
			h := filepath.Join(p, ".claude/settings.local.json")
			writeConfigT(t, m, `{"mcpServers":{"other":{"command":"other"}}}`)
			writeConfigT(t, h, `{}`)
			if which == "mcp" {
				writeConfigT(t, m, `{"mcpServers":{"serenity":{"command":"different"}}}`)
			} else {
				writeConfigT(t, h, `{"hooks":{"PreToolUse":[{"matcher":"^ExitPlanMode$","hooks":[{"type":"command","command":"serenity -C /other check --claude-hook"}]}]}}`)
			}
			mb, hb := readFileT(t, m), readFileT(t, h)
			_, _, code := connectCommand(t, "", "-C", root, "connect", "claude", "--config-dir", p)
			if code == 0 || readFileT(t, m) != mb || readFileT(t, h) != hb {
				t.Fatal("conflict changed files")
			}
		})
	}
}
func TestConnectClaudeMalformedConfig(t *testing.T) {
	for _, tc := range []struct{ name, path, data string }{
		{"syntax", ".mcp.json", "{"}, {"trailing", ".mcp.json", "{} {}"}, {"array", ".mcp.json", "[]"}, {"null", ".mcp.json", "null"}, {"servers", ".mcp.json", `{"mcpServers":[]}`},
		{"hook_syntax", ".claude/settings.local.json", "{"}, {"hooks_type", ".claude/settings.local.json", `{"hooks":[]}`}, {"event_type", ".claude/settings.local.json", `{"hooks":{"PreToolUse":{}}}`}, {"entry_type", ".claude/settings.local.json", `{"hooks":{"PreToolUse":[null]}}`}, {"handler_type", ".claude/settings.local.json", `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":42}]}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := t.TempDir()
			path := filepath.Join(p, tc.path)
			writeConfigT(t, path, tc.data)
			_, _, code := connectCommand(t, "", "connect", "claude", "--config-dir", p)
			if code == 0 || readFileT(t, path) != tc.data {
				t.Fatal("invalid config accepted or changed")
			}
			other := ".mcp.json"
			if tc.path == other {
				other = ".claude/settings.local.json"
			}
			if _, e := os.Stat(filepath.Join(p, other)); !os.IsNotExist(e) {
				t.Fatal("created companion")
			}
		})
	}
	for _, rel := range []string{".mcp.json", ".claude/settings.local.json", ".claude"} {
		t.Run("symlink_"+rel, func(t *testing.T) {
			p := t.TempDir()
			outside := t.TempDir()
			target := filepath.Join(outside, "file")
			writeConfigT(t, target, "{}")
			dest := filepath.Join(p, rel)
			if rel == ".claude" {
				target = outside
			}
			if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(target, dest); e != nil {
				t.Fatal(e)
			}
			_, _, code := connectCommand(t, "", "connect", "claude", "--config-dir", p)
			if code == 0 || readFileT(t, filepath.Join(outside, "file")) != "{}" {
				t.Fatal("symlink modified")
			}
			if _, e := os.Stat(filepath.Join(outside, "settings.local.json")); !os.IsNotExist(e) {
				t.Fatal("escaped project")
			}
		})
	}
}
func TestConnectClaudeGateInput(t *testing.T) {
	root := initBrainRepo(t)
	p := t.TempDir()
	installClaudeT(t, root, p)
	bin := buildSerenityBinary(t)
	command := installedHookT(t, p)
	out, stderr, code := runInstalledHookT(t, bin, command, hookPayloadT(t, "ExitPlanMode", "Spend $800"))
	if code != 2 || out != "" || !strings.Contains(stderr, "unverified") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
	out, stderr, code = runInstalledHookT(t, bin, command, hookPayloadT(t, "Bash", "ignored"))
	if code != 0 || out != "" || stderr != "" {
		t.Fatalf("unrelated event: %d %q %q", code, out, stderr)
	}
}
func TestConnectClaudeGateMalformed(t *testing.T) {
	root := initBrainRepo(t)
	for _, input := range []string{"", "{", "null", "{} {}", "{}", `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{}}`, `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{"plan":null}}`, `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{"plan":42}}`, hookPayloadT(t, "ExitPlanMode", " "), hookPayloadT(t, "ExitPlanMode", strings.Repeat("x", 1<<20))} {
		out, stderr, code := connectCommand(t, input, "-C", root, "check", "--claude-hook")
		if code != 2 || out != "" || stderr == "" {
			t.Fatalf("input %.60q: %d %q %q", input, code, out, stderr)
		}
	}
	out, stderr, code := connectCommand(t, hookPayloadT(t, "ExitPlanMode", "test"), "-C", t.TempDir(), "check", "--claude-hook")
	if code != 2 || out != "" || stderr == "" {
		t.Fatal("invalid brain not blocked")
	}
}
func TestConnectClaudeGateVerdicts(t *testing.T) {
	for _, status := range []string{"pass", "violated", "unverified", "no_applicable_constraints", "unknown", ""} {
		for _, engineErr := range []error{nil, errors.New("engine failed")} {
			var stderr bytes.Buffer
			e := claudeGateResult(status, engineErr, &stderr)
			if status == "pass" && engineErr == nil {
				if e != nil || stderr.Len() != 0 {
					t.Fatal("pass must be silent")
				}
			} else {
				var x *ExitError
				if !errors.As(e, &x) || x.Code != 2 || stderr.Len() == 0 {
					t.Fatal("must block", status, e)
				}
			}
		}
	}
	root := initBrainRepo(t)
	out, _, code := connectCommand(t, "", "-C", root, "check", "--json", "--actions", `[]`)
	if code != 0 || !strings.Contains(out, "no_applicable_constraints") {
		t.Fatal("ordinary CLI contract changed")
	}
	out, _, code = connectCommand(t, "", "-C", root, "check", "--json", "plan")
	if code != 1 || !strings.Contains(out, "unverified") {
		t.Fatal("ordinary CLI unverified changed")
	}
}
func TestConnectClaudeGateInjection(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	parent := t.TempDir()
	weird := filepath.Join(parent, "brain's space $(touch injected)")
	if e := os.Rename(root, weird); e != nil {
		t.Fatal(e)
	}
	p := t.TempDir()
	installClaudeT(t, weird, p)
	marker := filepath.Join(t.TempDir(), "injected")
	plan := "Spend $800; $(touch " + marker + ") `touch " + marker + "`\n```serenity:actions\n[]\n```"
	out, stderr, code := runInstalledHookT(t, bin, installedHookT(t, p), hookPayloadT(t, "ExitPlanMode", plan))
	if code != 2 || out != "" || !strings.Contains(stderr, "unverified") {
		t.Fatalf("hook did not execute safely: %d %q %q", code, out, stderr)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("plan executed")
	}
}
func TestConnectClaudeDocs(t *testing.T) {
	doc := readFileT(t, "../../docs/operator/claude.md")
	for _, want := range []string{"--config-dir", "settings.local.json", "--stdio", "unverified", "zero tools", "exit 2", "https://code.claude.com/docs/en/hooks"} {
		if !strings.Contains(doc, want) {
			t.Fatal("missing documentation", want)
		}
	}
	out, _, code := connectCommand(t, "", "connect", "claude", "--help")
	if code != 0 || !strings.Contains(out, "--config-dir") || !strings.Contains(out, "--print") {
		t.Fatal("examples differ from help")
	}
}

func TestConnectClaudeDefaultProject(t *testing.T) {
	project, brain := t.TempDir(), t.TempDir()
	t.Chdir(project)
	_, _, code := connectCommand(t, "", "-C", brain, "connect", "claude")
	if code != 0 {
		t.Fatal(code)
	}
	if _, e := os.Stat(filepath.Join(project, ".claude/settings.local.json")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(brain, ".mcp.json")); !os.IsNotExist(e) {
		t.Fatal("default writes brain instead of cwd")
	}
}

func TestConnectClaudeAtomicReplacement(t *testing.T) {
	p := t.TempDir()
	path := filepath.Join(p, ".mcp.json")
	writeConfigT(t, path, `{"other":true}`)
	if e := os.Chmod(path, 0640); e != nil {
		t.Fatal(e)
	}
	original, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = original.Close() }()
	installClaudeT(t, t.TempDir(), p)
	// The already-open original inode retains its old content after replacement.
	// Direct truncate/write would change this descriptor's bytes.
	var b bytes.Buffer
	if _, e = b.ReadFrom(original); e != nil {
		t.Fatal(e)
	}
	if b.String() != `{"other":true}` {
		t.Fatal("configuration was overwritten in place")
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatal("changed permissions")
	}
	for _, dir := range []string{p, filepath.Join(p, ".claude")} {
		entries, e := os.ReadDir(dir)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".serenity-") {
				t.Fatal("temporary file leaked")
			}
		}
	}
}

func TestConnectClaudeSymlinkProjectAncestor(t *testing.T) {
	parent, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(parent, "link")
	if e := os.Symlink(outside, link); e != nil {
		t.Fatal(e)
	}
	_, _, code := connectCommand(t, "", "connect", "claude", "--config-dir", filepath.Join(link, "new-project"))
	if code == 0 {
		t.Fatal("symlink ancestor accepted")
	}
	entries, e := os.ReadDir(outside)
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatal("wrote through ancestor symlink")
	}
}
