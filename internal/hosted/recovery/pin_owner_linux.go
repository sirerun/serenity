//go:build linux

package recovery

import (
	"os"

	"golang.org/x/sys/unix"
)

func pinOwnerPlatformRenameNoReplaceAt(dir *os.File, oldName, newName string) error {
	return unix.Renameat2(int(dir.Fd()), oldName, int(dir.Fd()), newName, unix.RENAME_NOREPLACE)
}
