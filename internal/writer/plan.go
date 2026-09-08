package writer

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// SnapshotFiles reads exact canonical paths, rejecting symlinks and non-files.
// A nil value records a path that does not exist yet.
func SnapshotFiles(root string, paths []string) (map[string][]byte, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	snapshot := make(map[string][]byte, len(paths))
	for _, path := range paths {
		if err := publicationPath(dir, path); err != nil {
			return nil, err
		}
		raw, err := readPublicationFile(dir, path)
		if err != nil {
			return nil, err
		}
		snapshot[path] = raw
	}
	return snapshot, nil
}

// PlanFiles runs the real canonical writers against an isolated snapshot, then
// returns their byte transitions. Existing entity pages retain all bytes outside
// claim/detail/metadata fences. No production canonical file changes here.
// The caller must publish the returned plan with PublishFiles; approval flows
// additionally persist it first so an interrupted application can be resumed.
func PlanFiles(root string, snapshot map[string][]byte, build func(preview string) error) ([]FileChange, error) {
	return planFiles(root, snapshot, "", nil, build)
}

// PlanReviewedFiles permits exactly one reviewed entity page to be Git-dirty.
// Its bytes must match the captured human edit; all other dependencies stay clean.
// An unchanged page remains in the plan so its eventual commit is guarded.
func PlanReviewedFiles(root string, snapshot map[string][]byte, path string, human []byte, build func(string) error) ([]FileChange, error) {
	if !strings.HasPrefix(path, "brain/entities/") || human == nil || !sameFileBytes(snapshot[path], human) {
		return nil, fmt.Errorf("writer: reviewed page does not match snapshot")
	}
	return planFiles(root, snapshot, path, human, build)
}

func planFiles(root string, snapshot map[string][]byte, reviewed string, human []byte, build func(string) error) ([]FileChange, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	if info, err := dir.Lstat(".serenity"); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("writer: unsafe planning directory")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := dir.MkdirAll(".serenity", 0700); err != nil {
		return nil, err
	}
	preview, err := os.MkdirTemp(filepath.Join(root, ".serenity"), "canonical-plan-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(preview) }()
	for path, raw := range snapshot {
		if err := publicationPath(dir, path); err != nil {
			return nil, err
		}
		if raw == nil {
			continue
		}
		target := filepath.Join(preview, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, raw, 0600); err != nil {
			return nil, err
		}
	}
	if err := build(preview); err != nil {
		return nil, err
	}
	after := map[string][]byte{}
	err = filepath.WalkDir(filepath.Join(preview, "brain"), func(path string, entry fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) && path == filepath.Join(preview, "brain") {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("writer: planner produced a non-regular file")
		}
		rel, err := filepath.Rel(preview, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := publicationPath(dir, rel); err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if original := snapshot[rel]; original != nil && strings.HasPrefix(rel, "brain/entities/") {
			raw, err = MergeClaimFences(original, raw)
			if err != nil {
				return err
			}
		}
		after[rel] = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	for path, raw := range snapshot {
		if raw != nil && after[path] == nil {
			return nil, fmt.Errorf("writer: planner removed canonical file %s", path)
		}
	}
	names := make([]string, 0, len(after))
	changed := false
	for path, raw := range after {
		names = append(names, path)
		if !sameFileBytes(snapshot[path], raw) {
			changed = true
		}
	}
	sort.Strings(names)
	if err := checkSnapshot(root, snapshot, reviewed, human); err != nil {
		return nil, err
	}

	if !changed && reviewed == "" {
		return nil, nil
	}
	changes := make([]FileChange, 0, len(names))
	for _, path := range names {
		changes = append(changes, FileChange{Path: path, Before: snapshot[path], After: after[path]})
	}
	return changes, nil
}

// CheckSnapshot requires unchanged bytes and clean Git state for every canonical
// dependency. It also guards staging decisions against an uncommitted prior.
func CheckSnapshot(root string, snapshot map[string][]byte) error {
	return checkSnapshot(root, snapshot, "", nil)
}

func checkSnapshot(root string, snapshot map[string][]byte, reviewed string, human []byte) error {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	// Git failures are errors, not evidence of a clean tree. Check every existing
	// read dependency, including unchanged files that informed the plan.
	for path, raw := range snapshot {
		if err := publicationPath(dir, path); err != nil {
			return err
		}
		actual, err := readPublicationFile(dir, path)
		if err != nil {
			return err
		}
		if !sameFileBytes(actual, raw) {
			return fmt.Errorf("writer: canonical file changed while planning: %s", path)
		}
		cmd := exec.Command("git", "--literal-pathspecs", "status", "--porcelain", "--", path)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("writer: cannot verify canonical Git state: %w", err)
		}
		if path == reviewed && sameFileBytes(actual, human) {
			continue
		}
		if len(bytes.TrimSpace(out)) != 0 {
			return fmt.Errorf("%w: %s", ErrDirtyTree, path)
		}
	}
	return nil
}
