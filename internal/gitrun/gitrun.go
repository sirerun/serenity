// Package gitrun is the one place the local product spawns git (ADR 018,
// deep review 001 SEC-H05). A repository's own configuration can name
// programs for git to execute -- core.fsmonitor on every index-refreshing
// subcommand (ls-files --others, status, diff), hooks on writes, ext::
// transports on fetch -- so a repository whose state an attacker controls
// is a code-execution vector for any process that runs git inside it.
// Every call site therefore builds its command through one of three
// constructors that differ only in how much of the repository they trust:
//
//   - Brain(dir) is for the brain repository this process owns. It
//     disables fsmonitor and ext:: transports but keeps repository hooks,
//     because `serenity init` installs the post-commit auto-push hook
//     (UC-040) and Flush relies on it firing.
//   - Foreign(dir) is for repositories the process does not own: connector
//     targets, imported brains, restored bundles before validation. It
//     additionally ignores repository hooks and the global and system
//     configuration, takes no optional locks, and refuses every
//     subcommand that is not on a read-only allowlist.
//   - Quarantine(dir) is for private owned staging workspaces. It permits
//     local bundle restoration writes with hooks and global/system config
//     ignored. Bundle cloning uses CloneBundle's fresh workspace.
//
// All runners scrub the inherited environment: every GIT_* variable is dropped
// except GIT_SSH_COMMAND (the daemon's key selection), GIT_TERMINAL_PROMPT
// is pinned to 0 so no subcommand can block on a credential prompt, and
// everything else (PATH, HOME, LANG, TMPDIR, SSH_AUTH_SOCK, ...) passes
// through untouched. Callers may not pass the global options that would
// redirect the runner (-C, --git-dir, --work-tree, --exec-path) or add
// configuration (-c, --config-env); ErrReservedOption names the attempt.
//
// A drift test in this package parses the local-product packages for raw
// exec.Command("git", ...) calls; T24.30 widens it to the whole module once
// the hosted call sites migrate under their own file claims.
package gitrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// ErrForeignWrite is returned by a Foreign runner for any subcommand
	// outside the read-only allowlist.
	ErrForeignWrite = errors.New("gitrun: foreign repository allows read-only subcommands only")
	// ErrReservedOption is returned when a caller passes a global git
	// option the runner owns (-c, -C, --git-dir, ...) or no subcommand.
	ErrReservedOption = errors.New("gitrun: caller passed a git option reserved for the runner")
)

// hardening is the -c prefix every runner adds. core.fsmonitor=false stops
// git from spawning a repository-configured monitor program on index
// refresh; protocol.ext.allow=never stops ext:: remotes from running a
// shell. Both are -c overrides, so they beat every configuration scope.
var hardening = []string{
	"-c", "core.fsmonitor=false",
	"-c", "protocol.ext.allow=never",
}

// foreignHardening is added by Foreign: hooks resolve under a path that
// cannot hold any, so none run.
var foreignHardening = []string{
	"-c", "core.hooksPath=" + os.DevNull,
}

// readOnly lists the subcommands a Foreign runner may run. Each one reads
// the object store, index or worktree without writing refs, objects or the
// worktree; status and diff may refresh the index, which GIT_OPTIONAL_LOCKS=0
// suppresses.
var readOnly = map[string]bool{
	"cat-file":     true,
	"describe":     true,
	"diff":         true,
	"diff-tree":    true,
	"for-each-ref": true,
	"log":          true,
	"ls-files":     true,
	"ls-tree":      true,
	"rev-list":     true,
	"rev-parse":    true,
	"show":         true,
	"show-ref":     true,
	"status":       true,
}

// reservedWithValue are global options that consume the next argument and
// would redirect or reconfigure the runner. reservedPrefixes covers their
// --opt=value spellings.
var reservedWithValue = map[string]bool{
	"-c": true, "-C": true, "--git-dir": true, "--work-tree": true,
	"--exec-path": true, "--config-env": true, "--namespace": true,
	"--super-prefix": true,
}

var reservedPrefixes = []string{
	"--git-dir=", "--work-tree=", "--exec-path=", "--config-env=",
	"--namespace=", "--super-prefix=",
}

// Runner spawns git subcommands in one repository under one trust level.
type Runner struct {
	dir         string
	foreign     bool
	quarantine  bool
	bundleClone bool
}

// Brain returns a runner for the brain repository this process owns: the
// hardening prefix applies, repository hooks and the operator's global
// configuration are honored, and every subcommand is permitted.
func Brain(dir string) *Runner { return &Runner{dir: dir} }

// Foreign returns a runner for a repository this process does not own:
// hooks, global and system configuration are ignored, optional index
// writes are skipped, and only read-only subcommands run.
func Foreign(dir string) *Runner { return &Runner{dir: dir, foreign: true} }

// Quarantine runs Git in an owned private staging workspace. It permits
// writes needed to restore a local bundle, but ignores hooks and global/system
// configuration and blocks built-in network transports. Callers must supply
// trusted command arguments and keep the workspace private until validated.
// Repository configuration is still read; use CloneBundle for cloning instead
// of allowing repository URL rewrites to choose a transport.
// It must not replace Foreign for inspecting someone else's repository.
func Quarantine(dir string) *Runner { return &Runner{dir: dir, quarantine: true} }

// CloneBundle restores a regular local bundle into a new target directory.
// Git runs from a fresh private workspace with ancestor discovery disabled,
// so repository URL rewrites and custom transport policy cannot redirect it.
// The caller owns the target and must validate it before publication.
func CloneBundle(ctx context.Context, bundle, target string) (output []byte, err error) {
	bundle, err = filepath.Abs(bundle)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(bundle)
	if err != nil {
		return nil, fmt.Errorf("inspect local Git bundle: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("gitrun: bundle must be a regular local file")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return nil, errors.New("gitrun: bundle target already exists")
		}
		return nil, err
	}
	workspace, err := os.MkdirTemp("", "serenity-git-bundle-")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(workspace)) }()
	runner := Quarantine(workspace)
	runner.bundleClone = true
	cmd, err := runner.Command(ctx, "clone", "--quiet", "--", bundle, target)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, "GIT_CEILING_DIRECTORIES="+filepath.Dir(workspace))
	return cmd.CombinedOutput()
}

// Dir reports the repository directory the runner spawns git in.
func (r *Runner) Dir() string { return r.dir }

// Command builds the hardened *exec.Cmd for `git args...` without running
// it, so a caller can attach stdin or separate stdout and stderr buffers.
// It returns ErrReservedOption for a caller-supplied global option the
// runner owns and, for Foreign, ErrForeignWrite for any subcommand outside
// the read-only allowlist. The returned command's Args and Env are fully
// formed; callers should not append to them.
func (r *Runner) Command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	sub, err := subcommand(args)
	if err != nil {
		return nil, err
	}
	if r.foreign && !readOnly[sub] {
		return nil, fmt.Errorf("%w: %s", ErrForeignWrite, sub)
	}
	if r.quarantine && sub == "clone" && !r.bundleClone {
		return nil, errors.New("gitrun: quarantine cloning requires CloneBundle")
	}
	full := make([]string, 0, len(hardening)+len(foreignHardening)+len(args))
	full = append(full, hardening...)
	if r.foreign || r.quarantine {
		full = append(full, foreignHardening...)
	}
	if r.quarantine {
		full = append(full, "-c", "protocol.allow=never", "-c", "protocol.file.allow=always")
		// Per-protocol repository policy overrides protocol.allow's default.
		// Pin each built-in network transport as well as the default policy.
		for _, protocol := range []string{"http", "https", "git", "ssh", "ftp", "ftps", "rsync"} {
			full = append(full, "-c", "protocol."+protocol+".allow=never")
		}
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.dir
	cmd.Env = r.env(os.Environ())
	return cmd, nil
}

// Output runs the subcommand and returns its stdout; a non-zero exit is an
// *exec.ExitError carrying stderr, as with exec.Cmd.Output.
func (r *Runner) Output(ctx context.Context, args ...string) ([]byte, error) {
	cmd, err := r.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

// CombinedOutput runs the subcommand and returns stdout and stderr
// interleaved, as with exec.Cmd.CombinedOutput.
func (r *Runner) CombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd, err := r.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	return cmd.CombinedOutput()
}

// Run runs the subcommand and reports only its exit status.
func (r *Runner) Run(ctx context.Context, args ...string) error {
	cmd, err := r.Command(ctx, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// subcommand finds the first non-option argument, rejecting the global
// options the runner reserves for itself on the way.
func subcommand(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if reservedWithValue[a] {
			return "", fmt.Errorf("%w: %s", ErrReservedOption, a)
		}
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(a, p) {
				return "", fmt.Errorf("%w: %s", ErrReservedOption, a)
			}
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a, nil
	}
	return "", fmt.Errorf("%w: no subcommand", ErrReservedOption)
}

// env scrubs the inherited environment: every GIT_* variable is dropped
// except GIT_SSH_COMMAND, GIT_TERMINAL_PROMPT is pinned to 0, and Foreign
// runners additionally ignore global and system configuration and skip
// optional index writes. Every other variable passes through.
func (r *Runner) env(inherited []string) []string {
	out := make([]string, 0, len(inherited)+4)
	for _, kv := range inherited {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "GIT_") && key != "GIT_SSH_COMMAND" {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	if r.foreign || r.quarantine {
		out = append(out,
			"GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_OPTIONAL_LOCKS=0",
		)
	}
	if r.quarantine {
		out = append(out, "GIT_NO_LAZY_FETCH=1")
	}
	return out
}
