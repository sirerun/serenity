package cli

import (
	"encoding/json"
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
	dirA, dirB := t.TempDir(), t.TempDir()

	cfg := config.Default()
	cfg.Connectors = map[string]any{
		"file":     map[string]any{"path": t.TempDir()},
		"git_repo": []any{map[string]any{"path": dirA}, map[string]any{"path": dirB}},
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
	cfg.Connectors = map[string]any{"file": map[string]any{}}
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
	cfg.Connectors = map[string]any{"imap": map[string]any{"account": "you@gmail.com"}}

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
