package writer

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileChange is a durable publication plan. Nil Before means the file did not
// exist. Unchanged entries protect read dependencies during a resumed write.
type FileChange struct {
	Path   string `json:"path"`
	Before []byte `json:"before"`
	After  []byte `json:"after"`
}

// PublishFiles applies only a previously prepared byte transition. Every file
// must still match its before or after image; an intervening edit stops the
// entire pass before any write. Repeating a partially published plan is safe.
// The caller persists the plan before calling this method and Flushes afterward.
func PublishFiles(q *Queue, root string, changes []FileChange) error {
	result := q.Submit(Job{Render: func() ([]byte, error) {
		dir, err := os.OpenRoot(root)
		if err != nil {
			return nil, err
		}
		defer func() { _ = dir.Close() }()
		seen := map[string]bool{}
		for _, change := range changes {
			if change.After == nil {
				return nil, fmt.Errorf("writer: deletion is not a publication transition")
			}
			if seen[change.Path] {
				return nil, fmt.Errorf("writer: duplicate publication path %q", change.Path)
			}
			seen[change.Path] = true
			if err := publicationPath(dir, change.Path); err != nil {
				return nil, err
			}
			actual, err := readPublicationFile(dir, change.Path)
			if err != nil {
				return nil, err
			}
			if !sameFileBytes(actual, change.Before) && !sameFileBytes(actual, change.After) {
				return nil, fmt.Errorf("writer: publication stopped: %s changed since approval was prepared", change.Path)
			}
		}
		for _, change := range changes {
			if sameFileBytes(change.Before, change.After) {
				continue
			}
			actual, err := readPublicationFile(dir, change.Path)
			if err != nil {
				return nil, err
			}
			if sameFileBytes(actual, change.Before) {
				if err := writePublicationFile(dir, change.Path, change.After); err != nil {
					return nil, err
				}
			} else if !sameFileBytes(actual, change.After) {
				return nil, fmt.Errorf("writer: publication stopped: %s changed during publication", change.Path)
			}
			q.MarkTouched(filepath.Join(root, filepath.FromSlash(change.Path)))
		}
		return nil, nil
	}})
	return result.Err
}

func sameFileBytes(a, b []byte) bool { return (a == nil) == (b == nil) && bytes.Equal(a, b) }

func publicationPath(root *os.Root, path string) error {
	if filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path || strings.Contains(path, "\\") || (!strings.HasPrefix(path, "brain/entities/") && !strings.HasPrefix(path, "brain/claims/")) {
		return fmt.Errorf("writer: invalid canonical publication path %q", path)
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		if parts[i] == "" || parts[i] == ".." || parts[i] == "." {
			return fmt.Errorf("writer: invalid publication path %q", path)
		}
		partial := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(partial)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("writer: publication path is a symlink: %s", partial)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("writer: publication parent is not a directory: %s", partial)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("writer: publication target is not a regular file: %s", partial)
		}
	}
	return nil
}

func readPublicationFile(root *os.Root, path string) ([]byte, error) {
	b, err := root.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

func writePublicationFile(root *os.Root, path string, data []byte) error {
	parent := filepath.Dir(path)
	if err := root.MkdirAll(parent, 0755); err != nil {
		return err
	}
	// Staging is derived state, so a killed process cannot leave an untracked
	// temporary beside canonical files. Random names never clobber another file.
	for _, dir := range []string{".serenity", ".serenity/publication"} {
		if info, err := root.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("writer: staging directory is a symlink")
		}
	}
	if err := root.MkdirAll(".serenity/publication", 0700); err != nil {
		return err
	}
	temp := ".serenity/publication/" + rand.Text()

	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = root.Remove(temp) }()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	mode := fs.FileMode(0644)
	if info, err := root.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := root.Chmod(temp, mode); err != nil {
		return err
	}
	if err := root.Rename(temp, path); err != nil {
		return err
	}
	dir, err := root.Open(parent)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
