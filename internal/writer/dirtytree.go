package writer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrDirtyTree is returned by Fence/Shard when the target file carries an
// uncommitted human edit: the dirty-tree guard (RFC 0001 §7.7, ADR 004)
// pauses the write rather than racing it against a human's file state,
// which is truth (§7.2). The write is not lost -- see PendingPath.
var ErrDirtyTree = errors.New("writer: dirty tree -- human edit pending, machine write paused")

// PendingRecord is the on-disk shape of one paused write: runtime state
// (derived, never canonical, never committed to git) that holds both
// sides of the conflict until it is resolved. Until M2's disposition
// store exists, resolving means hand-inspecting and deleting the file
// (ADR 004 D1); M2 imports these records as dirty_edit items instead.
type PendingRecord struct {
	Path       string `json:"path"`
	Human      string `json:"human"`
	Machine    string `json:"machine"`
	DetectedAt string `json:"detected_at"`
}

// PendingPath returns the runtime-state path a paused write against path
// would be recorded at: .serenity/pending/<key>.json. key identifies the
// conflict (an entity slug for fence pages; slug-family for shards) --
// callers pick it, this just joins it under the well-known directory.
func PendingPath(root, key string) string {
	return filepath.Join(root, ".serenity", "pending", key+".json")
}

// dirty reports whether path carries an uncommitted modification, per
// `git status --porcelain -- path` run at root -- a per-file check, never
// whole-repo dirtiness (T0.4 scope). A brand-new, untracked file ("??")
// is not dirty: there is no prior committed state for a human edit to
// conflict with, so a first-ever write always proceeds. Any failure to
// ask git (no repo, git missing, nothing at path yet) is treated as
// clean -- the guard only blocks writes it can positively identify as
// conflicting, never ones it merely cannot evaluate.
func dirty(root, path string) bool {
	cmd := exec.Command("git", "status", "--porcelain", "--", path)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	line := strings.TrimRight(string(out), "\n")
	return line != "" && !strings.HasPrefix(line, "??")
}

// guard runs the dirty-tree check for one queued write. On a clean tree
// it submits render through q exactly as an unguarded write would. On a
// dirty tree it never calls render -- the file on disk is left exactly
// as the human left it -- and instead records both sides at
// PendingPath(root, key) and returns ErrDirtyTree.
func guard(q *Queue, root, path, key string, machine []byte, render func() ([]byte, error)) ([]byte, error) {
	if !dirty(root, path) {
		res := q.Submit(Job{Path: path, Render: render})
		return res.Bytes, res.Err
	}

	human, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dirty-tree guard: read human copy of %s: %w", path, err)
	}
	rec := PendingRecord{
		Path:       path,
		Human:      string(human),
		Machine:    string(machine),
		DetectedAt: time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dirty-tree guard: encode pending record: %w", err)
	}
	pp := PendingPath(root, key)
	if err := os.MkdirAll(filepath.Dir(pp), 0o755); err != nil {
		return nil, fmt.Errorf("dirty-tree guard: %w", err)
	}
	if err := os.WriteFile(pp, b, 0o644); err != nil {
		return nil, fmt.Errorf("dirty-tree guard: write pending record: %w", err)
	}
	return nil, ErrDirtyTree
}
