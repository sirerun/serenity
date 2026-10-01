//go:build darwin

package backup

import "golang.org/x/sys/unix"

func renameNoReplace(oldPath, newPath string) error {
	return publicationRenameError(oldPath, newPath, unix.RenameatxNp(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_EXCL))
}
