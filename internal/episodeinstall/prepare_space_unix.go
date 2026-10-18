//go:build darwin || linux

package episodeinstall

import "syscall"

func prepareCapacity(parent string, required int64) error {
	var stats syscall.Statfs_t
	if required < 0 || syscall.Statfs(parent, &stats) != nil || stats.Bsize <= 0 {
		return ErrUnqualified
	}
	if uint64(stats.Bavail) < uint64(required)/uint64(stats.Bsize)+1 {
		return ErrUnqualified
	}
	return nil
}
