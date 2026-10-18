//go:build darwin

package episodeinstall

import (
	"golang.org/x/sys/unix"
	"path/filepath"
)

// renameExclusive uses pinned directory descriptors and never replaces a slot.
func renameExclusive(from, to string) error {
	source, err := renameParent(filepath.Dir(from))
	if err != nil {
		return ErrIncomplete
	}
	defer source.Close()
	destination, err := renameParent(filepath.Dir(to))
	if err != nil {
		return ErrIncomplete
	}
	defer destination.Close()
	if unix.RenameatxNp(int(source.Fd()), filepath.Base(from), int(destination.Fd()), filepath.Base(to), unix.RENAME_EXCL) != nil {
		return ErrIncomplete
	}
	return nil
}
