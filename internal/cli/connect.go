package cli

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/secrets"
)

// newConnectCmd is `serenity connect` (plan T4.3, T4.8): --rotate-token
// and bare status-only `connect` (T4.3), plus the `claude` subcommand
// (T4.8) that actually wires an external client in.
func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect an external client (e.g. Claude Code) to the Serenity daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rotate, err := cmd.Flags().GetBool("rotate-token")
			if err != nil {
				return err
			}
			provision, err := cmd.Flags().GetBool("provision-token")
			if err != nil {
				return err
			}
			profile, hasProfile, err := resolveCredentialProfile(cmd)
			if err != nil {
				return err
			}
			if provision && !hasProfile {
				return fmt.Errorf("--provision-token requires --%s NAME", credentialProfileFlagName)
			}
			if hasProfile {
				switch {
				case provision:
					return runConnectProvisionProfile(profile, cmd.OutOrStdout())
				case rotate:
					return runConnectRotateProfile(profile, cmd.OutOrStdout())
				default:
					return runConnectProfileStatus(profile, cmd.OutOrStdout())
				}
			}
			if rotate {
				return runConnectRotateToken(cmd.OutOrStdout())
			}
			return runConnectStatus(cmd.OutOrStdout())
		},
	}
	cmd.Flags().Bool("rotate-token", false, "mint a new daemon bearer token, invalidating the old one (the legacy shared token, or the named --credential-profile's own token)")
	cmd.Flags().Bool("provision-token", false, "create a fresh, independent bearer token for --credential-profile NAME (requires --credential-profile; never copies the legacy token's value)")
	addCredentialProfileFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("rotate-token", "provision-token")
	cmd.AddCommand(newConnectClaudeCmd())
	return cmd
}

// runConnectProvisionProfile is `serenity connect --credential-profile NAME
// --provision-token` (RFC-BRAIN-AUTH-02): the only way a profile's token is
// ever created. Idempotent -- an already-provisioned profile is left
// unchanged and reported as such, never silently re-minted or replaced
// (that is --rotate-token's job). The printed note is the CLI-help-visible
// disclosure the contract requires: a profile name is an operator-selected
// label, not a stored, persistent per-brain binding.
func runConnectProvisionProfile(name string, out io.Writer) error {
	_, created, err := secrets.EnsureProfileDaemonToken(name)
	if err != nil {
		return fmt.Errorf("provision credential profile %s: %w", name, err)
	}
	if created {
		_, _ = fmt.Fprintf(out, "credential profile %q provisioned with a fresh, independent daemon auth token (service %q)\n", name, secrets.Service)
	} else {
		_, _ = fmt.Fprintf(out, "credential profile %q already provisioned; token unchanged\n", name)
	}
	_, _ = fmt.Fprintln(out, "this is an operator-selected label, not a stored per-brain binding -- pass the exact same --credential-profile on every `serve --http` invocation for this brain, and never reuse it for a different one")
	return nil
}

// runConnectRotateProfile is `serenity connect --credential-profile NAME
// --rotate-token`: scoped rotation. Only NAME's own keychain entry changes;
// every other profile and the legacy shared token are provably untouched,
// since each is a distinct keychain account (internal/secrets.profileAccountKey).
func runConnectRotateProfile(name string, out io.Writer) error {
	if _, err := secrets.RotateProfileDaemonToken(name); err != nil {
		return fmt.Errorf("rotate credential profile %s: %w", name, err)
	}
	_, _ = fmt.Fprintf(out, "credential profile %q rotated (service %q); its previous token no longer authenticates -- every other profile and the legacy shared token are unaffected\n", name, secrets.Service)
	return nil
}

// runConnectProfileStatus is bare `serenity connect --credential-profile
// NAME` (no --provision-token/--rotate-token): read-only, exactly like bare
// `connect` for the legacy token -- it never mints a token as a side effect
// of checking one.
func runConnectProfileStatus(name string, out io.Writer) error {
	if _, err := secrets.ProfileDaemonToken(name); err != nil {
		_, _ = fmt.Fprintf(out, "credential profile %q has no token yet -- run `serenity connect --credential-profile %s --provision-token`\n", name, name)
	} else {
		_, _ = fmt.Fprintf(out, "credential profile %q has a token in the OS keychain (service %q)\n", name, secrets.Service)
	}
	_, _ = fmt.Fprintln(out, "note: this is a status check on an operator-selected label, not proof of a stored per-brain binding -- the same name used against a different brain would resolve to this identical token")
	return nil
}

// runConnectRotateToken is `serenity connect --rotate-token` (RFC §14,
// ADR 010: "a leaked token is revoked by one command"). The old token
// stops authenticating on the next request; nothing else needs to
// restart, since the HTTP transport reads the token from the keychain
// per request rather than caching it.
func runConnectRotateToken(out io.Writer) error {
	if _, err := secrets.RotateDaemonToken(); err != nil {
		return fmt.Errorf("rotate daemon auth token: %w", err)
	}
	_, _ = fmt.Fprintf(out, "daemon auth token rotated (service %q); the previous token no longer authenticates\n", secrets.Service)
	return nil
}

// runConnectStatus is bare `serenity connect` with no flags.
func runConnectStatus(out io.Writer) error {
	if _, err := secrets.DaemonToken(); err != nil {
		_, _ = fmt.Fprintln(out, "no daemon auth token yet -- run `serenity init` first")
	} else {
		_, _ = fmt.Fprintf(out, "daemon auth token present in the OS keychain (service %q)\n", secrets.Service)
	}
	_, _ = fmt.Fprintln(out, "client config: run `serenity connect claude` to install the MCP server + pre-plan check hook for Claude Code")
	_, _ = fmt.Fprintln(out, "available now: `serenity connect --rotate-token`, `serenity connect claude [--print]`")
	return nil
}

// Claude integration is project-local; the brain may live elsewhere.
const claudeSettingsFile = "settings.local.json"

func newConnectClaudeCmd() *cobra.Command {
	var printOnly bool
	var project string
	cmd := &cobra.Command{Use: "claude", Short: "Install project-local Claude Code MCP and pre-plan hook", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConnectClaude(flagRoot, project, printOnly, cmd.OutOrStdout())
		}}
	cmd.Flags().StringVar(&project, "config-dir", ".", "Claude project directory (defaults to current working directory)")
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the MCP stanza as JSON without writing files")
	return cmd
}

func claudeMCPServerEntry(root string) map[string]any {
	return map[string]any{"command": "serenity", "args": []any{"-C", root, "serve", "--stdio"}}
}
func shellSingleQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
func claudeHookEntry(root string) map[string]any {
	return map[string]any{"matcher": "^ExitPlanMode$", "hooks": []any{map[string]any{"type": "command", "command": "serenity -C " + shellSingleQuote(root) + " check --claude-hook"}}}
}

func runConnectClaude(root, project string, printOnly bool, out io.Writer) error {
	brain, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve brain root: %w", err)
	}
	stanza := map[string]any{"mcpServers": map[string]any{"serenity": claudeMCPServerEntry(brain)}}
	if printOnly {
		return json.NewEncoder(out).Encode(stanza)
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return fmt.Errorf("resolve project: %w", err)
	}
	// macOS exposes its system temporary directory through /var -> /private/var.
	// Resolve only this trusted platform prefix; user-selected descendant links
	// remain subject to the same rejection as other project ancestors.
	tempBase := filepath.Clean(os.TempDir())
	if project == tempBase || strings.HasPrefix(project, tempBase+string(filepath.Separator)) {
		resolved, e := filepath.EvalSymlinks(tempBase)
		if e != nil {
			return fmt.Errorf("resolve system temporary directory: %w", e)
		}
		project = resolved + strings.TrimPrefix(project, tempBase)
	}
	for parent := filepath.Dir(project); ; parent = filepath.Dir(parent) {
		info, e := os.Lstat(parent)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return fmt.Errorf("inspect project ancestor: %w", e)
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink project ancestor %s", parent)
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	// Check destinations before reading either document. Root-relative writes below
	// additionally prevent a directory symlink swap from escaping the project.
	for _, p := range []string{project, filepath.Join(project, ".claude"), filepath.Join(project, ".mcp.json"), filepath.Join(project, ".claude", claudeSettingsFile)} {
		fi, e := os.Lstat(p)
		if errors.Is(e, fs.ErrNotExist) {
			continue
		}
		if e != nil {
			return fmt.Errorf("inspect config path: %w", e)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink config path %s", p)
		}
	}
	mcpPath := filepath.Join(project, ".mcp.json")
	hookPath := filepath.Join(project, ".claude", claudeSettingsFile)
	mcp, err := readJSONObject(mcpPath)
	if err != nil {
		return err
	}
	settings, err := readJSONObject(hookPath)
	if err != nil {
		return err
	}
	servers, err := objectField(mcp, "mcpServers")
	if err != nil {
		return err
	}
	hooks, err := objectField(settings, "hooks")
	if err != nil {
		return err
	}
	var entries []any
	if v, exists := hooks["PreToolUse"]; exists {
		var ok bool
		entries, ok = v.([]any)
		if !ok {
			return fmt.Errorf("hooks.PreToolUse must be an array")
		}
	}
	wantServer := claudeMCPServerEntry(brain)
	mcpChanged := true
	if existing, ok := servers["serenity"]; ok {
		if !reflect.DeepEqual(existing, wantServer) {
			return fmt.Errorf("conflicting mcpServers.serenity; resolve it before installing")
		}
		mcpChanged = false
	}
	wantHook := claudeHookEntry(brain)
	hookChanged := true
	for _, v := range entries {
		entry, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("PreToolUse entry must be an object")
		}
		if matcher, exists := entry["matcher"]; exists {
			if _, ok := matcher.(string); !ok {
				return fmt.Errorf("hook matcher must be a string")
			}
		}
		list, ok := entry["hooks"].([]any)
		if !ok {
			return fmt.Errorf("hook entry hooks must be an array")
		}
		for _, h := range list {
			item, ok := h.(map[string]any)
			if !ok {
				return fmt.Errorf("hook handler must be an object")
			}
			kind, ok := item["type"].(string)
			if !ok || kind == "" {
				return fmt.Errorf("hook handler type must be a string")
			}
			if kind == "command" {
				if command, ok := item["command"].(string); !ok || command == "" {
					return fmt.Errorf("command hook requires a command string")
				}
			}
			command, _ := item["command"].(string)
			if strings.Contains(command, "--claude-hook") || strings.Contains(command, "hook claude-plan") {
				if !reflect.DeepEqual(entry, wantHook) || !hookChanged {
					return fmt.Errorf("conflicting Serenity pre-plan hook; resolve it before installing")
				}
				hookChanged = false
			}
		}
	}
	if !mcpChanged && !hookChanged {
		_, err = fmt.Fprintln(out, "Serenity Claude integration already installed (no changes)")
		return err
	}
	if mcpChanged {
		servers["serenity"] = wantServer
		mcp["mcpServers"] = servers
	}
	if hookChanged {
		hooks["PreToolUse"] = append(entries, wantHook)
		settings["hooks"] = hooks
	}
	// Both documents are now validated. Every individual replacement is atomic.
	if err := os.MkdirAll(project, 0755); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	dir, err := os.OpenRoot(project)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if mcpChanged {
		if err := writeJSONObject(dir, ".mcp.json", mcp); err != nil {
			return err
		}
	}
	if hookChanged {
		if err := dir.MkdirAll(".claude", 0700); err != nil {
			return fmt.Errorf("create Claude settings directory: %w", err)
		}
		if err := writeJSONObject(dir, filepath.Join(".claude", claudeSettingsFile), settings); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, "Installed .mcp.json and .claude/settings.local.json; free-text checks currently block as unverified. See docs/operator/claude.md.")
	return err
}

func objectField(doc map[string]any, key string) (map[string]any, error) {
	v, ok := doc[key]
	if !ok {
		return map[string]any{}, nil
	}
	obj, ok := v.(map[string]any)
	if !ok || obj == nil {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return obj, nil
}
func readJSONObject(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("config %s must be an object", path)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("config %s has trailing data", path)
	}
	return doc, nil
}
func writeJSONObject(dir *os.Root, path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	mode := fs.FileMode(0600)
	if fi, err := dir.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("config %s is not a regular file", path)
		}
		mode = fi.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect config: %w", err)
	}
	tmp := filepath.Join(filepath.Dir(path), ".serenity-"+rand.Text()+".tmp")
	f, err := dir.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create config temporary file: %w", err)
	}
	defer func() { _ = dir.Remove(tmp) }()
	modeErr := f.Chmod(mode)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(modeErr, writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := dir.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
