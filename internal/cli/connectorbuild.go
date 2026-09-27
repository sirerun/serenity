package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	fileconn "github.com/sirerun/serenity/internal/connector/file"
	"github.com/sirerun/serenity/internal/connector/gitrepo"
	imapconn "github.com/sirerun/serenity/internal/connector/imap"
)

// buildConnectors constructs one connector.Connector per entry under
// serenity.yml's typed `connectors:` section (T1.15; typed and contained
// by T24.8 / ADR 018). Connectors with no configured entry are simply
// absent from the result -- a brain repo with nothing configured yet polls
// nothing, not an error.
//
// Every connector `path` is resolved to an absolute, cleaned path and must
// lie under one of the allowlisted roots -- the user's home directory, plus
// whatever `connectors.roots` adds (see connectorRoots). serenity.yml is
// synced through the brain remote, so a path it carries is not first-party
// input: an escaping path fails here, naming the path, before it is ever
// handed to git or walked as a directory (SEC-H05, config half).
//
// file: a single watched directory (RFC section 10.1's file-watcher
// connector). Always constructed in poll mode (fileconn.NewPoll), never
// watch mode: `serenity sync` is a one-shot CLI invocation, and watch
// mode's fsnotify goroutine only accumulates change events while it runs
// in the background -- started moments before Poll is called, it would
// see none. Poll mode's "rescan the whole tree on every Poll call" is
// exactly the one-shot model sync uses; watch mode remains available to a
// long-running caller (a future daemon) via the package directly. Only one
// directory is supported today: internal/connector/file.Connector.Name()
// always returns the constant "file" (no per-root distinguishing suffix),
// so two configured roots would collide on one cursor/job-history slot --
// disclosed limitation, not silently wrong. Multiple file roots is
// unblocked follow-up work (mirroring gitrepo's own path-derived Name()).
//
// git_repo: a list of repositories to crawl (RFC section 10.1's "5 repos"
// M1 acceptance criterion) -- gitrepo.Connector.Name() already derives a
// distinct name per repo root, so this is the one connector kind that
// supports multiple instances out of the box.
//
// imap: exactly one mailbox, matching `serenity connectors auth imap`'s
// existing `{account: ...}` shape; the app password is read from the OS
// keychain by imapconn.NewGmail, never from serenity.yml.
func buildConnectors(root string, cfg *config.Config) ([]connector.Connector, error) {
	var cs []connector.Connector

	roots, err := connectorRoots(cfg)
	if err != nil {
		return nil, err
	}

	if c := cfg.Connectors.IMAP; c != nil {
		if c.Account == "" {
			return nil, fmt.Errorf("connectors.imap: account is required")
		}
		cs = append(cs, imapconn.NewGmail(c.Account))
	}

	if c := cfg.Connectors.File; c != nil {
		if c.Path == "" {
			return nil, fmt.Errorf("connectors.file: path is required")
		}
		path, err := containConnectorPath("connectors.file.path", c.Path, roots)
		if err != nil {
			return nil, err
		}
		cs = append(cs, fileconn.NewPoll(path))
	}

	for i, c := range cfg.Connectors.GitRepo {
		if c.Path == "" {
			return nil, fmt.Errorf("connectors.git_repo[%d]: path is required", i)
		}
		path, err := containConnectorPath(fmt.Sprintf("connectors.git_repo[%d].path", i), c.Path, roots)
		if err != nil {
			return nil, err
		}
		cs = append(cs, gitrepo.New(gitrepo.Config{RepoRoot: path, BrainRoot: root}))
	}

	return cs, nil
}

// connectorRoots returns the cleaned allowlist of directories a connector
// path may resolve under: the user's home directory (always), followed by
// every `connectors.roots` entry. Each configured root is validated the
// same way a connector path is -- it must be absolute -- and is cleaned,
// so a relative or traversal-shaped root cannot widen the allowlist to
// somewhere the operator did not name.
func connectorRoots(cfg *config.Config) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("connectors: resolve home directory for the path allowlist: %w", err)
	}
	roots := []string{filepath.Clean(home)}
	for i, r := range cfg.Connectors.Roots {
		if r == "" {
			return nil, fmt.Errorf("connectors.roots[%d]: root is empty", i)
		}
		if !filepath.IsAbs(r) {
			return nil, fmt.Errorf("connectors.roots[%d]: %q is not an absolute path", i, r)
		}
		roots = append(roots, filepath.Clean(r))
	}
	return roots, nil
}

// containConnectorPath resolves p to an absolute, cleaned path (a relative
// p is resolved against the process working directory, as filepath.Abs
// does) and requires the result to be under one of roots. It returns the
// resolved path, or an error naming key and the escaping path.
func containConnectorPath(key, p string, roots []string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%s: resolve %q: %w", key, p, err)
	}
	abs = filepath.Clean(abs)
	for _, root := range roots {
		if pathUnder(abs, root) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%s: %q is outside every allowed root (%s); add its root under connectors.roots to allow it", key, abs, strings.Join(roots, ", "))
}

// pathUnder reports whether the cleaned absolute path p equals root or is
// lexically inside it. The comparison is on path components, so
// "/home/alice-other" is not under "/home/alice".
func pathUnder(p, root string) bool {
	if p == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(p, prefix)
}
