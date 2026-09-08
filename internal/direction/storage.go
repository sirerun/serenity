package direction

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/dira/ledger"
)

// openEntries pins the ledger directory below the brain root. Symlinks are
// refused at each boundary; os.Root also confines any subsequent resolution.
// Reads never create directories. A missing read directory remains NotExist.
func (s *Store) openEntries(create bool) (*os.Root, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	for _, path := range []string{".dira", ".dira/entries"} {
		info, err := root.Lstat(path)
		if os.IsNotExist(err) && create {
			if err := root.Mkdir(path, 0755); err != nil && !os.IsExist(err) {
				return nil, err
			}
			parent, openErr := root.Open(filepath.Dir(path))
			if openErr != nil {
				return nil, openErr
			}
			syncErr := parent.Sync()
			closeErr := parent.Close()
			if syncErr != nil {
				return nil, syncErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			info, err = root.Lstat(path)
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("direction: unsafe ledger directory %s", path)
		}
	}
	return root.OpenRoot(".dira/entries")
}

func entryName(id string) (string, error) {
	if !ledger.ValidID(id) {
		return "", fmt.Errorf("direction: invalid ledger entry id %q", id)
	}
	return id + ".md", nil
}

func regularEntry(root *os.Root, name string, missing bool) (os.FileInfo, error) {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) && missing {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("direction: ledger entry is not a regular file: %s", name)
	}
	return info, nil
}

// readEntry uses one file descriptor for bytes and version. Atomic replacement
// then gives readers a complete old or new entry, with matching file metadata.
func readEntry(root *os.Root, name string) ([]byte, os.FileInfo, error) {
	if _, err := regularEntry(root, name, false); err != nil {
		return nil, nil, err
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("direction: unsafe entry descriptor")
	}
	raw, err := io.ReadAll(file)
	return raw, info, err
}

// writeEntry syncs complete bytes before making them visible. Link is an atomic
// exclusive create; rename is an atomic replacement. The temporary name is not
// a ledger entry and is ignored by List if a process dies before cleanup.
func writeEntry(root *os.Root, name string, raw []byte, exclusive bool) error {
	info, err := regularEntry(root, name, true)
	if err != nil {
		return err
	}
	if exclusive && info != nil {
		return fmt.Errorf("%w: %s", ledger.ErrExists, name)
	}
	mode := os.FileMode(0644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := fmt.Sprintf(".serenity-%x.tmp", nonce)
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = root.Remove(temp) }()
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Recheck a substituted non-file before publication. Root-bound operations
	// never follow a destination symlink when linking or renaming.
	if _, err := regularEntry(root, name, true); err != nil {
		return err
	}
	if exclusive {
		if err := root.Link(temp, name); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("%w: %s", ledger.ErrExists, name)
			}
			return err
		}
		if err := root.Remove(temp); err != nil {
			return err
		}
	} else if err := root.Rename(temp, name); err != nil {
		return err
	}
	return syncEntries(root)
}

func syncEntries(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
