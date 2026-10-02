//go:build darwin

package privatefs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func ownershipEnforced(path string) error {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return err
	}
	if fs.Flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		return errors.New("privatefs: filesystem ownership enforcement required")
	}
	return nil
}
