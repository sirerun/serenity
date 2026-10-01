//go:build darwin

package recovery

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func verifyFilesystemOwnership(path string) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return fmt.Errorf("%w: inspect filesystem ownership mode: %v", ErrPlanUntrustedDir, err)
	}
	if stat.Flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		return fmt.Errorf("%w: filesystem ignores file ownership metadata", ErrPlanUntrustedDir)
	}
	return nil
}
