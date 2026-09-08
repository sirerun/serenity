//go:build unix

package gbrain

import (
	"fmt"
	"os"
	"syscall"
)

// Kernel locks are released on SIGKILL; no stale PID file needs manual removal.
func lockImport(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("gbrain import: another import holds the brain lock: %w", err)
	}
	return nil
}
