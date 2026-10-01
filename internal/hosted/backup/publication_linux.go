//go:build linux

package backup

import "golang.org/x/sys/unix"

func renameNoReplace(oldPath, newPath string) error {
	return publicationRenameError(oldPath, newPath, unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE))
}
