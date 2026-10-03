//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func lockFileContext(ctx context.Context, f *os.File, shared bool) error {
	mode := unix.LOCK_EX | unix.LOCK_NB
	if shared {
		mode = unix.LOCK_SH | unix.LOCK_NB
	}
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := unix.Flock(int(f.Fd()), mode)
		if err == nil {
			return nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func openLockFile(path string, create bool) (*os.File, error) {
	flags := unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Open(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
func openRegularFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err, f.Close())
	}
	return f, nil
}
func fileIdentity(i os.FileInfo) (uint64, uint64, error) {
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, ErrSnapshotLeaseInvalid
	}
	return uint64(s.Dev), uint64(s.Ino), nil
}
