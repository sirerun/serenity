//go:build linux

package episodeinstall

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// renameExclusive uses pinned directory descriptors and never replaces a slot.
func renameExclusive(from, to string) error {
	source, err := renameParent(filepath.Dir(from))
	if err != nil {
		return ErrIncomplete
	}
	defer func() { _ = source.Close() }()
	destination, err := renameParent(filepath.Dir(to))
	if err != nil {
		return ErrIncomplete
	}
	defer func() { _ = destination.Close() }()
	if unix.Renameat2(int(source.Fd()), filepath.Base(from), int(destination.Fd()), filepath.Base(to), unix.RENAME_NOREPLACE) != nil {
		return ErrIncomplete
	}
	return nil
}
