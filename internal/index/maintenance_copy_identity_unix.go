//go:build darwin || linux

package index

import (
	"os"
	"syscall"
)

func maintenancePrivateInfo(info os.FileInfo, directory bool) bool {
	if info == nil {
		return false
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	} else if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && (directory || stat.Nlink == 1)
}
func maintenanceOpenPrivate(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
