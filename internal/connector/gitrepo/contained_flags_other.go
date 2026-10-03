//go:build !darwin && !linux

package gitrepo

import "os"

// Root confines paths on supported platforms; nonblocking/no-follow flags are
// supplied separately for the qualified Darwin and Linux targets.
func containedReadFlags() int { return os.O_RDONLY }
