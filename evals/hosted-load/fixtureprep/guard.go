package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// MarkerFile names the file whose presence makes a directory a fixture this tool prepared. verify refuses a
// directory without it, and prepare refuses an output path whose ancestors already hold one.
const MarkerFile = "FIXTURE-ONLY.json"

// ErrGuard wraps every guardrail refusal so callers and tests can tell a refusal from an I/O failure.
var ErrGuard = errors.New("fixtureprep: guardrail refused")

func refuse(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrGuard, fmt.Sprintf(format, args...))
}

// serviceRoots are well-known service data locations. A fixture never belongs under one.
var serviceRoots = []string{"/var/lib", "/srv", "/opt", "/data", "/etc", "/usr", "/bin", "/sbin", "/root", "/home/serenity", "/mnt/serenity"}

// CheckOutputDir refuses an output path that could touch anything but a brand-new, owned fixture directory.
// It returns the cleaned, symlink-resolved parent so the caller creates the tree where the checks ran.
//
// Refused: a relative, unclean or URL-like path; any existing path (file, directory, symlink or empty directory);
// a parent that is missing or not a directory; an ancestor that is a service data directory (holds control.db),
// another fixture (holds the marker) or a Git work tree; and well-known service roots.
func CheckOutputDir(out string) (string, error) {
	switch {
	case out == "":
		return "", refuse("-out is required")
	case strings.ContainsRune(out, 0) || strings.Contains(out, "://"):
		return "", refuse("-out must be a local filesystem path, not a URL")
	case !filepath.IsAbs(out):
		return "", refuse("-out must be an absolute path, got %q", out)
	case filepath.Clean(out) != out:
		return "", refuse("-out must be a clean path with no .. or repeated separators, got %q", out)
	}
	if _, err := os.Lstat(out); err == nil {
		return "", refuse("%s already exists; the fixture tool never writes into an existing directory", out)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("inspect -out: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil {
		return "", refuse("parent of -out must exist: %v", err)
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", refuse("parent of -out is not a directory: %s", parent)
	}
	resolved := filepath.Join(parent, filepath.Base(out))
	for _, root := range serviceRoots {
		if resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			return "", refuse("%s is under %s, a service data location", resolved, root)
		}
	}
	for dir := parent; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"control.db", MarkerFile, ".git"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
				return "", refuse("%s holds %s: refusing to nest a fixture inside a service data directory, another fixture or a Git work tree", dir, name)
			}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return parent, nil
}

// CheckLoopbackEndpoint accepts only an http(s) URL whose host is a loopback IP literal. The tool never dials it: the
// value is recorded in the marker as the only endpoint a load run over this fixture may target. A hostname
// (including localhost, which can be re-pointed), a credential in the URL, a query, a fragment or a path is refused.
func CheckLoopbackEndpoint(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", refuse("-endpoint is not a URL")
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", refuse("-endpoint must be http or https, got %q", u.Scheme)
	case u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/"):
		return "", refuse("-endpoint must be a bare origin: no credentials, path, query or fragment")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsLoopback() {
		return "", refuse("-endpoint host %q must be a loopback IP literal (127.0.0.0/8 or ::1); public, private-network and hostname endpoints are refused", u.Hostname())
	}
	return u.Scheme + "://" + u.Host, nil
}
