//go:build darwin || linux

package direction

import (
	"fmt"
	"os"
	"syscall"
)

func lockPublication(root *os.Root) (*os.File, error) {
	if info, err := root.Lstat("lock"); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("direction: unsafe lock file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := root.OpenFile("lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("direction: another application is running: %w", err)
	}
	return file, nil
}
