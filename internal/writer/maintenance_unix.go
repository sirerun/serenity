//go:build darwin || linux

package writer

import (
	"os"
	"syscall"
)

func maintenanceCreate(state *os.Root, name string) (*os.File, error) {
	return state.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
}
func maintenanceOpen(state *os.Root, name string) (*os.File, error) {
	info, err := state.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, ErrMaintenancePending
	}
	return state.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
func maintenanceSync(state *os.Root) error {
	dir, err := state.Open(".")
	if err != nil {
		return err
	}
	returnErr := dir.Sync()
	closeErr := dir.Close()
	if returnErr != nil {
		return returnErr
	}
	return closeErr
}
