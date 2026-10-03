//go:build darwin || linux

package gitrepo

import (
	"errors"
	"os"
	"syscall"
)

// Nonblocking open lets the descriptor check refuse a raced-in FIFO; no-follow
// also refuses a leaf replaced by a symlink after the initial Lstat.
func containedReadFlags() int { return os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW }

func containedSymlinkError(err error) bool { return errors.Is(err, syscall.ELOOP) }
