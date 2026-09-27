package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/index"
)

// TestBuildConnectorsEmpty pins the "nothing configured, nothing to poll"
// contract -- a fresh serenity.yml (config.Default) has a nil Connectors
// map, and buildConnectors must return an empty slice, not an error.
func TestBuildConnectorsEmpty(t *testing.T) {
	cs, err := buildConnectors(t.TempDir(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 0 {
		t.Fatalf("buildConnectors on an empty config returned %d connector(s), want 0: %+v", len(cs), cs)
	}
}

// TestBuildConnectorsFileAndGitRepo proves every documented connectors.*
// shape decodes into the right connector type with the right identity,
// including multiple git_repo entries (the M1 "5 repos" acceptance
// criterion) each getting a distinct Name().
func TestBuildConnectorsFileAndGitRepo(t *testing.T) {
	root := t.TempDir()
	dirA, dirB, fileDir := t.TempDir(), t.TempDir(), t.TempDir()

	cfg := config.Default()
	cfg.Connectors = config.Connectors{
		// t.TempDir()s share one parent, which is outside $HOME; allowlist
		// it so containment (T24.8) admits them.
		Roots:   []string{filepath.Dir(dirA)},
		File:    &config.FileConnector{Path: fileDir},
		GitRepo: []config.GitRepoConnector{{Path: dirA}, {Path: dirB}},
	}

	cs, err := buildConnectors(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("buildConnectors returned %d connector(s), want 3 (1 file + 2 git_repo): %+v", len(cs), cs)
	}

	names := map[string]bool{}
	for _, c := range cs {
		if names[c.Name()] {
			t.Fatalf("duplicate connector Name() %q -- cursors would collide", c.Name())
		}
		names[c.Name()] = true
	}
	if !names["file"] {
		t.Fatalf("expected a connector named \"file\", got names %v", names)
	}
}

// TestBuildConnectorsMissingPathErrors proves a malformed connectors.file
// entry (no path) is a configuration error, not a silently-skipped
// connector or a nil-pointer panic later at Poll time.
func TestBuildConnectorsMissingPathErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Connectors = config.Connectors{File: &config.FileConnector{}}
	if _, err := buildConnectors(t.TempDir(), cfg); err == nil {
		t.Fatal("expected an error for connectors.file with no path, got nil")
	}
}

// TestBuildConnectorsIMAPUsesAuthedAccount proves the imap shape
// `serenity connectors auth imap` already writes to serenity.yml
// (connectors.go's runConnectorsAuthIMAP) is exactly what buildConnectors
// consumes.
func TestBuildConnectorsIMAPUsesAuthedAccount(t *testing.T) {
	cfg := config.Default()
	cfg.Connectors = config.Connectors{IMAP: &config.IMAPConnector{Account: "you@gmail.com"}}

	cs, err := buildConnectors(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Name() != "imap:you@gmail.com" {
		t.Fatalf("buildConnectors(imap) = %+v, want exactly one connector named imap:you@gmail.com", cs)
	}
}

// TestLastCursorSkipsInterruptedAndOtherConnectors proves lastCursor finds
// the most recent NON-EMPTY cursor for the named connector -- skipping an
// interrupted job (which carries no cursor, SweepInterrupted's own
// contract) in favor of an earlier real one, and never returning another
// connector's cursor.
func TestLastCursorSkipsInterruptedAndOtherConnectors(t *testing.T) {
	real := connector.Cursor(json.RawMessage(`{"uid":42}`))
	jobs := []index.Job{
		{Connector: "file", Status: index.JobInterrupted, Cursor: nil}, // most recent, no cursor
		{Connector: "imap:you@gmail.com", Status: index.JobSucceeded, Cursor: json.RawMessage(`{"uid":99}`)},
		{Connector: "file", Status: index.JobSucceeded, Cursor: json.RawMessage(real)}, // older, real cursor
	}

	got := lastCursor(jobs, "file")
	if string(got) != string(real) {
		t.Fatalf("lastCursor(file) = %s, want %s", got, real)
	}

	if got := lastCursor(jobs, "git-repo:nope"); got != nil {
		t.Fatalf("lastCursor for an unseen connector = %s, want nil", got)
	}
}

// TestBuildConnectorsRejectsGitRepoPathOutsideRoots pins SEC-H05's path
// containment half (ADR 018 decision 3): a git_repo path delivered through
// a synced serenity.yml that escapes every allowlisted root (the home
// directory by default) is refused at connector build, naming the path,
// rather than handed to git as a repository root.
func TestBuildConnectorsRejectsGitRepoPathOutsideRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	setGitRepoPaths(cfg, outside)

	_, err := buildConnectors(home, cfg)
	if err == nil {
		t.Fatalf("buildConnectors accepted git_repo path %q outside every allowlisted root; want an error naming it", outside)
	}
	if !strings.Contains(err.Error(), outside) {
		t.Fatalf("buildConnectors error = %q, want it to name the escaping path %q", err, outside)
	}
}

// TestBuildConnectorsRejectsTraversalOutOfRoot proves containment is on
// the cleaned absolute path: a path that starts under home but climbs out
// with ".." is still refused, and the error names the cleaned form.
func TestBuildConnectorsRejectsTraversalOutOfRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	escaping := filepath.Join(home, "..", "..", "etc")

	cfg := config.Default()
	setGitRepoPaths(cfg, escaping)

	_, err := buildConnectors(home, cfg)
	if err == nil {
		t.Fatalf("buildConnectors accepted traversal path %q; want an error", escaping)
	}
	if !strings.Contains(err.Error(), filepath.Clean(escaping)) {
		t.Fatalf("buildConnectors error = %q, want it to name the cleaned path %q", err, filepath.Clean(escaping))
	}
}

// setGitRepoPaths writes one git_repo entry per path into cfg.
func setGitRepoPaths(cfg *config.Config, paths ...string) {
	var list []config.GitRepoConnector
	for _, p := range paths {
		list = append(list, config.GitRepoConnector{Path: p})
	}
	cfg.Connectors = config.Connectors{GitRepo: list}
}

// TestBuildConnectorsAcceptsPathUnderConfiguredRoot proves connectors.roots
// is a real allowlist extension: the same out-of-home path
// TestBuildConnectorsRejectsGitRepoPathOutsideRoots refuses is accepted
// once its parent is listed, and the connector is built on the cleaned
// absolute path.
func TestBuildConnectorsAcceptsPathUnderConfiguredRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := t.TempDir()
	repo := filepath.Join(shared, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Connectors = config.Connectors{
		Roots:   []string{shared + string(filepath.Separator)},
		GitRepo: []config.GitRepoConnector{{Path: filepath.Join(shared, ".", "repo")}},
	}

	cs, err := buildConnectors(home, cfg)
	if err != nil {
		t.Fatalf("buildConnectors with %q allowlisted: %v", shared, err)
	}
	if len(cs) != 1 {
		t.Fatalf("buildConnectors returned %d connector(s), want 1: %+v", len(cs), cs)
	}
}

// TestBuildConnectorsRejectsRelativeRoot proves a root is validated the
// same way a path is: a relative connectors.roots entry cannot widen the
// allowlist to wherever the process happens to run.
func TestBuildConnectorsRejectsRelativeRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	cfg.Connectors = config.Connectors{Roots: []string{"../shared"}}

	_, err := buildConnectors(t.TempDir(), cfg)
	if err == nil {
		t.Fatal("buildConnectors accepted a relative connectors.roots entry; want an error")
	}
	if !strings.Contains(err.Error(), "connectors.roots[0]") || !strings.Contains(err.Error(), "../shared") {
		t.Fatalf("buildConnectors error = %q, want it to name connectors.roots[0] and the offending value", err)
	}
}

// TestBuildConnectorsRejectsFilePathOutsideRoots proves containment covers
// the file connector's path too, not only git_repo.
func TestBuildConnectorsRejectsFilePathOutsideRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outside := t.TempDir()

	cfg := config.Default()
	cfg.Connectors = config.Connectors{File: &config.FileConnector{Path: outside}}

	_, err := buildConnectors(home, cfg)
	if err == nil {
		t.Fatalf("buildConnectors accepted file path %q outside every allowlisted root; want an error", outside)
	}
	if !strings.Contains(err.Error(), "connectors.file.path") || !strings.Contains(err.Error(), outside) {
		t.Fatalf("buildConnectors error = %q, want it to name connectors.file.path and %q", err, outside)
	}
}
